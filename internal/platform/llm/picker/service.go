// Package picker is the language-model picker's service (§8.1): it probes the local Ollama, serves
// the hosted-provider catalog with live models, validates hosted keys, hot-swaps the active
// selection, persists the choice, and runs model pulls in the background.
//
// It speaks plain view types; the HTTP surface maps them to its own. Everything it reads or writes
// comes in through Config, so the composition root decides where settings, secrets and progress
// events live.
package picker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/platform/llm"
	"github.com/loomarr/loomarr/internal/platform/llm/selection"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store"
)

var (
	// ErrModelNotPulled: Select was asked for a local model the host hasn't downloaded.
	ErrModelNotPulled = errors.New("model not pulled")
	// ErrNotLocal: pull was called while the provider isn't local Ollama.
	ErrNotLocal = errors.New("provider is not local ollama")
	// ErrKeyInvalid: a hosted select/test key failed live validation.
	ErrKeyInvalid = errors.New("api key invalid or provider unreachable")
	// ErrUnknownProvider: a select/test named a provider not in the curated catalog.
	ErrUnknownProvider = errors.New("unknown provider")
	// ErrInvalidHostedModel: a curated hosted provider received a model id in the
	// wrong provider-specific shape (for example an Ollama name:tag for OpenRouter).
	ErrInvalidHostedModel = errors.New("hosted model must use a provider/model slug")
)

// ModelView is one catalog model annotated for the machine (§8.1).
type ModelView struct {
	Tag         string
	Label       string
	VRAMGiB     float64
	Fit         string
	Pulled      bool
	RuntimeOK   bool
	Tools       bool
	Vision      bool
	Recommended bool
	Why         string
}

// HostedModelView is one hosted model, ranked from live metadata (§8.1).
type HostedModelView struct {
	ID            string
	Label         string
	Why           string
	Recommended   bool
	Tools         bool
	Vision        bool
	Transcription bool
}

// HostedProviderView is one curated hosted provider (§8.1). No key.
type HostedProviderView struct {
	Key           string
	Label         string
	BaseURL       string
	KeysURL       string
	Note          string
	KeyConfigured bool
	Active        bool
	ModelsLive    bool
	Models        []HostedModelView
}

// DiscoverModelView is one downloadable local-model candidate, already sized + fit-ranked
// for this machine (§8.1).
type DiscoverModelView struct {
	ID          string
	Label       string
	Quant       string
	PullRef     string
	SizeGiB     float64
	Fit         string
	Downloads   int64
	Role        string
	Recommended bool
	Note        string
}

// Status is the LLM host and selection (§8.1).
type Status struct {
	Provider    string
	Model       string
	Local       bool
	GPUName     string
	VRAMGiB     float64
	OllamaVer   string
	Reachable   bool
	Recommended string
	Catalog     []ModelView
	Hosted      []HostedProviderView
}

// SelectRequest is a provider/model/key selection (§8.1).
type SelectRequest struct {
	Provider string
	Model    string
	APIKey   string
	BaseURL  string // required for the "custom" provider; ignored otherwise
}

// Launcher starts one request-launched operation under the generation that owns it.
type Launcher func(timeout time.Duration, run func(context.Context) error, done func(context.Context, error)) error

// OperationWriter persists the durable record of an interactive operation.
type OperationWriter interface {
	UpsertInteractiveOperation(context.Context, store.InteractiveOperation) error
}

// Settings writes the raw settings the registry does not declare.
type Settings interface {
	SetSetting(ctx context.Context, key, value string) error
}

// Secrets reads and writes the per-provider keys, which are protected at rest.
type Secrets interface {
	ReadKey(ctx context.Context, key string) (string, error)
	WriteKey(ctx context.Context, key, value string) error
}

// Events receives the progress of a model pull.
type Events interface {
	// PullProgress reports one pull-progress frame. completed and total are bytes for the layer
	// downloading (0 when unknown); the client derives rate and ETA from successive frames.
	PullProgress(jobID, model, status string, percent int, completed, total int64, errMsg string)
}

// Config is what the Service needs.
type Config struct {
	Swap *llm.Swappable
	// OllamaBase resolves the Ollama host URL LIVE (llm.url via the settings
	// snapshot). The picker probe/pull must read the CURRENT url so configuring it
	// through the wizard takes effect with no restart (§8.1 / config-design §3) —
	// exactly like the suggester's Swappable hot-swaps. A frozen base URL was the
	// live-smoke bug: after PATCHing llm.url the picker still reported unreachable.
	OllamaBase func() string
	// SaveSettings writes the non-secret llm.* trio through the SETTINGS SERVICE, not
	// straight to the store. Writing to the store directly skipped SetDB, so the
	// service's in-memory snapshot kept the old value: the picker said "In use" and
	// the suggester really did hot-swap (selectLocal calls swap.Set itself), while
	// every settings *read* — including the wizard's `llm` check — still saw the
	// previous value until the next reboot. The Watch in the composition root exists precisely
	// to react to this write, and it never fired for the same reason.
	SaveSettings func(ctx context.Context, edits map[string]string) error
	Settings     Settings
	Secrets      Secrets
	Events       Events
	Log          *slog.Logger
	NewID        func() string
	Start        Launcher
	Operations   OperationWriter
	PullTimeout  time.Duration
}

// discoverTTL is how long a compatible-download list is reused before re-hitting HF.
const discoverTTL = 10 * time.Minute

// Service is the picker.
type Service struct {
	c Config

	// discoverCache memoizes the machine-compatible download list. Building it fans out
	// N per-repo calls to Hugging Face, and HF rate-limits an anonymous IP (429) — so a
	// naive "fetch on every AI-page load" trips the limit fast. A short TTL keyed by
	// detected VRAM makes reloads instant and keeps us a good HF citizen; a fresh pull
	// (which changes what's installed, not what's downloadable) doesn't need to bust it.
	discoverMu    sync.Mutex
	discoverAt    time.Time
	discoverVRAM  float64
	discoverCache []DiscoverModelView
}

// New returns the picker over c.
func New(c Config) *Service { return &Service{c: c} }

// prober builds a Prober against the CURRENT Ollama base (cheap; stateless).
func (s *Service) prober() *llm.Prober { return llm.NewProber(s.c.OllamaBase()) }

// Status reports the host, the active selection and both catalogs.
func (s *Service) Status(ctx context.Context) (Status, error) {
	sel := s.c.Swap.Selection()
	local := sel.Provider == "ollama" || sel.Provider == ""

	out := Status{
		Provider: sel.Provider,
		Model:    sel.Model,
		Local:    local,
	}

	// A hosted lineup model can deliberately pair with local vision, so the role
	// picker always needs the installed Ollama catalog even when hosted is active.
	{
		p := s.prober()
		probe := p.Probe(ctx)
		out.GPUName, out.VRAMGiB, out.OllamaVer, out.Reachable = probe.GPUName, probe.VRAMGiB, probe.OllamaVersion, probe.Reachable
		for _, e := range p.Catalog(probe) {
			if e.Recommended {
				out.Recommended = e.Tag
			}
			out.Catalog = append(out.Catalog, ModelView{
				Tag: e.Tag, Label: e.Label, VRAMGiB: e.ApproxVRAMGiB,
				Fit: string(e.Fit), Pulled: e.Pulled, RuntimeOK: e.RuntimeOK,
				Tools: e.Tools, Vision: e.Vision, Recommended: e.Recommended, Why: e.Why,
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
func (s *Service) hostedCatalog(ctx context.Context, active llm.Selection) []HostedProviderView {
	var views []HostedProviderView
	for _, hp := range llm.HostedCatalog() {
		key := s.storedKeyFor(ctx, hp.Key)
		isActive := selection.HostedSelectionMatches(active, hp)
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
		view := HostedProviderView{
			Key: hp.Key, Label: hp.Label, BaseURL: hp.BaseURL, KeysURL: hp.KeysURL,
			Note: hp.Note, KeyConfigured: key != "", Active: isActive, ModelsLive: live,
		}
		for _, m := range models {
			view.Models = append(view.Models, HostedModelView{
				ID: m.ID, Label: m.Label, Why: m.Why, Recommended: m.Recommended, Tools: m.Tools,
				Vision: m.Vision, Transcription: m.Transcription,
			})
		}
		views = append(views, view)
	}
	return views
}

// storedKeyFor returns the persisted key for a hosted provider, or "" if none. The
// key is namespaced per provider so switching providers doesn't reuse the wrong key.
func (s *Service) storedKeyFor(ctx context.Context, provider string) string {
	if v, err := s.c.Secrets.ReadKey(ctx, settings.LLMAPIKey.Key()+"."+provider); err == nil {
		return v
	}
	// A provider configured through the ordinary settings form initially has the
	// generic "openai" wire kind and base llm.api_key. Match it by its canonical URL
	// so the picker can use that saved key to fetch/select a model; selectHosted then
	// persists the provider-branded namespaced copy.
	if s.c.Swap != nil {
		active := s.c.Swap.Selection()
		if hp, ok := llm.HostedProviderByKey(provider); ok && selection.HostedSelectionMatches(active, hp) {
			return active.APIKey
		}
	}
	return ""
}

// Select switches the active provider and model.
func (s *Service) Select(ctx context.Context, req SelectRequest) error {
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
func (s *Service) selectLocal(ctx context.Context, model string) error {
	probe := s.prober().Probe(ctx)
	if !containsModel(probe.PulledModels, model) {
		return ErrModelNotPulled
	}
	sel := llm.Selection{Provider: "ollama", URL: s.c.OllamaBase(), Model: model}
	if err := s.persist(ctx, sel); err != nil {
		return err
	}
	s.c.Swap.Set(sel)
	s.c.Log.Info("llm selected", "provider", "ollama", "model", model)
	return nil
}

// selectHosted swaps to a hosted provider — the key (given or stored) is validated
// live before committing, so a bad key fails here, not on the next suggestion job.
func (s *Service) selectHosted(ctx context.Context, provider, baseURL, model, apiKey string) error {
	hp, ok := hostedBase(provider, baseURL)
	if !ok {
		return ErrUnknownProvider
	}
	// OpenRouter ids are unambiguously namespaced (vendor/model). Reject a local
	// Ollama tag—or any other bare value—before touching credentials or the network.
	// Custom deliberately stays provider-defined: it may be Ollama's own /v1 mode.
	if provider == "openrouter" && !validOpenRouterModelID(model) {
		return ErrInvalidHostedModel
	}
	// Reuse a stored key if the caller didn't supply one (re-selecting a model on an
	// already-configured provider shouldn't require re-pasting the key).
	if apiKey == "" {
		apiKey = s.storedKeyFor(ctx, provider)
	}
	if err := llm.ValidateKey(ctx, hp.BaseURL, apiKey); err != nil {
		s.c.Log.Warn("hosted key validation failed", "provider", provider, "err", err)
		return ErrKeyInvalid
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
		return ErrUnknownProvider // nothing selectable — provider returned no models
	}
	sel := llm.Selection{Provider: provider, URL: hp.BaseURL, Model: model, APIKey: apiKey}
	if err := s.persist(ctx, sel); err != nil {
		return err
	}
	// Persist the key namespaced per provider so each provider keeps its own.
	if apiKey != "" {
		if err := s.c.Secrets.WriteKey(ctx, settings.LLMAPIKey.Key()+"."+provider, apiKey); err != nil {
			return err
		}
	}
	// Persist the branded key so the brand survives restart (llm.provider only held
	// the wire kind). selection.Resolve reads this back to restore Selection.Provider.
	if err := s.c.Settings.SetSetting(ctx, selection.HostedProviderSetting, provider); err != nil {
		return err
	}
	s.c.Swap.Set(sel)
	s.c.Log.Info("llm selected", "provider", provider, "model", model)
	return nil
}

func validOpenRouterModelID(model string) bool {
	if model == "" || strings.TrimSpace(model) != model || strings.ContainsAny(model, "\t\r\n ") {
		return false
	}
	if strings.HasSuffix(strings.ToLower(model), ":batch") {
		return false
	}
	vendor, id, found := strings.Cut(model, "/")
	return found && vendor != "" && id != ""
}

// Test validates a hosted provider + key without swapping.
func (s *Service) Test(ctx context.Context, provider, baseURL, apiKey string) error {
	hp, ok := hostedBase(provider, baseURL)
	if !ok {
		return ErrUnknownProvider
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
func (s *Service) Discover(ctx context.Context) ([]DiscoverModelView, error) {
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
		s.c.Log.Warn("llm discover failed", "err", err)
		return nil, err
	}
	out := make([]DiscoverModelView, 0, len(models))
	for _, m := range models {
		out = append(out, DiscoverModelView{
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

// Pull starts a background model download and returns its job id.
func (s *Service) Pull(ctx context.Context, model string) (string, error) {
	// Pull is local-only — there's nothing to download for a hosted provider.
	if p := s.c.Swap.Provider(); p != "ollama" && p != "" {
		return "", ErrNotLocal
	}
	if s.c.Start == nil || s.c.Operations == nil {
		return "", errors.New("model pull lifecycle is not configured")
	}
	jobID := s.c.NewID()
	now := time.Now
	operation := store.InteractiveOperation{
		ID: jobID, Kind: store.InteractiveOperationLLMPull, Subject: model,
		Status: store.InteractiveOperationQueued, StartedAt: now().UTC(), UpdatedAt: now().UTC(),
	}
	if err := s.c.Operations.UpsertInteractiveOperation(ctx, operation); err != nil {
		return "", fmt.Errorf("queue model pull: %w", err)
	}
	err := s.c.Start(s.c.PullTimeout, func(operationCtx context.Context) error {
		operation.Status = store.InteractiveOperationRunning
		operation.UpdatedAt = now().UTC()
		s.persistPullOperation(operationCtx, operation)
		s.publishPull(jobID, model, "starting", 0, 0, 0, "")
		return s.prober().Pull(operationCtx, model, func(pp llm.PullProgress) {
			operation.Percent, operation.Completed, operation.Total = pp.Percent(), pp.Completed, pp.Total
			operation.UpdatedAt = now().UTC()
			s.persistPullOperation(operationCtx, operation)
			s.publishPull(jobID, model, pp.Status, pp.Percent(), pp.Completed, pp.Total, "")
		})
	}, func(completionCtx context.Context, runErr error) {
		operation.CompletedAt, operation.UpdatedAt = now().UTC(), now().UTC()
		if runErr != nil {
			operation.Status = store.InteractiveOperationError
			operation.Error = runErr.Error()
			s.persistPullOperation(completionCtx, operation)
			s.c.Log.Warn("llm pull failed", "model", model, "err", runErr)
			s.publishPull(jobID, model, "error", operation.Percent, operation.Completed, operation.Total, runErr.Error())
			return
		}
		operation.Status = store.InteractiveOperationSuccess
		operation.Percent = 100
		s.persistPullOperation(completionCtx, operation)
		s.c.Log.Info("llm pull complete", "model", model)
		s.publishPull(jobID, model, "success", 100, operation.Completed, operation.Total, "")
	})
	if err != nil {
		operation.Status = store.InteractiveOperationError
		operation.Error = err.Error()
		operation.CompletedAt, operation.UpdatedAt = now().UTC(), now().UTC()
		s.persistPullOperation(ctx, operation)
		return "", err
	}
	return jobID, nil
}

func (s *Service) persistPullOperation(ctx context.Context, operation store.InteractiveOperation) {
	if err := s.c.Operations.UpsertInteractiveOperation(ctx, operation); err != nil && s.c.Log != nil {
		s.c.Log.Error("could not persist model pull state", "operation", operation.ID,
			"status", operation.Status, "err", err)
	}
}

// persist writes the provider/url/model settings (the non-secret trio) through the
// settings service, so the write hot-applies (config-design §3) instead of landing in
// the store where only the next boot would notice it. The API key is NOT written here:
// it is namespaced per provider (`llm.api_key.<provider>`, see selectHosted), which is
// not a registry key, so it cannot go through the registry-validated PATCH path.
func (s *Service) persist(ctx context.Context, sel llm.Selection) error {
	return s.c.SaveSettings(ctx, map[string]string{
		// The `llm.provider` setting is the WIRE KIND (enum: ollama | openai), not the
		// branded catalog key. Every hosted provider (openrouter, custom, …) is an
		// OpenAI-compatible client (see selection.Provider + hosted.go), so it persists as
		// "openai". Persisting the branded key here would 502 on the enum ("openrouter is
		// not one of [ollama openai]").
		// The branded key still lives on the in-memory Selection (for the openai/…
		// model ids) and namespaces the stored API key; only the coarse setting is
		// normalized to the wire kind.
		settings.LLMProvider.Key(): wireKind(sel.Provider),
		settings.LLMURL.Key():      sel.URL,
		settings.LLMModel.Key():    sel.Model,
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

// publishPull emits one pull-progress frame (§7, type=llm_pull).
func (s *Service) publishPull(jobID, model, status string, percent int, completed, total int64, errMsg string) {
	if s.c.Events == nil {
		return
	}
	s.c.Events.PullProgress(jobID, model, status, percent, completed, total, errMsg)
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
