package settings

import "context"

// SettingRow mirrors store.SettingRow's shape without importing store (settings
// is imported BY store's callers, not the reverse; a store→settings import would
// cycle). The store adapter maps its rows to these.
type SettingRow struct {
	Key       string
	Value     string
	UpdatedBy string
	// EnvOverride: this key has been taken back from the environment (§3.1).
	EnvOverride bool
}

// StoreLoader adapts a store into a settings.Loader. It's constructed in the
// composition root with a mapping closure, so settings never imports store.
type StoreLoader struct {
	// list returns the current overrides as settings.SettingRow (the caller maps
	// store.SettingRow → settings.SettingRow, breaking the import cycle).
	List func(ctx context.Context) ([]SettingRow, error)
}

// LoadSnapshot implements Loader with one store read, keeping values and environment-
// override ownership from the same durable generation.
func (l StoreLoader) LoadSnapshot(ctx context.Context) (Snapshot, error) {
	rows, err := l.List(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	out := Snapshot{
		Values:       make(map[string]string, len(rows)),
		EnvOverrides: make(map[string]bool),
	}
	for _, r := range rows {
		out.Values[r.Key] = r.Value
		if r.EnvOverride {
			out.EnvOverrides[r.Key] = true
		}
	}
	return out, nil
}
