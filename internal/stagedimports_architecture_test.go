package internal_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// stagedImporters lists, for each staged package (built and tested, not wired into any install),
// every other package directory that imports it, tests included. It matches the "Staged, not wired"
// table in docs/design/filler.md. A new importer is wiring: update that table in the same change.
var stagedImporters = map[string][]string{
	"internal/filler/screening":            {"internal/filler/records"},
	"internal/filler/structure/assessment": {"internal/filler/screening"},
	"internal/filler/structure/window":     {"internal/filler/records", "internal/filler/structure/assessment"},
}

func TestStagedPackagesGainNoImporters(t *testing.T) {
	const module = "github.com/loomarr/loomarr/"
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	found := map[string][]string{}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := entry.Name()
		if entry.IsDir() {
			if path != root && (strings.HasPrefix(name, ".") || name == "node_modules" || name == "testdata" || path == filepath.Join(root, "web")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		dir, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		dir = filepath.ToSlash(dir)
		for _, spec := range file.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			staged := strings.TrimPrefix(imported, module)
			if _, ok := stagedImporters[staged]; ok && dir != staged && !slices.Contains(found[staged], dir) {
				found[staged] = append(found[staged], dir)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for staged, allowed := range stagedImporters {
		got := found[staged]
		slices.Sort(got)
		if !slices.Equal(got, allowed) {
			t.Errorf("importers of %s = %v, want %v; update stagedImporters and the staged table in docs/design/filler.md", staged, got, allowed)
		}
	}
}
