package internal_test

import (
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"testing"
)

// dialectNames are the backend identities (storedb.Dialect, re-exported by internal/store).
var dialectNames = map[string]bool{
	"DialectPostgres": true,
	"DialectSQLite":   true,
}

// dialectExceptions are the only packages that may name a backend. Everything else asks a
// storedb helper for the SQL (LockRows, XactLock, ByDialect) or an internal/store capability
// (SupportsReplicas, CanMigrateToPostgres) for the behaviour, so a dialect difference has one
// owner (docs/design/decisions/0001-postgres-and-sqlite.md).
var dialectExceptions = map[string]string{
	modulePath + "/internal/jobs": "selects River's driver per backend: the pgx driver and the SQLite driver " +
		"are different Go packages, so the choice cannot be a SQL helper",
	modulePath + "/internal/store": "is the SQL adapter that implements both backends: constructors, " +
		"placeholders, goose dialect, pg_notify invalidations, SQLite-only backup and the SQLite-to-Postgres move",
	modulePath + "/internal/store/storedb": "defines the dialect and owns the helpers that turn it into SQL " +
		"(row locks, advisory locks, per-dialect text)",
}

// A backend comparison is a dialect difference spreading. A package outside the allowlist names
// neither backend; a new difference goes into a storedb helper or an internal/store capability.
func TestBackendNamesStayInsideTheStoreAdapter(t *testing.T) {
	refs := dialectReferences(t, loomarrPackages(t))
	seen := map[string]bool{}
	for _, ref := range refs {
		seen[ref.packagePath] = true
		if _, ok := dialectExceptions[ref.packagePath]; !ok {
			t.Errorf("%s:%d names %s — use a storedb helper (LockRows, XactLock, ByDialect) or an "+
				"internal/store capability (SupportsReplicas, CanMigrateToPostgres) instead of comparing the backend",
				ref.file, ref.line, ref.name)
		}
	}
	for packagePath, rationale := range dialectExceptions {
		if rationale == "" {
			t.Errorf("%s has no dialect exception rationale", packagePath)
		}
		if !seen[packagePath] {
			t.Errorf("%s is an obsolete dialect exception — remove it", packagePath)
		}
	}
}

type dialectReference struct {
	packagePath string
	file        string
	line        int
	name        string
}

func dialectReferences(t *testing.T, packages map[string]*build.Package) []dialectReference {
	t.Helper()
	var refs []dialectReference
	for packagePath, pkg := range packages {
		for _, name := range append(append([]string(nil), pkg.GoFiles...), pkg.CgoFiles...) {
			path := filepath.Join(pkg.Dir, name)
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			ast.Inspect(file, func(node ast.Node) bool {
				ident, ok := node.(*ast.Ident)
				if !ok || !dialectNames[ident.Name] {
					return true
				}
				refs = append(refs, dialectReference{
					packagePath: packagePath, file: path, line: fset.Position(ident.Pos()).Line, name: ident.Name,
				})
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
