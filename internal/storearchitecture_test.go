package internal_test

import (
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
)

var aggregateStoreExceptions = map[string]string{
	modulePath + "/cmd/loomarr":           "owns the process-lifetime store and its shutdown",
	modulePath + "/cmd/seed":              "is an explicit whole-database development command",
	modulePath + "/internal/api":          "retains complete HTTP dependency assembly outside this refactor",
	modulePath + "/internal/app":          "is the composition root that narrows the store for domain modules",
	modulePath + "/internal/channels":     "is a migration checkpoint while channel consumers are narrowed",
	modulePath + "/internal/devbootstrap": "is a migration checkpoint while development bootstrap is narrowed",
	modulePath + "/internal/scheduler":    "is a migration checkpoint while backend selection is made explicit",
	modulePath + "/internal/suggest":      "contains compile-time conformance assertions for its narrow store roles",
	modulePath + "/internal/testkit":      "owns shared complete-store factories for tests",
}

// Aggregate store references are deliberately rare. The full store exists for construction,
// migration, conformance and process lifecycle; domain modules should name only the persistence
// role they consume. Keeping the temporary migration checkpoints explicit makes this gate useful
// before the refactor is complete: a new package cannot silently join the aggregate surface, and
// every completed slice removes its own exception.
func TestAggregateStoreReferencesStayAtApprovedSeams(t *testing.T) {
	refs := aggregateStoreReferences(t, loomarrPackages(t))
	seen := map[string]bool{}
	for _, ref := range refs {
		seen[ref.packagePath] = true
		if _, ok := aggregateStoreExceptions[ref.packagePath]; !ok {
			t.Errorf("%s:%d references store.Store — domain modules must accept a narrow persistence role", ref.file, ref.line)
		}
	}
	for packagePath, rationale := range aggregateStoreExceptions {
		if rationale == "" {
			t.Errorf("%s has no aggregate-store exception rationale", packagePath)
		}
		if !seen[packagePath] {
			t.Errorf("%s is an obsolete aggregate-store exception — remove it", packagePath)
		}
	}
}

type aggregateStoreReference struct {
	packagePath string
	file        string
	line        int
}

func aggregateStoreReferences(t *testing.T, packages map[string]*build.Package) []aggregateStoreReference {
	t.Helper()
	var refs []aggregateStoreReference
	for packagePath, pkg := range packages {
		for _, name := range append(append([]string(nil), pkg.GoFiles...), pkg.CgoFiles...) {
			path := filepath.Join(pkg.Dir, name)
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			aliases := map[string]bool{}
			for _, imp := range file.Imports {
				importPath, err := strconv.Unquote(imp.Path.Value)
				if err != nil {
					t.Fatalf("unquote import in %s: %v", path, err)
				}
				if importPath != modulePath+"/internal/store" {
					continue
				}
				alias := "store"
				if imp.Name != nil {
					alias = imp.Name.Name
				}
				if alias == "." {
					pos := fset.Position(imp.Pos())
					refs = append(refs, aggregateStoreReference{packagePath: packagePath, file: path, line: pos.Line})
					continue
				}
				if alias != "_" {
					aliases[alias] = true
				}
			}
			if len(aliases) == 0 {
				continue
			}
			ast.Inspect(file, func(node ast.Node) bool {
				selector, ok := node.(*ast.SelectorExpr)
				if !ok || selector.Sel.Name != "Store" {
					return true
				}
				ident, ok := selector.X.(*ast.Ident)
				if !ok || !aliases[ident.Name] {
					return true
				}
				pos := fset.Position(selector.Pos())
				refs = append(refs, aggregateStoreReference{packagePath: packagePath, file: path, line: pos.Line})
				return true
			})
		}
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].packagePath != refs[j].packagePath {
			return refs[i].packagePath < refs[j].packagePath
		}
		if refs[i].file != refs[j].file {
			return refs[i].file < refs[j].file
		}
		return refs[i].line < refs[j].line
	})
	return refs
}
