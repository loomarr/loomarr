package internal_test

import (
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
)

var aggregateStoreExceptions = map[string]string{
	modulePath + "/cmd/loomarr":  "owns the process-lifetime store and its shutdown",
	modulePath + "/internal/api": "retains complete HTTP dependency assembly outside this refactor",
	modulePath + "/internal/app": "is the composition root that narrows the store for domain modules",
	modulePath + "/internal/filler/records": "is the aggregate's filler half: it extends the core store.Store " +
		"with filler persistence (#1747)",
	modulePath + "/internal/filler/storeadapter": "is where the aggregate store is narrowed to the persistence roles " +
		"the filler seams declare, as the composition root did before these adapters moved here",
	modulePath + "/internal/proposals/accept": "contains compile-time conformance assertions for its narrow store roles",
	modulePath + "/internal/testkit":          "owns shared complete-store factories for tests",
}

// aggregateStorePackages export the aggregate. records.Store embeds store.Store, so naming it
// is naming the aggregate; without it here, moving a package onto records.Store would hide its
// aggregate reference from the allowlist.
var aggregateStorePackages = map[string]bool{
	modulePath + "/internal/platform/store": true,
	modulePath + "/internal/filler/records": true,
}

// Aggregate store references are deliberately rare. The full store exists for construction,
// migration, conformance and process lifecycle; domain modules should name only the persistence
// role they consume. The package allowlist keeps those retained seams explicit and prevents a
// new domain package from silently inheriting the aggregate surface.
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
				if !aggregateStorePackages[importPath] {
					continue
				}
				alias := filepath.Base(importPath)
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

// clusterPackages are the store's cluster packages, which own their row types and sentinels.
var clusterPackages = map[string]bool{
	modulePath + "/internal/platform/store/identitystore":    true,
	modulePath + "/internal/platform/store/clipstore":        true,
	modulePath + "/internal/platform/store/jobstore":         true,
	modulePath + "/internal/platform/store/activitystore":    true,
	modulePath + "/internal/platform/store/channeldatastore": true,
	modulePath + "/internal/platform/store/channelstore":     true,
	modulePath + "/internal/platform/store/proposalstore":    true,
	modulePath + "/internal/platform/store/librarystore":     true,
}

// A cluster's names have one spelling (identitystore.User, clipstore.Clip), so a reader lands on
// the package that owns them. The store root therefore re-exports none of them.
func TestStoreRootReExportsNoClusterName(t *testing.T) {
	storePkg := loomarrPackages(t)[modulePath+"/internal/platform/store"]
	if storePkg == nil || len(storePkg.GoFiles) == 0 {
		t.Fatal("internal/platform/store is not in the package graph — the scan would vacuously pass")
	}
	for name, cluster := range storeClusterAliases(t, storePkg) {
		t.Errorf("store re-exports %s.%s: consumers name the cluster package", cluster, name)
	}
}

func TestStoreClusterAliasesFindsTypesAndValues(t *testing.T) {
	dir := t.TempDir()
	src := `package store
import (
	"` + modulePath + `/internal/platform/store/clipstore"
	"` + modulePath + `/internal/platform/store/storedb"
)
type Clip = clipstore.Clip
var ErrTaxonConflict = clipstore.ErrTaxonConflict
var ErrNotFound = storedb.ErrNotFound`
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	got := storeClusterAliases(t, &build.Package{Dir: dir, GoFiles: []string{"a.go"}})
	want := map[string]string{"Clip": "clipstore", "ErrTaxonConflict": "clipstore"}
	if !maps.Equal(got, want) {
		t.Errorf("storeClusterAliases = %v, want %v", got, want)
	}
}

// storeClusterAliases maps each exported store declaration whose value is a cluster package's
// identifier (type User = identitystore.User, var ErrTaxonConflict = clipstore.ErrTaxonConflict)
// to that cluster package's name.
func storeClusterAliases(t *testing.T, storePkg *build.Package) map[string]string {
	t.Helper()
	aliases := map[string]string{}
	for _, name := range storePkg.GoFiles {
		path := filepath.Join(storePkg.Dir, name)
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		clusters := map[string]bool{}
		for importPath := range clusterPackages {
			for alias := range importNames(t, path, file, importPath) {
				clusters[alias] = true
			}
		}
		if len(clusters) == 0 {
			continue
		}
		record := func(name *ast.Ident, value ast.Expr) {
			selector, ok := value.(*ast.SelectorExpr)
			if !ok || !name.IsExported() {
				return
			}
			if ident, ok := selector.X.(*ast.Ident); ok && clusters[ident.Name] {
				aliases[name.Name] = ident.Name
			}
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gen.Specs {
				switch spec := spec.(type) {
				case *ast.TypeSpec:
					record(spec.Name, spec.Type)
				case *ast.ValueSpec:
					for i, name := range spec.Names {
						if i < len(spec.Values) {
							record(name, spec.Values[i])
						}
					}
				}
			}
		}
	}
	return aliases
}

// importNames returns the names file uses for importPath: its base name, or the rename.
func importNames(t *testing.T, path string, file *ast.File, importPath string) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	for _, imp := range file.Imports {
		got, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			t.Fatalf("unquote import in %s: %v", path, err)
		}
		if got != importPath {
			continue
		}
		name := filepath.Base(importPath)
		if imp.Name != nil {
			name = imp.Name.Name
		}
		if name != "_" && name != "." {
			names[name] = true
		}
	}
	return names
}
