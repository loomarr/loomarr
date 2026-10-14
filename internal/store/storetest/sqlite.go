// Package storetest builds the conformance suites' private databases: one migrated, boot-seeded
// template per backend, cloned for every assertion. The core store suite and the filler store
// suite share it, so both prove the same property against the same production adapters.
//
// It never imports the store packages: each caller passes its own open function, so the core
// store's in-package tests can use it without an import cycle.
package storetest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// Closer is what a factory needs from the store it opens.
type Closer interface{ Close() error }

// OpenFunc opens a store from a DATABASE_URL, migrating when autoMigrate is set.
type OpenFunc[S Closer] func(ctx context.Context, databaseURL string, autoMigrate bool) (S, error)

// SQLiteTemplate migrates and boot-seeds one database, folds its WAL into the main file through
// checkpoint, and returns the file's bytes: a complete, closed database that any number of copies
// can start from.
func SQLiteTemplate[S Closer](ctx context.Context, open OpenFunc[S], checkpoint func(S) error, path string) ([]byte, error) {
	template, err := open(ctx, "sqlite://"+path, true)
	if err != nil {
		return nil, err
	}
	if err := checkpoint(template); err != nil {
		_ = template.Close()
		return nil, fmt.Errorf("checkpoint: %w", err)
	}
	if err := template.Close(); err != nil {
		return nil, fmt.Errorf("close: %w", err)
	}
	return os.ReadFile(path)
}

// SQLiteClones migrates and boot-seeds one clean SQLite database, then gives every conformance
// assertion a byte-for-byte private copy. The assertions still open the copied file through the
// production SQLite adapter (including WAL and connection pragmas), but they do not replay the
// complete forward-only migration history once per assertion.
//
// Dedicated migration, startup, downgrade, historical-data, and restart tests deliberately keep
// opening with autoMigrate: this is only the already-current-schema starting point for
// backend-agnostic store behavior.
func SQLiteClones[S Closer](t *testing.T, open OpenFunc[S], checkpoint func(S) error) func(*testing.T) S {
	t.Helper()
	dir := t.TempDir()
	templateBytes, err := SQLiteTemplate(context.Background(), open, checkpoint, filepath.Join(dir, "conformance-template.db"))
	if err != nil {
		t.Fatalf("create sqlite conformance template: %v", err)
	}

	var sequence atomic.Uint64
	return func(t *testing.T) S {
		t.Helper()
		path := filepath.Join(dir, fmt.Sprintf("conformance-%d.db", sequence.Add(1)))
		if err := os.WriteFile(path, templateBytes, 0o600); err != nil {
			t.Fatalf("clone sqlite conformance template: %v", err)
		}
		s, err := open(context.Background(), "sqlite://"+path, false)
		if err != nil {
			t.Fatalf("open cloned sqlite conformance store: %v", err)
		}
		t.Cleanup(func() { _ = s.Close() })
		return s
	}
}
