package agents

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/diagnostics"
)

// recent_events decodes the route's real reply type, so a change to EventView's wire shape fails
// here rather than as a tool error in front of a user. It did once: occurredAt is epoch ms, and a
// tool that read it as RFC 3339 failed on every call against a live server while the API test
// harness, which has no event retention, only ever saw the route's 501.
//
// With no level asked for, warnings and errors are two reads merged newest first.
func TestRecentEventsMergesWarningsAndErrorsNewestFirst(t *testing.T) {
	pages := map[string]diagnostics.EventPage{
		"warn":  {Items: []diagnostics.EventView{{Event: "w-old", Level: "warn", OccurredAt: 1000}, {Event: "w-new", Level: "warn", OccurredAt: 3000}}},
		"error": {Items: []diagnostics.EventView{{Event: "e-mid", Level: "error", OccurredAt: 2000}}, NextCursor: "more"},
	}
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/household":
			_, _ = w.Write([]byte(`{"timeZone":"UTC"}`))
		case "/v1/diagnostics/events":
			_ = json.NewEncoder(w).Encode(pages[r.URL.Query().Get("level")])
		default:
			http.NotFound(w, r)
		}
	})
	ts := &toolset{d: NewDispatcher(api)}

	_, out, err := ts.recentEvents(context.Background(), nil, eventsIn{Count: 2})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range out.Events {
		got = append(got, e.Event)
	}
	if len(got) != 2 || got[0] != "w-new" || got[1] != "e-mid" || !out.More {
		t.Fatalf("events = %v, more = %v; want [w-new e-mid] and more", got, out.More)
	}
}
