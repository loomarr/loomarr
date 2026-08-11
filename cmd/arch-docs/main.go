// Command arch-docs regenerates the package map inside docs/design.md §2 from the
// code itself — each package's own doc comment plus its imports.
//
// It exists because §2 was the one part of the design doc a newcomer reads first and
// the one part nothing kept honest: its diagram omitted `filler` (explicitly, "to keep
// the diagram legible") and `playout` (which arrived later as §9.1), which between them
// are two of the five largest packages. A hand-maintained architecture map is the same
// shape as `scripts/check-retired.sh` and the Makefile's TAGS list — a list that drifts
// — so this one is generated and CI diffs it, exactly as `make config-docs` does for the
// settings registry.
//
// ⚠ It owns ONE marker-delimited block and must never write outside it. design.md is
// hand-written and authoritative for behaviour (CLAUDE.md doc-first); this tool supplies
// the mechanical inventory that prose should not have to restate.
//
// Deliberately emits no size metrics. See scan.go's Package doc for why.
package main

import (
	"log/slog"
	"os"
)

// anchor is the §2 heading the block is filed under on first run.
const anchor = "## 2. Architecture"

func main() {
	docPath := "docs/design.md"
	if len(os.Args) > 1 {
		docPath = os.Args[1]
	}
	root := "internal"
	if len(os.Args) > 2 {
		root = os.Args[2]
	}

	pkgs, err := scan(root)
	if err != nil {
		slog.Error("scan packages", "root", root, "err", err)
		os.Exit(1)
	}

	current, err := os.ReadFile(docPath)
	if err != nil {
		slog.Error("read design doc", "path", docPath, "err", err)
		os.Exit(1)
	}

	next, err := splice(string(current), render(pkgs), anchor)
	if err != nil {
		slog.Error("splice generated block", "path", docPath, "err", err)
		os.Exit(1)
	}

	if next == string(current) {
		return // no write, so the file mtime does not churn on a no-op run
	}
	if err := os.WriteFile(docPath, []byte(next), 0o644); err != nil {
		slog.Error("write design doc", "path", docPath, "err", err)
		os.Exit(1)
	}
}
