package settings

import (
	"context"
	"testing"
	"time"
)

// memPersister is an in-memory Persister backed by a map the tests can inspect.
type memPersister struct{ m map[string]string }

func newMemPersister() *memPersister { return &memPersister{m: map[string]string{}} }

func (p *memPersister) Upsert(_ context.Context, k, v, _ string, _ time.Time) error {
	p.m[k] = v
	return nil
}
func (p *memPersister) Delete(_ context.Context, k string) error {
	delete(p.m, k)
	return nil
}

// patchService builds a Service over the real registry, sharing a persister and a
// loader backed by the SAME map so a save round-trips into the snapshot.
func patchService(t *testing.T, env map[string]string) (*Service, *memPersister) {
	t.Helper()
	p := newMemPersister()
	s, err := New(context.Background(), NewRegistry(), fakeLoader{m: p.m}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.env = func(k string) (string, bool) { v, ok := env[k]; return v, ok }
	return s, p
}

// A valid edit persists and hot-applies; an unknown key and a bad value are
// rejected without persisting (config-design §8).
func TestPatch_SaveInvalidUnknown(t *testing.T) {
	s, p := patchService(t, nil)
	res, err := s.Patch(context.Background(), p, map[string]string{
		"library.url":    "http://emby:8096",
		"job.workers":    "not-a-number",
		"does.not.exist": "x",
	}, "matt")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]PatchResult{}
	for _, r := range res {
		got[r.Key] = r
	}
	if got["library.url"].Status != PatchSaved {
		t.Errorf("library.url: %+v want saved", got["library.url"])
	}
	if got["job.workers"].Status != PatchInvalid {
		t.Errorf("job.workers: %+v want invalid", got["job.workers"])
	}
	if got["does.not.exist"].Status != PatchInvalid {
		t.Errorf("unknown key: %+v want invalid", got["does.not.exist"])
	}
	// Only the valid one persisted, and it hot-applied (URL normalized).
	if p.m["job.workers"] != "" {
		t.Error("bad value must not persist")
	}
	if r := s.Resolve("library.url"); r.Value != "http://emby:8096" || r.Provenance != ProvenanceDB {
		t.Errorf("hot-apply: %+v want normalized/db", r)
	}
}

// An env-pinned key returns pinned and is NOT written (config-design §3, §8).
func TestPatch_EnvPinnedRejected(t *testing.T) {
	s, p := patchService(t, map[string]string{"LIBRARY_URL": "http://pinned:8096"})
	res, _ := s.Patch(context.Background(), p, map[string]string{"library.url": "http://other:8096"}, "matt")
	if res[0].Status != PatchPinned {
		t.Errorf("env-pinned edit: %+v want pinned", res[0])
	}
	if _, ok := p.m["library.url"]; ok {
		t.Error("must not persist over an env pin")
	}
}

// An empty value clears an override (config-design §9).
func TestPatch_EmptyClears(t *testing.T) {
	s, p := patchService(t, nil)
	p.m["library.url"] = "http://emby:8096"
	s.SetDB(map[string]string{"library.url": "http://emby:8096"})
	res, _ := s.Patch(context.Background(), p, map[string]string{"library.url": ""}, "matt")
	if res[0].Status != PatchSaved {
		t.Errorf("clear: %+v want saved", res[0])
	}
	if _, ok := p.m["library.url"]; ok {
		t.Error("empty value should delete the override")
	}
	if r := s.Resolve("library.url"); r.Provenance != ProvenanceDefault {
		t.Errorf("after clear, provenance = %s want default", r.Provenance)
	}
}

// A bad SECRET value's problem message never echoes the value (config-design §4).
func TestPatch_SecretProblemRedacted(t *testing.T) {
	s := Setting{Key: "library.token", Kind: KindSecret}
	// A secret's parse never fails on shape (any string is valid), so force the
	// redaction path directly: patchProblem must never include the value.
	msg := patchProblem(s, errWith("the-secret-value-leaked"))
	if msg == "the-secret-value-leaked" || containsStr(msg, "leaked") {
		t.Errorf("secret problem leaked the value: %q", msg)
	}
}

// List masks secrets: the value is withheld, Set/Preview convey state (§4, §8).
func TestList_MasksSecrets(t *testing.T) {
	s, _ := patchService(t, nil)
	s.SetDB(map[string]string{"library.token": "super-secret-token-1234"})
	entries := s.List(context.Background(), nil)
	for _, e := range entries {
		if e.Setting.Key == "library.token" {
			if e.Value != nil {
				t.Errorf("secret value must not be in List: %v", e.Value)
			}
			if !e.Set || e.Preview != "…1234" {
				t.Errorf("secret masking: set=%v preview=%q want true/…1234", e.Set, e.Preview)
			}
		}
	}
}

// --- tiny helpers (no extra imports) ---

type strError string

func (e strError) Error() string { return string(e) }
func errWith(s string) error     { return strError(s) }
func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
