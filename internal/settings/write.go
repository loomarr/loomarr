package settings

import (
	"context"
	"time"
)

// Persister is the write half of the store the service needs for PATCH
// (accept-interfaces; no store import). The composition root adapts store.Store.
type Persister interface {
	Upsert(ctx context.Context, key, value, updatedBy string, at time.Time) error
	Delete(ctx context.Context, key string) error
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

// Patch applies per-key edits (config-design §8), persisting each valid change,
// then refreshes the snapshot so the change hot-applies. Order per key:
//   - unknown key      → invalid
//   - env-pinned key   → pinned (the env wins; a UI write can't override it, §3)
//   - empty value      → clear the override (revert to env/default, §9)
//   - parses           → persist, saved
//   - fails validation → invalid (problem message, secret-safe)
//
// The snapshot is reloaded from the persister once at the end (single read-through),
// so a batch of edits hot-applies atomically from the reader's view.
func (s *Service) Patch(ctx context.Context, p Persister, edits map[string]string, updatedBy string) ([]PatchResult, error) {
	now := time.Now()
	results := make([]PatchResult, 0, len(edits))
	changed := false

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
		// Empty clears an optional key (config-design §9): revert to env/default.
		if raw == "" {
			if err := p.Delete(ctx, key); err != nil {
				return nil, err
			}
			changed = true
			results = append(results, PatchResult{Key: key, Status: PatchSaved})
			continue
		}
		// Validate by parsing; a bad value is rejected (never persisted).
		if _, err := set.parse(raw); err != nil {
			results = append(results, PatchResult{Key: key, Status: PatchInvalid, Problem: patchProblem(set, err)})
			continue
		}
		if err := p.Upsert(ctx, key, raw, updatedBy, now); err != nil {
			return nil, err
		}
		changed = true
		results = append(results, PatchResult{Key: key, Status: PatchSaved})
	}

	if changed {
		if err := s.reload(ctx); err != nil {
			return results, err
		}
	}
	return results, nil
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
	db, err := s.loader.LoadAll(ctx)
	if err != nil {
		return err
	}
	s.SetDB(db)
	return nil
}
