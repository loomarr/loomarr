package app

import (
	"context"
	"log/slog"
	"strings"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/events"
	"github.com/mantonx/loomarr/internal/llm"
	"github.com/mantonx/loomarr/internal/store"
)

// Settings-store keys for the in-app LLM selection (§8.1). Present ⇒ they override
// the LLM_* env defaults. The api_key is a secret (never echoed by any endpoint).
const (
	setLLMProvider = "llm.provider"
	setLLMURL      = "llm.url"
	setLLMModel    = "llm.model"
	setLLMAPIKey   = "llm.api_key" //nolint:gosec // settings key name, not a credential
)

// buildLLM constructs the suggester's provider (an llm.Swappable so an in-app
// selection hot-swaps it with no restart) plus the SystemLLMService that powers
// /v1/system/llm* (§8.1). The active selection is the persisted settings (if any)
// overlaid on the LLM_* env defaults, so a UI choice survives reboots.
func buildLLM(ctx context.Context, set resolved, st store.Store, bus *events.Bus, log *slog.Logger) (llm.Provider, api.SystemLLMService) {
	sel := resolveSelection(set)
	sw := llm.NewSwappable(buildProviderFor, sel)

	// Hot-swap on a persisted llm.* change (config-design §3 hot-apply, §8.1): the
	// in-app model picker writes the settings store; the Watch fires and the
	// Swappable rebuilds the live provider with no restart. Runs until ctx is done.
	go func() {
		ch := set.svc.Watch(setLLMProvider, setLLMURL, setLLMModel, setLLMAPIKey)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ch:
				sw.Set(resolveSelection(set))
				log.Info("llm provider hot-swapped from a settings change")
			}
		}
	}()

	svc := &systemLLMService{
		swap:       sw,
		ollamaBase: func() string { return ollamaBase(set) }, // resolved live per probe/pull
		store:      st,
		bus:        bus,
		log:        log,
		newID:      newID,
	}
	return sw, svc
}

// buildProviderFor is the Swappable factory: build the concrete provider for a
// Selection. Ollama for local, the OpenAI-compatible client for everything else.
func buildProviderFor(sel llm.Selection) llm.Provider {
	if sel.Provider == "ollama" || sel.Provider == "" {
		return llm.NewOllama(sel.URL, sel.Model)
	}
	return llm.NewOpenAI(sel.URL, sel.Model, sel.APIKey)
}

// resolveSelection reads the active LLM selection from the settings service
// (§8.1). The registry already resolves llm.provider/url/model as env > db >
// default; the per-provider api key is a namespaced key (llm.api_key.<provider>)
// the registry doesn't declare, so it's read from the store directly, falling
// back to the registry's base llm.api_key (the LLM_API_KEY env pin).
func resolveSelection(set resolved) llm.Selection {
	sel := llm.Selection{
		Provider: set.str(setLLMProvider),
		URL:      set.str(setLLMURL),
		Model:    set.str(setLLMModel),
		APIKey:   set.str(setLLMAPIKey),
	}
	if sel.Provider != "" && sel.Provider != "ollama" {
		if v, err := set.svc.LoadRaw(setLLMAPIKey + "." + sel.Provider); err == nil && v != "" {
			sel.APIKey = v
		}
	}
	return sel
}

// ollamaBase is the Ollama base URL for local probes/pulls. It's llm.url when the
// provider is ollama; otherwise the conventional default (a hosted-by-default
// install can still probe/manage a local Ollama if one is running).
func ollamaBase(set resolved) string {
	if set.str(setLLMProvider) == "ollama" && set.str(setLLMURL) != "" {
		return set.str(setLLMURL)
	}
	return "http://localhost:11434"
}

// systemLLMService implements api.SystemLLMService (§8.1). It probes the local host,
// serves the hosted catalog with live models, validates hosted keys, hot-swaps the
// active selection via the Swappable, persists to the settings store, and streams
// pulls over the event bus.
type systemLLMService struct {
	swap *llm.Swappable
	// ollamaBase resolves the Ollama host URL LIVE (llm.url via the settings
	// snapshot). The picker probe/pull must read the CURRENT url so configuring it
	// through the wizard takes effect with no restart (§8.1 / config-design §3) —
	// exactly like the suggester's Swappable hot-swaps. A frozen base URL was the
	// live-smoke bug: after PATCHing llm.url the picker still reported unreachable.
	ollamaBase func() string
	store      store.Store
	bus        *events.Bus
	log        *slog.Logger
	newID      func() string
}

// prober builds a Prober against the CURRENT Ollama base (cheap; stateless).
func (s *systemLLMService) prober() *llm.Prober { return llm.NewProber(s.ollamaBase()) }

func (s *systemLLMService) Status(ctx context.Context) (api.SystemLLMStatus, error) {
	sel := s.swap.Selection()
	local := sel.Provider == "ollama" || sel.Provider == ""

	out := api.SystemLLMStatus{
		Provider: sel.Provider,
		Model:    sel.Model,
		Local:    local,
	}

	// Local catalog: only meaningful when the active provider is Ollama (a probe of
	// a non-local install would just be an idle localhost:11434 poll — skip it).
	if local {
		p := s.prober()
		probe := p.Probe(ctx)
		out.GPUName, out.VRAMGiB, out.OllamaVer, out.Reachable = probe.GPUName, probe.VRAMGiB, probe.OllamaVersion, probe.Reachable
		for _, e := range p.Catalog(probe) {
			if e.Recommended {
				out.Recommended = e.Tag
			}
			out.Catalog = append(out.Catalog, api.LLMModelView{
				Tag: e.Tag, Label: e.Label, VRAMGiB: e.ApproxVRAMGiB,
				Fit: string(e.Fit), Pulled: e.Pulled, RuntimeOK: e.RuntimeOK,
				Recommended: e.Recommended, Why: e.Why,
			})
		}
	}

	// Hosted catalog: always present so the UI can offer a switch. For a provider
	// that has a key (stored or the active one), fetch its LIVE model list; else the
	// curated fallback. Keys are never included in the response.
	out.Hosted = s.hostedCatalog(ctx, sel)
	return out, nil
}

// hostedCatalog builds the hosted-provider views, fetching live models where a key
// is available and flagging the active provider + keyConfigured. Never returns keys.
func (s *systemLLMService) hostedCatalog(ctx context.Context, active llm.Selection) []api.HostedProviderView {
	var views []api.HostedProviderView
	for _, hp := range llm.HostedCatalog() {
		key := s.storedKeyFor(ctx, hp.Key)
		isActive := active.Provider == hp.Key
		// Use the active in-memory key if this is the active provider (covers a
		// just-selected key not yet re-read from the store).
		if isActive && active.APIKey != "" {
			key = active.APIKey
		}
		// The custom template carries no base URL; when it's the active provider the
		// live base lives on the selection, so surface it (and enable live models).
		if hp.Key == llm.CustomProviderKey && isActive {
			hp.BaseURL = active.URL
		}
		models, live := hp.Fallback, false
		if key != "" {
			models, live = hp.LiveModels(ctx, key)
		}
		view := api.HostedProviderView{
			Key: hp.Key, Label: hp.Label, BaseURL: hp.BaseURL, KeysURL: hp.KeysURL,
			Note: hp.Note, KeyConfigured: key != "", Active: isActive, ModelsLive: live,
		}
		for _, m := range models {
			view.Models = append(view.Models, api.HostedModelView{
				ID: m.ID, Label: m.Label, Why: m.Why, Recommended: m.Recommended, Tools: m.Tools,
			})
		}
		views = append(views, view)
	}
	return views
}

// storedKeyFor returns the persisted key for a hosted provider, or "" if none. The
// key is namespaced per provider so switching providers doesn't reuse the wrong key.
func (s *systemLLMService) storedKeyFor(ctx context.Context, provider string) string {
	if v, err := s.store.GetSetting(ctx, setLLMAPIKey+"."+provider); err == nil {
		return v
	}
	return ""
}

func (s *systemLLMService) Select(ctx context.Context, req api.SelectRequest) error {
	provider := req.Provider
	if provider == "" {
		provider = "ollama"
	}

	if provider == "ollama" {
		return s.selectLocal(ctx, req.Model)
	}
	return s.selectHosted(ctx, provider, req.BaseURL, req.Model, req.APIKey)
}

// hostedBase returns the curated provider with its BaseURL resolved to what we
// should actually drive: the caller's supplied URL for "custom" (the catalog entry
// has none), else the curated base. ok=false for an unknown provider or a custom
// select with no base URL. The returned hp is a value copy — safe to mutate.
func hostedBase(provider, suppliedBase string) (hp llm.HostedProvider, ok bool) {
	hp, found := llm.HostedProviderByKey(provider)
	if !found {
		return hp, false
	}
	if provider == llm.CustomProviderKey {
		hp.BaseURL = strings.TrimRight(suppliedBase, "/")
		return hp, hp.BaseURL != ""
	}
	return hp, true
}

// selectLocal swaps to a local Ollama model — it must be pulled first.
func (s *systemLLMService) selectLocal(ctx context.Context, model string) error {
	probe := s.prober().Probe(ctx)
	if !containsModel(probe.PulledModels, model) {
		return api.ErrModelNotPulled
	}
	sel := llm.Selection{Provider: "ollama", URL: s.ollamaBase(), Model: model}
	if err := s.persist(ctx, sel); err != nil {
		return err
	}
	s.swap.Set(sel)
	s.log.Info("llm selected", "provider", "ollama", "model", model)
	return nil
}

// selectHosted swaps to a hosted provider — the key (given or stored) is validated
// live before committing, so a bad key fails here, not on the next suggestion job.
func (s *systemLLMService) selectHosted(ctx context.Context, provider, baseURL, model, apiKey string) error {
	hp, ok := hostedBase(provider, baseURL)
	if !ok {
		return api.ErrUnknownProvider
	}
	// Reuse a stored key if the caller didn't supply one (re-selecting a model on an
	// already-configured provider shouldn't require re-pasting the key).
	if apiKey == "" {
		apiKey = s.storedKeyFor(ctx, provider)
	}
	if err := llm.ValidateKey(ctx, hp.BaseURL, apiKey); err != nil {
		s.log.Warn("hosted key validation failed", "provider", provider, "err", err)
		return api.ErrKeyInvalid
	}
	// Default the model when none was given — but LIVE-aware: prefer a curated
	// recommendation that STILL EXISTS on the provider (ids churn), else the first
	// live id. This keeps the empty-model default from ever selecting a stale/renamed
	// hardcoded id. LiveModels already returns curated-and-live first, so [0] is the
	// best current default; it falls back to the curated list only if /models fails.
	if model == "" {
		if live, _ := hp.LiveModels(ctx, apiKey); len(live) > 0 {
			model = live[0].ID
		}
	}
	if model == "" {
		return api.ErrUnknownProvider // nothing selectable — provider returned no models
	}
	sel := llm.Selection{Provider: provider, URL: hp.BaseURL, Model: model, APIKey: apiKey}
	if err := s.persist(ctx, sel); err != nil {
		return err
	}
	// Persist the key namespaced per provider so each provider keeps its own.
	if apiKey != "" {
		if err := s.store.SetSetting(ctx, setLLMAPIKey+"."+provider, apiKey); err != nil {
			return err
		}
	}
	s.swap.Set(sel)
	s.log.Info("llm selected", "provider", provider, "model", model)
	return nil
}

func (s *systemLLMService) Test(ctx context.Context, provider, baseURL, apiKey string) error {
	hp, ok := hostedBase(provider, baseURL)
	if !ok {
		return api.ErrUnknownProvider
	}
	if apiKey == "" {
		apiKey = s.storedKeyFor(ctx, provider)
	}
	return llm.ValidateKey(ctx, hp.BaseURL, apiKey)
}

func (s *systemLLMService) Pull(ctx context.Context, model string) (string, error) {
	// Pull is local-only — there's nothing to download for a hosted provider.
	if p := s.swap.Provider(); p != "ollama" && p != "" {
		return "", api.ErrNotLocal
	}
	jobID := s.newID()
	go func() {
		bg := context.Background()
		s.publishPull(jobID, model, "starting", 0, 0, 0, "")
		if err := s.prober().Pull(bg, model, func(pp llm.PullProgress) {
			s.publishPull(jobID, model, pp.Status, pp.Percent(), pp.Completed, pp.Total, "")
		}); err != nil {
			s.log.Warn("llm pull failed", "model", model, "err", err)
			s.publishPull(jobID, model, "error", -1, 0, 0, err.Error())
			return
		}
		s.log.Info("llm pull complete", "model", model)
		s.publishPull(jobID, model, "success", 100, 0, 0, "")
	}()
	return jobID, nil
}

// persist writes the provider/url/model settings (the non-secret trio). The key is
// stored separately, namespaced per provider (see selectHosted).
func (s *systemLLMService) persist(ctx context.Context, sel llm.Selection) error {
	if err := s.store.SetSetting(ctx, setLLMProvider, sel.Provider); err != nil {
		return err
	}
	if err := s.store.SetSetting(ctx, setLLMURL, sel.URL); err != nil {
		return err
	}
	return s.store.SetSetting(ctx, setLLMModel, sel.Model)
}

// publishPull emits one pull-progress frame on the SSE bus (§7, type=llm_pull).
// completed/total are bytes for the layer downloading (0 when unknown); the FE
// shows "X of Y GB" and derives rate/ETA from successive frames (§8.1).
func (s *systemLLMService) publishPull(jobID, model, status string, percent int, completed, total int64, errMsg string) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(events.Event{
		Type: "llm_pull",
		Payload: map[string]any{
			"jobId": jobID, "model": model, "status": status,
			"percent": percent, "completed": completed, "total": total, "error": errMsg,
		},
	})
}

// containsModel reports whether tag is among the pulled model names ("qwen3:8b");
// a bare "qwen3" also matches a pulled "qwen3:latest".
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
