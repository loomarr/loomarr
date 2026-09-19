package api_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const apiTestServerOwner = "harness_test.go"

func TestAPITestServersUseTheSharedHarness(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var violations []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		source, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		violations = append(violations, apiTestServerViolations(entry.Name(), source)...)
	}
	if len(violations) != 0 {
		t.Fatalf("API tests must create full HTTP servers only through %s:\n%s",
			apiTestServerOwner, strings.Join(violations, "\n"))
	}
}

func TestAPITestServerArchitectureRule(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		source   string
		want     string
	}{
		{
			name: "allows central lifecycle owner", filename: apiTestServerOwner,
			source: `package api_test
import "net/http/httptest"
func startHarness() *routeHarness { httptest.NewServer(nil); return nil }
type routeHarness struct{}`,
		},
		{
			name: "allows focused harness constructor", filename: "route_test.go",
			source: `package api_test
func newRouteHarness() *routeHarness { return nil }
type routeHarness struct{}`,
		},
		{
			name: "rejects inline server", filename: "route_test.go",
			source: `package api_test
import "net/http/httptest"
func TestRoute() { httptest.NewServer(nil) }`,
			want: "route_test.go:3: full server creation belongs in harness_test.go",
		},
		{
			name: "rejects aliased TLS server", filename: "route_test.go",
			source: `package api_test
import testhttp "net/http/httptest"
func TestRoute() { testhttp.NewTLSServer(nil) }`,
			want: "route_test.go:3: full server creation belongs in harness_test.go",
		},
		{
			name: "rejects raw server constructor", filename: apiTestServerOwner,
			source: `package api_test
import "net/http/httptest"
func newServer() *httptest.Server { return nil }`,
			want: "harness_test.go:3: newServer returns raw *httptest.Server",
		},
		{
			name: "rejects raw handler constructor", filename: "route_test.go",
			source: `package api_test
import "net/http"
func newHandler() http.Handler { return nil }`,
			want: "route_test.go:3: newHandler returns raw http.Handler",
		},
		{
			name: "rejects opaque dot import", filename: "route_test.go",
			source: `package api_test
import . "net/http/httptest"
func TestRoute() { NewServer(nil) }`,
			want: `route_test.go:2: dot import of "net/http/httptest" bypasses the API test-server architecture rule`,
		},
		{
			name: "rejects internal package server", filename: "route_test.go",
			source: `package api
import "net/http/httptest"
func TestRoute() { httptest.NewServer(nil) }`,
			want: "route_test.go:3: full server creation belongs in harness_test.go",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strings.Join(apiTestServerViolations(tt.filename, []byte(tt.source)), "\n")
			if got != tt.want {
				t.Fatalf("violations = %q, want %q", got, tt.want)
			}
		})
	}
}

func apiTestServerViolations(filename string, source []byte) []string {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, source, 0)
	if err != nil {
		return []string{filename + ": parse: " + err.Error()}
	}
	httpNames := make(map[string]bool)
	httptestNames := make(map[string]bool)
	var violations []string
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		if path != "net/http" && path != "net/http/httptest" {
			continue
		}
		name := strings.TrimPrefix(path, "net/http/")
		if path == "net/http" {
			name = "http"
		}
		if spec.Name != nil {
			name = spec.Name.Name
		}
		if name == "." {
			position := filename + ":" + strconv.Itoa(fset.Position(spec.Pos()).Line)
			violations = append(violations, position+`: dot import of "`+path+`" bypasses the API test-server architecture rule`)
			continue
		}
		if path == "net/http" {
			httpNames[name] = true
		} else {
			httptestNames[name] = true
		}
	}

	if filename != apiTestServerOwner {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			qualifier, ok := selector.X.(*ast.Ident)
			if !ok || !httptestNames[qualifier.Name] || !isHTTPTestServerConstructor(selector.Sel.Name) {
				return true
			}
			position := filename + ":" + strconv.Itoa(fset.Position(call.Pos()).Line)
			violations = append(violations, position+": full server creation belongs in "+apiTestServerOwner)
			return true
		})
	}

	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Type.Results == nil {
			continue
		}
		for _, result := range function.Type.Results.List {
			if rawType := rawAPITestServerResult(result.Type, httpNames, httptestNames); rawType != "" {
				position := filename + ":" + strconv.Itoa(fset.Position(function.Pos()).Line)
				violations = append(violations, position+": "+function.Name.Name+" returns raw "+rawType)
			}
		}
	}

	sort.Strings(violations)
	return violations
}

func isHTTPTestServerConstructor(name string) bool {
	switch name {
	case "NewServer", "NewTLSServer", "NewUnstartedServer":
		return true
	default:
		return false
	}
}

func rawAPITestServerResult(result ast.Expr, httpNames, httptestNames map[string]bool) string {
	rawType := ""
	ast.Inspect(result, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		qualifier, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		switch {
		case httptestNames[qualifier.Name] && selector.Sel.Name == "Server":
			rawType = "*httptest.Server"
		case httpNames[qualifier.Name] && selector.Sel.Name == "Handler":
			rawType = "http.Handler"
		}
		return rawType == ""
	})
	return rawType
}
