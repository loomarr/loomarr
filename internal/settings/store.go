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

// RowLister is the store capability the loader needs: list every persisted
// override with audit metadata. store.Store satisfies this structurally via a
// tiny adapter (the store returns store.SettingRow; StoreLoader maps it).
type RowLister interface {
	ListSettings(ctx context.Context) ([]SettingRow, error)
}

// StoreLoader adapts a store into a settings.Loader. It's constructed in the
// composition root with a mapping closure, so settings never imports store.
type StoreLoader struct {
	// list returns the current overrides as settings.SettingRow (the caller maps
	// store.SettingRow → settings.SettingRow, breaking the import cycle).
	List func(ctx context.Context) ([]SettingRow, error)
}

// LoadAll implements Loader: the whole override map, key→value.
func (l StoreLoader) LoadAll(ctx context.Context) (map[string]string, error) {
	rows, err := l.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.Key] = r.Value
	}
	return out, nil
}

// LoadEnvOverrides implements EnvOverrideLoader: the set of keys an admin has taken back
// from the environment (§3.1). Only true entries are returned — absence is the default and
// the overwhelmingly common case, so a map of every key mapped to false would be noise.
func (l StoreLoader) LoadEnvOverrides(ctx context.Context) (map[string]bool, error) {
	rows, err := l.List(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, r := range rows {
		if r.EnvOverride {
			out[r.Key] = true
		}
	}
	return out, nil
}

// Load implements Loader: one key's stored value.
func (l StoreLoader) Load(ctx context.Context, key string) (string, bool, error) {
	all, err := l.LoadAll(ctx)
	if err != nil {
		return "", false, err
	}
	v, ok := all[key]
	return v, ok, nil
}
