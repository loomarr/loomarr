package testkit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/store"
)

// A template another schema's run pruned after this process found it must not fail the next test:
// MigratedSQLiteDSN rebuilds (or re-finds) the shared template and still hands out a store at HEAD.
func TestMigratedSQLiteDSN_RecoversWhenTheSharedTemplateIsPruned(t *testing.T) {
	_ = MigratedSQLiteDSN(t) // resolve the shared template
	shared := templatePath

	// Point this process at a copy, then delete it, as a prune would.
	pruned := filepath.Join(t.TempDir(), "pruned.db")
	if err := copyFile(shared, pruned); err != nil {
		t.Fatal(err)
	}
	templatePath = pruned
	t.Cleanup(func() { templatePath = shared })
	if err := os.Remove(pruned); err != nil {
		t.Fatal(err)
	}

	st, err := store.Open(t.Context(), MigratedSQLiteDSN(t), true)
	if err != nil {
		t.Fatalf("open a store after the template was pruned: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.ListChannels(t.Context()); err != nil {
		t.Fatalf("the recovered store is not at HEAD: %v", err)
	}
}

func TestSQLiteMigrationDigestIsStable(t *testing.T) {
	a, err := store.SQLiteMigrationDigest()
	if err != nil {
		t.Fatal(err)
	}
	b, err := store.SQLiteMigrationDigest()
	if err != nil {
		t.Fatal(err)
	}
	if a != b || len(a) != 64 {
		t.Fatalf("digest = %q then %q, want one 64-hex-digit value", a, b)
	}
}
