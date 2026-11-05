// Command arch-docs regenerates the package map from the code itself — each package's own
// doc comment plus its imports — grouped by the area table in areas.go: an index block in
// docs/design/package-map.md, and one whole page per area in docs/design/areas/.
//
// It exists because design.md §2, where the map first lived, was the one part of the
// design doc a newcomer read first and the one part nothing kept honest: its diagram omitted `filler` (explicitly, "to keep
// the diagram legible") and `playout` (which arrived later as §9.1), which between them
// are two of the five largest packages. A hand-maintained architecture map is the same
// shape as `scripts/check-retired.sh` and the Makefile's TAGS list — a list that drifts
// — so this one is generated and CI diffs it, exactly as `make config-docs` does for the
// settings registry.
//
// ⚠ In package-map.md it owns ONE marker-delimited block and must never write outside it;
// it owns the areas/ directory whole. The index page's heading and introduction are hand-written, and the design docs stay authoritative for
// behaviour (AGENTS.md doc-first); this tool supplies the mechanical inventory that prose
// should not have to restate.
//
// Deliberately emits no size metrics. See scan.go's Package doc for why.
package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// defaultDocPath is the page that holds the generated block.
const defaultDocPath = "docs/design/package-map.md"

// anchor is the heading the block is filed under on first run.
const anchor = "# Package map"

func main() {
	docPath := defaultDocPath
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

	listed, _ := areaInventory(areas)
	for _, p := range pkgs {
		if !listed[p.Name] {
			slog.Error("package has no area; add it to one area in cmd/arch-docs/areas.go", "package", p.Name)
			os.Exit(1)
		}
	}

	current, err := os.ReadFile(docPath)
	if err != nil {
		slog.Error("read design doc", "path", docPath, "err", err)
		os.Exit(1)
	}

	next, err := splice(string(current), render(areas, pkgs), anchor)
	if err != nil {
		slog.Error("splice generated block", "path", docPath, "err", err)
		os.Exit(1)
	}
	if err := writeIfChanged(docPath, next); err != nil {
		slog.Error("write design doc", "path", docPath, "err", err)
		os.Exit(1)
	}

	if err := writeAreaPages(filepath.Join(filepath.Dir(docPath), areaPagesDir), renderAreaPages(areas, pkgs)); err != nil {
		slog.Error("write area pages", "err", err)
		os.Exit(1)
	}
}

// writeAreaPages writes one page per area into dir and removes any other file there, so a
// renamed or retired area cannot leave its old page behind.
func writeAreaPages(dir string, pages map[string]string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if _, ok := pages[strings.TrimSuffix(e.Name(), ".md")]; !ok || !strings.HasSuffix(e.Name(), ".md") {
			if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
				return err
			}
		}
	}
	for name, page := range pages {
		if err := writeIfChanged(filepath.Join(dir, name+".md"), page); err != nil {
			return err
		}
	}
	return nil
}

// writeIfChanged skips an identical write, so file mtimes do not churn on a no-op run.
func writeIfChanged(path, content string) error {
	if current, err := os.ReadFile(path); err == nil && string(current) == content {
		return nil
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
