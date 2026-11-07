package api_test

import (
	"bufio"
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/events"
)

// ⚠ **THE SILENT FAILURE THIS FILE EXISTS FOR.** huma names an SSE frame after the Go TYPE
// of its payload — sse.Message has no Event field. Publish a type that is absent from
// eventTypeMap and the frame goes out with NO `event:` line, so `addEventListener("title", …)`
// never fires. huma prints "unknown event type" and a stack to stderr, which sounds like
// enough until you notice it bypasses the app's slog entirely, fails nothing, and still
// returns 200 — so the client waits forever and CI stays green. Neither the compiler nor any
// other test in this package notices.
//
// So there are two guards, and they catch different halves:
//
//   - TestEveryPublishedEventIsInTheEventTypeMap parses what internal/app actually publishes
//     and checks the name→type binding. Catches a NEW frame added without a map entry.
//   - TestEveryFrameStreamsWithItsEventName puts each payload through the real router and
//     reads the bytes off the wire. Catches the binding being wrong in a way the syntax tree
//     cannot show.

// fakeBus is an EventSource whose channel the test drives.
type fakeBus struct{ ch chan events.Event }

func newFakeBus() *fakeBus { return &fakeBus{ch: make(chan events.Event, 8)} }

func (f *fakeBus) Subscribe() (<-chan events.Event, func()) { return f.ch, func() {} }

func newEventsServer(t *testing.T) (*httptest.Server, *fakeBus) {
	t.Helper()
	bus := newFakeBus()
	srv := httptest.NewServer(api.Router(slog.New(slog.DiscardHandler), api.Options{
		Auth:   testAuthorizer{},
		Log:    slog.New(slog.DiscardHandler),
		Events: bus,
	}))
	t.Cleanup(srv.Close)
	return srv, bus
}

// publishedFrames parses the production files of internal/app and returns each publish site's
// event name and payload type: every `events.Event{Type: "name", Payload: api.TypeName{…}}`
// composite literal, by syntax tree rather than by text, so formatting cannot hide one.
func publishedFrames(t *testing.T) map[string]string {
	t.Helper()
	entries, err := os.ReadDir("../app")
	if err != nil {
		t.Fatal(err)
	}
	published := map[string]string{} // event name -> payload type name
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join("..", "app", e.Name()), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if lit, ok := n.(*ast.CompositeLit); ok && isSelector(lit.Type, "events", "Event") {
				if name, payload := publishSite(lit); name != "" && payload != "" {
					published[name] = payload
				}
			}
			return true
		})
	}
	return published
}

// publishSite returns the Type string and the api payload type name of one events.Event literal.
func publishSite(lit *ast.CompositeLit) (name, payload string) {
	for _, el := range lit.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, _ := kv.Key.(*ast.Ident)
		switch {
		case key == nil:
		case key.Name == "Type":
			if s, ok := kv.Value.(*ast.BasicLit); ok && s.Kind == token.STRING {
				name, _ = strconv.Unquote(s.Value)
			}
		case key.Name == "Payload":
			if p, ok := kv.Value.(*ast.CompositeLit); ok {
				if sel, ok := p.Type.(*ast.SelectorExpr); ok && isSelector(sel, "api", sel.Sel.Name) {
					payload = sel.Sel.Name
				}
			}
		}
	}
	return name, payload
}

func isSelector(e ast.Expr, pkg, name string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == pkg
}

// TestEveryPublishedEventIsInTheEventTypeMap parses the real publish sites in internal/app and
// asserts each one's (name, payload type) pair is what registerEvents binds.
//
// Parsing a sibling package is unusual, and deliberate: the binding being guarded spans the two
// packages, and there is no runtime seam that sees a publish site which has not fired yet. The
// vacuous-guard check below is what keeps a guard that stopped finding sites from passing forever.
func TestEveryPublishedEventIsInTheEventTypeMap(t *testing.T) {
	published := publishedFrames(t)

	// ⚠ A guard that matches nothing passes forever.
	if len(published) < 8 {
		t.Fatalf("only %d publish sites found (%v) — the syntax match stopped finding them, so "+
			"this guard is no longer guarding anything", len(published), published)
	}

	declared := api.EventFrameTypes()
	for name, typ := range published {
		want, ok := declared[name]
		if !ok {
			t.Errorf("internal/app publishes frame %q (api.%s) but eventTypeMap has no entry for it.\n"+
				"huma names a frame after its payload's Go type, so this frame ships with NO event: "+
				"line and every listener for %q silently ignores it.", name, typ, name)
			continue
		}
		if want != typ {
			t.Errorf("frame %q is published as api.%s but eventTypeMap binds it to api.%s — "+
				"the frame will be named after whatever %s is bound to, not %q.", name, typ, want, typ, name)
		}
	}

	// The other direction: a declared frame nothing publishes is a type in the spec and the
	// generated client that can never arrive. Worth knowing about, if only to delete.
	var orphaned []string
	for name := range declared {
		if _, ok := published[name]; !ok {
			orphaned = append(orphaned, name)
		}
	}
	sort.Strings(orphaned)
	if len(orphaned) > 0 {
		t.Errorf("eventTypeMap declares frames nothing publishes: %s\n"+
			"They reach the spec and the generated client but can never arrive.",
			strings.Join(orphaned, ", "))
	}
}

// TestEveryFrameStreamsWithItsEventName is the end-to-end half: publish each payload through
// the real router and read the `event:` line back off the wire.
func TestEveryFrameStreamsWithItsEventName(t *testing.T) {
	for _, tc := range []struct {
		want    string
		payload any
	}{
		{"title", api.TitleEvent{Key: "movie:tmdb:90000001", State: "available", Name: "Ashgrove Anomaly"}},
		{"channel", api.ChannelEvent{ChannelID: "ch_1", Status: "live"}},
		{"suggestion", api.SuggestionEvent{JobID: "j1", Phase: "scoring", Round: 3}},
		{"job", api.JobEvent{Name: "library-scan"}},
		{"activity", api.ActivityEvent{}},
		{"health", api.HealthEvent{}},
		{"settings", api.SettingsEvent{}},
		{"filler_ingest", api.FillerIngestEvent{JobID: "j1", Status: "success", Fetched: 2}},
		{"filler_split", api.FillerSplitEvent{JobID: "j1", Status: "success", ProposalID: "p1"}},
		{"llm_pull", api.LLMPullEvent{JobID: "j1", Model: "llama", Status: "pulling", Percent: 42}},
		{"database", api.DatabaseEvent{Phase: "migrating", Parity: "unknown"}},
		{"playout", api.PlayoutEvent{Active: 2}},
	} {
		t.Run(tc.want, func(t *testing.T) {
			srv, bus := newEventsServer(t)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/v1/events", nil)
			req.Header.Set("Authorization", "Bearer "+memberToken)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET /v1/events = %d, want 200", resp.StatusCode)
			}

			bus.ch <- events.Event{Type: tc.want, Payload: tc.payload}

			// Scan until the event line or the deadline. The stream opens with a comment, so
			// the first line is not the frame.
			sc := bufio.NewScanner(resp.Body)
			deadline := time.Now().Add(5 * time.Second)
			for sc.Scan() {
				line := sc.Text()
				if strings.HasPrefix(line, "event: ") {
					if got := strings.TrimPrefix(line, "event: "); got != tc.want {
						t.Fatalf("frame arrived as %q, want %q", got, tc.want)
					}
					return
				}
				if time.Now().After(deadline) {
					break
				}
			}
			t.Fatalf("no `event: %s` line arrived — a payload type missing from eventTypeMap "+
				"streams unnamed, which is exactly this failure", tc.want)
		})
	}
}

func TestEventStreamClosesWhenGenerationShutsDown(t *testing.T) {
	bus := newFakeBus()
	shutdown := make(chan struct{})
	srv := httptest.NewServer(api.Router(slog.New(slog.DiscardHandler), api.Options{
		Auth: testAuthorizer{}, Log: slog.New(slog.DiscardHandler), Events: bus, Shutdown: shutdown,
	}))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/events", nil)
	req.Header.Set("Authorization", "Bearer "+memberToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(io.Discard, resp.Body)
		done <- err
	}()
	close(shutdown)
	select {
	case err := <-done:
		t.Logf("SSE stream ended: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("SSE stream stayed open after generation shutdown")
	}
}

// §19 negative case: the stream is member-visible, so anonymous is refused. Pinned because
// /v1/events moved from a hand-written requireRole check to the operation's declared role.
func TestEventStream_RequiresASession(t *testing.T) {
	srv, _ := newEventsServer(t)
	resp := do(t, srv, http.MethodGet, "/v1/events", "", "")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous GET /v1/events = %d, want 401", resp.StatusCode)
	}
}
