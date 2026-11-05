package internal_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// A test asserts on behaviour, never on the text of Go or TypeScript source. A test that reads a
// source file and matches its contents with strings.Contains or a regexp passes while the behaviour
// it names is broken (the text is still there, the call path is not) and fails on harmless
// refactors (a rename, a reflow, a moved helper). Exercise the behaviour, or, where the intent is
// structural, parse the source with go/parser and check the syntax tree.
//
// The guard parses every Go test file in this module (every `_test.go` outside dot-dirs,
// node_modules, testdata, web/, root build output and nested modules, regardless of build tags) and
// flags a file that reads a source file as text. The detection is syntactic:
//
//   - A read is a call to os.ReadFile, os.Open, os.OpenFile, ioutil.ReadFile or fs.ReadFile.
//   - A read is a source read when its path expression names source: a string literal ending in a
//     source extension (sourceExtensions) reaches it directly, through filepath.Join or any other
//     call or concatenation, through a local or package-level value, a range over a table, or a
//     helper's parameter at one of its call sites in the file.
//   - A read whose path the guard cannot resolve is a source read when its enclosing top-level
//     function also selects files by a source extension (strings.HasSuffix, filepath.Glob,
//     filepath.Match, or a filepath.Ext comparison): that is a walk over the source tree.
//   - A path rooted in t.TempDir(), os.TempDir() or os.MkdirTemp, or naming a testdata directory,
//     is test output or a fixture, never repo source.
//   - A read assigned to a name the same function hands to a go/parser Parse* call is syntax-tree
//     input, not text under assertion, and is not flagged. An AST guard that lets go/parser open
//     the file (ParseFile with a nil src) never reads it as text at all.
//   - A `//go:embed` directive naming a source file is a source read.
//
// The guard is a syntactic net, not a proof: a path that reaches a read through a predicate helper
// (a walk that keeps files for which isFixture(name) holds) is invisible to it. The web half of
// this rule is web/scripts/source-text-tests.mjs, run by `pnpm imports:check`.

// sourceExtensions are the file extensions of Go and TypeScript/JavaScript source. A `.ts` file
// written under t.TempDir() is an MPEG-TS segment, which the temp-root rule already excludes.
var sourceExtensions = []string{".go", ".ts", ".tsx", ".mts", ".cts", ".js", ".jsx", ".mjs", ".cjs"}

// sourceTextTestAllowlist maps a repo-relative test file to why it still reads source as text. It
// is a ratchet: an entry that no longer reads source fails the guard, and a new entry needs a reason.
var sourceTextTestAllowlist = map[string]string{}

func TestNoTestAssertsOnSourceText(t *testing.T) {
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
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, filepath.Join(root, rel), nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", rel, err)
		}
		reads := sourceTextReads(fset, file)
		if len(reads) == 0 {
			continue
		}
		violating[rel] = true
		if _, ok := sourceTextTestAllowlist[rel]; ok {
			continue
		}
		for _, r := range reads {
			t.Errorf("%s:%d %s. Tests assert on behaviour, not on source text: exercise the code, or "+
				"check structure with go/parser (AGENTS.md, Tests).", rel, r.Line, r.Reason)
		}
	}

	stale := []string{}
	for rel, reason := range sourceTextTestAllowlist {
		if strings.TrimSpace(reason) == "" {
			t.Errorf("allowlist entry %s has no reason", rel)
		}
		if !violating[rel] {
			stale = append(stale, rel)
		}
	}
	sort.Strings(stale)
	for _, rel := range stale {
		t.Errorf("%s no longer reads source as text; remove it from sourceTextTestAllowlist", rel)
	}
}

func TestSourceTextDetection(t *testing.T) {
	trips := map[string]string{
		"literal path": `package x
import ("os"; "strings"; "testing")
func TestX(t *testing.T) {
	b, _ := os.ReadFile("handler.go")
	if !strings.Contains(string(b), "Register(") { t.Fatal() }
}`,
		"joined path through a local": `package x
import ("os"; "path/filepath"; "testing")
func TestX(t *testing.T) {
	p := filepath.Join("..", "..", "web", "src", "page.tsx")
	_, _ = os.ReadFile(p)
}`,
		"package-level path": `package x
import ("os"; "testing")
const target = "internal/api/api.go"
func TestX(t *testing.T) { _, _ = os.Open(target) }`,
		"helper parameter": `package x
import ("os"; "testing")
func read(t *testing.T, path string) string { b, _ := os.ReadFile(path); return string(b) }
func TestX(t *testing.T) { _ = read(t, "export.go") }`,
		"closure parameter": `package x
import ("os"; "testing")
func TestX(t *testing.T) {
	read := func(name string) []byte { b, _ := os.ReadFile(name); return b }
	_ = read("api.go")
}`,
		"table": `package x
import ("os"; "testing")
func TestX(t *testing.T) {
	for _, name := range []string{"a.go", "b.go"} { _, _ = os.ReadFile(name) }
}`,
		"walk over source": `package x
import ("io/fs"; "os"; "path/filepath"; "strings"; "testing")
func TestX(t *testing.T) {
	_ = filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if !strings.HasSuffix(path, ".go") { return nil }
		b, _ := os.ReadFile(path)
		_ = strings.Contains(string(b), "PoolOf(")
		return nil
	})
}`,
		"embedded source": `package x
import _ "embed"
//go:embed main.go
var src string`,
	}
	passes := map[string]string{
		"non-source artefact": `package x
import ("os"; "path/filepath"; "testing")
func TestX(t *testing.T) { _, _ = os.ReadFile(filepath.Join("..", "docs", "design", "package-map.md")) }`,
		"temp output": `package x
import ("os"; "path/filepath"; "testing")
func TestX(t *testing.T) { _, _ = os.Open(filepath.Join(t.TempDir(), "seg.ts")) }`,
		"testdata fixture": `package x
import ("os"; "path/filepath"; "testing")
func TestX(t *testing.T) { _, _ = os.ReadFile(filepath.Join("testdata", "input.go")) }`,
		"ast guard": `package x
import ("go/parser"; "go/token"; "os"; "path/filepath"; "strings"; "testing")
func TestX(t *testing.T) {
	names, _ := filepath.Glob("*.go")
	for _, n := range names {
		if strings.HasSuffix(n, "_test.go") { continue }
		src, _ := os.ReadFile(n)
		_, _ = parser.ParseFile(token.NewFileSet(), n, src, 0)
	}
}`,
		"writes source fixtures, reads generated output": `package x
import ("os"; "path/filepath"; "testing")
func write(t *testing.T, root, rel, src string) { _ = os.WriteFile(filepath.Join(root, rel), []byte(src), 0o644) }
func TestX(t *testing.T) {
	root := t.TempDir()
	write(t, root, "top/top.go", "package top")
	_, _ = os.ReadFile(filepath.Join(root, "doc.md"))
}`,
		"walk that only lists names": `package x
import ("os"; "strings"; "testing")
func TestX(t *testing.T) {
	entries, _ := os.ReadDir(".")
	for _, e := range entries { _ = strings.HasSuffix(e.Name(), ".go") }
}`,
	}
	scan := func(name, src string) []sourceRead {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, name+"_test.go", src, parser.ParseComments)
		if err != nil {
			t.Fatalf("%s: parse: %v", name, err)
		}
		return sourceTextReads(fset, file)
	}
	for name, src := range trips {
		if len(scan(name, src)) == 0 {
			t.Errorf("%s: not flagged, want a source read", name)
		}
	}
	for name, src := range passes {
		if got := scan(name, src); len(got) != 0 {
			t.Errorf("%s: flagged %+v, want none", name, got)
		}
	}
}

// moduleTestFiles lists every `_test.go` file of this module, repo-relative and slash-separated.
func moduleTestFiles(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == root {
				return nil
			}
			if strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			switch d.Name() {
			case "node_modules", "testdata", "web":
				return filepath.SkipDir
			}
			rel, rerr := filepath.Rel(root, path)
			if rerr != nil {
				return rerr
			}
			if rootBuildOutput[filepath.ToSlash(rel)] {
				return filepath.SkipDir
			}
			if _, serr := os.Stat(filepath.Join(path, "go.mod")); serr == nil {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(out)
	return out, err
}

// sourceRead is one place a test file reads source as text.
type sourceRead struct {
	Line   int
	Reason string
}

// pathKind classifies a path expression. A higher kind wins when parts combine.
type pathKind int

const (
	pathUnknown   pathKind = iota // nothing in the expression says what it names
	pathArtefact                  // names a non-source file (a doc, YAML, JSON, a Makefile)
	pathSource                    // names a Go or TypeScript source file
	pathGenerated                 // rooted in a temp dir or a testdata fixture
)

type sourceTextScan struct {
	file    *ast.File
	pkgVals map[string][]ast.Expr
	locals  map[*ast.FuncDecl]map[string][]ast.Expr
}

func sourceTextReads(fset *token.FileSet, file *ast.File) []sourceRead {
	s := &sourceTextScan{file: file, pkgVals: map[string][]ast.Expr{}, locals: map[*ast.FuncDecl]map[string][]ast.Expr{}}
	for _, d := range file.Decls {
		switch d := d.(type) {
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				if vs, ok := spec.(*ast.ValueSpec); ok {
					addValueSpec(s.pkgVals, vs)
				}
			}
		}
	}

	var out []sourceRead
	for _, group := range file.Comments {
		for _, c := range group.List {
			patterns, ok := strings.CutPrefix(c.Text, "//go:embed ")
			if !ok {
				continue
			}
			for _, p := range strings.Fields(patterns) {
				if literalKind(strings.Trim(p, "\"`")) == pathSource {
					out = append(out, sourceRead{fset.Position(c.Pos()).Line, "embeds source " + p})
				}
			}
		}
	}
	for _, d := range file.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		selects := selectsSourceFiles(fn)
		parsed := parsedReads(fn)
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || parsed[call] {
				return true
			}
			arg, ok := readPathArg(call)
			if !ok {
				return true
			}
			switch s.kind(arg, fn, 0) {
			case pathSource:
				out = append(out, sourceRead{fset.Position(call.Pos()).Line, "reads a source file as text"})
			case pathUnknown:
				if selects {
					out = append(out, sourceRead{fset.Position(call.Pos()).Line,
						"reads a file its function selected by a source extension"})
				}
			}
			return true
		})
	}
	return out
}

func addValueSpec(into map[string][]ast.Expr, vs *ast.ValueSpec) {
	for i, name := range vs.Names {
		switch {
		case len(vs.Values) == len(vs.Names):
			into[name.Name] = append(into[name.Name], vs.Values[i])
		case len(vs.Values) == 1:
			into[name.Name] = append(into[name.Name], vs.Values[0])
		}
	}
}

// readPathArg returns the path argument of a call that reads a file's contents.
func readPathArg(call *ast.CallExpr) (ast.Expr, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil, false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return nil, false
	}
	index := -1
	switch pkg.Name + "." + sel.Sel.Name {
	case "os.ReadFile", "os.Open", "os.OpenFile", "ioutil.ReadFile":
		index = 0
	case "fs.ReadFile":
		index = 1
	}
	if index < 0 || len(call.Args) <= index {
		return nil, false
	}
	return call.Args[index], true
}

// parsedReads returns the reads in fn whose result is handed to go/parser: syntax-tree input, not
// text under assertion. A read is "handed" when it is assigned to a name that fn passes (directly
// or through a conversion) to a parser.Parse* call.
func parsedReads(fn *ast.FuncDecl) map[*ast.CallExpr]bool {
	parsed := map[string]bool{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "parser" || !strings.HasPrefix(sel.Sel.Name, "Parse") {
			return true
		}
		for _, a := range call.Args {
			ast.Inspect(a, func(n ast.Node) bool {
				if id, ok := n.(*ast.Ident); ok {
					parsed[id.Name] = true
				}
				return true
			})
		}
		return true
	})
	out := map[*ast.CallExpr]bool{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok || len(as.Rhs) != 1 || len(as.Lhs) == 0 {
			return true
		}
		call, ok := as.Rhs[0].(*ast.CallExpr)
		if !ok {
			return true
		}
		if _, isRead := readPathArg(call); !isRead {
			return true
		}
		if id, ok := as.Lhs[0].(*ast.Ident); ok && parsed[id.Name] {
			out[call] = true
		}
		return true
	})
	return out
}

// selectsSourceFiles reports whether a function picks files by a source extension.
func selectsSourceFiles(fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CallExpr:
			sel, ok := n.Fun.(*ast.SelectorExpr)
			if !ok {
				break
			}
			switch sel.Sel.Name {
			case "HasSuffix", "Glob", "Match":
				for _, a := range n.Args {
					if lit, ok := a.(*ast.BasicLit); ok && stringLiteralKind(lit) == pathSource {
						found = true
					}
				}
			}
		case *ast.BinaryExpr:
			if n.Op != token.EQL && n.Op != token.NEQ {
				break
			}
			for _, side := range [][2]ast.Expr{{n.X, n.Y}, {n.Y, n.X}} {
				lit, ok := side[0].(*ast.BasicLit)
				call, isCall := side[1].(*ast.CallExpr)
				if !ok || !isCall || stringLiteralKind(lit) != pathSource {
					continue
				}
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Ext" {
					found = true
				}
			}
		}
		return !found
	})
	return found
}

func stringLiteralKind(lit *ast.BasicLit) pathKind {
	if lit.Kind != token.STRING {
		return pathUnknown
	}
	v, err := strconv.Unquote(lit.Value)
	if err != nil {
		return pathUnknown
	}
	return literalKind(v)
}

// literalKind classifies one string literal by the file it names.
func literalKind(v string) pathKind {
	for _, part := range strings.FieldsFunc(v, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == "testdata" {
			return pathGenerated
		}
	}
	for _, ext := range sourceExtensions {
		if strings.HasSuffix(v, ext) {
			return pathSource
		}
	}
	ext := filepath.Ext(v)
	if len(ext) < 2 || strings.ContainsAny(v, " \n\t") {
		return pathUnknown
	}
	for _, r := range ext[1:] {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return pathUnknown
		}
	}
	return pathArtefact
}

// kind classifies a path expression inside fn, following values, tables and parameters.
func (s *sourceTextScan) kind(e ast.Expr, fn *ast.FuncDecl, depth int) pathKind {
	if depth > 8 || e == nil {
		return pathUnknown
	}
	switch e := e.(type) {
	case *ast.BasicLit:
		return stringLiteralKind(e)
	case *ast.ParenExpr:
		return s.kind(e.X, fn, depth+1)
	case *ast.StarExpr:
		return s.kind(e.X, fn, depth+1)
	case *ast.UnaryExpr:
		return s.kind(e.X, fn, depth+1)
	case *ast.IndexExpr:
		return s.kind(e.X, fn, depth+1)
	case *ast.SliceExpr:
		return s.kind(e.X, fn, depth+1)
	case *ast.SelectorExpr:
		return s.kind(e.X, fn, depth+1)
	case *ast.KeyValueExpr:
		return max(s.kind(e.Key, fn, depth+1), s.kind(e.Value, fn, depth+1))
	case *ast.BinaryExpr:
		return max(s.kind(e.X, fn, depth+1), s.kind(e.Y, fn, depth+1))
	case *ast.CompositeLit:
		k := pathUnknown
		for _, el := range e.Elts {
			k = max(k, s.kind(el, fn, depth+1))
		}
		return k
	case *ast.CallExpr:
		if isTempDirCall(e) {
			return pathGenerated
		}
		k := pathUnknown
		for _, a := range e.Args {
			k = max(k, s.kind(a, fn, depth+1))
		}
		return k
	case *ast.Ident:
		return s.identKind(e, fn, depth)
	}
	return pathUnknown
}

func isTempDirCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && (sel.Sel.Name == "TempDir" || sel.Sel.Name == "MkdirTemp")
}

func (s *sourceTextScan) identKind(id *ast.Ident, fn *ast.FuncDecl, depth int) pathKind {
	// A parameter of the innermost enclosing function takes its value from that function's call
	// sites in this file.
	if params, index, name, ok := enclosingParam(fn, id); ok {
		k := pathUnknown
		for caller, args := range s.callSites(fn, params, name) {
			for _, call := range args {
				if index < len(call.Args) {
					k = max(k, s.kind(call.Args[index], caller, depth+1))
				}
			}
		}
		return k
	}
	locals, ok := s.locals[fn]
	if !ok {
		locals = localValues(fn)
		s.locals[fn] = locals
	}
	values := locals[id.Name]
	if len(values) == 0 {
		values = s.pkgVals[id.Name]
	}
	k := pathUnknown
	for _, v := range values {
		if v == ast.Expr(id) {
			continue
		}
		k = max(k, s.kind(v, fn, depth+1))
	}
	return k
}

// enclosingParam finds the innermost function in fn (fn itself or a func literal) that declares
// id as a parameter. It returns that function's type, the parameter index and, for a func
// literal bound to a name, that name ("" when the literal is passed inline).
func enclosingParam(fn *ast.FuncDecl, id *ast.Ident) (*ast.FuncType, int, string, bool) {
	var (
		bestType  *ast.FuncType
		bestIndex = -1
		bestName  string
		bestSpan  token.Pos = -1
	)
	consider := func(ft *ast.FuncType, body *ast.BlockStmt, name string) {
		if body == nil || id.Pos() < body.Pos() || id.Pos() > body.End() || ft.Params == nil {
			return
		}
		i := 0
		for _, field := range ft.Params.List {
			for _, n := range field.Names {
				if n.Name == id.Name {
					span := body.End() - body.Pos()
					if bestSpan < 0 || span < bestSpan {
						bestType, bestIndex, bestName, bestSpan = ft, i, name, span
					}
				}
				i++
			}
			if len(field.Names) == 0 {
				i++
			}
		}
	}
	name := fn.Name.Name
	consider(fn.Type, fn.Body, name)
	bound := map[*ast.FuncLit]string{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if as, ok := n.(*ast.AssignStmt); ok && len(as.Lhs) == len(as.Rhs) {
			for i, r := range as.Rhs {
				if lit, ok := r.(*ast.FuncLit); ok {
					if lhs, ok := as.Lhs[i].(*ast.Ident); ok {
						bound[lit] = lhs.Name
					}
				}
			}
		}
		if lit, ok := n.(*ast.FuncLit); ok {
			consider(lit.Type, lit.Body, bound[lit])
		}
		return true
	})
	if bestIndex < 0 {
		return nil, 0, "", false
	}
	if bestType != fn.Type && bestName == "" {
		// A func literal passed inline (a WalkDir callback, t.Run) has no call site to read.
		return bestType, bestIndex, "", true
	}
	return bestType, bestIndex, bestName, true
}

// callSites returns the calls of the function named name, grouped by the top-level function they
// appear in. A top-level function is called from anywhere in the file; a bound func literal only
// from inside fn.
func (s *sourceTextScan) callSites(fn *ast.FuncDecl, ft *ast.FuncType, name string) map[*ast.FuncDecl][]*ast.CallExpr {
	out := map[*ast.FuncDecl][]*ast.CallExpr{}
	if name == "" {
		return out
	}
	scope := []*ast.FuncDecl{fn}
	if ft == fn.Type {
		scope = scope[:0]
		for _, d := range s.file.Decls {
			if f, ok := d.(*ast.FuncDecl); ok && f.Body != nil {
				scope = append(scope, f)
			}
		}
	}
	for _, caller := range scope {
		ast.Inspect(caller.Body, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if callee, ok := call.Fun.(*ast.Ident); ok && callee.Name == name {
					out[caller] = append(out[caller], call)
				}
			}
			return true
		})
	}
	return out
}

// localValues maps each name assigned or declared in fn to the expressions it takes. A range
// variable takes the ranged expression, so a table of literal paths flows into its loop body.
func localValues(fn *ast.FuncDecl) map[string][]ast.Expr {
	out := map[string][]ast.Expr{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			for i, l := range n.Lhs {
				id, ok := l.(*ast.Ident)
				if !ok || id.Name == "_" {
					continue
				}
				switch {
				case len(n.Rhs) == len(n.Lhs):
					out[id.Name] = append(out[id.Name], n.Rhs[i])
				case len(n.Rhs) == 1:
					out[id.Name] = append(out[id.Name], n.Rhs[0])
				}
			}
		case *ast.DeclStmt:
			if gd, ok := n.Decl.(*ast.GenDecl); ok {
				for _, spec := range gd.Specs {
					if vs, ok := spec.(*ast.ValueSpec); ok {
						addValueSpec(out, vs)
					}
				}
			}
		case *ast.RangeStmt:
			for _, v := range []ast.Expr{n.Key, n.Value} {
				if id, ok := v.(*ast.Ident); ok && id.Name != "_" {
					out[id.Name] = append(out[id.Name], n.X)
				}
			}
		}
		return true
	})
	return out
}
