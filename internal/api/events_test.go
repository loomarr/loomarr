package api_test

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/events"
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
//   - TestEveryPublishedEventIsInTheEventTypeMap scans what internal/app actually publishes
//     and checks the name→type binding. Catches a NEW frame added without a map entry.
//   - TestEveryFrameStreamsWithItsEventName puts each payload through the real router and
//     reads the bytes off the wire. Catches the binding being wrong in a way source-reading
//     cannot see.

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

// publishedFrame matches `Type: "name", Payload: api.TypeName{` across a publish site, which
// the repo writes both on one line and split across two.
var publishedFrame = regexp.MustCompile(`Type:\s*"([a-z_]+)",\s*\n?\s*Payload:\s*api\.([A-Za-z]+)\{`)

// TestEveryPublishedEventIsInTheEventTypeMap reads the real publish sites in internal/app and
// asserts each one's (name, payload type) pair is what registerEvents binds.
//
// Source-scanning a sibling package is unusual, and deliberate: the binding being guarded
// spans the two packages, and there is no runtime seam that sees a publish site which has not
// fired yet. The vacuous-guard check below is what keeps a regex that stopped matching from
// passing forever.
func TestEveryPublishedEventIsInTheEventTypeMap(t *testing.T) {
	entries, err := os.ReadDir("../app")
	if err != nil {
		t.Fatal(err)
	}
	published := map[string]string{} // event name -> payload type name
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		b, err := os.ReadFile("../app/" + name)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range publishedFrame.FindAllStringSubmatch(string(b), -1) {
			published[m[1]] = m[2]
		}
	}

	// ⚠ A guard that matches nothing passes forever.
	if len(published) < 8 {
		t.Fatalf("only %d publish sites found (%v) — the regex stopped matching, so this guard "+
			"is no longer guarding anything", len(published), published)
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
		{"title", api.TitleEvent{Key: "movie:tmdb:1", State: "available", Name: "In Flames"}},
		{"channel", api.ChannelEvent{ChannelID: "ch_1", Status: "live"}},
		{"suggestion", api.SuggestionEvent{JobID: "j1", Phase: "scoring", Round: 3}},
		{"job", api.JobEvent{Name: "library-scan"}},
		{"activity", api.ActivityEvent{}},
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
	case <-done:
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
