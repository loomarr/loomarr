package settings

import (
	"context"
	"time"
)

// PersistedSetting is one validated, canonical setting value ready for durable storage.
type PersistedSetting struct {
	Key   string
	Value string
}

// PersistenceBatch is one PATCH's valid durable mutations. Invalid and environment-pinned
// edits never enter the batch; all included upserts and deletes commit together.
type PersistenceBatch struct {
	Upserts   []PersistedSetting
	Deletes   []string
	UpdatedBy string
	UpdatedAt time.Time
}

// Persister is the write half of the store the service needs for PATCH
// (accept-interfaces; no store import). The composition root adapts store.Store.
type Persister interface {
	// Apply durably commits every mutation in batch or none of them.
	Apply(ctx context.Context, batch PersistenceBatch) error
}

// EnvOverrideSetter is the write half for §3.1's unlock. Separate from Persister because
// it writes AUTHORITY over a key rather than a value, and the two must not be conflated:
// an ordinary save must never disturb the claim.
type EnvOverrideSetter interface {
	SetEnvOverride(ctx context.Context, key string, on bool, seed, by string) error
}

// EnvOverrideStatus is the outcome of claiming a key or handing it back (§3.1).
type EnvOverrideStatus string

const (
	EnvOverrideApplied EnvOverrideStatus = "applied"
	EnvOverrideUnknown EnvOverrideStatus = "unknown"
	// EnvOverrideNotPinned: asked to unlock a key the environment does not pin. Refused
	// rather than silently accepted — it would store a claim over nothing, and the field
	// would then advertise "overriding X" about a variable that was never set.
	EnvOverrideNotPinned EnvOverrideStatus = "not_pinned"
)

// SetEnvOverride takes a key back from the environment, or hands it back (config-design §3.1).
//
// Unlocking SEEDS the stored value from the env value it is taking over, so the act transfers
// authority without changing what the app is doing: an operator correcting one character of a
// URL must not knock the service offline the moment they click unlock. The value changes on
// their next save, deliberately.
//
// ⚠ A SECRET NEVER SEEDS. Copying a credential out of the environment into the database would
// put it in every §16 backup — a security change wearing a convenience's clothes. An unlocked
// secret is simply unset, and the operator enters one through the §4 replace-only flow.
//
// Bootstrap keys (DATABASE_URL, LISTEN_ADDR, …) are not in the registry at all, so they fall
// out here as `unknown`: they are read before the database opens, and a flag stored in that
// database could not affect them. Refusing beats accepting a write that does nothing.
func (s *Service) SetEnvOverride(
	ctx context.Context, w EnvOverrideSetter, key string, on bool, updatedBy string,
) (EnvOverrideStatus, error) {
	set, ok := s.reg.Get(key)
	if !ok {
		return EnvOverrideUnknown, nil
	}

	var seed string
	if on {
		raw, pinned, err := s.envValue(set)
		if err != nil || !pinned || raw == "" {
			// Not actually pinned (or an unreadable *_FILE): nothing to take over. The
			// empty-is-unset rule (§3) applies here exactly as it does in resolution, so
			// `SEERR_URL=` is not something an operator can claim.
			return EnvOverrideNotPinned, nil
		}
		if !set.IsSecret() {
			seed = raw
		}
	}

	if err := w.SetEnvOverride(ctx, key, on, seed, updatedBy); err != nil {
		return "", err
	}
	// Reload so the claim hot-applies; without this the next read still resolves to env
	// and the unlock looks like it did nothing until a restart.
	if err := s.reload(ctx); err != nil {
		return "", err
	}
	return EnvOverrideApplied, nil
}

// PatchStatus is one key's PATCH outcome (config-design §8).
type PatchStatus string

const (
	PatchSaved   PatchStatus = "saved"
	PatchInvalid PatchStatus = "invalid"
	PatchPinned  PatchStatus = "pinned"
)

// PatchResult reports what happened to one key in a PATCH.
type PatchResult struct {
	Key     string
	Status  PatchStatus
	Problem string // set when Status == PatchInvalid; never echoes a secret (§4)
}

// Patch applies per-key edits (config-design §8), persisting all valid changes together,
// then refreshes the snapshot so the change hot-applies. Order per key:
//   - unknown key      → invalid
//   - env-pinned key   → pinned (the env wins; a UI write can't override it, §3)
//   - empty on secret  → invalid (replace-only; clear via DELETE — §9)
//   - empty value      → clear the override (revert to env/default, §9)
//   - parses           → persist, saved
//   - fails validation → invalid (problem message, secret-safe)
//
// The snapshot is reloaded from the persister once at the end (single read-through),
// so a batch of edits hot-applies atomically from the reader's view.
func (s *Service) Patch(ctx context.Context, p Persister, edits map[string]string, updatedBy string) ([]PatchResult, error) {
	now := time.Now()
	results := make([]PatchResult, 0, len(edits))
	batch := PersistenceBatch{
		Upserts:   make([]PersistedSetting, 0, len(edits)),
		Deletes:   make([]string, 0, len(edits)),
		UpdatedBy: updatedBy,
		UpdatedAt: now,
	}

	for key, raw := range edits {
		set, ok := s.reg.Get(key)
		if !ok {
			results = append(results, PatchResult{Key: key, Status: PatchInvalid, Problem: "unknown setting"})
			continue
		}
		// An env pin cannot be overridden from the UI (config-design §3).
		if s.Provenance(key) == ProvenanceEnv {
			results = append(results, PatchResult{Key: key, Status: PatchPinned, Problem: "set via environment"})
			continue
		}
		// Empty clears an optional key (config-design §9) — but NEVER a secret. A GET
		// deliberately returns no value for a secret (§4), so a client that reads the
		// settings list and writes it back would submit "" for every secret and silently
		// destroy the stored Emby token, Seerr/TMDB/LLM keys. Secrets are replace-only;
		// clearing one is the explicit DELETE /v1/settings/{key} (§8). Rejecting here
		// makes that round-trip loud and harmless instead of quietly destructive.
		if raw == "" {
			if set.IsSecret() {
				results = append(results, PatchResult{
					Key:     key,
					Status:  PatchInvalid,
					Problem: "a stored secret is replace-only; send a new value, or DELETE the key to clear it",
				})
				continue
			}
			batch.Deletes = append(batch.Deletes, key)
			results = append(results, PatchResult{Key: key, Status: PatchSaved})
			continue
		}
		// Validate by parsing; a bad value is rejected (never persisted). Persist the
		// CANONICAL form the parse produced, not the raw input — parsing normalizes
		// (a URL's trailing slash stripped, a secret's surrounding whitespace trimmed,
		// a duration canonicalized), and storing raw would drop that normalization so
		// the stored value and the resolved value disagree.
		parsed, err := set.parse(raw)
		if err != nil {
			results = append(results, PatchResult{Key: key, Status: PatchInvalid, Problem: patchProblem(set, err)})
			continue
		}
		batch.Upserts = append(batch.Upserts, PersistedSetting{Key: key, Value: ValueString(parsed)})
		results = append(results, PatchResult{Key: key, Status: PatchSaved})
	}

	if len(batch.Upserts) > 0 || len(batch.Deletes) > 0 {
		if err := p.Apply(ctx, batch); err != nil {
			return nil, err
		}
		if err := s.reload(ctx); err != nil {
			return results, err
		}
	}
	return results, nil
}

// Clear drops a key's stored override so it reverts to env/default — the EXPLICIT
// clear path (config-design §8). It is the only way to unset a secret, since an
// empty-string PATCH on one is rejected as replace-only (§9). Hot-applies like any
// write. The result reuses PATCH's vocabulary so the API can map it to a status:
// invalid → unknown key (404), pinned → the env wins (409), saved → cleared (204).
func (s *Service) Clear(ctx context.Context, p Persister, key string) (PatchResult, error) {
	if _, ok := s.reg.Get(key); !ok {
		return PatchResult{Key: key, Status: PatchInvalid, Problem: "unknown setting"}, nil
	}
	if s.Provenance(key) == ProvenanceEnv {
		return PatchResult{Key: key, Status: PatchPinned, Problem: "set via environment"}, nil
	}
	if err := p.Apply(ctx, PersistenceBatch{
		Deletes: []string{key}, UpdatedAt: time.Now(),
	}); err != nil {
		return PatchResult{}, err
	}
	res := PatchResult{Key: key, Status: PatchSaved}
	if err := s.reload(ctx); err != nil {
		return res, err
	}
	return res, nil
}

// patchProblem renders a validation error for the API, suppressing the message
// entirely for a secret so a bad secret value's shape can't leak (§4 redaction).
func patchProblem(set Setting, err error) string {
	if set.IsSecret() {
		return "invalid value"
	}
	return err.Error()
}

// reload re-reads the override snapshot from the loader (used after a write so the
// change hot-applies). The service keeps the loader for this refresh.
func (s *Service) reload(ctx context.Context) error {
	if s.loader == nil {
		return nil
	}
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	snapshot, err := s.loader.LoadSnapshot(ctx)
	if err != nil {
		return err
	}
	s.setSnapshot(snapshot)
	return nil
}

// Refresh forces the in-memory snapshot to re-read the durable store through the
// same serialized path used after local writes and by the Postgres replica refresher.
// The periodic refresher provides bounded convergence for other replicas' writes;
// explicit callers such as a connection test use Refresh when they need an immediate
// view. SQLite has no periodic reader because it runs as a single replica. Refresh is
// a no-op when no loader is wired (unit tests without a store).
func (s *Service) Refresh(ctx context.Context) error { return s.reload(ctx) }
