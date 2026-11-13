package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	opsapi "github.com/loomarr/loomarr/internal/api/ops"
	"github.com/loomarr/loomarr/internal/api/system"
	"github.com/loomarr/loomarr/internal/people/protectedsettings"
	"github.com/loomarr/loomarr/internal/people/secretprotection"
	"github.com/loomarr/loomarr/internal/platform/events"
	"github.com/loomarr/loomarr/internal/platform/llm"
	"github.com/loomarr/loomarr/internal/platform/llm/picker"
	"github.com/loomarr/loomarr/internal/platform/llm/selection"
	"github.com/loomarr/loomarr/internal/platform/metrics"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// modelPullTimeout bounds a multi-gigabyte streaming download without imposing an HTTP-client
// deadline. It is intentionally generous but finite: generation shutdown remains the earlier bound.
const modelPullTimeout = 6 * time.Hour

// resolveSelection reads the active LLM selection from the settings (§8.1).
func resolveSelection(set resolved) llm.Selection { return selection.Resolve(set, set.svc) }

// buildProviderFor is the Swappable factory: the concrete provider for a Selection.
func buildProviderFor(sel llm.Selection, recorder *metrics.Recorder) llm.Provider {
	return selection.Provider(sel, recorder)
}

// ollamaBase is the Ollama base URL for local probes and pulls.
func ollamaBase(set resolved) string { return selection.OllamaBase(set) }

// buildLLM constructs the suggester's provider (an llm.Swappable so an in-app
// selection hot-swaps it with no restart) plus the SystemLLMService that powers
// /v1/system/llm* (§8.1). The active selection is the persisted settings (if any)
// overlaid on the LLM_* env defaults, so a UI choice survives reboots.
func buildLLM(ctx context.Context, set resolved, st store.Store, bus *events.Bus, log *slog.Logger, metricRecorder *metrics.Recorder, owner *generationLifecycle) (llm.Provider, system.SystemLLMService) {
	sel := resolveSelection(set)
	sw := llm.NewSwappable(func(selection llm.Selection) llm.Provider {
		return buildProviderFor(selection, metricRecorder)
	}, sel)

	// Preload the model at boot (§8.2) so the first channel someone describes doesn't
	// pay the ~9s cold load. Backgrounded and best-effort: startup must not block on a
	// model load, and an Ollama that isn't up yet is a normal state at boot — the next
	// Chat loads the model itself, so a failure here costs latency, never correctness.
	selection.Warm(ctx, sw, log)

	// Hot-swap on a persisted llm.* change (config-design §3 hot-apply, §8.1): the
	// in-app model picker writes the settings store; the Watch fires and the
	// Swappable rebuilds the live provider with no restart. Runs until ctx is done.
	//
	// keep_alive is watched too: it is a property of how the provider is BUILT
	// (WithKeepAlive at construction), so without it here an operator's change would
	// sit in the store, apply to nothing, and appear to have been ignored until restart.
	owner.goRun(func(ctx context.Context) {
		ch := set.svc.Watch(settings.LLMProvider.Key(), settings.LLMURL.Key(), settings.LLMModel.Key(),
			settings.LLMAPIKey.Key(), settings.LLMKeepAlive.Key())
		for {
			select {
			case <-ctx.Done():
				return
			case <-ch:
				sw.Set(resolveSelection(set))
				log.Info("llm provider hot-swapped from a settings change")
				// Warm the newly-selected model: a swap points at weights that are almost
				// certainly not resident, and the operator who just picked one is the very
				// next person to run a job.
				selection.Warm(ctx, sw, log)
			}
		}
	})

	return sw, &systemLLMService{picker: picker.New(picker.Config{
		Swap:         sw,
		OllamaBase:   func() string { return ollamaBase(set) }, // resolved live per probe/pull
		SaveSettings: llmSettingsSaver(set, st, bus),
		Settings:     st,
		Secrets:      llmSecrets{store: st, protection: set.protection},
		Events:       llmPullEvents{bus: bus},
		Log:          log,
		NewID:        newID,
		Start:        owner.startInteractiveOperation,
		Operations:   st,
		PullTimeout:  modelPullTimeout,
	})}
}

// llmSettingsSaver persists the picker's llm.* edits through the settings service and announces a
// committed write on the live-update stream, so other tabs refetch settings-derived reads.
func llmSettingsSaver(set resolved, st store.Store, bus *events.Bus) func(context.Context, map[string]string) error {
	return func(ctx context.Context, edits map[string]string) error {
		results, err := set.svc.Patch(ctx, protectedsettings.NewPersister(st, set.protection), edits, "system")
		saved := false
		for _, r := range results {
			saved = saved || r.Status == settings.PatchSaved
		}
		// A persistence error can follow keys the batch already committed, so it announces too.
		if (saved || err != nil) && bus != nil {
			bus.Publish(events.Event{Type: "settings", Payload: opsapi.SettingsEvent{}})
		}
		if err != nil {
			return err
		}
		// A per-key refusal (an env pin, or a value the registry rejects) is NOT an
		// error from Patch's point of view — it is a result. Swallowing it would
		// recreate the bug in a new costume: the picker would report success while
		// the environment kept winning, and the operator's choice would evaporate
		// on restart with nothing having said no.
		for _, r := range results {
			if r.Status != settings.PatchSaved {
				return fmt.Errorf("could not save %s: %s", r.Key, r.Problem)
			}
		}
		return nil
	}
}

// llmSecrets reads and writes the picker's per-provider keys through the secret-protection
// manager, so they are protected at rest like every other stored secret.
type llmSecrets struct {
	store      store.Store
	protection *secretprotection.Manager
}

func (s llmSecrets) ReadKey(ctx context.Context, key string) (string, error) {
	return protectedsettings.Read(ctx, s.store, s.protection, key)
}

func (s llmSecrets) WriteKey(ctx context.Context, key, value string) error {
	return protectedsettings.Write(ctx, s.store, s.protection, key, value)
}

// llmPullEvents publishes a model pull's progress on the SSE bus (§7, type=llm_pull). The FE
// shows "X of Y GB" and derives rate/ETA from successive frames (§8.1).
type llmPullEvents struct{ bus *events.Bus }

func (e llmPullEvents) PullProgress(jobID, model, status string, percent int, completed, total int64, errMsg string) {
	if e.bus == nil {
		return
	}
	e.bus.Publish(events.Event{
		Type: "llm_pull",
		Payload: opsapi.LLMPullEvent{
			JobID: jobID, Model: model, Status: status,
			Percent: percent, Completed: completed, Total: total, Error: errMsg,
		},
	})
}

// systemLLMService implements system.SystemLLMService (§8.1) over the picker, mapping its
// view types to the API's.
type systemLLMService struct{ picker *picker.Service }

func (s *systemLLMService) Status(ctx context.Context) (system.SystemLLMStatus, error) {
	st, err := s.picker.Status(ctx)
	if err != nil {
		return system.SystemLLMStatus{}, err
	}
	out := system.SystemLLMStatus{
		Provider: st.Provider, Model: st.Model, Local: st.Local, GPUName: st.GPUName,
		VRAMGiB: st.VRAMGiB, OllamaVer: st.OllamaVer, Reachable: st.Reachable, Recommended: st.Recommended,
	}
	for _, m := range st.Catalog {
		out.Catalog = append(out.Catalog, system.LLMModelView(m))
	}
	for _, p := range st.Hosted {
		view := system.HostedProviderView{
			Key: p.Key, Label: p.Label, BaseURL: p.BaseURL, KeysURL: p.KeysURL, Note: p.Note,
			KeyConfigured: p.KeyConfigured, Active: p.Active, ModelsLive: p.ModelsLive,
		}
		for _, m := range p.Models {
			view.Models = append(view.Models, system.HostedModelView(m))
		}
		out.Hosted = append(out.Hosted, view)
	}
	return out, nil
}

func (s *systemLLMService) Select(ctx context.Context, req system.SelectRequest) error {
	return s.picker.Select(ctx, picker.SelectRequest(req))
}

func (s *systemLLMService) Test(ctx context.Context, provider, baseURL, apiKey string) error {
	return s.picker.Test(ctx, provider, baseURL, apiKey)
}

func (s *systemLLMService) Pull(ctx context.Context, model string) (string, error) {
	return s.picker.Pull(ctx, model)
}

func (s *systemLLMService) Discover(ctx context.Context) ([]system.DiscoverModelView, error) {
	models, err := s.picker.Discover(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]system.DiscoverModelView, 0, len(models))
	for _, m := range models {
		out = append(out, system.DiscoverModelView(m))
	}
	return out, nil
}
