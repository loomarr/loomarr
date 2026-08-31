package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOfflineInspectionEntrypointHasNoCredentialOrProviderCapabilities(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var offline *ast.FuncDecl
	var dispatch *ast.FuncDecl
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && function.Name.Name == "runOfflineInspection" {
				offline = function
			}
			if ok && function.Name.Name == "run" {
				dispatch = function
			}
		}
	}
	if offline == nil {
		t.Fatal("offline inspection must have a separate capability-free entrypoint")
	}
	if offline.Type.Params.NumFields() != 3 {
		t.Fatalf("runOfflineInspection has %d parameter fields, want args plus stdout/stderr only", offline.Type.Params.NumFields())
	}
	forbidden := map[string]bool{
		"APIKey": true, "context": true, "getenv": true, "http": true, "os": true,
		"paidRunCapabilities": true, "runCapabilities": true, "runOpenRouterReview": true,
		"RunOpenRouterReview": true,
	}
	ast.Inspect(offline, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if ok && forbidden[identifier.Name] {
			t.Errorf("offline inspection entrypoint has forbidden capability identifier %q", identifier.Name)
		}
		return true
	})
	if dispatch == nil || len(dispatch.Body.List) != 2 {
		t.Fatal("run must dispatch offline inspection before constructing paid capabilities")
	}
	ifStatement, ok := dispatch.Body.List[0].(*ast.IfStmt)
	if !ok || !returnsCall(ifStatement.Body, "runOfflineInspection") {
		t.Fatal("run must return from the offline entrypoint before its paid branch")
	}
	paidReturn, ok := dispatch.Body.List[1].(*ast.ReturnStmt)
	if !ok || len(paidReturn.Results) != 1 || calledFunction(paidReturn.Results[0]) != "runPaidReviewOrRecovery" {
		t.Fatal("paid capabilities must be confined to the post-dispatch paid entrypoint")
	}
}

func returnsCall(body *ast.BlockStmt, name string) bool {
	if body == nil || len(body.List) != 1 {
		return false
	}
	statement, ok := body.List[0].(*ast.ReturnStmt)
	return ok && len(statement.Results) == 1 && calledFunction(statement.Results[0]) == name
}

func calledFunction(expression ast.Expr) string {
	call, ok := expression.(*ast.CallExpr)
	if !ok {
		return ""
	}
	identifier, _ := call.Fun.(*ast.Ident)
	if identifier == nil {
		return ""
	}
	return identifier.Name
}

func TestCommandPackageDocumentsPaidReviewAndOfflineInspection(t *testing.T) {
	raw, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	comment := string(raw[:min(len(raw), 512)])
	if !strings.Contains(comment, "paid") || !strings.Contains(comment, "offline") {
		t.Fatalf("command package comment must document paid review and offline inspection: %q", comment)
	}
}
