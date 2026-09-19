package api_test

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// testServerConstructors classifies the deliberately retained route-family
// harnesses while their common lifecycle is migrated behind apiHarness. Every
// entry needs a route-family rationale, and stale entries fail the test.
var testServerConstructors = map[string]string{
	"dashboard_panels_test.go:serverWithPanels":      "dashboard panels",
	"dashboard_test.go:newDashboardServer":           "dashboard playout",
	"events_test.go:newEventsServer":                 "events",
	"fillersources_test.go:serverWithClips":          "filler sources",
	"help_test.go:newHelpServer":                     "help",
	"icons_test.go:newIconsHandlerWithConfig":        "icon handler configuration",
	"icons_test.go:newIconsServer":                   "icons",
	"icons_test.go:newIconsServerWithConfig":         "icon configuration",
	"jobs_test.go:serverWithJobs":                    "jobs",
	"pods_test.go:newPodsServer":                     "filler pods",
	"proposaljourneys_test.go:proposalJourneyServer": "proposal journeys",
	"search_test.go:newConfiguredSearchHandler":      "search",
}

func TestAPITestServerConstructorsStayClassified(t *testing.T) {
	t.Helper()
	constructors := apiTestServerConstructors(t)
	seen := make(map[string]bool, len(constructors))
	for _, constructor := range constructors {
		seen[constructor] = true
		if _, ok := testServerConstructors[constructor]; !ok {
			t.Errorf("unclassified API test-server constructor %q", constructor)
		}
	}
	for constructor, family := range testServerConstructors {
		if family == "" {
			t.Errorf("API test-server constructor %q has no route-family classification", constructor)
		}
		if !seen[constructor] {
			t.Errorf("obsolete API test-server constructor classification %q", constructor)
		}
	}
}

func apiTestServerConstructors(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var constructors []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, entry.Name(), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", entry.Name(), err)
		}
		if file.Name.Name != "api_test" {
			continue
		}
		for _, declaration := range file.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || strings.HasPrefix(fn.Name.Name, "Test") || fn.Type.Results == nil {
				continue
			}
			var result bytes.Buffer
			for _, field := range fn.Type.Results.List {
				if err := printer.Fprint(&result, fset, field.Type); err != nil {
					t.Fatalf("print results for %s: %v", fn.Name.Name, err)
				}
			}
			resultType := result.String()
			if !strings.Contains(resultType, "httptest.Server") && !strings.Contains(resultType, "http.Handler") {
				continue
			}
			constructors = append(constructors, filepath.ToSlash(entry.Name())+":"+fn.Name.Name)
		}
	}
	sort.Strings(constructors)
	return constructors
}
