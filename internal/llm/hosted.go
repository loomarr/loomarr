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

	"github.com/loomarr/loomarr/internal/httpx"
)

// This file is the curated catalog of HOSTED (OpenAI-compatible) PROVIDERS Loomarr
// exposes (§8.1) — the hosted analog of catalog.go. The curation here is
// deliberately PROVIDER-LEVEL only (label, base URL, keys URL, note): those are
// stable and don't rot. Live models are projected by advertised capability; model
// recommendations belong to the versioned role policy in role_policy.go. The only per-model
// hardcoding is a tiny FALLBACK list shown before a key is entered (when no live
// metadata is available), clearly marked as such.
//
// Every provider speaks POST {baseURL}/chat/completions with tools, so the ONE
// openai.go client drives them all — the entry just supplies the base URL.

// HostedModel is one model offered by a hosted provider (§8.1). For a live model
// most fields are derived from the provider's /models metadata. Recommended + Why
// are setup guidance derived from a small family preference table plus live price
// and capability metadata. They are not certification or execution authority.
type HostedModel struct {
	ID            string `json:"id"`                      // exact model id for the API (LLM_MODEL)
	Label         string `json:"label"`                   // human name (provider-supplied or the id)
	Why           string `json:"why,omitempty"`           // rule-derived rationale ("cheap, tool-capable")
	Recommended   bool   `json:"recommended,omitempty"`   // rule-selected as a top pick for grounding
	Tools         bool   `json:"tools,omitempty"`         // provider advertises tool-calling for this model
	Vision        bool   `json:"vision,omitempty"`        // provider advertises image input
	Video         bool   `json:"video,omitempty"`         // provider advertises direct video input
	Transcription bool   `json:"transcription,omitempty"` // provider advertises speech-to-text output
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

// hostedCatalog is the curated provider truth. Model ids are neither an allowlist nor
// quality advice: the pickable list is fetched live from /models. The tiny fallback
// only keeps setup usable before credentials or when that metadata endpoint fails.
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
		// Fallback: shown only before a key is set or live metadata is unavailable.
		Fallback: []HostedModel{{
			ID: "openai/gpt-5.4-mini", Label: "GPT-5.4 Mini",
			Recommended: true, Tools: true, Vision: true,
			Why: "Best balance — strong reasoning without flagship pricing.",
		}},
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

// modelMeta is the subset of a /models entry Loomarr projects into compatibility
// evidence. Rich providers populate capabilities and pricing; thin ones may return
// little more than an id, in which case Loomarr shows the ids without guessing.
type modelMeta struct {
	ID           string   `json:"id"`
	Aliases      []string `json:"aliases"` // llama.cpp: extra names the server answers to
	Name         string   `json:"name"`
	Context      int      `json:"context_length"`
	SupportedP   []string `json:"supported_parameters"`
	Architecture struct {
		InputModalities  []string `json:"input_modalities"`
		OutputModalities []string `json:"output_modalities"`
	} `json:"architecture"`
	Pricing struct {
		Prompt     string `json:"prompt"`
		Completion string `json:"completion"`
	} `json:"pricing"`
}

func (m modelMeta) supportsVision() bool {
	return slices.Contains(m.Architecture.InputModalities, "image")
}

func (m modelMeta) supportsVideo() bool {
	return slices.Contains(m.Architecture.InputModalities, "video")
}

func (m modelMeta) supportsTranscription() bool {
	return slices.Contains(m.Architecture.OutputModalities, "transcription")
}

// supportsTools reports whether the provider advertises tool-calling for this model
// (the #1 grounding requirement). True when supported_parameters lists "tools". When
// the field is absent (thin provider), returns false — we don't guess capability.
func (m modelMeta) supportsTools() bool {
	return slices.Contains(m.SupportedP, "tools") || slices.Contains(m.SupportedP, "tool_choice")
}

// blendedCostPerMTok is a deliberately simple comparison aid for models in the
// same preference tier. A lineup request uses both input and output tokens, so
// showing either price alone would make the cheaper half look like the whole bill.
func (m modelMeta) blendedCostPerMTok() float64 {
	prompt, promptOK := parseHostedPrice(m.Pricing.Prompt)
	completion, completionOK := parseHostedPrice(m.Pricing.Completion)
	if !promptOK && !completionOK {
		return math.Inf(1)
	}
	if !promptOK {
		prompt = 0
	}
	if !completionOK {
		completion = 0
	}
	return (prompt + completion) * 1_000_000
}

func parseHostedPrice(raw string) (float64, bool) {
	if raw == "" {
		return 0, false
	}
	price, err := strconv.ParseFloat(raw, 64)
	return price, err == nil && price >= 0
}

// hostedPreference is the small, durable judgment layer that live provider
// metadata cannot supply: which current families make useful lineup-planning
// choices. Availability, tool capability, price, and context remain live.
//
// The top three intentionally answer different questions: a balanced default, a
// value option, and a highest-quality option. Lightweight Luna/Nano families and
// arbitrary preview models remain searchable, but upstream order alone never
// promotes them as Loomarr guidance.
type hostedPreference struct {
	fragment string
	priority int
	role     string
	label    string
}

var hostedPreferences = []hostedPreference{
	{fragment: "gpt-5.4-mini", priority: 900, role: "Best balance", label: "GPT-5.4 Mini"},
	{fragment: "gemini-3-flash", priority: 800, role: "Best value", label: "Gemini 3 Flash"},
	{fragment: "gpt-5.6-sol", priority: 700, role: "Highest quality", label: "GPT-5.6 Sol"},
	{fragment: "claude-sonnet-4.6", priority: 690, role: "Highest quality", label: "Claude Sonnet 4.6"},
	{fragment: "gpt-5-mini", priority: 650, role: "Best value", label: "GPT-5 Mini"},
	{fragment: "gemini-2.5-flash", priority: 640, role: "Best value", label: "Gemini 2.5 Flash"},
	{fragment: "gpt-4.1-mini", priority: 600, role: "Best balance", label: "GPT-4.1 Mini"},
	{fragment: "claude-haiku-4.5", priority: 590, role: "Best value", label: "Claude Haiku 4.5"},
	{fragment: "gpt-4o-mini", priority: 580, role: "Best value", label: "GPT-4o Mini"},
	{fragment: "claude-sonnet", priority: 500, role: "Highest quality", label: "Claude Sonnet"},
	{fragment: "gemini-2.5-pro", priority: 490, role: "Highest quality", label: "Gemini 2.5 Pro"},
	{fragment: "gpt-4.1", priority: 480, role: "Highest quality", label: "GPT-4.1"},
}

func preferenceFor(id string) (hostedPreference, bool) {
	lower := strings.ToLower(id)
	for _, preference := range hostedPreferences {
		if strings.Contains(lower, preference.fragment) {
			return preference, true
		}
	}
	return hostedPreference{}, false
}

// LiveModels fetches the provider's CURRENT models from {baseURL}/models and
// projects advertised capabilities without inferring semantic quality:
//
//  1. HARD FILTER: exclude batch-only variants because Loomarr uses synchronous chat
//     completions. When metadata is rich, also keep only models the provider says
//     support tool-calling — grounding is impossible without it.
//  2. GUIDE setup with three differentiated, family-based choices. The table is
//     judgment only; availability, price, context, and capabilities stay live.
//
// When the provider returns THIN metadata (just ids — OpenAI/Groq/Gemini's /models),
// Loomarr degrades gracefully to the live id list without capability claims. On any
// fetch failure it returns the tiny
// curated Fallback with live=false so the UI is never empty.
func (hp HostedProvider) LiveModels(ctx context.Context, apiKey string) (models []HostedModel, live bool) {
	metas, err := fetchModels(ctx, hp.BaseURL, apiKey, hp.Key == "openrouter")
	if err != nil || len(metas) == 0 {
		return hp.Fallback, false
	}
	live = true

	// OpenRouter exposes Batch API variants in the same catalog as synchronous
	// chat-completions models. They can advertise tools yet return 404 on the endpoint
	// Loomarr uses, so they are never a selectable catalog entry.
	if hp.Key == "openrouter" {
		metas = slices.DeleteFunc(metas, func(m modelMeta) bool {
			return strings.HasSuffix(strings.ToLower(m.ID), ":batch")
		})
	}

	// Does this provider expose capability metadata at all? If NONE of the models
	// advertise supported_parameters, it's a thin provider — don't hard-filter
	// (we'd drop everything) or infer capabilities from absent data.
	rich := false
	for _, m := range metas {
		if len(m.SupportedP) > 0 || len(m.Architecture.InputModalities) > 0 || len(m.Architecture.OutputModalities) > 0 {
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

	// Rich provider: retain every model usable by at least one Loomarr role. Preferred
	// lineup families lead; everything else keeps provider order behind search.
	var usable []modelMeta
	for _, m := range metas {
		if !m.supportsTools() && !m.supportsVision() && !m.supportsVideo() && !m.supportsTranscription() {
			continue
		}
		usable = append(usable, m)
	}
	slices.SortStableFunc(usable, func(a, b modelMeta) int {
		pa, aPreferred := preferenceFor(a.ID)
		pb, bPreferred := preferenceFor(b.ID)
		if aPreferred != bPreferred {
			if aPreferred {
				return -1
			}
			return 1
		}
		if !aPreferred {
			return 0
		}
		if pa.priority != pb.priority {
			return pb.priority - pa.priority
		}
		if aCost, bCost := a.blendedCostPerMTok(), b.blendedCostPerMTok(); aCost != bCost {
			if aCost < bCost {
				return -1
			}
			return 1
		}
		return b.Context - a.Context
	})

	recommended := false
	for _, m := range usable {
		preference, preferred := preferenceFor(m.ID)
		model := HostedModel{
			ID: m.ID, Label: labelOf(m), Tools: m.supportsTools(),
			Vision: m.supportsVision(), Video: m.supportsVideo(), Transcription: m.supportsTranscription(),
		}
		if preferred && m.supportsTools() {
			model.Why = hostedWhy(m, preference)
			if !recommended {
				model.Recommended = true
				recommended = true
			}
		}
		models = append(models, model)
	}
	return models, live
}

func hostedWhy(model modelMeta, preference hostedPreference) string {
	detail := preference.label + ", tool-capable"
	if cost := model.blendedCostPerMTok(); !math.IsInf(cost, 1) {
		detail += fmt.Sprintf(", ~$%.2f/1M input + output tokens", cost)
	}
	if model.Context >= 100_000 {
		detail += fmt.Sprintf(", %dk context", model.Context/1000)
	}
	return preference.role + " — " + detail
}

func labelOf(m modelMeta) string {
	if m.Name != "" {
		return m.Name
	}
	return m.ID
}

// fetchModels GETs {baseURL}/models and returns the metadata entries. The OpenAI
// /models shape is {"data":[{...}]}; rich providers add supported_parameters/pricing.
func fetchModels(ctx context.Context, baseURL, apiKey string, allModalities bool) ([]modelMeta, error) {
	base := strings.TrimRight(baseURL, "/")
	modelsURL := base + "/models"
	if allModalities {
		modelsURL += "?output_modalities=all&sort=most-popular"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
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
