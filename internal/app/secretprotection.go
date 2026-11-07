package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/loomarr/loomarr/internal/config"
	"github.com/loomarr/loomarr/internal/secretprotection"
	"github.com/loomarr/loomarr/internal/settings"
	"github.com/loomarr/loomarr/internal/store"
)

func buildSecretProtection(ctx context.Context, st store.Store, dataDir string) (*secretprotection.Manager, error) {
	if dataDir == "" {
		if path := store.SQLitePath(st); path != "" {
			dataDir = filepath.Dir(path)
		} else {
			dataDir = config.ConventionalDataDir
		}
	}
	loaded, err := secretprotection.LoadInstallationKey(secretprotection.InstallationKeyOptions{DataDir: dataDir})
	if err != nil {
		return nil, fmt.Errorf("initialize database secret protection: %w", err)
	}
	previous, replacing, err := secretprotection.LoadPreviousInstallationKey(nil)
	if err != nil {
		return nil, fmt.Errorf("initialize database secret protection: %w", err)
	}
	if replacing {
		manager, previousErr := secretprotection.NewManager(ctx, st, secretprotection.ManagerOptions{InstallationKey: previous})
		if previousErr == nil {
			if err := manager.ReplaceInstallationKey(ctx, loaded.Key); err == nil {
				return manager, nil
			} else if !errors.Is(err, secretprotection.ErrInstallationKeyMismatch) {
				return nil, fmt.Errorf("replace database installation key: %w", err)
			}
		} else if !errors.Is(previousErr, secretprotection.ErrInstallationKeyMismatch) {
			return nil, fmt.Errorf("initialize database secret protection with previous key: %w", previousErr)
		}
		// A second replica may have completed the replacement concurrently.
		// Verifying the new current key below distinguishes that safe state.
	}
	manager, err := secretprotection.NewManager(ctx, st, secretprotection.ManagerOptions{InstallationKey: loaded.Key})
	if err != nil {
		return nil, fmt.Errorf("initialize database secret protection: %w", err)
	}
	return manager, nil
}

func protectedSetting(reg *settings.Registry, key string) bool {
	if strings.HasPrefix(key, "secret.") || strings.HasPrefix(key, "llm.api_key.") {
		return true
	}
	setting, ok := reg.Get(key)
	return ok && setting.IsSecret()
}

func readProtectedSetting(ctx context.Context, st store.Store, protection *secretprotection.Manager, key string) (string, error) {
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
	plain, err := protection.OpenLatest(ctx, settingRecord(key), value)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func writeProtectedSetting(ctx context.Context, st store.Store, protection *secretprotection.Manager, key, value string) error {
	if protection == nil {
		return st.SetSetting(ctx, key, value)
	}
	if err := protection.Refresh(ctx); err != nil {
		return err
	}
	envelope, err := protection.Seal(settingRecord(key), []byte(value))
	if err != nil {
		return err
	}
	return st.SetSetting(ctx, key, envelope)
}

func settingRecord(key string) secretprotection.Record {
	return secretprotection.Record{Kind: "setting", ID: key, Field: "value"}
}

// openProtectedSettings lists every setting with protected values decrypted. A protected value
// stored as plaintext predates encrypted settings (v0.2.0-beta.1); the error names every such
// key, never its value, and no row is rewritten.
func openProtectedSettings(
	ctx context.Context,
	st store.Store,
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
		if !protectedSetting(reg, rows[i].Key) || rows[i].Value == "" {
			continue
		}
		if !secretprotection.IsEnvelope(rows[i].Value) {
			plaintext = append(plaintext, fmt.Errorf(
				"setting %q is stored unencrypted: this database predates encrypted settings (v0.2.0-beta.1); "+
					"upgrade through a v0.2.x release first, or clear that setting and re-enter it", rows[i].Key))
			continue
		}
		plain, err := protection.Open(settingRecord(rows[i].Key), rows[i].Value)
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

func reencryptProtectedSettings(
	ctx context.Context, st store.Store, reg *settings.Registry, protection *secretprotection.Manager,
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
		if !protectedSetting(reg, row.Key) || row.Value == "" {
			continue
		}
		plain, err := protection.Open(settingRecord(row.Key), row.Value)
		if err != nil {
			return fmt.Errorf("open setting %q for data-key rotation: %w", row.Key, err)
		}
		envelope, err := protection.Seal(settingRecord(row.Key), plain)
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
