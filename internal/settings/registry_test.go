package settings

import (
	"strings"
	"testing"
	"time"
)

// The declared registry must build without panicking — no duplicate keys/envs,
// no malformed enum, every key+env non-empty. This is the contract's smoke test.
func TestRegistry_BuildsCleanly(t *testing.T) {
	r := NewRegistry()
	if len(r.All()) == 0 {
		t.Fatal("registry is empty")
	}
	// Every setting parses its own default (a default that won't parse is a bug in
	// the declaration — resolution would fall through to a zero value silently).
	for _, s := range r.All() {
		if s.Default == nil {
			continue
		}
		raw := defaultRaw(t, s)
		if raw == "" {
			continue // empty default (e.g. a secret / optional URL) — nothing to parse
		}
		if _, err := s.parse(raw); err != nil {
			t.Errorf("%s: declared default %q does not parse: %v", s.Key, raw, err)
		}
	}
}

// Registry invariants that catch drift: keys are dotted, envs are SCREAMING_SNAKE,
// enum settings have values, secrets have no non-empty default (never ship a secret).
func TestRegistry_Invariants(t *testing.T) {
	all := NewRegistry().All()
	known := make(map[string]bool, len(all))
	for _, s := range all {
		known[s.Key] = true
	}
	for _, s := range all {
		if strings.ToUpper(s.EnvVar) != s.EnvVar {
			t.Errorf("%s: env var %q should be upper-case", s.Key, s.EnvVar)
		}
		if s.Key != strings.ToLower(s.Key) {
			t.Errorf("%s: key should be lower-case", s.Key)
		}
		if s.Kind == KindEnum && len(s.Enum) == 0 {
			t.Errorf("%s: enum setting has no values", s.Key)
		}
		// Every enum option must carry both a value AND a display label (config-design §5):
		// the label is registry-owned so the UI never re-derives proper-noun casing.
		for _, o := range s.Enum {
			if o.Value == "" || o.Label == "" {
				t.Errorf("%s: enum option %+v must have a value and a label", s.Key, o)
			}
		}
		// A ShowWhen controller must reference a real key, or the field's conditional
		// visibility silently never (or always) fires.
		for ctrlKey, allowed := range s.ShowWhen {
			if !known[ctrlKey] {
				t.Errorf("%s: ShowWhen references unknown key %q", s.Key, ctrlKey)
			}
			if len(allowed) == 0 {
				t.Errorf("%s: ShowWhen[%q] lists no allowed values", s.Key, ctrlKey)
			}
		}
		if s.IsSecret() {
			if str, ok := s.Default.(string); ok && str != "" {
				t.Errorf("%s: a secret must not ship a non-empty default", s.Key)
			}
		}
		if s.Doc == "" {
			t.Errorf("%s: missing Doc (UI help + generated docs derive from it)", s.Key)
		}
	}
}

// The AI provider keys are the conditional-field showcase (config-design §5): url + key
// are hidden for Ollama (local, no key), shown for a hosted OpenAI-compatible service.
func TestRegistry_AIConditionalFields(t *testing.T) {
	reg := NewRegistry()
	for _, key := range []string{"llm.url", "llm.api_key"} {
		s, ok := reg.Get(key)
		if !ok {
			t.Fatalf("%s not declared", key)
		}
		allowed := s.ShowWhen["llm.provider"]
		if len(allowed) != 1 || allowed[0] != "openai" {
			t.Errorf("%s should ShowWhen llm.provider=openai, got %v", key, s.ShowWhen)
		}
	}
}

// The §8.1 model-selection keys already persisted by systemllm.go must be exactly
// the ones the registry declares — else the in-app picker and the registry drift.
func TestRegistry_LLMKeysMatchModelSelection(t *testing.T) {
	r := NewRegistry()
	for _, k := range []string{"llm.provider", "llm.url", "llm.model", "llm.api_key"} {
		if _, ok := r.Get(k); !ok {
			t.Errorf("registry missing %q (systemllm.go persists it)", k)
		}
	}
}

// The defaults that MOVED from config.Config to the registry (config-design §1
// shrink) resolve to their design.md §15 values — coverage that used to live in
// config_test.go, preserved here where the keys now live.
func TestRegistry_MovedDefaults(t *testing.T) {
	r := NewRegistry()
	cases := map[string]string{
		"request.ttl":      "48h",
		"session.ttl":      "720h",
		"reconcile.every":  "5m",
		"job.workers":      "2",
		"season.precision": "series",
		"llm.provider":     "ollama",
		"sched.ordering":   "syndication",
	}
	for key, want := range cases {
		s, ok := r.Get(key)
		if !ok {
			t.Errorf("registry missing moved key %q", key)
			continue
		}
		if got := defaultRaw(t, s); got != want {
			t.Errorf("%s default = %q, want %q", key, got, want)
		}
	}
}

// A duplicate key must panic — the contract can't silently shadow a key.
func TestRegistry_DuplicateKeyPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected panic on duplicate key")
		}
	}()
	newRegistry([]Setting{
		{Key: "a", EnvVar: "A", Kind: KindString, Doc: "x"},
		{Key: "a", EnvVar: "B", Kind: KindString, Doc: "y"},
	})
}

// URL normalization: scheme required, trailing slash stripped (config-design §9).
func TestRegistry_URLNormalization(t *testing.T) {
	s := Setting{Key: "library.url", Kind: KindURL}
	got, err := s.parse("http://emby:8096/")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got != "http://emby:8096" {
		t.Errorf("normalized = %q, want trailing slash stripped", got)
	}
	if _, err := s.parse("emby:8096"); err == nil {
		t.Error("expected error on schemeless URL")
	}
}

// KindCron validates via the cron parser (§18.1): a valid 6-field expr passes through; an
// invalid one is rejected like any bad setting.
func TestRegistry_CronValidation(t *testing.T) {
	s := Setting{Key: "job.reconcile.schedule", Kind: KindCron}
	got, err := s.parse("0 */5 * * * *")
	if err != nil {
		t.Fatalf("valid cron rejected: %v", err)
	}
	if got != "0 */5 * * * *" {
		t.Errorf("cron = %q, want passed through", got)
	}
	if _, err := s.parse("every 5 minutes plz"); err == nil {
		t.Error("expected error on an invalid cron expression")
	}
}

// Secret shape sanity guard (config-design §9): trims surrounding whitespace,
// rejects internal whitespace or a too-short value — so a stray error string or a
// fat-fingered fragment can't be stored as a token/key. The message is secret-safe
// (redacted upstream in patchProblem), so this checks the accept/reject decision and
// the trimming, not the message text.
func TestRegistry_SecretShapeGuard(t *testing.T) {
	s := Setting{Key: "library.token", Kind: KindSecret}

	// The exact corruption that motivated the guard: a connection-test hint got stored
	// as library.token, then every probe 401'd. It has spaces, so it's rejected now.
	if _, err := s.parse("set a media server flavor (emby | jellyfin)"); err == nil {
		t.Error("expected the space-containing error string to be rejected as a secret")
	}
	// Too short (a 1–3 char fragment).
	if _, err := s.parse("ab"); err == nil {
		t.Error("expected a too-short secret to be rejected")
	}
	// But a short REAL-looking key still passes — the floor is low on purpose so it
	// can't retroactively invalidate an existing stored key on the read path (§9).
	if _, err := s.parse("tmdbkey"); err != nil {
		t.Errorf("a plausible short key must still pass the guard: %v", err)
	}
	// Internal whitespace of any kind.
	for _, bad := range []string{"has a space", "has\ttab", "has\nnewline"} {
		if _, err := s.parse(bad); err == nil {
			t.Errorf("expected internal-whitespace secret %q to be rejected", bad)
		}
	}
	// A real-looking token is accepted AND trimmed of surrounding whitespace.
	got, err := s.parse("  a1b2c3d4e5f6g7h8  ")
	if err != nil {
		t.Fatalf("valid secret rejected: %v", err)
	}
	if got != "a1b2c3d4e5f6g7h8" {
		t.Errorf("secret = %q, want surrounding whitespace trimmed", got)
	}
	// Empty passes the guard (replace-only is enforced in Patch, not here).
	if _, err := s.parse(""); err != nil {
		t.Errorf("empty secret should pass parse (replace-only handled in Patch): %v", err)
	}
}

// Enum validation is fail-closed: an off-list value is rejected.
func TestRegistry_EnumFailClosed(t *testing.T) {
	s := Setting{Key: "library.flavor", Kind: KindEnum, Enum: []EnumOption{opt("emby", "Emby"), opt("jellyfin", "Jellyfin")}}
	if _, err := s.parse("plex"); err == nil {
		t.Error("expected error on off-enum value")
	}
	if _, err := s.parse("emby"); err != nil {
		t.Errorf("valid enum rejected: %v", err)
	}
}

// defaultRaw renders a declared Default back to its string form for the parse
// round-trip check (mirrors how a default is stored/emitted).
func defaultRaw(t *testing.T, s Setting) string {
	t.Helper()
	switch v := s.Default.(type) {
	case string:
		return v
	case int:
		return itoa(v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	case time.Duration:
		return v.String()
	default:
		return ""
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
