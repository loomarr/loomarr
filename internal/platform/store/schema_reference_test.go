package store

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// The committed database reference must be what the migrations and the descriptions produce, so a
// migration that adds a column fails here until someone says, in plain words, what it holds (#2446).
func TestSchemaReference_DescribesEveryTableAndColumnAndIsCommitted(t *testing.T) {
	generated, err := SchemaReference(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(file), "..", "..", "..", "docs", "reference", "database.md")
	committed, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read committed reference (run `make db-docs`?): %v", err)
	}
	if string(committed) != string(generated) {
		t.Errorf("docs/reference/database.md is stale: run `make db-docs` and commit "+
			"(committed %d bytes, generated %d)", len(committed), len(generated))
	}
}
