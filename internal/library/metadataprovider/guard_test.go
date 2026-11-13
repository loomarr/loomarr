package metadataprovider_test

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Decision 16 (#2151): precedence is expressed over capabilities, and no code decides anything by
// which provider it is talking to. A rule keyed on one provider's id is a rule a new provider
// silently falls outside of. This gate type-checks every production package in the module that
// names the contract, so an ID is tracked through variables, fields and calls, and fails on:
//
//   - == or != with a value of type ID on either side (a check against "" is allowed),
//   - a switch on a value of type ID,
//   - a strings, bytes or slices function given a value of type ID or a []ID,
//   - an ID built from a string literal outside the file that declares the set,
//   - a type assertion or type switch from Provider to anything but an interface (a concrete
//     provider type is a provider id by another name),
//   - a map literal keyed by ID.
//
// Using an ID as a map index, stamping it as provenance, or passing it to the registry is fine.
// Imports other than the contract are stubbed, so the check reads only source files, which the
// test cache tracks: a new violation is never a cached pass.

const idDeclFile = "metadataprovider.go"

func TestNoCodeComparesProviderIDs(t *testing.T) {
	root, module := moduleRoot(t)
	contractPath := module + "/internal/library/metadataprovider"
	contractDir := filepath.Join(root, "internal", "library", "metadataprovider")

	fset := token.NewFileSet()
	contractFiles := parseDir(t, fset, contractDir)
	contract, contractInfo := typeCheck(fset, contractPath, contractFiles, stubImporter{})
	if contract.Scope().Lookup("ID") == nil || contract.Scope().Lookup("Provider") == nil {
		t.Fatal("the contract declares no ID or Provider; the gate would check nothing")
	}
	report := func(dir string, findings []string) {
		rel, _ := filepath.Rel(root, dir)
		for _, f := range findings {
			t.Errorf("%s/%s — express the rule over capabilities (decision 16)", filepath.ToSlash(rel), f)
		}
	}
	report(contractDir, violations(fset, contractFiles, contractInfo, contract))

	checked := 1
	imp := stubImporter{known: map[string]*types.Package{contractPath: contract}}
	for _, top := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				return nil
			}
			if name := d.Name(); name == "testdata" || name == "node_modules" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			if p == contractDir || !importsContract(t, p, contractPath) {
				return nil
			}
			rel, _ := filepath.Rel(root, p)
			files := parseDir(t, fset, p)
			_, info := typeCheck(fset, module+"/"+filepath.ToSlash(rel), files, imp)
			report(p, violations(fset, files, info, contract))
			checked++
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if checked < 5 {
		t.Fatalf("checked %d packages that name the contract; the gate is reading nothing", checked)
	}
}

func TestGuardCatchesProviderIDComparisons(t *testing.T) {
	const contractSrc = `package metadataprovider
type ID string
const (IDSonarr ID = "sonarr"; IDNFO ID = "nfo")
type Provider interface{ ID() ID }
type Lister interface{ List() }
type Source struct{ Provider ID; Basis string }
`
	const src = `package x
import (
	"slices"
	"strings"
	mp "example/internal/library/metadataprovider"
	"example/internal/library/metadataprovider/arr"
)
func f(p mp.Provider, s mp.Source, ids []mp.ID) {
	if p.ID() == "sonarr" {}
	if s.Provider != mp.IDNFO {}
	if string(p.ID()) == "nfo" {}
	id := s.Provider; if id == mp.IDSonarr {}
	var local mp.ID = p.ID(); if local != "x" {}
	switch p.ID() {}
	_ = mp.ID("nfo")
	_ = strings.EqualFold(string(p.ID()), "sonarr")
	_ = strings.HasPrefix(string(s.Provider), "son")
	_ = slices.Contains(ids, mp.IDSonarr)
	_ = map[mp.ID]int{mp.IDSonarr: 1}
	_, _ = p.(*arr.Sonarr)
	switch p.(type) { case *arr.Sonarr: }
}
func clean(p mp.Provider, m map[mp.ID]int, s mp.Source) {
	_ = m[mp.IDSonarr]
	s.Provider = mp.IDNFO
	if p.ID() == "" {}
	if s.Basis == "manager" {}
	_, _ = p.(mp.Lister)
	switch p.(type) { case mp.Lister, nil: }
	_ = make(map[mp.ID]int)
	_ = strings.ToUpper(s.Basis)
}
`
	fset := token.NewFileSet()
	parse := func(name, text string) *ast.File {
		f, err := parser.ParseFile(fset, name, text, 0)
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	contract, _ := typeCheck(fset, "example/internal/library/metadataprovider", []*ast.File{parse("mp.go", contractSrc)}, stubImporter{})
	files := []*ast.File{parse("x.go", src)}
	_, info := typeCheck(fset, "example/internal/x", files, stubImporter{known: map[string]*types.Package{contract.Path(): contract}})
	got := violations(fset, files, info, contract)

	var lines []int
	for _, finding := range got {
		_, rest, _ := strings.Cut(finding, ":")
		line, _ := strconv.Atoi(strings.SplitN(rest, ":", 2)[0])
		lines = append(lines, line)
	}
	sort.Ints(lines)
	want := []int{9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21}
	if fmt.Sprint(lines) != fmt.Sprint(want) {
		t.Fatalf("flagged lines %v, want %v (one per line of f, none in clean):\n%s", lines, want, strings.Join(got, "\n"))
	}
}

// TestContractImportsNoLoomarrPackage keeps the contract a leaf, so the importer, the API and
// every provider can name it without a cycle. The precedence rules live in a subpackage because
// they reuse the audience ladder.
func TestContractImportsNoLoomarrPackage(t *testing.T) {
	_, module := moduleRoot(t)
	files := parseDir(t, token.NewFileSet(), ".")
	if len(files) == 0 {
		t.Fatal("no production files read")
	}
	for _, f := range files {
		for _, spec := range f.Imports {
			if p, _ := strconv.Unquote(spec.Path.Value); strings.HasPrefix(p, module+"/") {
				t.Errorf("the contract imports %s; it imports nothing from Loomarr", p)
			}
		}
	}
}

// stubImporter returns the known packages, and an empty package for any other import. Types
// from a stub are invalid, which is enough: the gate only needs the contract's types exactly.
type stubImporter struct {
	known map[string]*types.Package
}

func (s stubImporter) Import(p string) (*types.Package, error) {
	if pkg, ok := s.known[p]; ok {
		return pkg, nil
	}
	pkg := types.NewPackage(p, path.Base(p))
	pkg.MarkComplete()
	return pkg, nil
}

// typeCheck checks files leniently: errors from stubbed imports are expected and ignored.
func typeCheck(fset *token.FileSet, pkgPath string, files []*ast.File, imp types.Importer) (*types.Package, *types.Info) {
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Uses:  map[*ast.Ident]types.Object{},
		Defs:  map[*ast.Ident]types.Object{},
	}
	conf := types.Config{Importer: imp, Error: func(error) {}, FakeImportC: true}
	pkg, _ := conf.Check(pkgPath, fset, files, info)
	return pkg, info
}

// parseDir parses a directory's production files.
func parseDir(t *testing.T, fset *token.FileSet, dir string) []*ast.File {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var files []*ast.File
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	return files
}

// importsContract reports whether any production file in dir imports the contract.
func importsContract(t *testing.T, dir, contractPath string) bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, e.Name()), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range f.Imports {
			if p, _ := strconv.Unquote(spec.Path.Value); p == contractPath {
				return true
			}
		}
	}
	return false
}

// violations returns each finding in files as "file:line:col: what".
func violations(fset *token.FileSet, files []*ast.File, info *types.Info, contract *types.Package) []string {
	idType := contract.Scope().Lookup("ID").Type()
	providerType := contract.Scope().Lookup("Provider").Type()

	var unwrap func(ast.Expr) ast.Expr
	unwrap = func(e ast.Expr) ast.Expr {
		switch x := e.(type) {
		case *ast.ParenExpr:
			return unwrap(x.X)
		case *ast.CallExpr:
			if fn, ok := x.Fun.(*ast.Ident); ok && fn.Name == "string" && len(x.Args) == 1 {
				return unwrap(x.Args[0])
			}
		}
		return e
	}
	typeOf := func(e ast.Expr) types.Type { return info.TypeOf(unwrap(e)) }
	isID := func(e ast.Expr) bool {
		t := typeOf(e)
		return t != nil && types.Identical(t, idType)
	}
	isIDOrIDs := func(e ast.Expr) bool {
		if isID(e) {
			return true
		}
		s, ok := typeOf(e).(*types.Slice)
		return ok && types.Identical(s.Elem(), idType)
	}
	isEmptyString := func(e ast.Expr) bool {
		lit, ok := unwrap(e).(*ast.BasicLit)
		return ok && lit.Kind == token.STRING && (lit.Value == `""` || lit.Value == "``")
	}
	isLiteral := func(e ast.Expr) bool {
		lit, ok := unwrap(e).(*ast.BasicLit)
		return ok && lit.Kind == token.STRING
	}
	isProvider := func(e ast.Expr) bool {
		t := info.TypeOf(e)
		return t != nil && types.Identical(t, providerType)
	}
	isInterface := func(e ast.Expr) bool {
		if id, ok := e.(*ast.Ident); ok && id.Name == "nil" {
			return true
		}
		t := info.TypeOf(e)
		return t != nil && types.IsInterface(t)
	}
	stdlibPackage := func(e ast.Expr) bool {
		sel, ok := e.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		x, ok := sel.X.(*ast.Ident)
		if !ok {
			return false
		}
		pkg, ok := info.Uses[x].(*types.PkgName)
		if !ok {
			return false
		}
		switch pkg.Imported().Path() {
		case "strings", "bytes", "slices":
			return true
		}
		return false
	}

	var out []string
	for _, f := range files {
		declFile := filepath.Base(fset.Position(f.Pos()).Filename) == idDeclFile && f.Name.Name == contract.Name()
		report := func(n ast.Node, what string) {
			pos := fset.Position(n.Pos())
			out = append(out, fmt.Sprintf("%s:%d:%d: %s", filepath.Base(pos.Filename), pos.Line, pos.Column, what))
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.BinaryExpr:
				if (x.Op == token.EQL || x.Op == token.NEQ) && !declFile && (isID(x.X) || isID(x.Y)) &&
					!isEmptyString(x.X) && !isEmptyString(x.Y) {
					report(x, "compares a provider ID")
				}
			case *ast.SwitchStmt:
				if x.Tag != nil && isID(x.Tag) {
					report(x, "switches on a provider ID")
				}
			case *ast.CallExpr:
				if tv, ok := info.Types[x.Fun]; ok && tv.IsType() && types.Identical(tv.Type, idType) &&
					len(x.Args) == 1 && isLiteral(x.Args[0]) && !declFile {
					report(x, "builds a provider ID from a string")
				}
				if stdlibPackage(x.Fun) {
					for _, arg := range x.Args {
						if isIDOrIDs(arg) {
							report(x, "inspects a provider ID with a library function")
							break
						}
					}
				}
			case *ast.TypeAssertExpr:
				if x.Type != nil && isProvider(x.X) && !isInterface(x.Type) {
					report(x, "asserts a Provider to a concrete provider type")
				}
			case *ast.TypeSwitchStmt:
				var subject ast.Expr
				switch a := x.Assign.(type) {
				case *ast.ExprStmt:
					subject = a.X
				case *ast.AssignStmt:
					subject = a.Rhs[0]
				}
				if ta, ok := subject.(*ast.TypeAssertExpr); ok && isProvider(ta.X) {
					for _, stmt := range x.Body.List {
						for _, typ := range stmt.(*ast.CaseClause).List {
							if !isInterface(typ) {
								report(typ, "switches a Provider on a concrete provider type")
							}
						}
					}
				}
			case *ast.CompositeLit:
				if m, ok := info.TypeOf(x).(*types.Map); ok && types.Identical(m.Key(), idType) && len(x.Elts) > 0 {
					report(x, "keys a map literal by provider ID")
				}
			}
			return true
		})
	}
	return out
}

// moduleRoot returns the repository root and module path from the nearest go.mod.
func moduleRoot(t *testing.T) (string, string) {
	t.Helper()
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	for {
		f, err := os.Open(filepath.Join(dir, "go.mod"))
		if err == nil {
			defer func() { _ = f.Close() }()
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				if mod, ok := strings.CutPrefix(scanner.Text(), "module "); ok {
					return dir, strings.TrimSpace(mod)
				}
			}
			t.Fatal("go.mod names no module")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the package")
		}
		dir = parent
	}
}
