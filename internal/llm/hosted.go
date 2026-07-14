package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/mantonx/loomarr/internal/httpx"
)

// This file is the curated, in-code catalog of HOSTED (OpenAI-compatible) providers
// Loomarr knows how to recommend (§8.1) — the hosted analog of catalog.go. Like the
// local catalog it is deliberately NOT a live registry scrape: a fixed, reviewable
// list keeps recommendations deterministic and lets us encode Loomarr-specific truth
// (which models tool-call + JSON well for grounding, rough cost) a registry can't.
//
// Every provider here speaks POST {baseURL}/chat/completions with tools, so the ONE
// openai.go client drives them all — the entry just supplies the base URL + models.

// HostedModel is one recommended model for a hosted provider.
type HostedModel struct {
	ID    string `json:"id"`    // exact model id for the API (LLM_MODEL)
	Label string `json:"label"` // human name
	Why   string `json:"why"`   // one-line cost/suitability note
}

// HostedProvider is one curated OpenAI-compatible provider (§8.1).
type HostedProvider struct {
	// Key is Loomarr's stable identifier for the provider ("openrouter", "openai", …)
	// — used in select/test requests, NOT the API key.
	Key string `json:"key"`
	// Label is a human name ("OpenRouter").
	Label string `json:"label"`
	// BaseURL is the OpenAI-compatible base the openai client points at.
	BaseURL string `json:"baseUrl"`
	// KeysURL is where a user gets an API key (shown in the UI).
	KeysURL string `json:"keysUrl"`
	// Models are a few recommended tool-calling models. Not exhaustive — a user can
	// still type any model the provider serves; these are the vetted defaults.
	Models []HostedModel `json:"models"`
	// Note is a one-line provider-level hint (free tier, one-key-many-models, …).
	Note string `json:"note,omitempty"`
}

// hostedCatalog is the curated truth. Model ids move fast upstream — treat these as
// sensible defaults, refreshed as the sanctioned update path (like the local list),
// not a guarantee the id still exists. The user can always type a current id.
var hostedCatalog = []HostedProvider{
	{
		Key: "openrouter", Label: "OpenRouter", BaseURL: "https://openrouter.ai/api/v1",
		KeysURL: "https://openrouter.ai/keys",
		Note:    "One key → many providers (OpenAI, Anthropic, Gemini, Llama, Qwen). Most flexible.",
		Models: []HostedModel{
			{ID: "openai/gpt-4o-mini", Label: "GPT-4o mini", Why: "Cheap, excellent tool-caller — a strong default."},
			{ID: "google/gemini-2.5-flash", Label: "Gemini 2.5 Flash", Why: "Very cheap, great grounding; strong at owned-title recall."},
			{ID: "anthropic/claude-haiku-4.5", Label: "Claude Haiku 4.5", Why: "Fast Anthropic model; strong reasoning."},
		},
	},
	{
		Key: "openai", Label: "OpenAI", BaseURL: "https://api.openai.com/v1",
		KeysURL: "https://platform.openai.com/api-keys",
		Note:    "The reference OpenAI-compatible endpoint.",
		Models: []HostedModel{
			{ID: "gpt-4o-mini", Label: "GPT-4o mini", Why: "Cheap, excellent tool-caller — ~$0.0004/job."},
			{ID: "gpt-4o", Label: "GPT-4o", Why: "Higher quality, higher cost; strong grounding."},
		},
	},
	{
		Key: "anthropic", Label: "Anthropic (Claude)", BaseURL: "https://api.anthropic.com/v1",
		KeysURL: "https://console.anthropic.com/settings/keys",
		Note:    "Claude via its OpenAI-compatible endpoint. Strong grounding.",
		Models: []HostedModel{
			{ID: "claude-haiku-4-5", Label: "Claude Haiku 4.5", Why: "Fast + strong tool-use; a good balance."},
			{ID: "claude-sonnet-4-5", Label: "Claude Sonnet 4.5", Why: "Highest quality; higher cost."},
		},
	},
	{
		Key: "groq", Label: "Groq", BaseURL: "https://api.groq.com/openai/v1",
		KeysURL: "https://console.groq.com/keys",
		Note:    "Free tier, very fast inference. Good for a no-cost trial.",
		Models: []HostedModel{
			{ID: "llama-3.3-70b-versatile", Label: "Llama 3.3 70B", Why: "Free-tier, fast, strong tool-calling."},
		},
	},
	{
		Key: "gemini", Label: "Google Gemini", BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai",
		KeysURL: "https://aistudio.google.com/apikey",
		Note:    "Free tier available; very cheap paid.",
		Models: []HostedModel{
			{ID: "gemini-2.5-flash", Label: "Gemini 2.5 Flash", Why: "Cheap, solid tool-use; the A/B recall winner."},
		},
	},
}

// HostedCatalog returns the curated hosted-provider catalog (§8.1).
func HostedCatalog() []HostedProvider { return hostedCatalog }

// HostedProviderByKey looks up a curated provider by its Loomarr key. ok=false for an
// unknown key (the caller then rejects the select/test — we only drive vetted bases).
func HostedProviderByKey(key string) (HostedProvider, bool) {
	for _, p := range hostedCatalog {
		if p.Key == key {
			return p, true
		}
	}
	return HostedProvider{}, false
}

// LiveModels fetches the provider's CURRENT model ids from {baseURL}/models (§8.1),
// then overlays the curated annotations (label/why) on ids we recognize and marks
// the curated "recommended" ones. Hosted model ids churn upstream, so the live list
// is the source of truth for WHAT EXISTS; curation only adds guidance.
//
// On any failure (no key, unreachable, provider without a /models endpoint) it
// returns the curated fallback list with ok=false — the UI still shows sensible
// defaults, just flagged as not-live. This never errors hard: a stale-but-present
// picker beats an empty one.
func (hp HostedProvider) LiveModels(ctx context.Context, apiKey string) (models []HostedModel, live bool) {
	ids, err := fetchModelIDs(ctx, hp.BaseURL, apiKey)
	if err != nil || len(ids) == 0 {
		return hp.Models, false // curated fallback
	}
	// Index curated annotations by id for overlay.
	ann := map[string]HostedModel{}
	for _, m := range hp.Models {
		ann[m.ID] = m
	}
	// Build the live list. Recognized ids get their curated label/why; others get a
	// bare entry (still selectable). Curated recommendations that are STILL live are
	// surfaced first so the vetted defaults stay at the top of a long list.
	live = true
	seen := map[string]bool{}
	// 1) curated-and-still-live first, in curated order (the recommended defaults).
	for _, m := range hp.Models {
		if slices.Contains(ids, m.ID) {
			models = append(models, m)
			seen[m.ID] = true
		}
	}
	// 2) then everything else the provider returns, annotated if we know it.
	for _, id := range ids {
		if seen[id] {
			continue
		}
		if a, ok := ann[id]; ok {
			models = append(models, a)
		} else {
			models = append(models, HostedModel{ID: id, Label: id})
		}
		seen[id] = true
	}
	return models, live
}

// fetchModelIDs GETs {baseURL}/models and returns the model ids. The OpenAI
// /models shape is {"data":[{"id":"…"},…]}.
func fetchModelIDs(ctx context.Context, baseURL, apiKey string) ([]string, error) {
	base := strings.TrimRight(baseURL, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/models", nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	client := httpx.New(httpx.TimeoutProbe)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("models: status %d", resp.StatusCode)
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(out.Data))
	for _, m := range out.Data {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	return ids, nil
}

// ValidateKey does a cheap live check that a hosted provider + key is actually
// AUTHORIZED, WITHOUT running a suggestion (§8.1 test / validate-on-select).
//
// It does NOT use /models: that endpoint is a public catalog on some providers
// (notably OpenRouter — 200 with any key, even none), so it can't tell a good key
// from a bad one. Instead it sends a minimal 1-token chat/completions with the
// Bearer key — an actual inference call is the only thing guaranteed to exercise
// the key across every OpenAI-compatible provider. A 401/403 ⇒ bad key; a 2xx (or
// even a 400 "model not found"-style app error, which still means the key was
// accepted) ⇒ authorized. Costs a fraction of a cent.
func ValidateKey(ctx context.Context, baseURL, apiKey string) error {
	base := strings.TrimRight(baseURL, "/")
	body, _ := json.Marshal(map[string]any{
		// A model that exists on all the catalog providers isn't guaranteed, so we
		// don't assert a specific one — we only care whether the KEY is accepted.
		// Providers reject a bad key at auth (401) BEFORE resolving the model, so a
		// bad model id can't mask a bad key.
		"model":      "gpt-4o-mini",
		"messages":   []map[string]string{{"role": "user", "content": "hi"}},
		"max_tokens": 1,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	client := httpx.New(httpx.TimeoutProbe)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("could not reach %s: %w", base, err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("key rejected (%d) — check the API key", resp.StatusCode)
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil // authorized
	case resp.StatusCode == http.StatusBadRequest:
		// A 400 means the key was ACCEPTED but the request/model was rejected — the
		// key is valid (auth happens before request validation). Treat as authorized.
		return nil
	default:
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		if e.Error.Message != "" {
			return fmt.Errorf("provider returned %d: %s", resp.StatusCode, e.Error.Message)
		}
		return fmt.Errorf("provider returned %d", resp.StatusCode)
	}
}
