package metadataprovider_test

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// Decision 16 (#2151): precedence is expressed over capabilities, and no code compares provider
// ids. A comparison against one provider's id is a rule that a new provider silently falls
// outside of, so the moment one is written the registry stops being the place a provider is
// described. This gate reads the production source of the whole module and fails on:
//
//   - == or != with a provider ID constant (metadataprovider.IDSonarr …) on either side,
//   - == or != between a provider's ID() and a non-empty string literal,
//   - a switch on ID(), or a case listing a provider ID constant,
//   - a provider ID built from a string literal outside the file that declares the set.
//
// Using an ID as a map key, stamping it as provenance, or passing it to the registry is fine.
// The gate reads source, which the test cache tracks, so a new violation is never a cached pass.

const idDeclFile = "metadataprovider.go"

func TestNoCodeComparesProviderIDs(t *testing.T) {
	root, module := moduleRoot(t)
	pkgPath := module + "/internal/metadataprovider"
	consts := idConstants(t)
	if len(consts) == 0 {
		t.Fatal("found no provider ID constants; the gate would check nothing")
	}
	scanned := 0
	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if name := d.Name(); name == "testdata" || name == "node_modules" || strings.HasPrefix(name, ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			inRoot := filepath.ToSlash(filepath.Dir(rel)) == "internal/metadataprovider"
			fset := token.NewFileSet()
			head, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			names := importNames(head, pkgPath)
			if !inRoot && len(names) == 0 {
				return nil
			}
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			scanned++
			allowLiteral := inRoot && filepath.Base(rel) == idDeclFile
			for _, finding := range idComparisons(fset, file, names, consts, inRoot, allowLiteral) {
				t.Errorf("%s: %s — express the rule over capabilities (decision 16)", rel, finding)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if scanned < 3 {
		t.Fatalf("scanned %d files that name metadataprovider; the gate is reading nothing", scanned)
	}
}

func TestGuardCatchesProviderIDComparisons(t *testing.T) {
	src := `package x
import mp "example/internal/metadataprovider"
func f(p mp.Provider, s mp.Source) {
	if p.ID() == "sonarr" {}
	if s.Provider != mp.IDRadarr {}
	if mp.IDNFO == s.Provider {}
	if string(p.ID()) == "nfo" {}
	switch p.ID() {}
	switch s.Provider { case mp.IDSonarr: }
	_ = mp.ID("radarr")
}
func clean(p mp.Provider, m map[mp.ID]int, s mp.Source) {
	_ = m[mp.IDSonarr]
	s.Provider = mp.IDNFO
	if p.ID() == "" {}
	if s.Basis == "manager" {}
	if s.Catalog == mp.NamespaceTVDB {}
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "x.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	names := importNames(file, "example/internal/metadataprovider")
	consts := map[string]bool{"IDSonarr": true, "IDRadarr": true, "IDNFO": true}
	got := idComparisons(fset, file, names, consts, false, false)
	if len(got) != 7 {
		t.Fatalf("found %d violations, want 7 (and none in clean):\n%s", len(got), strings.Join(got, "\n"))
	}
	for _, finding := range got {
		if line, _ := strconv.Atoi(strings.SplitN(finding, ":", 2)[0]); line >= 12 {
			t.Errorf("flagged allowed code: %s", finding)
		}
	}
}

// TestContractImportsNoLoomarrPackage keeps the contract a leaf, so the importer, the API and
// every provider can name it without a cycle. The precedence rules live in a subpackage because
// they reuse the audience ladder.
func TestContractImportsNoLoomarrPackage(t *testing.T) {
	_, module := moduleRoot(t)
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	files := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		files++
		f, err := parser.ParseFile(token.NewFileSet(), e.Name(), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range f.Imports {
			if path, _ := strconv.Unquote(spec.Path.Value); strings.HasPrefix(path, module+"/") {
				t.Errorf("%s imports %s; the contract imports nothing from Loomarr", e.Name(), path)
			}
		}
	}
	if files == 0 {
		t.Fatal("no production files read")
	}
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

// idConstants returns the names of the constants of type ID this package declares.
func idConstants(t *testing.T) map[string]bool {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), idDeclFile, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs := spec.(*ast.ValueSpec)
			if typ, ok := vs.Type.(*ast.Ident); ok && typ.Name == "ID" {
				for _, n := range vs.Names {
					out[n.Name] = true
				}
			}
		}
	}
	return out
}

// importNames returns the names a file uses for the contract package.
func importNames(f *ast.File, pkgPath string) map[string]bool {
	names := map[string]bool{}
	for _, spec := range f.Imports {
		path, _ := strconv.Unquote(spec.Path.Value)
		if path != pkgPath {
			continue
		}
		name := "metadataprovider"
		if spec.Name != nil {
			name = spec.Name.Name
		}
		names[name] = true
	}
	return names
}

// idComparisons returns each violation in f as "line:col: what".
func idComparisons(fset *token.FileSet, f *ast.File, names, consts map[string]bool, inRoot, allowLiteral bool) []string {
	isConst := func(e ast.Expr) bool {
		switch x := e.(type) {
		case *ast.SelectorExpr:
			pkg, ok := x.X.(*ast.Ident)
			return ok && names[pkg.Name] && consts[x.Sel.Name]
		case *ast.Ident:
			return inRoot && consts[x.Name]
		}
		return false
	}
	isIDType := func(e ast.Expr) bool {
		switch x := e.(type) {
		case *ast.SelectorExpr:
			pkg, ok := x.X.(*ast.Ident)
			return ok && names[pkg.Name] && x.Sel.Name == "ID"
		case *ast.Ident:
			return inRoot && x.Name == "ID"
		}
		return false
	}
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
	isIDCall := func(e ast.Expr) bool {
		call, ok := unwrap(e).(*ast.CallExpr)
		if !ok || len(call.Args) != 0 {
			return false
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		return ok && sel.Sel.Name == "ID"
	}
	isLiteral := func(e ast.Expr) bool {
		lit, ok := unwrap(e).(*ast.BasicLit)
		return ok && lit.Kind == token.STRING && lit.Value != `""` && lit.Value != "``"
	}

	var out []string
	report := func(n ast.Node, what string) {
		pos := fset.Position(n.Pos())
		out = append(out, fmt.Sprintf("%d:%d: %s", pos.Line, pos.Column, what))
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.BinaryExpr:
			if x.Op != token.EQL && x.Op != token.NEQ {
				return true
			}
			l, r := unwrap(x.X), unwrap(x.Y)
			switch {
			case isConst(l) || isConst(r):
				report(x, "compares with a provider ID constant")
			case (isIDCall(l) && isLiteral(r)) || (isIDCall(r) && isLiteral(l)):
				report(x, "compares a provider's ID() with a string")
			}
		case *ast.SwitchStmt:
			if x.Tag != nil && isIDCall(x.Tag) {
				report(x, "switches on a provider's ID()")
			}
			for _, stmt := range x.Body.List {
				clause := stmt.(*ast.CaseClause)
				if slices.ContainsFunc(clause.List, isConst) {
					report(clause, "switches on a provider ID constant")
				}
			}
		case *ast.CallExpr:
			if !allowLiteral && isIDType(x.Fun) && len(x.Args) == 1 && isLiteral(x.Args[0]) {
				report(x, "builds a provider ID from a string")
			}
		}
		return true
	})
	return out
}
