// Package selection resolves which language model the install talks to: the active provider,
// URL, model, key and residency hint from the settings, the concrete provider built for them, and
// the warm-up that preloads it.
//
// The in-app LLM selection (§8.1) is the settings.LLMProvider/LLMURL/LLMModel/LLMAPIKey/
// LLMKeepAlive handles: a stored value overrides the LLM_* env defaults. The api key is a secret
// (never echoed by any endpoint) and keep_alive is the local model-residency hint (§8.2),
// local-only: the registry hides it for a hosted provider, and Provider only applies it to Ollama.
package selection

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/platform/llm"
	"github.com/loomarr/loomarr/internal/platform/metrics"
	"github.com/loomarr/loomarr/internal/platform/settings"
)

// HostedProviderSetting preserves the BRANDED hosted provider key (openrouter, custom, …)
// across restart. llm.provider only stores the wire kind (ollama|openai) — its
// enum can't hold a brand — so without this the brand (and thus the namespaced key
// + catalog Active match) would be lost on reload. A raw store setting, not a
// declared enum, so it accepts any catalog key. Empty ⇒ a plain ollama/openai.
const HostedProviderSetting = "llm.hosted_provider"

// Raw reads the settings the registry does not declare: the persisted brand and the per-provider
// keys, and which source set the base key.
type Raw interface {
	Provenance(key string) settings.Provenance
	LoadRaw(key string) (string, error)
}

// KeepAliveArg renders the resolved llm.keep_alive duration for the Ollama wire
// (§8.2). The registry parses it as a time.Duration; Ollama takes a string.
//
// A NEGATIVE value is Ollama's "keep loaded indefinitely", and 0 is "unload as soon as
// this request finishes" — both are meaningful, so neither may collapse to "". Only an
// unset key would yield "" (inherit Ollama's own default), which the registry's 30m
// default makes unreachable in practice; the branch stays because a default can change
// and silently swapping "unload now" for "inherit" would be invisible.
func KeepAliveArg(d time.Duration) string {
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

// Warm preloads the active model in the BACKGROUND (§8.2). Fire-and-forget by
// design: every caller is on a path that must not wait — app startup and the
// settings hot-swap watcher — and a warm-up is a latency optimization whose failure
// mode is simply the old behavior (the next Chat loads the model itself).
//
// A hosted provider isn't a Warmer, so Swappable.Warm no-ops and this costs one
// goroutine that returns immediately.
func Warm(ctx context.Context, w llm.Warmer, log *slog.Logger) {
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

// Provider is the Swappable factory: build the concrete provider for a
// Selection. Ollama for local, the OpenAI-compatible client for everything else.
func Provider(sel llm.Selection, recorder *metrics.Recorder) llm.Provider {
	if sel.Provider == "ollama" || sel.Provider == "" {
		// KeepAlive is local-only (§8.2) — a hosted endpoint has no residency to manage,
		// which is why it's applied here rather than in the shared construction below.
		return llm.NewOllama(sel.URL, sel.Model).WithKeepAlive(sel.KeepAlive).WithMetrics(recorder)
	}
	return llm.NewOpenAIForProvider(sel.Provider, sel.URL, sel.Model, sel.APIKey).WithMetrics(recorder)
}

// Resolve reads the active LLM selection from the settings
// (§8.1). The registry already resolves llm.provider/url/model as env > db >
// default; the per-provider api key is a namespaced key (llm.api_key.<provider>)
// the registry doesn't declare, so it's read from the store directly, falling
// back to the registry's base llm.api_key (the LLM_API_KEY env pin).
func Resolve(set settings.Reader, raw Raw) llm.Selection {
	sel := llm.Selection{
		Provider:  settings.LLMProvider.Get(set),
		URL:       settings.LLMURL.Get(set),
		Model:     settings.LLMModel.Get(set),
		APIKey:    settings.LLMAPIKey.Get(set),
		KeepAlive: KeepAliveArg(settings.LLMKeepAlive.Get(set)),
	}
	// Restore the BRANDED provider (openrouter, custom, …) that llm.provider flattened
	// to the "openai" wire kind: llm.hosted_provider holds the brand persisted at
	// select time. This keeps the namespaced-key lookup and catalog Active match
	// working across restart (an env-pinned openai with no brand stays "openai").
	//
	// The stored brand is only honoured while it still describes the resolved server: an
	// operator who later pins llm.url in the environment (env > db, config-design §3) is no
	// longer talking to the picker's provider, and re-branding would send the stale
	// namespaced key to the pinned server.
	envKey := raw.Provenance(settings.LLMAPIKey.Key()) == settings.ProvenanceEnv
	if sel.Provider == "openai" && !envKey {
		if v, err := raw.LoadRaw(HostedProviderSetting); err == nil && v != "" {
			if hp, ok := llm.HostedProviderByKey(v); ok && HostedSelectionMatches(sel, hp) {
				sel.Provider = v
			}
		}
	}
	// An env-pinned key is the operator's explicit choice: it outranks any persisted copy and
	// the persisted brand (whose only job is to pick that copy) is not applied at all.
	if sel.Provider != "" && sel.Provider != "ollama" && !envKey {
		if v, err := raw.LoadRaw(settings.LLMAPIKey.Key() + "." + sel.Provider); err == nil && v != "" {
			sel.APIKey = v
		}
	}
	return sel
}

// OllamaBase is the Ollama base URL for local probes/pulls. It's llm.url when the
// provider is ollama; otherwise the conventional default (a hosted-by-default
// install can still probe/manage a local Ollama if one is running).
func OllamaBase(set settings.Reader) string {
	if settings.LLMProvider.Get(set) == "ollama" && settings.LLMURL.Get(set) != "" {
		return settings.LLMURL.Get(set)
	}
	return llm.DefaultOllamaBase
}

// HostedSelectionMatches maps a generic persisted OpenAI-compatible selection back
// to the curated provider card. Before the first picker selection there is no branded
// llm.hosted_provider value yet, so the canonical base URL is the only honest signal.
func HostedSelectionMatches(sel llm.Selection, hp llm.HostedProvider) bool {
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
