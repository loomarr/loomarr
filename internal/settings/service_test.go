package settings

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeLoader is an in-memory Loader for resolution tests (no store dependency).
type fakeLoader struct{ m map[string]string }

func (f fakeLoader) Load(_ context.Context, k string) (string, bool, error) {
	v, ok := f.m[k]
	return v, ok, nil
}
func (f fakeLoader) LoadAll(context.Context) (map[string]string, error) { return f.m, nil }

// newTestService builds a Service with an injected env and db, over a tiny
// registry so the assertions are about resolution, not the full key set.
func newTestService(t *testing.T, env map[string]string, db map[string]string) *Service {
	t.Helper()
	reg := newRegistry([]Setting{
		{Key: "library.url", EnvVar: "LIBRARY_URL", Kind: KindURL, Default: "", Doc: "x"},
		{Key: "job.workers", EnvVar: "JOB_WORKERS", Kind: KindInt, Default: 2, Doc: "x"},
		{Key: "library.token", EnvVar: "LIBRARY_TOKEN", Kind: KindSecret, Default: "", Doc: "x"},
	})
	s, err := New(context.Background(), reg, fakeLoader{m: db}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.env = func(k string) (string, bool) { v, ok := env[k]; return v, ok }
	return s
}

// env beats db beats default (config-design §3, the core matrix).
func TestResolve_Precedence(t *testing.T) {
	// default only
	s := newTestService(t, nil, nil)
	if r := s.Resolve("job.workers"); r.Value != 2 || r.Provenance != ProvenanceDefault {
		t.Errorf("default: got %v/%s, want 2/default", r.Value, r.Provenance)
	}
	// db overrides default
	s = newTestService(t, nil, map[string]string{"job.workers": "5"})
	if r := s.Resolve("job.workers"); r.Value != 5 || r.Provenance != ProvenanceDB {
		t.Errorf("db: got %v/%s, want 5/db", r.Value, r.Provenance)
	}
	// env overrides db
	s = newTestService(t, map[string]string{"JOB_WORKERS": "9"}, map[string]string{"job.workers": "5"})
	if r := s.Resolve("job.workers"); r.Value != 9 || r.Provenance != ProvenanceEnv {
		t.Errorf("env: got %v/%s, want 9/env", r.Value, r.Provenance)
	}
}

func TestResolveMany_UsesOneSnapshotWithOrdinaryPrecedence(t *testing.T) {
	s := newTestService(t,
		map[string]string{"LIBRARY_URL": "http://env-emby:8096"},
		map[string]string{"library.url": "http://db-emby:8096", "job.workers": "5"},
	)
	got := s.ResolveMany("library.url", "job.workers", "library.token")
	if value := got["library.url"]; value.Value != "http://env-emby:8096" || value.Provenance != ProvenanceEnv {
		t.Errorf("library.url = %v/%s, want env value", value.Value, value.Provenance)
	}
	if value := got["job.workers"]; value.Value != 5 || value.Provenance != ProvenanceDB {
		t.Errorf("job.workers = %v/%s, want db value", value.Value, value.Provenance)
	}
	if value := got["library.token"]; value.Value != "" || value.Provenance != ProvenanceDefault {
		t.Errorf("library.token = %v/%s, want empty default", value.Value, value.Provenance)
	}
}

// A default resolves to the setting's TYPED value, not the raw string, exactly like the
// env/db paths — because the real registry (declared.go) stores every default as a raw
// STRING ("24h", "2", "true"), and a typed accessor (set.dur/intv/boolv → Value.(T)) must
// still get a real T. Regression for the rolling-window bug: an unparsed "24h" default
// failed the time.Duration assertion in set.dur → window 0 → no truncation → the whole
// 800-episode run materialized. (The prior tests hid this by hand-typing their defaults
// e.g. Default: 2; the real registry never does.)
func TestResolve_StringDefaultParsesToTypedValue(t *testing.T) {
	reg := newRegistry([]Setting{
		{Key: "sched.window_hours", EnvVar: "SCHED_WINDOW_HOURS", Kind: KindDuration, Default: "24h", Doc: "x"},
		{Key: "job.workers", EnvVar: "JOB_WORKERS", Kind: KindInt, Default: "2", Doc: "x"},
		{Key: "flag.on", EnvVar: "FLAG_ON", Kind: KindBool, Default: "true", Doc: "x"},
	})
	s, err := New(context.Background(), reg, fakeLoader{m: nil}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if r := s.Resolve("sched.window_hours"); r.Value != 24*time.Hour {
		t.Errorf("duration default: got %v (%T), want 24h time.Duration", r.Value, r.Value)
	}
	if r := s.Resolve("job.workers"); r.Value != 2 {
		t.Errorf("int default: got %v (%T), want int 2", r.Value, r.Value)
	}
	if r := s.Resolve("flag.on"); r.Value != true {
		t.Errorf("bool default: got %v (%T), want bool true", r.Value, r.Value)
	}
}

// A bad DB value self-heals to the default with a caution (config-design §3) —
// it must NOT crash and must NOT win.
func TestResolve_BadDBSelfHeals(t *testing.T) {
	s := newTestService(t, nil, map[string]string{"job.workers": "not-a-number"})
	r := s.Resolve("job.workers")
	if r.Value != 2 || r.Provenance != ProvenanceDefault || !r.Caution {
		t.Errorf("self-heal: got %v/%s caution=%v, want 2/default caution=true", r.Value, r.Provenance, r.Caution)
	}
}

// An invalid ENV value fails the boot (config-design §3) — the operator's typo
// is surfaced loudly, not silently self-healed. Exercised through the real New
// path with a real env var (t.Setenv), since New reads os.LookupEnv at boot.
func TestNew_BadEnvFailsBoot(t *testing.T) {
	reg := newRegistry([]Setting{{Key: "job.workers", EnvVar: "JOB_WORKERS", Kind: KindInt, Default: 2, Doc: "x"}})
	t.Setenv("JOB_WORKERS", "twelve")
	if _, err := New(context.Background(), reg, fakeLoader{m: nil}, nil); err == nil {
		t.Fatal("expected boot to fail on an unparseable env value")
	}
	// A valid env value boots fine.
	t.Setenv("JOB_WORKERS", "8")
	s, err := New(context.Background(), reg, fakeLoader{m: nil}, nil)
	if err != nil {
		t.Fatalf("valid env should boot: %v", err)
	}
	if r := s.Resolve("job.workers"); r.Value != 8 || r.Provenance != ProvenanceEnv {
		t.Errorf("got %v/%s, want 8/env", r.Value, r.Provenance)
	}
}

// The Docker-secrets idiom: <VAR>_FILE loads the value; both <VAR> and <VAR>_FILE
// set is an ambiguous boot error (config-design §3).
func TestEnvValue_FileAndAmbiguity(t *testing.T) {
	dir := t.TempDir()
	secretFile := filepath.Join(dir, "token")
	if err := os.WriteFile(secretFile, []byte("s3cr3t\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	set := Setting{Key: "library.token", EnvVar: "LIBRARY_TOKEN", Kind: KindSecret}

	// _FILE only → loads, trailing newline stripped.
	s := newTestService(t, map[string]string{"LIBRARY_TOKEN_FILE": secretFile}, nil)
	raw, ok, err := s.envValue(set)
	if err != nil || !ok || raw != "s3cr3t" {
		t.Errorf("_FILE load: got %q,%v,%v want s3cr3t,true,nil", raw, ok, err)
	}

	// both set → ambiguous error.
	s = newTestService(t, map[string]string{"LIBRARY_TOKEN": "x", "LIBRARY_TOKEN_FILE": secretFile}, nil)
	if _, _, err := s.envValue(set); err == nil {
		t.Error("expected ambiguity error when <VAR> and <VAR>_FILE both set")
	}
}

// An env var that is SET BUT EMPTY counts as unset (config-design §3), so an
// unfilled `.env` template line cannot shadow a value saved through the UI.
//
// This is a regression test for a bug the maintainer smoke caught end to end: with
// `LLM_MODEL=` in the environment, the §8.1 picker persisted `llm.model` to the db
// and hot-swapped the live suggester — the UI said "In use" and suggestions worked —
// while every *read* still resolved to the empty env pin. The checklist therefore
// reported "no model selected" immediately after one was, and because the boot path
// resolves the same setting, the operator's choice vanished on the next restart.
//
// Every unit test missed it because they all inject env maps and none had ever set a
// key to "".
func TestResolve_EmptyEnvIsUnset(t *testing.T) {
	// Empty env + a db value → the db value wins, with db provenance (so the UI shows
	// the field as editable rather than env-locked).
	s := newTestService(t, map[string]string{"LIBRARY_URL": ""}, map[string]string{"library.url": "http://emby:8096"})
	if r := s.Resolve("library.url"); r.Value != "http://emby:8096" || r.Provenance != ProvenanceDB {
		t.Errorf("empty env must not shadow the db: got %v/%s, want http://emby:8096/db", r.Value, r.Provenance)
	}

	// Empty env + no db → the default, NOT an empty env-provenance value.
	s = newTestService(t, map[string]string{"JOB_WORKERS": ""}, nil)
	if r := s.Resolve("job.workers"); r.Value != 2 || r.Provenance != ProvenanceDefault {
		t.Errorf("empty env with no db: got %v/%s, want 2/default", r.Value, r.Provenance)
	}

	// A non-empty pin still wins — the rule narrows to blank values only.
	s = newTestService(t, map[string]string{"LIBRARY_URL": "http://pinned:8096"}, map[string]string{"library.url": "http://emby:8096"})
	if r := s.Resolve("library.url"); r.Value != "http://pinned:8096" || r.Provenance != ProvenanceEnv {
		t.Errorf("a real pin must still win: got %v/%s", r.Value, r.Provenance)
	}
}

// An empty <VAR> must not make <VAR>_FILE "ambiguous": a blank line left in a
// template alongside a real Docker secret is exactly the case the rule forgives, and
// failing the boot over it would be the same footgun in a louder costume.
func TestEnvValue_EmptyDirectDoesNotBlockFile(t *testing.T) {
	dir := t.TempDir()
	secretFile := filepath.Join(dir, "token")
	if err := os.WriteFile(secretFile, []byte("s3cr3t\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	set := Setting{Key: "library.token", EnvVar: "LIBRARY_TOKEN", Kind: KindSecret}

	s := newTestService(t, map[string]string{"LIBRARY_TOKEN": "", "LIBRARY_TOKEN_FILE": secretFile}, nil)
	raw, ok, err := s.envValue(set)
	if err != nil || !ok || raw != "s3cr3t" {
		t.Errorf("empty <VAR> + <VAR>_FILE: got %q,%v,%v want s3cr3t,true,nil", raw, ok, err)
	}
}

// Boot must SAY it ignored an empty pin (config-design §3). An operator who meant to
// blank a setting should not have to infer that we disregarded them.
func TestNew_WarnsOnEmptyEnvPin(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	reg := newRegistry([]Setting{{Key: "llm.model", EnvVar: "LLM_MODEL", Kind: KindString, Default: "", Doc: "x"}})

	t.Setenv("LLM_MODEL", "")
	if _, err := New(context.Background(), reg, fakeLoader{m: nil}, log); err != nil {
		t.Fatalf("an empty pin must not fail the boot: %v", err)
	}
	if got := buf.String(); !strings.Contains(got, "LLM_MODEL") {
		t.Errorf("boot did not warn about the ignored empty pin; log was: %q", got)
	}

	// A pin that is actually set says nothing — the warning must stay rare enough to
	// be worth reading.
	buf.Reset()
	t.Setenv("LLM_MODEL", "qwen3:8b")
	if _, err := New(context.Background(), reg, fakeLoader{m: nil}, log); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Errorf("a real pin should log nothing, got: %q", buf.String())
	}
}

// Hot-apply: SetDB after a write notifies Watch subscribers of changed keys
// (config-design §3), and the snapshot reflects the new value immediately.
func TestWatch_HotApply(t *testing.T) {
	s := newTestService(t, nil, map[string]string{"job.workers": "2"})
	ch := s.Watch("job.workers")
	s.SetDB(map[string]string{"job.workers": "7"})

	if r := s.Resolve("job.workers"); r.Value != 7 {
		t.Errorf("snapshot not updated: got %v want 7", r.Value)
	}
	select {
	case c := <-ch:
		if c.Key != "job.workers" {
			t.Errorf("watch fired for %q, want job.workers", c.Key)
		}
	default:
		t.Error("watch did not fire on change")
	}
}

// Unchanged keys don't fire watchers (no spurious rebuilds).
func TestWatch_NoFireOnUnchanged(t *testing.T) {
	s := newTestService(t, nil, map[string]string{"job.workers": "2"})
	ch := s.Watch("job.workers")
	s.SetDB(map[string]string{"job.workers": "2"}) // same value
	select {
	case <-ch:
		t.Error("watch fired though value did not change")
	default:
	}
}
