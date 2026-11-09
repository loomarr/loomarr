package protectedsettings

import (
	"context"

	"github.com/loomarr/loomarr/internal/people/secretprotection"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// PersisterStore is the settings persistence the settings service writes through.
type PersisterStore interface {
	ApplySettingBatch(ctx context.Context, batch store.SettingBatch) error
	SetSettingEnvOverride(ctx context.Context, key string, on bool, seed, by string) error
	ListSettings(ctx context.Context) ([]store.SettingRow, error)
}

// Persister adapts the store to settings.Persister (the PATCH write path), sealing every protected
// value before it is stored.
type Persister struct {
	st         PersisterStore
	protection *secretprotection.Manager
}

// NewPersister returns the persister over st. A nil protection stores values as given.
func NewPersister(st PersisterStore, protection *secretprotection.Manager) Persister {
	return Persister{st: st, protection: protection}
}

// Apply commits one batch of upserts and deletes, sealing the protected upserts.
func (p Persister) Apply(ctx context.Context, batch settings.PersistenceBatch) error {
	if p.protection != nil {
		if err := p.protection.Refresh(ctx); err != nil {
			return err
		}
	}
	storeBatch := store.SettingBatch{
		Upserts:   make([]store.SettingMutation, 0, len(batch.Upserts)),
		Deletes:   append([]string(nil), batch.Deletes...),
		UpdatedBy: batch.UpdatedBy,
		UpdatedAt: batch.UpdatedAt,
	}
	for _, row := range batch.Upserts {
		value := row.Value
		if p.protection != nil && IsProtected(settings.NewRegistry(), row.Key) {
			var err error
			value, err = p.protection.Seal(Record(row.Key), []byte(value))
			if err != nil {
				return err
			}
		}
		storeBatch.Upserts = append(storeBatch.Upserts, store.SettingMutation{
			Key: row.Key, Value: value,
		})
	}
	return p.st.ApplySettingBatch(ctx, storeBatch)
}

// SetEnvOverride satisfies settings.EnvOverrideSetter (§3.1). Kept on the same adapter as
// the value writes but routed to a DIFFERENT store method, because the two must not share
// an UPSERT: a save that also wrote the claim would re-lock a key on the operator's next
// edit (pinned by the store conformance suite).
func (p Persister) SetEnvOverride(ctx context.Context, key string, on bool, seed, by string) error {
	return p.st.SetSettingEnvOverride(ctx, key, on, seed, by)
}

// AuditLister adapts store.ListSettings to settings.AuditLister (the audit metadata List attaches
// per key).
type AuditLister struct{ st PersisterStore }

// NewAuditLister returns the audit lister over st.
func NewAuditLister(st PersisterStore) AuditLister { return AuditLister{st: st} }

// Audit returns who last changed each stored setting, and when.
func (a AuditLister) Audit(ctx context.Context) (map[string]settings.AuditRow, error) {
	rows, err := a.st.ListSettings(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]settings.AuditRow, len(rows))
	for _, r := range rows {
		out[r.Key] = settings.AuditRow{UpdatedBy: r.UpdatedBy, UpdatedAt: r.UpdatedAt}
	}
	return out, nil
}
