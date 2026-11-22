package agents

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/diagnostics"
)

type recordedEvents struct{ events []diagnostics.Event }

func (r *recordedEvents) Record(_ context.Context, e diagnostics.Event) {
	r.events = append(r.events, e)
}

func codeModeHarness(t *testing.T) (*Handlers, *toolset, *recordedEvents) {
	t.Helper()
	rec := &recordedEvents{}
	h := New(Deps{Events: rec})
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/secret" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"title":"Forbidden"}`))
			return
		}
		_, _ = fmt.Fprintf(w, `{"password":"hunter2","path":%q}`, r.URL.Path)
	})
	return h, &toolset{d: NewDispatcher(api)}, rec
}

// Each run is audited: who, which device, how long, and every request's method, path and status,
// but never a body, a reply or the script itself.
func TestExecuteIsAudited(t *testing.T) {
	h, ts, rec := codeModeHarness(t)
	out, err := h.executeScript(context.Background(), ts, "u-boss", "dev-1", executeIn{Code: `
const a = await loomarr.request("POST", "/v1/things", { note: "private-body" });
const b = await loomarr.request("GET", "/v1/secret");
return [a.status, b.status];`})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Requests) != 2 || out.Requests[1] != "GET /v1/secret → 403" {
		t.Fatalf("requests = %v", out.Requests)
	}
	if len(rec.events) != 1 {
		t.Fatalf("%d audit events, want 1", len(rec.events))
	}
	e := rec.events[0]
	calls, _ := e.Attributes["requests"].([]string)
	if e.Name != "code_mode.execute" || e.ActorID != "u-boss" || e.Attributes["device"] != "dev-1" ||
		len(calls) != 2 || calls[0] != "POST /v1/things 200" || calls[1] != "GET /v1/secret 403" {
		t.Fatalf("audit event = %+v", e)
	}
	if all := fmt.Sprintf("%+v", e); strings.Contains(all, "private-body") || strings.Contains(all, "hunter2") || strings.Contains(all, "loomarr.request") {
		t.Fatalf("the audit carries a body, a reply or the script: %s", all)
	}
}

// A query can carry values that do not belong in a log; the audit keeps the path alone.
func TestAuditLeavesOutTheQuery(t *testing.T) {
	h, ts, rec := codeModeHarness(t)
	if _, err := h.executeScript(context.Background(), ts, "u", "d", executeIn{Code: `await loomarr.request("GET", "/v1/things?key=sekrit")`}); err != nil {
		t.Fatal(err)
	}
	if all := fmt.Sprintf("%+v", rec.events); strings.Contains(all, "sekrit") || !strings.Contains(all, "GET /v1/things 200") {
		t.Fatalf("audit = %s", all)
	}
}

func TestFailedExecuteIsAuditedToo(t *testing.T) {
	h, ts, rec := codeModeHarness(t)
	if _, err := h.executeScript(context.Background(), ts, "u-boss", "dev-1", executeIn{Code: `throw new Error("boom")`}); err == nil {
		t.Fatal("want the script's error")
	}
	if len(rec.events) != 1 || rec.events[0].Level != diagnostics.LevelWarn || !strings.Contains(fmt.Sprint(rec.events[0].Attributes["outcome"]), "boom") {
		t.Fatalf("audit = %+v, want one warning naming the failure", rec.events)
	}
}

func TestExecuteWaitsForNoOneWhenBusy(t *testing.T) {
	h, ts, _ := codeModeHarness(t)
	for range maxConcurrentScripts {
		h.scripts <- struct{}{}
	}
	if _, err := h.executeScript(context.Background(), ts, "u", "d", executeIn{Code: `return 1`}); !errors.Is(err, errScriptsBusy) {
		t.Fatalf("err = %v, want busy", err)
	}
}

func TestScriptTargetIsAnAPIPath(t *testing.T) {
	for raw, ok := range map[string]bool{
		"/v1/channels":                 true,
		"/v1/guide?from=1&to=2":        true,
		"/v1/../admin":                 false,
		"/v1/./channels":               false,
		"/health":                      false,
		"https://elsewhere.example/v1": false,
		"//elsewhere.example/v1/x":     false,
		"/v1/x#frag":                   false,
		// Encoding is decoded once here and again by the router: each of these is /v1/mcp there.
		"/v1/m%63p":             false,
		"/v1/m%2563p":           false,
		"/v1/mcp%3F":            false,
		"/v1/oauth%2Fcontinue":  false,
		"/v1/guide?title=a%20b": true, // a query may still be encoded
	} {
		if _, _, err := scriptTarget(raw); (err == nil) != ok {
			t.Errorf("scriptTarget(%q) err = %v, want ok=%v", raw, err, ok)
		}
	}
	for path, refused := range map[string]bool{
		"/v1/mcp": true, "/v1/mcp/": true, "/v1/oauth/token": true, "/v1/agents/code-mode/abc": true,
		"/v1/mcpx": false, "/v1/channels": false,
	} {
		if got := scriptRefuses(path); got != refused {
			t.Errorf("scriptRefuses(%q) = %v, want %v", path, got, refused)
		}
	}
}
