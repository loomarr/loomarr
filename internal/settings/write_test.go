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

// Patch persists the CANONICAL parsed value, not the raw input: a URL keeps no
// trailing slash and a secret is trimmed IN THE STORE, so the stored form and the
// resolved form agree (regression — Patch used to persist raw, dropping normalization).
func TestPatch_PersistsCanonicalValue(t *testing.T) {
	s, p := patchService(t, nil)
	res, err := s.Patch(context.Background(), p, map[string]string{
		"library.url":   "http://emby:8096/",    // trailing slash → stripped on store
		"library.token": "  a1b2c3d4e5f6g7h8  ", // surrounding whitespace → trimmed on store
	}, "matt")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if r.Status != PatchSaved {
			t.Fatalf("%s: %+v want saved", r.Key, r)
		}
	}
	if p.m["library.url"] != "http://emby:8096" {
		t.Errorf("stored url = %q, want the trailing slash stripped in the STORE", p.m["library.url"])
	}
	if p.m["library.token"] != "a1b2c3d4e5f6g7h8" {
		t.Errorf("stored token = %q, want surrounding whitespace trimmed in the STORE", p.m["library.token"])
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

// An empty value must NEVER clear a secret (config-design §9): secrets are
// replace-only, and a GET returns no value for them (§4) — so a client that writes
// the settings list back would otherwise wipe every stored key.
func TestPatch_EmptyOnSecretIsRejected(t *testing.T) {
	s, p := patchService(t, nil)
	p.m["library.token"] = "super-secret"
	s.SetDB(map[string]string{"library.token": "super-secret"})

	res, _ := s.Patch(context.Background(), p, map[string]string{"library.token": ""}, "matt")
	if res[0].Status != PatchInvalid {
		t.Errorf("empty secret: %+v want invalid", res[0])
	}
	if got := p.m["library.token"]; got != "super-secret" {
		t.Errorf("stored secret = %q; an empty PATCH must not clear it", got)
	}
	if res[0].Problem == "" {
		t.Error("rejection must explain how to proceed (replace, or DELETE to clear)")
	}
}

// The regression this guards: GET returns no value for a secret, so writing the whole
// settings list back submits "" for it. That round-trip must leave secrets intact.
func TestPatch_ListRoundTripDoesNotWipeSecrets(t *testing.T) {
	s, p := patchService(t, nil)
	p.m["library.token"] = "super-secret"
	p.m["library.url"] = "http://emby:8096"
	s.SetDB(map[string]string{"library.token": "super-secret", "library.url": "http://emby:8096"})

	// What a naive client sends back after reading the list: secrets come back empty.
	roundTrip := map[string]string{"library.url": "http://emby:8096", "library.token": ""}
	if _, err := s.Patch(context.Background(), p, roundTrip, "matt"); err != nil {
		t.Fatalf("patch: %v", err)
	}
	if got := p.m["library.token"]; got != "super-secret" {
		t.Errorf("secret survived round-trip? got %q want it untouched", got)
	}
}

// Clear is the explicit unset — the only way to drop a secret (config-design §8).
func TestClear(t *testing.T) {
	s, p := patchService(t, nil)
	p.m["library.token"] = "super-secret"
	s.SetDB(map[string]string{"library.token": "super-secret"})

	res, err := s.Clear(context.Background(), p, "library.token")
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if res.Status != PatchSaved {
		t.Errorf("clear: %+v want saved", res)
	}
	if _, ok := p.m["library.token"]; ok {
		t.Error("clear should delete the stored secret")
	}
}

func TestClear_UnknownKeyAndEnvPin(t *testing.T) {
	s, p := patchService(t, map[string]string{"JOB_WORKERS": "4"})

	if res, _ := s.Clear(context.Background(), p, "nope.missing"); res.Status != PatchInvalid {
		t.Errorf("unknown key: %+v want invalid", res)
	}
	// The environment wins — clearing can't override a pin (§3).
	if res, _ := s.Clear(context.Background(), p, "job.workers"); res.Status != PatchPinned {
		t.Errorf("env-pinned clear: %+v want pinned", res)
	}
}

// A bad SECRET value's problem message never echoes the value (config-design §4).
func TestPatch_SecretProblemRedacted(t *testing.T) {
	s := Setting{Key: "library.token", Kind: KindSecret}
	// A secret's parse CAN now fail on shape (whitespace / too-short, §9), but this
	// asserts the redaction of an arbitrary problem, so force the redaction path
	// directly: patchProblem must never include the value.
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
