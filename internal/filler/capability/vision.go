// Package capability resolves what this install can do for the filler detail work right now: which
// text model, speech-to-text and vision endpoint the settings name, the versions those choices
// stamp on the observations they produce, and the executors and signal loaders that run them.
//
// Every choice is read from settings on each call, so a change in Settings applies to the next clip
// rather than the next restart (config-design §8 hot-apply). The install's main language-model
// selection is injected, because filler does not own how it is resolved.
package capability

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/platform/llm"
	"github.com/loomarr/loomarr/internal/platform/metrics"
	"github.com/loomarr/loomarr/internal/platform/settings"
)

// visionWiring is the resolved endpoint the frame-reader talks to (§10 V54a, §15).
type visionWiring struct {
	provider, identity, url, model, key string
	// own reports that vision was pointed at its OWN service rather than inheriting the main
	// LLM's. It is logged, so "which endpoint is vision actually using" is answerable from the
	// process's output.
	own bool
}

// visionEndpoint resolves where clip frames get read, and by what. sel is the install's main
// language-model selection.
//
// ⚠ **`filler.vision.model` alone would be a half-separation, and the missing half fails silently.**
// If the provider were built from `llm.provider`/`llm.url`/`llm.api_key`, naming a LOCAL model
// while the main LLM was hosted would send an Ollama tag to the hosted endpoint (`llava:7b` →
// `https://openrouter.ai/api/v1` → 401 on every segment). `ground` reports that as zero looks, the
// gate refuses the reel with "a segment could not be classified", and nothing anywhere says why.
// A model name is useless without the host that serves it.
//
// ⚠ **The key is NEVER inherited once a vision provider is named.** Declaring a separate service
// means declaring its own credentials: inheriting would send the operator's hosted key to whatever
// host they just named, `localhost` included. A local Ollama needs none, so the common case sends
// nothing.
func visionEndpoint(set settings.Reader, sel llm.Selection) visionWiring {
	wireProvider := sel.Provider
	if wireProvider != "" && wireProvider != "ollama" {
		// OpenRouter and Custom are Loomarr's provider identities; both speak the
		// OpenAI-compatible wire used by the vision client.
		wireProvider = "openai"
	}
	v := visionWiring{
		provider: wireProvider,
		identity: sel.Provider,
		url:      sel.URL,
		key:      sel.APIKey,
		model:    settings.FillerVisionModel.Get(set),
	}
	// Empty ⇒ reuse the main model, for an install whose model already sees images. Same
	// separation `filler.language_model` makes for the audio gate.
	if v.model == "" {
		v.model = sel.Model
	}
	// ⚠ "" as well as `inherit`: the declared default is the word, but an env var set to empty
	// resolves to "" and means the same thing — inherit, not "no provider".
	if p := settings.FillerVisionProvider.Get(set); p != "" && p != "inherit" {
		v.provider, v.identity, v.url, v.key, v.own = p, p, settings.FillerVisionURL.Get(set), settings.FillerVisionAPIKey.Get(set), true
		// A local Ollama on the conventional port is the case worth needing no second setting —
		// the same default the probes and pulls apply.
		if v.url == "" && p == "ollama" {
			v.url = llm.DefaultOllamaBase
		}
	}
	return v
}

// HotVision re-resolves the vision endpoint on every call, so an operator changing
// `filler.vision.*` in Settings takes effect on the next clip rather than on the next restart
// (config-design §8 hot-apply).
//
// ⚠ **A provider captured at boot would be wrong here.** A settings subsystem whose whole point
// is `env > database > default` resolved live, wired to a knob that silently needs a restart,
// produces the worst kind of control: one that looks like it worked.
// The operator saves "Ollama", the next reel still 401s against the hosted endpoint, and nothing
// connects the two.
//
// ⚠ Memoised on the RESOLVED WIRING, not built per call. Rebuilding would discard the HTTP
// connection pool on every segment — 60 of them in one pass at the default `max_split_vision`.
type HotVision struct {
	set       settings.Reader
	selection func() llm.Selection
	log       *slog.Logger
	metrics   *metrics.Recorder

	mu   sync.Mutex
	raw  visionWiring // the wiring as settings resolved it: what memoisation compares
	last visionWiring // what was built, after reconcileModel
	cur  llm.VisionProvider
}

// NewHotVision returns the live vision provider. selection reads the install's main
// language-model selection; log and recorder may be nil.
func NewHotVision(set settings.Reader, selection func() llm.Selection, log *slog.Logger, recorder *metrics.Recorder) *HotVision {
	return &HotVision{set: set, selection: selection, log: log, metrics: recorder}
}

func (h *HotVision) AskAboutImages(ctx context.Context, prompt string, jpegs [][]byte) (llm.Response, error) {
	p, err := h.resolve()
	if err != nil {
		return llm.Response{}, err
	}
	return p.AskAboutImages(ctx, prompt, jpegs)
}

// Check resolves the provider for the current settings and reports why it cannot be built. The
// composition root calls it once at boot purely to say so; a failure is not fatal.
func (h *HotVision) Check() error {
	_, err := h.resolve()
	return err
}

// resolve returns the provider for the CURRENT settings, rebuilding only when they have changed.
//
// ⚠ The errors are the point of this method existing separately. `ground` treats a provider
// failure as "stop, try again next pass"; returning a nil provider with no error would be
// indistinguishable from vision being switched off.
func (h *HotVision) resolve() (llm.VisionProvider, error) {
	v := visionEndpoint(h.set, h.selection())
	if v.url == "" {
		return nil, fmt.Errorf("no vision endpoint configured: set filler.vision.url, or llm.url for the main provider")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cur != nil && h.raw == v {
		return h.cur, nil
	}
	raw := v
	v = h.reconcileModel(v)
	var p llm.VisionProvider
	switch v.provider {
	case "openai":
		p = llm.NewOpenAIForProvider(v.identity, v.url, v.model, v.key).WithMetrics(h.metrics)
	case "ollama":
		p = llm.NewOllama(v.url, v.model).WithMetrics(h.metrics)
	default:
		return nil, fmt.Errorf("provider %q has no vision path", v.provider)
	}
	if h.log != nil && h.raw != raw {
		// The line that answers "which endpoint is vision actually using" (V54a).
		h.log.Info("filler vision endpoint resolved",
			"provider", v.identity, "wire_provider", v.provider, "url", v.url, "model", v.model, "own_endpoint", v.own)
	}
	h.raw, h.last, h.cur = raw, v, p
	return p, nil
}

// reconcileModel answers a stale vision model honestly. With the provider inherited, filler.vision.model
// names a model the MAIN endpoint must serve; a name left over from another provider (an OpenRouter
// id against a llama.cpp server) was answered by whatever the server had loaded, so vision worked by
// accident and attribution recorded a model that never ran. Where the endpoint lists its models and
// does not list this one, say so once (the wiring is memoised) and use the main model. An endpoint
// that cannot list models is trusted: no list proves nothing. OpenRouter is a catalog whose ids are
// exactly these names, and a separately named vision service is the operator's declaration.
func (h *HotVision) reconcileModel(v visionWiring) visionWiring {
	main := h.selection()
	if v.provider != "openai" || v.own || v.identity == "openrouter" || v.model == main.Model || main.Model == "" {
		return v
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	served, err := llm.ServedModels(ctx, v.url, v.key)
	if err != nil || len(served) == 0 || slices.Contains(served, v.model) {
		return v
	}
	if h.log != nil {
		h.log.Warn("filler.vision.model is not served by the inherited LLM endpoint; using the main model instead — clear filler.vision.model or set it to a served model",
			"configured", v.model, "url", v.url, "using", main.Model, "served", served)
	}
	v.model = main.Model
	return v
}
