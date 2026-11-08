package internal_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// A test never migrates a fresh SQLite file; it copies the migrated template.
//
// Replaying every migration costs ~15 s per open under -race (modernc's SQLite reparses the whole
// schema after each DDL statement, and the race detector multiplies that), and the cost grows with
// each migration. Forty-four test files did it, and the race-policy lanes crossed their 15-minute
// timeout as migrations went from 152 to 187 (#2716): one internal/app test took 26 s under -race
// and 0.7 s without. testkit.MigratedSQLiteStore and testkit.MigratedSQLiteDSN copy a template
// migrated once per test binary.
//
// The guard parses every module test file and flags a call `<pkg>.Open(ctx, dsn, true)` whose dsn
// is built from a "sqlite://" literal, directly or through a local assigned in the same function.
// internal/platform/store tests migrations, and internal/testkit builds the template; both are
// exempt.
var freshMigrationAllowlist = map[string]string{
	"internal/api/api_test.go": "builds its own once-per-binary template (migratedTemplate), the same idea " +
		"as testkit's",
	"internal/integration/database_lifecycle_postgres_test.go": "seeds one large fixture at a path the " +
		"lifecycle test then hands to a real binary",
}

func TestNoTestMigratesAFreshSQLiteStore(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	files, err := moduleTestFiles(root)
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("found no _test.go files — the guard would pass over a tree it never examined")
	}

	violating := map[string]bool{}
	for _, rel := range files {
		slash := filepath.ToSlash(rel)
		if strings.HasPrefix(slash, "internal/platform/store/") || strings.HasPrefix(slash, "internal/testkit/") {
			continue
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, filepath.Join(root, rel), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", rel, err)
		}
		lines := freshMigrations(fset, file)
		if len(lines) == 0 {
			continue
		}
		violating[slash] = true
		if _, ok := freshMigrationAllowlist[slash]; ok {
			continue
		}
		t.Errorf("%s:%s migrates a fresh SQLite store: open testkit.MigratedSQLiteDSN(t) (or use "+
			"testkit.MigratedSQLiteStore) instead; a fresh migration costs ~15 s under -race (#2716)",
			slash, strings.Join(lines, ","))
	}
	allowed := make([]string, 0, len(freshMigrationAllowlist))
	for rel := range freshMigrationAllowlist {
		allowed = append(allowed, rel)
	}
	sort.Strings(allowed)
	for _, rel := range allowed {
		if !violating[rel] {
			t.Errorf("%s no longer migrates a fresh store: remove it from freshMigrationAllowlist", rel)
		}
	}
}

func TestFreshMigrationDetection(t *testing.T) {
	for name, tc := range map[string]struct {
		src  string
		want int
	}{
		"literal path": {`func TestX(t *testing.T) {
	_, _ = store.Open(ctx, "sqlite://"+filepath.Join(t.TempDir(), "a.db"), true)
}`, 1},
		"through a local": {`func TestX(t *testing.T) {
	dsn := "sqlite://" + filepath.Join(t.TempDir(), "a.db")
	_, _ = records.Open(ctx, dsn, true)
	_, _ = records.Open(ctx, dsn, true)
}`, 2},
		"template copy": {`func TestX(t *testing.T) {
	_, _ = store.Open(ctx, testkit.MigratedSQLiteDSN(t), true)
}`, 0},
		"no migration": {`func TestX(t *testing.T) {
	_, _ = store.Open(ctx, "sqlite://"+path, false)
}`, 0},
		"postgres": {`func TestX(t *testing.T) {
	_, _ = store.Open(ctx, startPostgres(t), true)
}`, 0},
		"a local of the same name elsewhere": {`func TestA(t *testing.T) {
	dsn := "sqlite://a.db"
	_ = dsn
}
func TestB(t *testing.T) {
	dsn := testkit.MigratedSQLiteDSN(t)
	_, _ = store.Open(ctx, dsn, true)
}`, 0},
	} {
		t.Run(name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "x_test.go", "package x\n"+tc.src, 0)
			if err != nil {
				t.Fatal(err)
			}
			if got := len(freshMigrations(fset, file)); got != tc.want {
				t.Fatalf("found %d fresh migrations, want %d", got, tc.want)
			}
		})
	}
}

// freshMigrations returns the line of each `<pkg>.Open(ctx, dsn, true)` in file whose dsn names a
// "sqlite://" literal, directly or through a local assigned in the enclosing function.
func freshMigrations(fset *token.FileSet, file *ast.File) []string {
	var lines []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		locals := map[string]ast.Expr{}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if as, ok := n.(*ast.AssignStmt); ok && len(as.Lhs) == len(as.Rhs) {
				for i, lhs := range as.Lhs {
					if id, ok := lhs.(*ast.Ident); ok {
						locals[id.Name] = as.Rhs[i]
					}
				}
			}
			return true
		})
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 3 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Open" {
				return true
			}
			if migrate, ok := call.Args[2].(*ast.Ident); !ok || migrate.Name != "true" {
				return true
			}
			dsn := call.Args[1]
			if id, ok := dsn.(*ast.Ident); ok && locals[id.Name] != nil {
				dsn = locals[id.Name]
			}
			if namesSQLiteLiteral(dsn) {
				lines = append(lines, strconv.Itoa(fset.Position(call.Pos()).Line))
			}
			return true
		})
	}
	return lines
}

func namesSQLiteLiteral(expr ast.Expr) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING && strings.HasPrefix(strings.Trim(lit.Value, "`\""), "sqlite://") {
			found = true
		}
		return !found
	})
	return found
}
