package testkit

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/mantonx/loomarr/internal/store"
)

// SQLiteStore opens a fresh migrated production store for a test. Domain tests use
// this shared adapter when behavior depends on real settings/channel persistence.
func SQLiteStore(t testing.TB) store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+filepath.Join(t.TempDir(), "loomarr.db"), true)
	if err != nil {
		t.Fatalf("open SQLite test store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}
