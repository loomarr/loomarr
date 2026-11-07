package httpx

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// A Target is the identity every outbound metric is labelled with. An untyped string literal
// converts to Target implicitly, so the compiler accepts `NewNamedObserved("notification-provider", …)`
// and the typo or drift it allows is invisible until a dashboard is empty. The guard walks every Go
// file in the module (tests included) and flags a string literal, or a Target("…") conversion of
// one, at the Target position of a NewNamed* constructor. It is syntactic: a call is recognised by
// the constructor's name, qualified by httpx or bare inside this package.

// targetConstructors maps each constructor that takes a Target to the argument index it sits at.
var targetConstructors = map[string]int{
	"NewNamed":               0,
	"NewNamedObserved":       0,
	"NewPublicNamedObserved": 0,
	"NewLLMNamed":            0,
}

// literalTargets returns "Constructor@line" for every call in src whose Target argument is a
// string literal or a Target(<literal>) conversion.
func literalTargets(t *testing.T, filename string, src any) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", filename, err)
	}
	var found []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := constructorName(call.Fun)
		index, ok := targetConstructors[name]
		if !ok || index >= len(call.Args) {
			return true
		}
		if isStringLiteralTarget(call.Args[index]) {
			found = append(found, name+"@"+strconv.Itoa(fset.Position(call.Pos()).Line))
		}
		return true
	})
	return found
}

func constructorName(fun ast.Expr) string {
	switch f := fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		if pkg, ok := f.X.(*ast.Ident); ok && pkg.Name == "httpx" {
			return f.Sel.Name
		}
	}
	return ""
}

func isStringLiteralTarget(arg ast.Expr) bool {
	switch a := arg.(type) {
	case *ast.BasicLit:
		return a.Kind == token.STRING
	case *ast.CallExpr:
		if len(a.Args) != 1 {
			return false
		}
		switch conv := a.Fun.(type) {
		case *ast.Ident:
			return conv.Name == "Target" && isStringLiteralTarget(a.Args[0])
		case *ast.SelectorExpr:
			return conv.Sel.Name == "Target" && isStringLiteralTarget(a.Args[0])
		}
	}
	return false
}

func TestNoStringLiteralPassedAsTarget(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "web", "vendor", "data", ".agent-data":
				return filepath.SkipDir
			}
			// A nested module (research/) is pinned to its own copy of the core and is not built against this httpx.
			if path != root {
				if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if _, err := os.Stat(path); err != nil {
			return err
		}
		if found := literalTargets(t, path, nil); len(found) > 0 {
			t.Errorf("%s passes a string literal as a Target (%s) — use an httpx.Target constant",
				filepath.ToSlash(path), strings.Join(found, ", "))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk module: %v", err)
	}
}

func TestLiteralTargetScanFlagsEveryForm(t *testing.T) {
	src := `package p

import "github.com/loomarr/loomarr/internal/platform/httpx"

func a() { httpx.NewNamed("x", 0) }
func b() { httpx.NewNamedObserved("x", 0, nil) }
func c() { httpx.NewPublicNamedObserved("x", 0, nil) }
func d() { httpx.NewLLMNamed("x", 0, nil) }
func e() { httpx.NewNamed(httpx.Target("x"), 0) }
func f() { httpx.NewNamed(httpx.TargetTMDB, 0) }
func g() { httpx.NewNamed(target, 0) }
func h() { other.NewNamed("x", 0) }
func i() { NewNamed("x", 0) }
`
	got := strings.Join(literalTargets(t, "p.go", src), " ")
	want := "NewNamed@5 NewNamedObserved@6 NewPublicNamedObserved@7 NewLLMNamed@8 NewNamed@9 NewNamed@13"
	if got != want {
		t.Fatalf("flagged %q, want %q", got, want)
	}
}
