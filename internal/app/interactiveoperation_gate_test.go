package app

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// Request-launched work must enter the generation-owned launcher. A raw goroutine or background
// context here recreates the restart race while leaving every ordinary success-path test green.
func TestInteractiveOperationAdaptersCannotEscapeApplicationLifecycle(t *testing.T) {
	targets := map[string]map[string]bool{
		"filler.go":    {"fillerServiceAdapter.ingest": false, "fillerServiceAdapter.Split": false},
		"systemllm.go": {"systemLLMService.Pull": false},
	}
	fset := token.NewFileSet()
	for path, wanted := range targets {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range file.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}
			receiver := receiverName(fn.Recv.List[0].Type)
			name := receiver + "." + fn.Name.Name
			if _, ok := wanted[name]; !ok {
				continue
			}
			wanted[name] = true
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				switch n := node.(type) {
				case *ast.GoStmt:
					t.Errorf("%s launches a raw goroutine at %s", name, fset.Position(n.Pos()))
				case *ast.CallExpr:
					selector, ok := n.Fun.(*ast.SelectorExpr)
					pkg, pkgOK := selectorPackage(selector)
					if ok && pkgOK && pkg == "context" && selector.Sel.Name == "Background" {
						t.Errorf("%s detaches with context.Background at %s", name, fset.Position(n.Pos()))
					}
				}
				return true
			})
		}
		for name, found := range wanted {
			if !found {
				t.Errorf("lifecycle gate target %s disappeared; update the gate with its replacement", name)
			}
		}
	}
}

func receiverName(expr ast.Expr) string {
	switch receiver := expr.(type) {
	case *ast.Ident:
		return receiver.Name
	case *ast.StarExpr:
		if ident, ok := receiver.X.(*ast.Ident); ok {
			return ident.Name
		}
	}
	return ""
}

func selectorPackage(selector *ast.SelectorExpr) (string, bool) {
	if selector == nil {
		return "", false
	}
	ident, ok := selector.X.(*ast.Ident)
	if !ok {
		return "", false
	}
	return ident.Name, true
}
