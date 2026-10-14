package settings

import (
	"context"
	"time"
)

// Entry is the resolved view of one setting for the API/UI (config-design §8):
// value + provenance + audit + (for secrets) the masked set/preview state. The
// secret value itself is NEVER placed in Value — callers get Set/Preview only.
type Entry struct {
	Setting    Setting
	Value      any // resolved typed value; ZERO for secrets (masked)
	Provenance Provenance
	Caution    bool
	Set        bool      // secrets: whether a value is stored/pinned
	Preview    string    // secrets: masked "…a1b2" tail
	UpdatedBy  string    // audit (empty for env/system writes)
	UpdatedAt  time.Time // audit (zero for env/system writes)
	// EnvOverride: taken back from the environment (§3.1).
	EnvOverride bool
	// EnvPinnable: the environment actually sets this key, so it is something the unlock
	// could act on. Computed from the live env rather than from provenance, because an
	// ALREADY-unlocked key resolves as `db` while its variable is still set — the UI needs
	// to keep offering "hand it back", and provenance alone can no longer tell it that.
	EnvPinnable bool
}

// AuditLister supplies the persisted audit rows (updated_by/at) so List can attach
// them. The composition root adapts store.ListSettings.
type AuditLister interface {
	Audit(ctx context.Context) (map[string]AuditRow, error)
}

// AuditRow is one key's audit metadata.
type AuditRow struct {
	UpdatedBy string
	UpdatedAt time.Time
}

// List returns every operator-managed setting resolved, with secrets masked and audit
// metadata attached (config-design §8). audit may be nil (no audit info).
func (s *Service) List(ctx context.Context, audit AuditLister) []Entry {
	var rows map[string]AuditRow
	if audit != nil {
		if r, err := audit.Audit(ctx); err == nil {
			rows = r
		}
	}
	out := make([]Entry, 0, len(s.reg.All()))
	for _, set := range s.reg.All() {
		if set.MigrationOnly {
			continue
		}
		r := s.Resolve(set.Key)
		e := Entry{
			Setting:     set,
			Provenance:  r.Provenance,
			Caution:     r.Caution,
			EnvOverride: r.EnvOverride,
		}
		// Asked of the environment directly, applying the same empty-is-unset rule as
		// resolution (§3): a blank `SEERR_URL=` is an unfilled template line, so offering
		// to "take it back" would be offering to claim nothing.
		if raw, ok, err := s.envValue(set); err == nil && ok && raw != "" {
			e.EnvPinnable = true
		}
		if set.IsSecret() {
			// Never expose the value. Set/Preview convey the masked state (§4).
			if str, _ := r.Value.(string); str != "" {
				e.Set = true
				e.Preview = preview(str)
			}
		} else {
			e.Value = r.Value
		}
		if row, ok := rows[set.Key]; ok {
			e.UpdatedBy = row.UpdatedBy
			e.UpdatedAt = row.UpdatedAt
		}
		out = append(out, e)
	}
	return out
}
