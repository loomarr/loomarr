package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/events"
	"github.com/mantonx/loomarr/internal/llm"
	"github.com/mantonx/loomarr/internal/settings"
	"github.com/mantonx/loomarr/internal/store"
)

// Settings-store keys for the in-app LLM selection (§8.1). Present ⇒ they override
// the LLM_* env defaults. The api_key is a secret (never echoed by any endpoint).
const (
	setLLMProvider = "llm.provider"
	setLLMURL      = "llm.url"
	setLLMModel    = "llm.model"
	setLLMAPIKey   = "llm.api_key" //nolint:gosec // settings key name, not a credential
	// setLLMKeepAlive is the local model-residency hint (§8.2). Local-only: the
	// registry hides it for a hosted provider, and buildProviderFor only applies it
	// to Ollama.
	setLLMKeepAlive = "llm.keep_alive"
	// setLLMHosted preserves the BRANDED hosted provider key (openrouter, custom, …)
	// across restart. llm.provider only stores the wire kind (ollama|openai) — its
	// enum can't hold a brand — so without this the brand (and thus the namespaced key
	// + catalog Active match) would be lost on reload. A raw store setting, not a
	// declared enum, so it accepts any catalog key. Empty ⇒ a plain ollama/openai.
	setLLMHosted = "llm.hosted_provider"
)

// buildLLM constructs the suggester's provider (an llm.Swappable so an in-app
// selection hot-swaps it with no restart) plus the SystemLLMService that powers
// /v1/system/llm* (§8.1). The active selection is the persisted settings (if any)
// overlaid on the LLM_* env defaults, so a UI choice survives reboots.
func buildLLM(ctx context.Context, set resolved, st store.Store, bus *events.Bus, log *slog.Logger) (llm.Provider, api.SystemLLMService) {
	sel := resolveSelection(set)
	sw := llm.NewSwappable(buildProviderFor, sel)

	// Preload the model at boot (§8.2) so the first channel someone describes doesn't
	// pay the ~9s cold load. Backgrounded and best-effort: startup must not block on a
	// model load, and an Ollama that isn't up yet is a normal state at boot — the next
	// Chat loads the model itself, so a failure here costs latency, never correctness.
	warm(ctx, sw, log)

	// Hot-swap on a persisted llm.* change (config-design §3 hot-apply, §8.1): the
	// in-app model picker writes the settings store; the Watch fires and the
	// Swappable rebuilds the live provider with no restart. Runs until ctx is done.
	//
	// keep_alive is watched too: it is a property of how the provider is BUILT
	// (WithKeepAlive at construction), so without it here an operator's change would
	// sit in the store, apply to nothing, and appear to have been ignored until restart.
	go func() {
		ch := set.svc.Watch(setLLMProvider, setLLMURL, setLLMModel, setLLMAPIKey, setLLMKeepAlive)
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
				warm(ctx, sw, log)
			}
		}
	}()

	svc := &systemLLMService{
		swap:       sw,
		ollamaBase: func() string { return ollamaBase(set) }, // resolved live per probe/pull
		saveSettings: func(ctx context.Context, edits map[string]string) error {
			results, err := set.svc.Patch(ctx, storePersister{st: st}, edits, "system")
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
		},
		store: st,
		bus:   bus,
		log:   log,
		newID: newID,
	}
	return sw, svc
}

// keepAliveArg renders the resolved llm.keep_alive duration for the Ollama wire
// (§8.2). The registry parses it as a time.Duration; Ollama takes a string.
//
// A NEGATIVE value is Ollama's "keep loaded indefinitely", and 0 is "unload as soon as
// this request finishes" — both are meaningful, so neither may collapse to "". Only an
// unset key would yield "" (inherit Ollama's own default), which the registry's 30m
// default makes unreachable in practice; the branch stays because a default can change
// and silently swapping "unload now" for "inherit" would be invisible.
func keepAliveArg(d time.Duration) string {
	if d == 0 {
		return "0"
	}
	return d.String()
}

// warmTimeout bounds a background warm-up. Generous, because it covers a real
// cold model load from disk (the ~9s measured for an 8B model can be far longer for
// a large model on a slow disk), but bounded so a wedged Ollama can't leak a
// goroutine for the life of the process.
const warmTimeout = 2 * time.Minute

// warm preloads the active model in the BACKGROUND (§8.2). Fire-and-forget by
// design: every caller is on a path that must not wait — app startup and the
// settings hot-swap watcher — and a warm-up is a latency optimization whose failure
// mode is simply the old behavior (the next Chat loads the model itself).
//
// A hosted provider isn't a Warmer, so Swappable.Warm no-ops and this costs one
// goroutine that returns immediately.
func warm(ctx context.Context, w llm.Warmer, log *slog.Logger) {
	go func() {
		wctx, cancel := context.WithTimeout(ctx, warmTimeout)
		defer cancel()
		start := time.Now()
		switch err := w.Warm(wctx); {
		case errors.Is(err, llm.ErrNothingToWarm):
			// No model configured yet (a fresh install before the §8.1 picker runs).
			// Say nothing: there was no attempt to report, and logging either a
			// failure or a success here would describe something that didn't happen.
			return
		case err != nil:
			// Debug, not Warn: at boot an Ollama that hasn't started yet is ordinary,
			// and this is an optimization nobody asked for — logging it as a problem
			// would train operators to ignore a level that should mean something.
			log.Debug("llm warm-up skipped", "err", err)
			return
		}
		log.Info("llm model warmed", "took", time.Since(start).Round(time.Millisecond))
	}()
}

// buildProviderFor is the Swappable factory: build the concrete provider for a
// Selection. Ollama for local, the OpenAI-compatible client for everything else.
func buildProviderFor(sel llm.Selection) llm.Provider {
	if sel.Provider == "ollama" || sel.Provider == "" {
		// KeepAlive is local-only (§8.2) — a hosted endpoint has no residency to manage,
		// which is why it's applied here rather than in the shared construction below.
		return llm.NewOllama(sel.URL, sel.Model).WithKeepAlive(sel.KeepAlive)
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
		Provider:  set.str(setLLMProvider),
		URL:       set.str(setLLMURL),
		Model:     set.str(setLLMModel),
		APIKey:    set.str(setLLMAPIKey),
		KeepAlive: keepAliveArg(set.dur(setLLMKeepAlive)),
	}
	// Restore the BRANDED provider (openrouter, custom, …) that llm.provider flattened
	// to the "openai" wire kind: llm.hosted_provider holds the brand persisted at
	// select time. This keeps the namespaced-key lookup and catalog Active match
	// working across restart (an env-pinned openai with no brand stays "openai").
	if sel.Provider == "openai" {
		if v, err := set.svc.LoadRaw(setLLMHosted); err == nil && v != "" {
			sel.Provider = v
		}
	}
	if sel.Provider != "" && sel.Provider != "ollama" {
		if v, err := set.svc.LoadRaw(setLLMAPIKey + "." + sel.Provider); err == nil && v != "" {
			sel.APIKey = v
		}
	}
	return sel
}

// defaultOllamaBase is where a local Ollama lives when nothing says otherwise. Shared with
// `visionEndpoint` (§10 V54a) so the two cannot drift onto different ports.
const defaultOllamaBase = "http://localhost:11434"

// ollamaBase is the Ollama base URL for local probes/pulls. It's llm.url when the
// provider is ollama; otherwise the conventional default (a hosted-by-default
// install can still probe/manage a local Ollama if one is running).
func ollamaBase(set resolved) string {
	if set.str(setLLMProvider) == "ollama" && set.str(setLLMURL) != "" {
		return set.str(setLLMURL)
	}
	return defaultOllamaBase
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
	// saveSettings writes the non-secret llm.* trio through the SETTINGS SERVICE, not
	// straight to the store. Writing to the store directly skipped SetDB, so the
	// service's in-memory snapshot kept the old value: the picker said "In use" and
	// the suggester really did hot-swap (selectLocal calls swap.Set itself), while
	// every settings *read* — including the wizard's `llm` check — still saw the
	// previous value until the next reboot. The Watch in buildLLM exists precisely to
	// react to this write, and it never fired for the same reason.
	saveSettings func(ctx context.Context, edits map[string]string) error
	store        store.Store
	bus          *events.Bus
	log          *slog.Logger
	newID        func() string

	// discoverCache memoizes the machine-compatible download list. Building it fans out
	// N per-repo calls to Hugging Face, and HF rate-limits an anonymous IP (429) — so a
	// naive "fetch on every AI-page load" trips the limit fast. A short TTL keyed by
	// detected VRAM makes reloads instant and keeps us a good HF citizen; a fresh pull
	// (which changes what's installed, not what's downloadable) doesn't need to bust it.
	discoverMu    sync.Mutex
	discoverAt    time.Time
	discoverVRAM  float64
	discoverCache []api.DiscoverModelView
}

// discoverTTL is how long a compatible-download list is reused before re-hitting HF.
const discoverTTL = 10 * time.Minute

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
				Tools: e.Tools, Recommended: e.Recommended, Why: e.Why,
			})
		}
	}

	// Hosted catalog: always present so the UI can offer a switch. For a provider
	// that has a key (stored or the active one), fetch its LIVE model list; else the
	// curated fallback. Keys are never included in the response.
	out.Hosted = s.hostedCatalog(ctx, sel)

	// Reachability for a HOSTED active provider (§8.1): the Ollama probe above only
	// runs for local, so without this a working OpenRouter/OpenAI reported
	// reachable:false and the "Test connection" button lied. A hosted provider is
	// reachable iff we just fetched its LIVE model list (ModelsLive) — a real GET of
	// its /models with the configured key, so no extra call and no key in the reply.
	if !local {
		for _, hv := range out.Hosted {
			if hv.Active {
				out.Reachable = hv.ModelsLive
				break
			}
		}
	}
	return out, nil
}

// hostedCatalog builds the hosted-provider views, fetching live models where a key
// is available and flagging the active provider + keyConfigured. Never returns keys.
func (s *systemLLMService) hostedCatalog(ctx context.Context, active llm.Selection) []api.HostedProviderView {
	var views []api.HostedProviderView
	for _, hp := range llm.HostedCatalog() {
		key := s.storedKeyFor(ctx, hp.Key)
		isActive := hostedSelectionMatches(active, hp)
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
	// A provider configured through the ordinary settings form initially has the
	// generic "openai" wire kind and base llm.api_key. Match it by its canonical URL
	// so the picker can use that saved key to fetch/select a model; selectHosted then
	// persists the provider-branded namespaced copy.
	if s.swap != nil {
		active := s.swap.Selection()
		if hp, ok := llm.HostedProviderByKey(provider); ok && hostedSelectionMatches(active, hp) {
			return active.APIKey
		}
	}
	return ""
}

// hostedSelectionMatches maps a generic persisted OpenAI-compatible selection back
// to the curated provider card. Before the first picker selection there is no branded
// llm.hosted_provider value yet, so the canonical base URL is the only honest signal.
func hostedSelectionMatches(sel llm.Selection, hp llm.HostedProvider) bool {
	if sel.Provider == hp.Key {
		return true
	}
	if sel.Provider != "openai" || sel.URL == "" {
		return false
	}
	openRouter, _ := llm.HostedProviderByKey("openrouter")
	if hp.Key == "openrouter" {
		return strings.TrimRight(sel.URL, "/") == strings.TrimRight(openRouter.BaseURL, "/")
	}
	return hp.Key == llm.CustomProviderKey &&
		strings.TrimRight(sel.URL, "/") != strings.TrimRight(openRouter.BaseURL, "/")
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
	// OpenRouter ids are unambiguously namespaced (vendor/model). Reject a local
	// Ollama tag—or any other bare value—before touching credentials or the network.
	// Custom deliberately stays provider-defined: it may be Ollama's own /v1 mode.
	if provider == "openrouter" && !validOpenRouterModelID(model) {
		return api.ErrInvalidHostedModel
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
	// Persist the branded key so the brand survives restart (llm.provider only held
	// the wire kind). resolveSelection reads this back to restore Selection.Provider.
	if err := s.store.SetSetting(ctx, setLLMHosted, provider); err != nil {
		return err
	}
	s.swap.Set(sel)
	s.log.Info("llm selected", "provider", provider, "model", model)
	return nil
}

func validOpenRouterModelID(model string) bool {
	if model == "" || strings.TrimSpace(model) != model || strings.ContainsAny(model, "\t\r\n ") {
		return false
	}
	vendor, id, found := strings.Cut(model, "/")
	return found && vendor != "" && id != ""
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

// Discover returns downloadable local models compatible with THIS machine (§8.1):
// popular Hugging Face GGUF repos sized against detected VRAM, best-fitting quant
// chosen, ranked best-first. It probes VRAM the same way the local catalog does, so
// the ranking matches the installed list's fit verdicts. A source outage logs +
// returns the error (the handler degrades it to an empty list + a "browse on
// huggingface.co" link); it never fails the page.
func (s *systemLLMService) Discover(ctx context.Context) ([]api.DiscoverModelView, error) {
	probe := s.prober().Probe(ctx)
	vram := probe.VRAMGiB

	// Serve a fresh-enough cached list for the same VRAM without re-hitting HF. This is
	// what stops repeated AI-page loads from tripping HF's anonymous rate limit (429).
	s.discoverMu.Lock()
	if s.discoverCache != nil && s.discoverVRAM == vram && time.Since(s.discoverAt) < discoverTTL {
		cached := s.discoverCache
		s.discoverMu.Unlock()
		return cached, nil
	}
	s.discoverMu.Unlock()

	models, err := llm.DiscoverCompatible(ctx, vram, probe.PulledModels)
	if err != nil {
		s.log.Warn("llm discover failed", "err", err)
		return nil, err
	}
	out := make([]api.DiscoverModelView, 0, len(models))
	for _, m := range models {
		out = append(out, api.DiscoverModelView{
			ID: m.ID, Label: m.Label, Quant: m.Quant, PullRef: m.PullRef,
			SizeGiB: m.SizeGiB, Fit: string(m.Fit), Downloads: m.Downloads,
			Role: string(m.Role), Recommended: m.Recommended, Note: m.Note,
		})
	}
	s.discoverMu.Lock()
	s.discoverCache, s.discoverVRAM, s.discoverAt = out, vram, time.Now()
	s.discoverMu.Unlock()
	return out, nil
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

// persist writes the provider/url/model settings (the non-secret trio) through the
// settings service, so the write hot-applies (config-design §3) instead of landing in
// the store where only the next boot would notice it. The API key is NOT written here:
// it is namespaced per provider (`llm.api_key.<provider>`, see selectHosted), which is
// not a registry key, so it cannot go through the registry-validated PATCH path.
func (s *systemLLMService) persist(ctx context.Context, sel llm.Selection) error {
	return s.saveSettings(ctx, map[string]string{
		// The `llm.provider` setting is the WIRE KIND (enum: ollama | openai), not the
		// branded catalog key. Every hosted provider (openrouter, custom, …) is an
		// OpenAI-compatible client (see buildProviderFor + hosted.go), so it persists as
		// "openai". Persisting the branded key here 502'd on the enum ("openrouter is
		// not one of [ollama openai]") — the bug that made the hosted picker unusable.
		// The branded key still lives on the in-memory Selection (for the openai/…
		// model ids) and namespaces the stored API key; only the coarse setting is
		// normalized to the wire kind.
		setLLMProvider: wireKind(sel.Provider),
		setLLMURL:      sel.URL,
		setLLMModel:    sel.Model,
	})
}

// wireKind maps a (possibly branded) provider to the llm.provider setting enum: a
// local Ollama stays "ollama"; every hosted provider is an OpenAI-compatible client.
func wireKind(provider string) string {
	if provider == "ollama" || provider == "" {
		return "ollama"
	}
	return "openai"
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
		Payload: api.LLMPullEvent{
			JobID: jobID, Model: model, Status: status,
			Percent: percent, Completed: completed, Total: total, Error: errMsg,
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
