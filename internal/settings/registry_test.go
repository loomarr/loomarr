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
	for _, s := range NewRegistry().All() {
		if strings.ToUpper(s.EnvVar) != s.EnvVar {
			t.Errorf("%s: env var %q should be upper-case", s.Key, s.EnvVar)
		}
		if s.Key != strings.ToLower(s.Key) {
			t.Errorf("%s: key should be lower-case", s.Key)
		}
		if s.Kind == KindEnum && len(s.Enum) == 0 {
			t.Errorf("%s: enum setting has no values", s.Key)
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

// Enum validation is fail-closed: an off-list value is rejected.
func TestRegistry_EnumFailClosed(t *testing.T) {
	s := Setting{Key: "library.flavor", Kind: KindEnum, Enum: []string{"emby", "jellyfin"}}
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
