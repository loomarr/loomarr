package settings

import (
	"context"
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

func TestRegistry_TMDBHelpNamesEveryEnabledSurface(t *testing.T) {
	setting, ok := NewRegistry().Get("tmdb.api_key")
	if !ok {
		t.Fatal("tmdb.api_key is not declared")
	}
	help := strings.ToLower(setting.Doc)
	for _, surface := range []string{"search", "icon", "suggestion"} {
		if !strings.Contains(help, surface) {
			t.Errorf("tmdb.api_key help %q does not mention %s", setting.Doc, surface)
		}
	}
}

// Every declared default must RESOLVE to the Go type its Kind promises, so a typed
// accessor gets a real value on a fresh install with nothing stored and nothing in env.
//
// ⚠ This asserts the resolved type, NOT the declared one. A string default is a
// supported declaration style — defaultResolved() parses it, which is why 19 keys
// declare `Default: "24h"` and still resolve to a time.Duration. Asserting the
// declaration instead would flag all of them and prove nothing about what callers get.
//
// ⚠ And it is the assertion TestRegistry_BuildsCleanly structurally cannot make: that
// test routes every default through defaultRaw(), converting to string BEFORE parsing,
// so it can only catch a default malformed as text (`Default: "yes"` on a bool) — never
// one whose parsed type disagrees with its Kind. The failure it misses is silent: a
// typed accessor's assertion fails and it answers its fallback, so the setting reads as
// its zero value no matter what is declared. That is the rolling-window horizon bug
// recorded on defaultResolved, one layer up.
func TestRegistry_DefaultsResolveToTheirKind(t *testing.T) {
	svc, err := New(context.Background(), NewRegistry(), fakeLoader{m: map[string]string{}}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// A fresh install: nothing stored, nothing pinned, so every key lands on its default.
	svc.env = func(string) (string, bool) { return "", false }

	for _, s := range NewRegistry().All() {
		if s.Default == nil {
			continue
		}
		v := svc.Resolve(s.Key).Value
		if v == nil {
			continue // an optional with no default value to type-check
		}
		var ok bool
		switch s.Kind {
		case KindBool:
			_, ok = v.(bool)
		case KindInt:
			_, ok = v.(int)
		case KindDuration:
			_, ok = v.(time.Duration)
		case KindString, KindEnum, KindSecret, KindURL, KindCron:
			_, ok = v.(string)
		case KindStringList:
			_, ok = v.([]string)
		default:
			t.Errorf("%s: unknown kind %q", s.Key, s.Kind)
			continue
		}
		if !ok {
			t.Errorf("%s: Kind %q resolves to %#v (%T) — a typed read asserts and falls back to the zero value",
				s.Key, s.Kind, v, v)
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

// The hosted key is conditional, while the endpoint remains visible for both providers:
// non-default/remote Ollama hosts must be configurable too.
func TestRegistry_AIConditionalFields(t *testing.T) {
	reg := NewRegistry()
	url, ok := reg.Get("llm.url")
	if !ok {
		t.Fatal("llm.url not declared")
	}
	if len(url.ShowWhen) != 0 {
		t.Errorf("llm.url must be visible for Ollama and hosted providers, got %v", url.ShowWhen)
	}
	key, ok := reg.Get("llm.api_key")
	if !ok {
		t.Fatal("llm.api_key not declared")
	}
	allowed := key.ShowWhen["llm.provider"]
	if len(allowed) != 1 || allowed[0] != "openai" {
		t.Errorf("llm.api_key should ShowWhen llm.provider=openai, got %v", key.ShowWhen)
	}
}

func TestRegistry_FillerWorkflowPresentation(t *testing.T) {
	r := NewRegistry()
	for _, s := range r.All() {
		if s.Group != GroupFiller {
			continue
		}
		if s.Label == "" {
			t.Errorf("%s: filler workflow controls need a human label", s.Key)
		}
	}

	for _, key := range []string{"filler.dir", "filler.watch_dir", "ingest.ytdlp_path", "ingest.ffmpeg_path", "ingest.whisper_path", "ingest.whisper_model", "filler.language_model"} {
		s, ok := r.Get(key)
		if !ok {
			t.Fatalf("%s not declared", key)
		}
		if s.Presentation != PresentationPath {
			t.Errorf("%s presentation = %q, want path", key, s.Presentation)
		}
	}

	for child, controller := range map[string]string{
		"filler.autofile.min_confidence":     "filler.autofile.enabled",
		"filler.autofile.normalize_loudness": "filler.autofile.enabled",
		"filler.autosplit.min_confidence":    "filler.autosplit.enabled",
	} {
		s, ok := r.Get(child)
		if !ok {
			t.Fatalf("%s not declared", child)
		}
		allowed := s.ShowWhen[controller]
		if len(allowed) != 1 || allowed[0] != "true" {
			t.Errorf("%s should ShowWhen %s=true, got %v", child, controller, s.ShowWhen)
		}
	}
}

func TestRegistry_FillerBreakBounds(t *testing.T) {
	r := NewRegistry()
	for key, invalid := range map[string]string{
		"filler.breaks_per_hour": "-1",
		"filler.break_duration":  "29s",
		"filler.pod_max":         "0",
	} {
		s, ok := r.Get(key)
		if !ok {
			t.Fatalf("%s not declared", key)
		}
		if _, err := s.parse(invalid); err == nil {
			t.Errorf("%s accepted invalid value %s", key, invalid)
		}
	}

	duration, _ := r.Get("filler.break_duration")
	if got, err := duration.parse("30s"); err != nil || got != 30*time.Second {
		t.Errorf("30s minimum should be accepted: got %#v, err %v", got, err)
	}

	breaks, _ := r.Get("filler.breaks_per_hour")
	if got, err := breaks.parse("0"); err != nil || got != 0 {
		t.Errorf("zero breaks should disable the inherited default: got %#v, err %v", got, err)
	}
}

func TestRegistry_GuideSettingsAreDiscoverable(t *testing.T) {
	r := NewRegistry()
	for _, key := range []string{"guide.timezone", "guide.retention_hours"} {
		s, ok := r.Get(key)
		if !ok {
			t.Fatalf("%s not declared", key)
		}
		if s.Advanced {
			t.Errorf("%s should be visible without opening Advanced", key)
		}
		if s.Label == "" {
			t.Errorf("%s needs a human label", key)
		}
	}
}

func TestRegistry_PlaybackProgressiveDisclosure(t *testing.T) {
	r := NewRegistry()
	for _, key := range []string{"playout.backend", "playout.quality_tier", "playout.audio_language", "playout.max_channels"} {
		s, ok := r.Get(key)
		if !ok {
			t.Fatalf("%s not declared", key)
		}
		if s.Advanced {
			t.Errorf("%s should be visible without opening Advanced", key)
		}
	}

	for _, key := range []string{"playout.encoder", "playout.ffmpeg_path", "playout.hls_dir", "playout.prepared_dir", "playout.prepared_budget_gb"} {
		s, ok := r.Get(key)
		if !ok {
			t.Fatalf("%s not declared", key)
		}
		if !s.Advanced {
			t.Errorf("%s should stay behind Advanced", key)
		}
	}
}

func TestRegistry_PlayoutCapacityDefaultsToAutomaticAndRejectsNegative(t *testing.T) {
	s, ok := NewRegistry().Get("playout.max_channels")
	if !ok {
		t.Fatal("playout.max_channels not declared")
	}
	if s.Default != "0" {
		t.Errorf("default = %#v, want string 0 (automatic measurement)", s.Default)
	}
	if got, err := s.parse("0"); err != nil || got != 0 {
		t.Errorf("automatic value 0 = %#v, %v; want typed 0", got, err)
	}
	if _, err := s.parse("-1"); err == nil {
		t.Error("negative capacity was accepted")
	}
}

func TestRegistry_ConnectionAndSecurityOverridesStayAdvanced(t *testing.T) {
	r := NewRegistry()
	for _, key := range []string{
		"sonarr.quality_profile",
		"sonarr.root_folder",
		"radarr.quality_profile",
		"radarr.root_folder",
		"tunarr.transcode_config_id",
		"cookie.secure",
	} {
		s, ok := r.Get(key)
		if !ok {
			t.Fatalf("%s not declared", key)
		}
		if !s.Advanced {
			t.Errorf("%s should stay behind Advanced", key)
		}
	}

	session, ok := r.Get("session.ttl")
	if !ok {
		t.Fatal("session.ttl not declared")
	}
	if session.Advanced {
		t.Error("session.ttl should remain an ordinary sign-in preference")
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
		"request.ttl":  "48h",
		"session.ttl":  "720h",
		"job.workers":  "2",
		"llm.provider": "ollama",
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
