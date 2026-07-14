package main

import (
	"context"
	"log/slog"
	"strings"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/config"
	"github.com/mantonx/loomarr/internal/events"
	"github.com/mantonx/loomarr/internal/llm"
	"github.com/mantonx/loomarr/internal/store"
)

// llmModelSettingKey is the settings-store key holding the in-app-selected local
// model (§8.1). Present ⇒ it overrides LLM_MODEL; absent ⇒ LLM_MODEL is the default.
const llmModelSettingKey = "llm.model"

// buildLLM constructs the suggester's provider and, for a LOCAL ollama provider,
// the SystemLLMService that powers /v1/system/llm* (§8.1). For local ollama the
// provider is an llm.Swappable so an in-app model selection hot-swaps the running
// suggester with no restart; the persisted setting (llm.model) overrides LLM_MODEL.
// For a hosted openai provider there's nothing local to probe/pull, so the service
// is nil (routes 501) and the provider is the plain OpenAI client.
func buildLLM(ctx context.Context, cfg *config.Config, st store.Store, bus *events.Bus, log *slog.Logger) (llm.Provider, api.SystemLLMService) {
	if cfg.LLMProvider != "ollama" {
		// Hosted / non-local: model is named in config, no probe/pull/swap surface.
		return llm.NewProvider(cfg.LLMProvider, cfg.LLMURL, cfg.LLMModel, cfg.LLMAPIKey), nil
	}

	// Local ollama: resolve the active model (persisted selection wins over env),
	// wrap it in a Swappable, and expose the probe/select/pull service.
	initial := cfg.LLMModel
	if v, err := st.GetSetting(ctx, llmModelSettingKey); err == nil && v != "" {
		initial = v
		log.Info("llm model from settings store", "model", v)
	}
	sw := llm.NewSwappable(func(model string) llm.Provider {
		return llm.NewOllama(cfg.LLMURL, model)
	}, initial)

	svc := &systemLLMService{
		swap:   sw,
		prober: llm.NewProber(cfg.LLMURL),
		store:  st,
		bus:    bus,
		log:    log,
		newID:  newID,
	}
	return sw, svc
}

// activeModel returns the model the suggester is currently using, for logging. A
// Swappable reports its live model; a plain provider falls back to the configured tag.
func activeModel(p llm.Provider, cfg *config.Config) string {
	if sw, ok := p.(*llm.Swappable); ok {
		return sw.Model()
	}
	return cfg.LLMModel
}

// systemLLMService implements api.SystemLLMService over the local Ollama host: it
// probes the machine, annotates the curated catalog, hot-swaps the active model via
// the Swappable, persists the choice to the settings store, and streams pulls over
// the event bus. Only constructed for a local ollama provider (§8.1).
type systemLLMService struct {
	swap   *llm.Swappable
	prober *llm.Prober
	store  store.Store
	bus    *events.Bus
	log    *slog.Logger
	newID  func() string
}

func (s *systemLLMService) Status(ctx context.Context) (api.SystemLLMStatus, error) {
	probe := s.prober.Probe(ctx)
	entries := s.prober.Catalog(probe)

	out := api.SystemLLMStatus{
		Provider:  "ollama",
		Model:     s.swap.Model(),
		Local:     true,
		GPUName:   probe.GPUName,
		VRAMGiB:   probe.VRAMGiB,
		OllamaVer: probe.OllamaVersion,
		Reachable: probe.Reachable,
		Catalog:   make([]api.LLMModelView, 0, len(entries)),
	}
	for _, e := range entries {
		if e.Recommended {
			out.Recommended = e.Tag
		}
		out.Catalog = append(out.Catalog, api.LLMModelView{
			Tag: e.Tag, Label: e.Label, VRAMGiB: e.ApproxVRAMGiB,
			Fit: string(e.Fit), Pulled: e.Pulled, RuntimeOK: e.RuntimeOK,
			Recommended: e.Recommended, Why: e.Why,
		})
	}
	return out, nil
}

func (s *systemLLMService) Select(ctx context.Context, model string) error {
	// Guard: only select a model the host has actually pulled — otherwise the next
	// suggestion job would fail at chat time with an opaque error. Re-probe pulled
	// models (cheap) rather than trusting a stale catalog.
	probe := s.prober.Probe(ctx)
	if !containsModel(probe.PulledModels, model) {
		return api.ErrModelNotPulled
	}
	// Persist first, then swap — a crash between the two just means the swap is
	// re-applied from the setting on next boot (the setting is the source of truth).
	if err := s.store.SetSetting(ctx, llmModelSettingKey, model); err != nil {
		return err
	}
	s.swap.SetModel(model)
	s.log.Info("llm model selected", "model", model)
	return nil
}

func (s *systemLLMService) Pull(ctx context.Context, model string) (string, error) {
	jobID := s.newID()
	// Run the pull detached from the request context so it survives the HTTP call;
	// progress streams over the event bus (a dropped frame is a latency bug — the
	// UI re-reads Status to see the model appear as pulled). context.Background is
	// deliberate: the download outlives the request that started it.
	go func() {
		bg := context.Background()
		s.publishPull(jobID, model, "starting", 0, "")
		err := s.prober.Pull(bg, model, func(status string, percent int) {
			s.publishPull(jobID, model, status, percent, "")
		})
		if err != nil {
			s.log.Warn("llm pull failed", "model", model, "err", err)
			s.publishPull(jobID, model, "error", -1, err.Error())
			return
		}
		s.log.Info("llm pull complete", "model", model)
		s.publishPull(jobID, model, "success", 100, "")
	}()
	return jobID, nil
}

// publishPull emits one pull-progress frame on the SSE bus (§7). type=llm_pull so
// the UI can filter; a dropped frame is a latency bug, never correctness (§8).
func (s *systemLLMService) publishPull(jobID, model, status string, percent int, errMsg string) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(events.Event{
		Type: "llm_pull",
		Payload: map[string]any{
			"jobId": jobID, "model": model, "status": status,
			"percent": percent, "error": errMsg,
		},
	})
}

// containsModel reports whether tag is among the pulled model names. Ollama reports
// tags verbatim (e.g. "qwen3:8b"); an exact match is what Select needs. A ":latest"
// convenience: "qwen3" also matches a pulled "qwen3:latest".
func containsModel(pulled []string, tag string) bool {
	for _, p := range pulled {
		if p == tag {
			return true
		}
		if !strings.Contains(tag, ":") && p == tag+":latest" {
			return true
		}
	}
	return false
}
