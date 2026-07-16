package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/mantonx/loomarr/internal/httpx"
)

// This file is the curated catalog of HOSTED (OpenAI-compatible) PROVIDERS Loomarr
// recommends (§8.1) — the hosted analog of catalog.go. The curation here is
// deliberately PROVIDER-LEVEL only (label, base URL, keys URL, note): those are
// stable and don't rot. The MODEL recommendations are NOT hardcoded — they are
// DERIVED from each provider's live /models metadata by rules (does it support
// tools? how cheap? how much context?), so they stay good as the catalog turns over
// year to year instead of pointing at renamed/retired ids. The only per-model
// hardcoding is a tiny FALLBACK list shown before a key is entered (when no live
// metadata is available), clearly marked as such.
//
// Every provider speaks POST {baseURL}/chat/completions with tools, so the ONE
// openai.go client drives them all — the entry just supplies the base URL.

// HostedModel is one model offered by a hosted provider (§8.1). For a live model
// most fields are derived from the provider's /models metadata; Recommended +Why
// are computed by the ranking rules (rankModels), NOT hardcoded per id.
type HostedModel struct {
	ID          string `json:"id"`                    // exact model id for the API (LLM_MODEL)
	Label       string `json:"label"`                 // human name (provider-supplied or the id)
	Why         string `json:"why,omitempty"`         // rule-derived rationale ("cheap, tool-capable")
	Recommended bool   `json:"recommended,omitempty"` // rule-selected as a top pick for grounding
	Tools       bool   `json:"tools,omitempty"`       // provider advertises tool-calling for this model
}

// HostedProvider is one curated OpenAI-compatible provider (§8.1). Only provider-
// level fields are curated; models come from live metadata (see LiveModels).
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
	// Fallback is a tiny list of model ids shown ONLY before a key is entered / when
	// the live list can't be fetched — a placeholder so the UI isn't empty. Live
	// metadata (LiveModels) supersedes it entirely. Kept short + provider-obvious.
	Fallback []HostedModel `json:"-"`
	// Note is a one-line provider-level hint (free tier, one-key-many-models, …).
	Note string `json:"note,omitempty"`
}

// hostedCatalog is the curated truth. IMPORTANT — the model ids here are NOT an
// allowlist and NOT the list the user picks from. The pickable list is fetched LIVE
// from each provider's /models (see LiveModels); these curated entries serve two
// narrower jobs: (1) ANNOTATION — overlay a human "why"/cost hint + top-of-list
// ranking onto the live ids we've vetted; (2) FALLBACK — a sensible placeholder list
// shown before a key is entered (when the live list can't be fetched). A curated id
// that gets renamed upstream degrades gracefully: the model still appears (from the
// live list), it just loses its hint until this list is refreshed (the sanctioned
// update path, like the local catalog). The PROVIDERS (base URLs, keys URLs) are the
// stable part; the models are live.
// The hosted surface is deliberately two entries (design §8.1): OpenRouter — the
// one blessed aggregator whose single key reaches every frontier family (OpenAI,
// Anthropic, Gemini, Llama, Qwen) with the richest live /models metadata — and
// Custom, a user-supplied OpenAI-compatible base URL for a direct vendor endpoint
// or a self-hosted runtime (vLLM/LM Studio/LocalAI/a gateway). We do NOT curate
// per-vendor entries: OpenRouter fronts them, and Custom reaches whatever it
// doesn't. CustomProviderKey has an empty BaseURL — the caller supplies it on
// select/test and it's gated by live validation, not by this list.
var hostedCatalog = []HostedProvider{
	{
		Key: "openrouter", Label: "OpenRouter", BaseURL: "https://openrouter.ai/api/v1",
		KeysURL: "https://openrouter.ai/keys",
		Note:    "One key → every frontier family (OpenAI, Anthropic, Gemini, Llama, Qwen). The blessed hosted path.",
		// Fallback: shown only before a key is set (no live metadata). Live ranking
		// supersedes this. Kept short + obvious; not an allowlist.
		Fallback: []HostedModel{{ID: "openai/gpt-4o-mini", Label: "GPT-4o mini"}},
	},
	{
		Key: CustomProviderKey, Label: "Custom (OpenAI-compatible)", BaseURL: "",
		KeysURL: "",
		Note:    "Any OpenAI-compatible /v1 endpoint you supply — a direct vendor, or self-hosted (vLLM, LM Studio, LocalAI, a gateway). Validated live before it's committed.",
	},
}

// CustomProviderKey is the pseudo-provider whose base URL the user supplies on
// select/test (rather than reading it from the curated catalog, §8.1).
const CustomProviderKey = "custom"

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

// modelMeta is the subset of a /models entry Loomarr ranks on. Rich providers
// (OpenRouter) populate SupportedParameters + Pricing; thin ones (OpenAI, Groq,
// Gemini) return little more than an id, in which case the rule engine degrades to
// "show the live ids" without a fabricated ranking.
type modelMeta struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Context    int      `json:"context_length"`
	SupportedP []string `json:"supported_parameters"`
	Pricing    struct {
		Prompt     string `json:"prompt"`
		Completion string `json:"completion"`
	} `json:"pricing"`
}

// supportsTools reports whether the provider advertises tool-calling for this model
// (the #1 grounding requirement). True when supported_parameters lists "tools". When
// the field is absent (thin provider), returns false — we don't guess capability.
func (m modelMeta) supportsTools() bool {
	return slices.Contains(m.SupportedP, "tools") || slices.Contains(m.SupportedP, "tool_choice")
}

// costPerMTok is a rough $/1M-token blend (prompt+completion) for ranking cheapest-
// first. Returns +Inf when pricing is absent/unparseable so unpriced models sort
// last among priced ones (but are still shown).
func (m modelMeta) costPerMTok() float64 {
	p := parsePrice(m.Pricing.Prompt)
	c := parsePrice(m.Pricing.Completion)
	if p < 0 && c < 0 {
		return math.Inf(1)
	}
	if p < 0 {
		p = 0
	}
	if c < 0 {
		c = 0
	}
	return (p + c) * 1_000_000 // per-token → per-1M
}

func parsePrice(s string) float64 {
	if s == "" {
		return -1
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return -1
	}
	return v
}

// familyTier maps a model FAMILY (substring of the id, provider-prefix-agnostic) to
// a quality tier for GROUNDED TOOL-CALLING — the one thing /models metadata can't
// tell us. Higher = better for picking themed titles reliably. We curate FAMILIES,
// not exact ids: "gpt-4o", "claude-sonnet" etc. survive across dated snapshots
// (gpt-4o-mini-2026-xx, …) and provider prefixes (openai/gpt-4o-mini on OpenRouter),
// so this barely rots — far slower than an exact-id list. This encodes JUDGMENT
// (these families reason well); availability + capability + cost stay live.
//
// Matched by case-insensitive substring so both "openai/gpt-4o-mini" (OpenRouter)
// and "gpt-4o-mini" (OpenAI direct) hit the same tier. Ordered most-specific first
// so "claude-sonnet" wins over a hypothetical bare "claude" entry.
type familyTier struct {
	family string // lowercase substring to match in the model id
	tier   int    // higher = better grounded tool-caller
	label  string // human family name for the rationale
}

var familyTiers = []familyTier{
	// Tier 3 — top grounded reasoners.
	{"gpt-4o", 3, "GPT-4o"},
	{"gpt-4.1", 3, "GPT-4.1"},
	{"claude-sonnet", 3, "Claude Sonnet"},
	{"claude-3.5-sonnet", 3, "Claude 3.5 Sonnet"},
	{"gemini-2.5-pro", 3, "Gemini 2.5 Pro"},
	{"gemini-1.5-pro", 3, "Gemini 1.5 Pro"},
	// Tier 2 — strong, cheaper workhorses (the sweet spot for a per-job suggester).
	{"claude-haiku", 2, "Claude Haiku"},
	{"claude-3.5-haiku", 2, "Claude 3.5 Haiku"},
	{"gemini-2.5-flash", 2, "Gemini 2.5 Flash"},
	{"gemini-1.5-flash", 2, "Gemini Flash"},
	{"llama-3.3", 2, "Llama 3.3"},
	{"llama-3.1", 2, "Llama 3.1"},
	{"qwen3", 2, "Qwen3"},
	{"qwen-2.5", 2, "Qwen 2.5"},
	{"mistral-large", 2, "Mistral Large"},
	// Tier 1 — capable but less proven for grounded JSON tool-use.
	{"mixtral", 1, "Mixtral"},
	{"gemma", 1, "Gemma"},
}

// tierOf returns the curated quality tier + family label for a model id (0 / "" if
// the family isn't tiered — such a model is still shown, just unranked).
func tierOf(id string) (int, string) {
	lid := strings.ToLower(id)
	for _, ft := range familyTiers {
		if strings.Contains(lid, ft.family) {
			return ft.tier, ft.label
		}
	}
	return 0, ""
}

// LiveModels fetches the provider's CURRENT models from {baseURL}/models and RANKS
// them by rules derived from the live metadata (§8.1) — NOT from hardcoded ids, so
// recommendations stay good as the catalog turns over:
//
//  1. HARD FILTER (when metadata is rich): keep only models the provider says support
//     tool-calling — grounding is impossible without it.
//  2. RANK: cheapest first (a suggestion job is small; cost dominates), context length
//     breaks ties. The top few tool-capable + cheap models are marked Recommended
//     with a rule-derived Why ("tool-calling, ~$X/1M tokens").
//
// When the provider returns THIN metadata (just ids — OpenAI/Groq/Gemini's /models),
// the rules can't rank, so it degrades gracefully to the live id list unranked (still
// current, just no "recommended" flags). On any fetch failure it returns the tiny
// curated Fallback with live=false so the UI is never empty.
func (hp HostedProvider) LiveModels(ctx context.Context, apiKey string) (models []HostedModel, live bool) {
	metas, err := fetchModels(ctx, hp.BaseURL, apiKey)
	if err != nil || len(metas) == 0 {
		return hp.Fallback, false
	}
	live = true

	// Does this provider expose capability metadata at all? If NONE of the models
	// advertise supported_parameters, it's a thin provider — don't hard-filter
	// (we'd drop everything) or rank on absent data.
	rich := false
	for _, m := range metas {
		if len(m.SupportedP) > 0 {
			rich = true
			break
		}
	}

	if !rich {
		// Thin provider: show the live ids as-is (current, unranked).
		for _, m := range metas {
			models = append(models, HostedModel{ID: m.ID, Label: labelOf(m)})
		}
		return models, live
	}

	// Rich provider: keep tool-capable models, then rank for the USE CASE — best
	// grounded tool-caller first, not merely cheapest:
	//   1. quality TIER (curated by family) descending — the durable judgment,
	//   2. cost ascending within a tier — cheaper of two equally-good families,
	//   3. bigger context as a final tie-break.
	// A tool-capable model with no tier (tier 0) sorts AFTER all tiered ones, so the
	// recommended set is always quality-first; untiered models remain selectable.
	var capable []modelMeta
	for _, m := range metas {
		if m.supportsTools() {
			capable = append(capable, m)
		}
	}
	if len(capable) == 0 {
		// No tool-capable model advertised — unusual, but don't hide everything.
		capable = metas
	}
	slices.SortStableFunc(capable, func(a, b modelMeta) int {
		ta, _ := tierOf(a.ID)
		tb, _ := tierOf(b.ID)
		if ta != tb {
			return tb - ta // higher tier first
		}
		if ca, cb := a.costPerMTok(), b.costPerMTok(); ca != cb {
			if ca < cb {
				return -1
			}
			return 1
		}
		return b.Context - a.Context
	})

	// Recommend the top-N — but ONLY tiered models (tier > 0). We never star an
	// untiered model just because it floated up: "recommended" must mean "a family
	// we vouch for for grounding", not "cheapest capable-looking".
	const recommendCount = 3
	recommended := 0
	for _, m := range capable {
		tier, fam := tierOf(m.ID)
		hm := HostedModel{ID: m.ID, Label: labelOf(m), Tools: m.supportsTools()}
		if tier > 0 && recommended < recommendCount {
			hm.Recommended = true
			hm.Why = whyFor(m, fam)
			recommended++
		}
		models = append(models, hm)
	}
	return models, live
}

// whyFor builds a rationale: the curated family (the quality signal) + live cost /
// context. The family is the durable "why this is good"; cost/context are live.
func whyFor(m modelMeta, family string) string {
	var parts []string
	if family != "" {
		parts = append(parts, family+" — strong grounded tool-caller")
	} else {
		parts = append(parts, "tool-calling")
	}
	if cost := m.costPerMTok(); !math.IsInf(cost, 1) {
		parts = append(parts, fmt.Sprintf("~$%.2f/1M tokens", cost))
	}
	if m.Context >= 100_000 {
		parts = append(parts, fmt.Sprintf("%dk context", m.Context/1000))
	}
	return strings.Join(parts, ", ")
}

func labelOf(m modelMeta) string {
	if m.Name != "" {
		return m.Name
	}
	return m.ID
}

// fetchModels GETs {baseURL}/models and returns the metadata entries. The OpenAI
// /models shape is {"data":[{...}]}; rich providers add supported_parameters/pricing.
func fetchModels(ctx context.Context, baseURL, apiKey string) ([]modelMeta, error) {
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
		Data []modelMeta `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	models := make([]modelMeta, 0, len(out.Data))
	for _, m := range out.Data {
		if m.ID != "" {
			models = append(models, m)
		}
	}
	return models, nil
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
