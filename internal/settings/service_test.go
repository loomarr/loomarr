package settings

import (
	"context"
	"os"
	"path/filepath"
	"testing"
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
