package metadataprovider_test

import (
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

// The Library is provider-neutral (#2151, scope A): its schema, models and scan name no catalog,
// so swapping in or adding one (IMDb, say) needs no change there. Which catalog a title's details
// come from is the catalog provider's declared Resolves; which namespaces ground a title is
// Namespace.Grounds. This gate fails on a namespace named in the Library's code: a Namespace*
// constant of the contract, or a string literal that is a namespace's value or quotes one in SQL
// ('tmdb'). The namespaces are read from the contract, so a new one is covered as it is declared.
//
// Code that reads an outside convention keyed by a namespace is allowed by file, with the reason.
var libraryNamespaceAllowed = map[string]string{
	"internal/library/scan/identity.go": "file names and arr items carry {tmdb-…}/{tvdb-…}/{imdb-…} tags and arr ids",
	"internal/library/scan/hooks.go":    "the arrs' webhooks name a movie by TMDB id and a series by TVDB id",
}

// libraryNamespaceScope is where the gate looks: every production file under these directories,
// and the store's library_*.go files. internal/library itself is the provisioning side, whose keys
// speak TMDB and TVDB by decision.
var libraryNamespaceScope = []string{"internal/library/scan"}

// notACatalog are the contract's namespaces that name no catalog: a local path.
var notACatalog = map[string]bool{"NamespaceFile": true}

func TestTheLibraryNamesNoCatalog(t *testing.T) {
	root, _ := moduleRoot(t)
	values := namespaceValues(t, filepath.Join(root, "internal", "metadataprovider", idDeclFile))
	var files []string
	for _, dir := range libraryNamespaceScope {
		err := filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go") {
				files = append(files, p)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	store, err := filepath.Glob(filepath.Join(root, "internal", "platform", "store", "library_*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range store {
		if !strings.HasSuffix(p, "_test.go") {
			files = append(files, p)
		}
	}
	if len(files) < 15 {
		t.Fatalf("read %d files; the gate would check nothing", len(files))
	}
	seen := map[string]bool{}
	for _, p := range files {
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		seen[rel] = true
		if _, ok := libraryNamespaceAllowed[rel]; ok {
			continue
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, finding := range namespaceNames(fset, f, values) {
			t.Errorf("%s:%s; the Library names no catalog (read the provider's capabilities, or Namespace.Grounds)",
				rel, finding)
		}
	}
	for rel := range libraryNamespaceAllowed {
		if !seen[rel] {
			t.Errorf("%s is allowed but no longer exists; drop it from libraryNamespaceAllowed", rel)
		}
	}
}

// The gate finds a namespace constant, a namespace literal and one quoted in SQL, and lets an
// open Namespace value and an unrelated string through.
func TestLibraryGuardCatchesNamespaces(t *testing.T) {
	const src = `package x

import mp "example/internal/metadataprovider"

func f(id mp.ExternalID) bool {
	_ = "SELECT value FROM library_ids WHERE namespace = 'tvdb'"
	_ = mp.Namespace("imdb")
	return id.Namespace == mp.NamespaceTMDB
}

func clean(id mp.ExternalID, ns mp.Namespace) bool {
	_ = "tmdb-like words in prose are fine"
	return id.Namespace == ns && ns.Grounds()
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	var lines []int
	for _, finding := range namespaceNames(fset, f, []string{"tmdb", "tvdb", "imdb"}) {
		n, _ := strconv.Atoi(strings.SplitN(finding, ":", 2)[0])
		lines = append(lines, n)
	}
	if want := []int{6, 7, 8}; !slices.Equal(lines, want) {
		t.Fatalf("findings on lines %v, want %v", lines, want)
	}
}

// namespaceValues reads the value of every Namespace* constant the contract declares.
func namespaceValues(t *testing.T, declFile string) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), declFile, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for i, name := range spec.Names {
			if !strings.HasPrefix(name.Name, "Namespace") || notACatalog[name.Name] || i >= len(spec.Values) {
				continue
			}
			if lit, ok := spec.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if v, err := strconv.Unquote(lit.Value); err == nil && v != "" {
					out = append(out, v)
				}
			}
		}
		return true
	})
	if !slices.Contains(out, "tmdb") {
		t.Fatalf("read namespaces %v from %s; the gate would check nothing", out, declFile)
	}
	return out
}

// namespaceNames lists, as "line: what", each place f names a namespace: a Namespace* constant
// of the contract (by whatever name the file imports it), or a string literal that is a
// namespace's value or quotes one ('tmdb').
func namespaceNames(fset *token.FileSet, f *ast.File, values []string) []string {
	contract := map[string]bool{}
	for _, spec := range f.Imports {
		p, _ := strconv.Unquote(spec.Path.Value)
		if !strings.HasSuffix(p, "/internal/metadataprovider") {
			continue
		}
		name := "metadataprovider"
		if spec.Name != nil {
			name = spec.Name.Name
		}
		contract[name] = true
	}
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SelectorExpr:
			if x, ok := n.X.(*ast.Ident); ok && contract[x.Name] && strings.HasPrefix(n.Sel.Name, "Namespace") &&
				n.Sel.Name != "Namespace" && !notACatalog[n.Sel.Name] {
				out = append(out, strconv.Itoa(fset.Position(n.Pos()).Line)+": "+x.Name+"."+n.Sel.Name)
			}
		case *ast.BasicLit:
			if n.Kind != token.STRING {
				return true
			}
			s, err := strconv.Unquote(n.Value)
			if err != nil {
				return true
			}
			for _, v := range values {
				if s == v || strings.Contains(s, "'"+v+"'") {
					out = append(out, strconv.Itoa(fset.Position(n.Pos()).Line)+": "+strconv.Quote(v))
					break
				}
			}
		}
		return true
	})
	return out
}
