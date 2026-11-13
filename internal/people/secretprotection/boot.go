package secretprotection

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/loomarr/loomarr/internal/platform/config"
)

// BootOptions locate the installation key and the database it protects.
type BootOptions struct {
	// DataDir is where a generated installation key is kept. Empty means the SQLite database's own
	// directory, else the conventional data directory.
	DataDir string
	// DatabasePath is the SQLite file, or empty for a Postgres store.
	DatabasePath string
	Log          *slog.Logger
}

// Boot loads the installation key and opens the manager over the repository. A previous key
// supplied for replacement is rewrapped to the current one first; a replacement a second replica
// already completed is recognised by verifying the current key.
func Boot(ctx context.Context, repository DataKeyRepository, o BootOptions) (*Manager, error) {
	dataDir := o.DataDir
	if dataDir == "" {
		if o.DatabasePath != "" {
			dataDir = filepath.Dir(o.DatabasePath)
		} else {
			dataDir = config.ConventionalDataDir
		}
	}
	loaded, err := LoadInstallationKey(InstallationKeyOptions{DataDir: dataDir})
	if err != nil {
		return nil, fmt.Errorf("initialize database secret protection: %w", err)
	}
	if keyBesideDatabase(loaded.Path, o.DatabasePath) && o.Log != nil {
		o.Log.Warn("the encryption key is stored beside the database it protects: a copy of this folder "+
			"(a volume backup, rsync, a sync tool) carries both, and the saved credentials with them. "+
			"Keep the key in a separate place and point LOOMARR_ENCRYPTION_KEY_FILE at it.",
			"key", loaded.Path, "database", o.DatabasePath)
	}
	previous, replacing, err := LoadPreviousInstallationKey(nil)
	if err != nil {
		return nil, fmt.Errorf("initialize database secret protection: %w", err)
	}
	if replacing {
		manager, previousErr := NewManager(ctx, repository, ManagerOptions{InstallationKey: previous})
		if previousErr == nil {
			if err := manager.ReplaceInstallationKey(ctx, loaded.Key); err == nil {
				return manager, nil
			} else if !errors.Is(err, ErrInstallationKeyMismatch) {
				return nil, fmt.Errorf("replace database installation key: %w", err)
			}
		} else if !errors.Is(previousErr, ErrInstallationKeyMismatch) {
			return nil, fmt.Errorf("initialize database secret protection with previous key: %w", previousErr)
		}
		// A second replica may have completed the replacement concurrently.
		// Verifying the new current key below distinguishes that safe state.
	}
	manager, err := NewManager(ctx, repository, ManagerOptions{InstallationKey: loaded.Key})
	if err != nil {
		return nil, fmt.Errorf("initialize database secret protection: %w", err)
	}
	return manager, nil
}

// keyBesideDatabase reports whether the installation key file sits in the SQLite database's own
// directory, where any copy of that folder carries the key and the ciphertext it protects together.
// A key from the environment (no path) or a Postgres store (no file) is never beside it.
func keyBesideDatabase(keyPath, databasePath string) bool {
	if keyPath == "" || databasePath == "" {
		return false
	}
	return filepath.Dir(filepath.Clean(keyPath)) == filepath.Dir(filepath.Clean(databasePath))
}
