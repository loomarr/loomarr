// Package protectedsettings keeps the secret settings encrypted at rest: it decides which keys
// are protected, reads and writes one through the secret-protection manager, opens the whole
// settings table for boot, and reseals every protected value after a data-key rotation.
package protectedsettings

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/loomarr/loomarr/internal/people/secretprotection"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// Store is the settings persistence the helpers read and write.
type Store interface {
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error
	ListSettings(ctx context.Context) ([]store.SettingRow, error)
	RewriteSettingValues(ctx context.Context, rows []store.SettingMutation) error
}

// IsProtected reports whether the setting's stored value is encrypted: a registry secret, a
// generated `secret.` value, or a per-provider `llm.api_key.` key.
func IsProtected(reg *settings.Registry, key string) bool {
	if strings.HasPrefix(key, "secret.") || strings.HasPrefix(key, "llm.api_key.") {
		return true
	}
	setting, ok := reg.Get(key)
	return ok && setting.IsSecret()
}

// Record is the encryption context a setting's value is bound to.
func Record(key string) secretprotection.Record {
	return secretprotection.Record{Kind: "setting", ID: key, Field: "value"}
}

// Read returns a protected setting's plaintext. With no protection the stored value is returned
// as is.
func Read(ctx context.Context, st Store, protection *secretprotection.Manager, key string) (string, error) {
	if protection == nil {
		return st.GetSetting(ctx, key)
	}
	value, err := st.GetSetting(ctx, key)
	if err != nil {
		return "", err
	}
	if !secretprotection.IsEnvelope(value) {
		return "", fmt.Errorf("protected setting %q is not encrypted", key)
	}
	plain, err := protection.OpenLatest(ctx, Record(key), value)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// Write stores a protected setting's value encrypted. With no protection the value is stored as
// is.
func Write(ctx context.Context, st Store, protection *secretprotection.Manager, key, value string) error {
	if protection == nil {
		return st.SetSetting(ctx, key, value)
	}
	if err := protection.Refresh(ctx); err != nil {
		return err
	}
	envelope, err := protection.Seal(Record(key), []byte(value))
	if err != nil {
		return err
	}
	return st.SetSetting(ctx, key, envelope)
}

// OpenAll lists every setting with protected values decrypted. A protected value stored as
// plaintext predates encrypted settings (v0.2.0-beta.1); the error names every such key, never its
// value, and no row is rewritten.
func OpenAll(
	ctx context.Context,
	st Store,
	reg *settings.Registry,
	protection *secretprotection.Manager,
) ([]store.SettingRow, error) {
	if err := protection.Refresh(ctx); err != nil {
		return nil, err
	}
	rows, err := st.ListSettings(ctx)
	if err != nil {
		return nil, err
	}
	var plaintext []error
	for i := range rows {
		if !IsProtected(reg, rows[i].Key) || rows[i].Value == "" {
			continue
		}
		if !secretprotection.IsEnvelope(rows[i].Value) {
			plaintext = append(plaintext, fmt.Errorf(
				"setting %q is stored unencrypted: this database predates encrypted settings (v0.2.0-beta.1); "+
					"upgrade through a v0.2.x release first, or clear that setting and re-enter it", rows[i].Key))
			continue
		}
		plain, err := protection.Open(Record(rows[i].Key), rows[i].Value)
		if err != nil {
			return nil, fmt.Errorf("open protected setting %q: %w", rows[i].Key, err)
		}
		rows[i].Value = string(plain)
	}
	if len(plaintext) > 0 {
		return nil, errors.Join(plaintext...)
	}
	return rows, nil
}

// ReencryptAll reseals every protected setting under the current data key, in one commit.
func ReencryptAll(
	ctx context.Context, st Store, reg *settings.Registry, protection *secretprotection.Manager,
) error {
	if err := protection.Refresh(ctx); err != nil {
		return err
	}
	rows, err := st.ListSettings(ctx)
	if err != nil {
		return err
	}
	rewrites := make([]store.SettingMutation, 0)
	for _, row := range rows {
		if !IsProtected(reg, row.Key) || row.Value == "" {
			continue
		}
		plain, err := protection.Open(Record(row.Key), row.Value)
		if err != nil {
			return fmt.Errorf("open setting %q for data-key rotation: %w", row.Key, err)
		}
		envelope, err := protection.Seal(Record(row.Key), plain)
		if err != nil {
			return fmt.Errorf("reseal setting %q after data-key rotation: %w", row.Key, err)
		}
		rewrites = append(rewrites, store.SettingMutation{Key: row.Key, Value: envelope})
	}
	if err := st.RewriteSettingValues(ctx, rewrites); err != nil {
		return fmt.Errorf("commit data-key rotation for settings: %w", err)
	}
	return nil
}
