package app

import (
	"context"
	"strings"
	"testing"

	"github.com/mantonx/loomarr/internal/settings"
)

// visionSet builds a live settings service over the DECLARED registry, so these tests resolve
// exactly as boot does — including defaults, kinds and validation. A hand-rolled map would let a
// key that the registry rejects pass here and fail on the operator's box.
func visionSet(t *testing.T, rows map[string]string) resolved {
	t.Helper()
	loader := settings.StoreLoader{List: func(context.Context) ([]settings.SettingRow, error) {
		out := make([]settings.SettingRow, 0, len(rows))
		for k, v := range rows {
			out = append(out, settings.SettingRow{Key: k, Value: v, UpdatedBy: "test"})
		}
		return out, nil
	}}
	svc, err := settings.New(context.Background(), settings.NewRegistry(), loader, nil)
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}
	return resolved{svc: svc}
}

// Unset vision provider ⇒ byte-identical to the pre-V54a behaviour. Every existing install must
// keep working with no new settings.
func TestVisionEndpoint_InheritsTheMainProviderWhenUnset(t *testing.T) {
	t.Parallel()
	v := visionEndpoint(visionSet(t, map[string]string{
		"llm.provider": "openai", "llm.url": "https://openrouter.ai/api/v1",
		"llm.model": "openai/gpt-4o-mini", "llm.api_key": "sk-main",
	}))
	if v.provider != "openai" || v.url != "https://openrouter.ai/api/v1" {
		t.Errorf("provider/url = %q/%q, want the main LLM's", v.provider, v.url)
	}
	if v.key != "sk-main" {
		t.Errorf("key = %q, want the main key inherited when the host is the same", v.key)
	}
	if v.model != "openai/gpt-4o-mini" {
		t.Errorf("model = %q, want llm.model when filler.vision.model is empty", v.model)
	}
	if v.own {
		t.Error("own = true, but no vision provider was named")
	}
}

func TestVisionEndpoint_InheritsTheSelectedProvidersNamespacedKey(t *testing.T) {
	t.Parallel()
	v := visionEndpoint(visionSet(t, map[string]string{
		"llm.provider":           "openai",
		"llm.hosted_provider":    "openrouter",
		"llm.url":                "https://openrouter.ai/api/v1",
		"llm.model":              "openai/gpt-4o-mini",
		"llm.api_key.openrouter": "provider-secret",
	}))
	if v.key != "provider-secret" {
		t.Errorf("key = %q, want the selected provider's namespaced key", v.key)
	}
	if v.provider != "openai" {
		t.Errorf("wire provider = %q, want openai-compatible", v.provider)
	}
}

// ⚠ THE SECURITY PROPERTY. Naming a vision provider must not send the operator's hosted key to
// the host they just named — here, localhost. Inheriting it would be a credential leak to any
// address an operator can type.
func TestVisionEndpoint_NeverInheritsTheKeyOnceAProviderIsNamed(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, provider, url string }{
		{"local ollama", "ollama", ""},
		{"a different hosted service", "openai", "https://vision.example.com/v1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := visionEndpoint(visionSet(t, map[string]string{
				"llm.provider": "openai", "llm.url": "https://openrouter.ai/api/v1",
				"llm.model": "openai/gpt-4o-mini", "llm.api_key": "sk-main",
				"filler.vision.provider": tc.provider, "filler.vision.url": tc.url,
			}))
			if v.key != "" {
				t.Fatalf("key = %q — the main LLM's key leaked to %q", v.key, v.url)
			}
			if !v.own {
				t.Error("own = false, but a vision provider was named")
			}
		})
	}
}

// The common case — hosted text, local vision — needs the provider knob alone.
func TestVisionEndpoint_LocalOllamaNeedsNoURL(t *testing.T) {
	t.Parallel()
	v := visionEndpoint(visionSet(t, map[string]string{
		"llm.provider": "openai", "llm.url": "https://openrouter.ai/api/v1", "llm.api_key": "sk-main",
		"filler.vision.provider": "ollama", "filler.vision.model": "llava:7b",
	}))
	if v.url != defaultOllamaBase {
		t.Errorf("url = %q, want the conventional local Ollama %q", v.url, defaultOllamaBase)
	}
	if v.provider != "ollama" {
		t.Errorf("provider = %q, want ollama", v.provider)
	}
	if v.model != "llava:7b" {
		t.Errorf("model = %q, want the vision model", v.model)
	}
}

// An explicit vision key is used when the operator supplies one — the knob has to be usable for a
// genuinely hosted vision service, or "never inherit" would just mean "never authenticate".
func TestVisionEndpoint_UsesItsOwnKeyWhenGiven(t *testing.T) {
	t.Parallel()
	v := visionEndpoint(visionSet(t, map[string]string{
		"llm.provider": "openai", "llm.url": "https://openrouter.ai/api/v1", "llm.api_key": "sk-main",
		"filler.vision.provider": "openai", "filler.vision.url": "https://vision.example.com/v1",
		"filler.vision.api_key": "sk-vision",
	}))
	if v.key != "sk-vision" {
		t.Errorf("key = %q, want the vision-specific key", v.key)
	}
	if v.url != "https://vision.example.com/v1" {
		t.Errorf("url = %q, want the vision-specific URL", v.url)
	}
}

// ⚠ The regression this whole change exists for: a LOCAL model name against a HOSTED endpoint.
// Before V54a this combination silently produced `llava:7b` → openrouter.ai → 401 on every
// segment, reported only as "a segment could not be classified".
func TestVisionEndpoint_LocalModelDoesNotGoToTheHostedEndpoint(t *testing.T) {
	t.Parallel()
	v := visionEndpoint(visionSet(t, map[string]string{
		"llm.provider": "openai", "llm.url": "https://openrouter.ai/api/v1", "llm.api_key": "sk-main",
		"filler.vision.provider": "ollama", "filler.vision.model": "llava:7b",
	}))
	if v.url == "https://openrouter.ai/api/v1" {
		t.Fatal("a local vision model is still being sent to the hosted endpoint")
	}
}

// ⚠ Hot-apply is the property a settings subsystem promises (config-design §8), and the reason
// this is a test rather than a comment: a provider captured at boot produces a control that LOOKS
// like it worked. The operator saves "Ollama", the next reel still 401s against the hosted
// endpoint, and nothing connects the two.
func TestHotVisionProvider_PicksUpASettingsChangeWithoutARestart(t *testing.T) {
	t.Parallel()
	rows := map[string]string{
		"llm.provider": "openai", "llm.url": "https://openrouter.ai/api/v1",
		"llm.model": "openai/gpt-4o-mini", "llm.api_key": "sk-main",
	}
	h := &hotVisionProvider{set: visionSet(t, rows)}

	if _, err := h.resolve(); err != nil {
		t.Fatalf("resolve with the main provider: %v", err)
	}
	if h.last.url != "https://openrouter.ai/api/v1" {
		t.Fatalf("precondition: resolved %q, want the hosted endpoint", h.last.url)
	}
	first := h.cur

	// The operator points vision at their local Ollama. No restart.
	rows["filler.vision.provider"] = "ollama"
	rows["filler.vision.model"] = "llava:7b"
	h.set = visionSet(t, rows)

	if _, err := h.resolve(); err != nil {
		t.Fatalf("resolve after the change: %v", err)
	}
	if h.last.url != defaultOllamaBase {
		t.Errorf("url = %q, want %q — the change did not apply", h.last.url, defaultOllamaBase)
	}
	if h.cur == first {
		t.Error("the provider was not rebuilt after the endpoint changed")
	}
	if h.last.key != "" {
		t.Errorf("key = %q leaked to the local endpoint", h.last.key)
	}
}

// ⚠ Memoised on the resolved wiring: rebuilding per call would discard the HTTP connection pool
// on every segment — 60 of them in one pass at the default `max_split_vision`.
func TestHotVisionProvider_ReusesTheProviderWhileSettingsAreUnchanged(t *testing.T) {
	t.Parallel()
	h := &hotVisionProvider{set: visionSet(t, map[string]string{
		"llm.provider": "ollama", "llm.url": defaultOllamaBase, "llm.model": "llava:7b",
	})}
	first, err := h.resolve()
	if err != nil {
		t.Fatal(err)
	}
	again, err := h.resolve()
	if err != nil {
		t.Fatal(err)
	}
	if first != again {
		t.Error("the provider was rebuilt though nothing changed")
	}
}

// An unreachable configuration must produce an ERROR the caller can report, never a nil provider.
// `ground` treats a provider failure as "stop, retry next pass"; a nil was indistinguishable from
// vision being switched off, which is how this went unnoticed.
func TestHotVisionProvider_SaysWhyWhenThereIsNoEndpoint(t *testing.T) {
	t.Parallel()
	h := &hotVisionProvider{set: visionSet(t, map[string]string{"llm.provider": "openai", "llm.url": ""})}
	_, err := h.resolve()
	if err == nil {
		t.Fatal("no error for a vision provider with no endpoint")
	}
	if !strings.Contains(err.Error(), "filler.vision.url") {
		t.Errorf("error %q does not name the setting that fixes it", err)
	}
}
