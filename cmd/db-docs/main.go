// Command db-docs generates docs/reference/database.md, the plain-words reference to every table
// and column, from a SQLite database migrated to head and the descriptions in
// internal/platform/store/schemadoc. `make db-docs` runs this; `make db-docs-verify` diffs the
// result. It fails, listing each gap, when a migration adds a table or column nobody described.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/loomarr/loomarr/internal/platform/store"
)

func main() {
	out := "docs/reference/database.md"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	dir, err := os.MkdirTemp("", "loomarr-db-docs-")
	if err != nil {
		slog.Error("temp dir", "err", err)
		os.Exit(1)
	}
	md, err := store.SchemaReference(context.Background(), dir)
	_ = os.RemoveAll(dir)
	if err != nil {
		slog.Error("build database reference", "err", err)
		os.Exit(1)
	}
	if err := os.WriteFile(out, md, 0o644); err != nil {
		slog.Error("write database reference", "err", err)
		os.Exit(1)
	}
}
