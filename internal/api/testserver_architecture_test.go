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
	"auth_flow_test.go:authServer":                        "authentication flow",
	"channelicon_test.go:newIconUploadServer":             "channel icons",
	"channelnumber_test.go:newServerWithNumbers":          "channel numbering",
	"channels_test.go:newInternalServerWithoutTunarr":     "channels internal backend",
	"channels_test.go:newServerWithSchedulerAndSuggest":   "channels suggestions",
	"channeltimeline_test.go:newTimelineServer":           "channel timeline",
	"channeltimeline_test.go:newTimelineServerWithImages": "channel timeline images",
	"dashboard_panels_test.go:serverWithPanels":           "dashboard panels",
	"dashboard_test.go:newDashboardServer":                "dashboard playout",
	"device_flow_test.go:deviceServer":                    "device authentication",
	"devlogin_test.go:devLoginServer":                     "development login",
	"devlogin_test.go:gatedServer":                        "development route gates",
	"events_test.go:newEventsServer":                      "events",
	"filler_test.go:newFillerServer":                      "filler",
	"filler_test.go:newFillerServerWithConfig":            "filler configuration",
	"filler_test.go:newFillerServerWithImages":            "filler images",
	"filler_test.go:newFillerServerWithIncomingConfig":    "filler incoming configuration",
	"filler_test.go:newFillerServerWithRuntimeConfig":     "filler runtime configuration",
	"fillermedia_test.go:newMediaServer":                  "filler media",
	"fillersources_test.go:serverWithClips":               "filler sources",
	"fillerwatch_test.go:newFillerWatchServer":            "filler watch",
	"help_test.go:newHelpServer":                          "help",
	"icons_test.go:newIconsHandlerWithConfig":             "icon handler configuration",
	"icons_test.go:newIconsServer":                        "icons",
	"icons_test.go:newIconsServerWithConfig":              "icon configuration",
	"invitations_test.go:invitationServer":                "invitations",
	"invitations_test.go:invitationServerWithDelivery":    "invitation delivery",
	"jobs_test.go:serverWithJobs":                         "jobs",
	"locations_test.go:locationServer":                    "installation location",
	"passwordrecovery_test.go:passwordRecoveryServer":     "password recovery",
	"pods_test.go:newPodsServer":                          "filler pods",
	"proposaljourneys_test.go:proposalJourneyServer":      "proposal journeys",
	"provisioning_test.go:provServer":                     "provisioning",
	"provisioning_test.go:provServerWithUsers":            "provisioning users",
	"search_test.go:newConfiguredSearchHandler":           "search",
	"sessions_test.go:newSessionsServer":                  "sessions",
	"settings_test.go:newAutoWireServer":                  "settings connector wiring",
	"settings_test.go:newAutoWireServerWithChannels":      "settings channel wiring",
	"settings_test.go:newSettingsServer":                  "settings",
	"ssoroutes_test.go:serverWithSSO":                     "single sign-on",
	"system_backups_test.go:serverWithBackups":            "system backups",
	"system_database_test.go:serverWithDatabase":          "system database",
	"system_llm_test.go:serverWithSystemLLM":              "system language model",
	"system_llm_test.go:serverWithSystemLLMStore":         "system language model storage",
	"system_restart_test.go:serverWithRestart":            "system restart",
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
