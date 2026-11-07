package settings

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// allowedKeyLiterals names the production files that may spell a registry key as a string. A
// file is listed only when its literal cannot be a handle; the value says why.
var allowedKeyLiterals = map[string]string{}

// TestNoSettingKeyLiteralsOutsideSettings keeps reads on the generated handles: a string literal
// equal to a registry key anywhere in production code under internal/ or cmd/ (outside this
// package) is a string-keyed read that a misspelling or retirement would not catch at compile
// time. MigrationOnly keys have no handle and are exempt.
func TestNoSettingKeyLiteralsOutsideSettings(t *testing.T) {
	handles := map[string]string{}
	for _, s := range NewRegistry().All() {
		if !s.MigrationOnly {
			handles[s.Key] = HandleName(s.Key)
		}
	}
	var found []string
	fset := token.NewFileSet()
	for _, root := range []string{"../../../internal", "../../../cmd"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			slashed := filepath.ToSlash(path)
			switch {
			case d.IsDir():
				if slashed == "../../../internal/platform/settings" {
					return filepath.SkipDir
				}
				return nil
			case !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go"):
				return nil
			}
			if _, ok := allowedKeyLiterals[strings.TrimPrefix(slashed, "../../../")]; ok {
				return nil
			}
			file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if err != nil {
				return fmt.Errorf("parse %s: %w", path, err)
			}
			ast.Inspect(file, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				value, err := strconv.Unquote(lit.Value)
				if err != nil {
					return true
				}
				if name, ok := handles[value]; ok {
					pos := fset.Position(lit.Pos())
					found = append(found, fmt.Sprintf("%s:%d: %q -> use settings.%s", strings.TrimPrefix(slashed, "../../../"), pos.Line, value, name))
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	slices.Sort(found)
	for _, f := range found {
		t.Error(f)
	}
}
