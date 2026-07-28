package settings

import (
	"context"
	"testing"
	"time"
)

// The unlock (config-design §3.1): a key an admin has taken back from the environment.
//
// The motivating case is the wizard — an operator puts a value in `.env` to get the app
// booting, reaches the wizard, and finds the field they need to correct is read-only with no
// route forward but editing a file on the host and restarting.

// unlockLoader is a fakeLoader that also carries the env-override set, so these tests exercise
// the real optional-capability path rather than reaching into Service internals.
type unlockLoader struct {
	m        map[string]string
	unlocked map[string]bool
}

func (f *unlockLoader) Load(_ context.Context, k string) (string, bool, error) {
	v, ok := f.m[k]
	return v, ok, nil
}
func (f *unlockLoader) LoadAll(context.Context) (map[string]string, error) { return f.m, nil }
func (f *unlockLoader) LoadEnvOverrides(context.Context) (map[string]bool, error) {
	return f.unlocked, nil
}

// SetEnvOverride doubles as the EnvOverrideSetter, so a claim written by the service is
// visible to the next reload — the round trip the hot-apply depends on.
func (f *unlockLoader) SetEnvOverride(_ context.Context, key string, on bool, seed, _ string) error {
	if on {
		f.unlocked[key] = true
		if _, exists := f.m[key]; !exists && seed != "" {
			f.m[key] = seed
		}
		return nil
	}
	delete(f.unlocked, key)
	return nil
}

func newUnlockService(t *testing.T, env map[string]string, l *unlockLoader) *Service {
	t.Helper()
	reg := newRegistry([]Setting{
		{Key: "library.url", EnvVar: "LIBRARY_URL", Kind: KindURL, Default: "", Doc: "x"},
		{Key: "job.workers", EnvVar: "JOB_WORKERS", Kind: KindInt, Default: 2, Doc: "x"},
		{Key: "library.token", EnvVar: "LIBRARY_TOKEN", Kind: KindSecret, Default: "", Doc: "x"},
	})
	s, err := New(context.Background(), reg, l, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.env = func(k string) (string, bool) { v, ok := env[k]; return v, ok }
	// New resolved the env before the injected env map existed; reload so the snapshot and
	// the env agree.
	if err := s.reload(context.Background()); err != nil {
		t.Fatalf("reload: %v", err)
	}
	return s
}

// The core inversion: with the claim set, the stored value beats the env var.
func TestEnvOverride_StoredValueBeatsEnvOnceUnlocked(t *testing.T) {
	l := &unlockLoader{
		m:        map[string]string{"library.url": "http://from-db:8096"},
		unlocked: map[string]bool{},
	}
	env := map[string]string{"LIBRARY_URL": "http://from-env:8096"}
	s := newUnlockService(t, env, l)

	// Locked: env wins, exactly as before §3.1.
	if r := s.Resolve("library.url"); r.Provenance != ProvenanceEnv || r.Value != "http://from-env:8096" {
		t.Fatalf("locked: got %v/%s, want the env value", r.Value, r.Provenance)
	}

	if st, err := s.SetEnvOverride(context.Background(), l, "library.url", true, "matt"); err != nil || st != EnvOverrideApplied {
		t.Fatalf("unlock: %s %v", st, err)
	}

	r := s.Resolve("library.url")
	if r.Value != "http://from-db:8096" {
		t.Errorf("unlocked: got %v, want the stored value to win", r.Value)
	}
	// Honestly `db` — not a fourth provenance — with the lock state reported separately, so
	// the UI can say "overriding LIBRARY_URL" rather than implying the variable is unset.
	if r.Provenance != ProvenanceDB {
		t.Errorf("provenance: got %s, want db", r.Provenance)
	}
	if !r.EnvOverride {
		t.Error("EnvOverride false — the field cannot distinguish this from an unset variable")
	}
}

// Unlocking must not change what the app is DOING. Seeding from the env value is what makes
// the act a transfer of authority rather than an edit: without it, unlocking a URL to correct
// one character would blank it and take the service down at the moment of the click.
func TestEnvOverride_UnlockSeedsFromEnvSoNothingChanges(t *testing.T) {
	l := &unlockLoader{m: map[string]string{}, unlocked: map[string]bool{}}
	env := map[string]string{"LIBRARY_URL": "http://seed-me:8096"}
	s := newUnlockService(t, env, l)

	before := s.Resolve("library.url").Value
	if _, err := s.SetEnvOverride(context.Background(), l, "library.url", true, "matt"); err != nil {
		t.Fatal(err)
	}
	if got := s.Resolve("library.url").Value; got != before {
		t.Errorf("unlocking changed the live value from %v to %v", before, got)
	}
	if l.m["library.url"] != "http://seed-me:8096" {
		t.Errorf("stored seed = %q, want the env value it took over", l.m["library.url"])
	}
}

// ⚠ A secret NEVER seeds: that would copy a credential out of the environment into the
// database and therefore into every §16 backup — a security change disguised as convenience.
func TestEnvOverride_SecretNeverSeedsFromEnv(t *testing.T) {
	l := &unlockLoader{m: map[string]string{}, unlocked: map[string]bool{}}
	env := map[string]string{"LIBRARY_TOKEN": "super-secret-token"}
	s := newUnlockService(t, env, l)

	if _, err := s.SetEnvOverride(context.Background(), l, "library.token", true, "matt"); err != nil {
		t.Fatal(err)
	}
	if v, ok := l.m["library.token"]; ok {
		t.Errorf("the secret was copied into the store as %q — it must stay unset", v)
	}
	// Still reported as overriding, so the field explains itself even with nothing stored.
	if r := s.Resolve("library.token"); !r.EnvOverride {
		t.Error("an unlocked secret must still report EnvOverride")
	}
}

// Handing the key back restores env precedence and KEEPS the stored value, so the round trip
// is reversible in both directions.
func TestEnvOverride_RelockRestoresEnvAndKeepsTheStoredValue(t *testing.T) {
	l := &unlockLoader{
		m:        map[string]string{"library.url": "http://from-db:8096"},
		unlocked: map[string]bool{"library.url": true},
	}
	env := map[string]string{"LIBRARY_URL": "http://from-env:8096"}
	s := newUnlockService(t, env, l)

	if _, err := s.SetEnvOverride(context.Background(), l, "library.url", false, "matt"); err != nil {
		t.Fatal(err)
	}
	r := s.Resolve("library.url")
	if r.Provenance != ProvenanceEnv || r.Value != "http://from-env:8096" {
		t.Errorf("relocked: got %v/%s, want the env value to win again", r.Value, r.Provenance)
	}
	if r.EnvOverride {
		t.Error("EnvOverride still set after handing the key back")
	}
	if l.m["library.url"] != "http://from-db:8096" {
		t.Error("re-locking discarded the stored value — the round trip must lose nothing")
	}
}

// Refused rather than silently accepted: storing a claim over a variable nobody set would
// make the field advertise "overriding LIBRARY_URL" about an env var that does not exist.
func TestEnvOverride_RefusesAKeyTheEnvDoesNotPin(t *testing.T) {
	l := &unlockLoader{m: map[string]string{}, unlocked: map[string]bool{}}
	s := newUnlockService(t, nil, l)

	st, err := s.SetEnvOverride(context.Background(), l, "library.url", true, "matt")
	if err != nil {
		t.Fatal(err)
	}
	if st != EnvOverrideNotPinned {
		t.Errorf("got %s, want not_pinned", st)
	}
	if len(l.unlocked) != 0 {
		t.Error("a claim was stored for a key the environment does not pin")
	}
}

// The empty-is-unset rule (§3) applies here exactly as it does in resolution: `LIBRARY_URL=`
// is an unfilled template line, not a pin, so there is nothing to take back.
func TestEnvOverride_EmptyPinIsNotSomethingToUnlock(t *testing.T) {
	l := &unlockLoader{m: map[string]string{}, unlocked: map[string]bool{}}
	s := newUnlockService(t, map[string]string{"LIBRARY_URL": ""}, l)

	st, err := s.SetEnvOverride(context.Background(), l, "library.url", true, "matt")
	if err != nil {
		t.Fatal(err)
	}
	if st != EnvOverrideNotPinned {
		t.Errorf("got %s, want not_pinned for an empty pin", st)
	}
}

// Bootstrap keys (DATABASE_URL, LISTEN_ADDR, …) are read before the database opens, so a flag
// stored IN that database cannot affect them. They are not in the registry, so they land here
// as unknown — refused rather than accepted as a write that would do nothing.
func TestEnvOverride_BootstrapKeysAreNotUnlockable(t *testing.T) {
	l := &unlockLoader{m: map[string]string{}, unlocked: map[string]bool{}}
	s := newUnlockService(t, map[string]string{"DATABASE_URL": "sqlite://x.db"}, l)

	st, err := s.SetEnvOverride(context.Background(), l, "database.url", true, "matt")
	if err != nil {
		t.Fatal(err)
	}
	if st != EnvOverrideUnknown {
		t.Errorf("got %s, want unknown", st)
	}
}

// Once unlocked, an ordinary PATCH must go through — the whole point is that the field becomes
// editable. This is the guard that made the wizard read-only, so it is asserted end to end
// rather than inferred from the provenance change.
func TestEnvOverride_UnlockedKeyBecomesPatchable(t *testing.T) {
	l := &unlockLoader{m: map[string]string{}, unlocked: map[string]bool{}}
	env := map[string]string{"JOB_WORKERS": "2"}
	s := newUnlockService(t, env, l)

	// Locked: PATCH is refused as pinned.
	res, err := s.Patch(context.Background(), patchPersister{l}, map[string]string{"job.workers": "9"}, "matt")
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Status != PatchPinned {
		t.Fatalf("locked: got %s, want pinned", res[0].Status)
	}

	if _, err := s.SetEnvOverride(context.Background(), l, "job.workers", true, "matt"); err != nil {
		t.Fatal(err)
	}
	res, err = s.Patch(context.Background(), patchPersister{l}, map[string]string{"job.workers": "9"}, "matt")
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Status != PatchSaved {
		t.Fatalf("unlocked: got %s (%s), want saved", res[0].Status, res[0].Problem)
	}
	if got := s.Resolve("job.workers"); got.Value != 9 {
		t.Errorf("resolved %v after save, want 9", got.Value)
	}
}

// patchPersister adapts the loader into the Persister the PATCH path needs.
type patchPersister struct{ l *unlockLoader }

func (p patchPersister) Upsert(_ context.Context, key, value, _ string, _ time.Time) error {
	p.l.m[key] = value
	return nil
}
func (p patchPersister) Delete(_ context.Context, key string) error {
	delete(p.l.m, key)
	return nil
}
