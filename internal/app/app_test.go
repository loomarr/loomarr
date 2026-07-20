package app

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mantonx/loomarr/internal/events"
	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/store"
)

// TestEventEmitterPublishesToBus is the #11 seam: the composition-root emitter
// must turn a provisioning domain event into a `title` SSE frame on the bus, so
// /v1/events actually delivers state changes (before this wire, nothing ever
// called Publish and every SSE client waited forever). It also asserts the
// nil-engine path is safe (#10): an event emitted before the scheduler is wired
// still reaches the bus and never panics.
func TestEventEmitterPublishesToBus(t *testing.T) {
	bus := events.NewBus()
	sub, unsubscribe := bus.Subscribe()
	defer unsubscribe()

	// engine deliberately left nil (unwired scheduler / pre-setEngine window).
	emit := &eventEmitter{bus: bus}

	ev := provision.DomainEvent{
		Key:   provision.Key("movie:tmdb:603"),
		State: provision.Available,
		Title: provision.Title{MediaType: provision.Movie, TMDBID: 603, Name: "The Matrix"},
	}
	emit.Emit(context.Background(), ev)

	select {
	case got := <-sub:
		if got.Type != "title" {
			t.Errorf("event type = %q, want title", got.Type)
		}
		payload, ok := got.Payload.(map[string]string)
		if !ok {
			t.Fatalf("payload type = %T, want map[string]string", got.Payload)
		}
		if payload["key"] != "movie:tmdb:603" || payload["state"] != "available" || payload["name"] != "The Matrix" {
			t.Errorf("payload = %v, want key/state/name for The Matrix available", payload)
		}
	default:
		t.Fatal("no event published to the bus — #11 seam still open (nothing reached the subscriber)")
	}
}

// Starting with no store is a SUPPORTED degraded mode, not a misconfiguration to crash
// on: main logs "running without a store (not ready)" and expects the server to keep
// serving so /readyz can report why. It didn't — with no store there is no settings
// service, and the first unguarded settings read panicked inside BuildHandler. A
// container missing DATABASE_URL therefore crash-looped instead of answering the probe
// that would have explained the problem.
//
// The assertion is deliberately just "builds and answers": the value here is that the
// process stays alive long enough to tell the operator what's wrong.
func TestBuildHandlerWithoutStoreServesReadinessInsteadOfPanicking(t *testing.T) {
	h, err := BuildHandler(context.Background(), nil, slog.New(slog.DiscardHandler), Overrides{})
	if err != nil {
		t.Fatalf("BuildHandler with no store returned an error: %v", err)
	}
	if h == nil {
		t.Fatal("BuildHandler returned a nil handler")
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("/readyz = %d, want 503 (not ready, but answering)", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "no store configured") {
		t.Errorf("/readyz body = %q, want the reason the operator needs", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("/healthz = %d, want 200 (the process is alive)", rec.Code)
	}
}

// BuildHandler must register the state-gauge collector (§17), so /metrics
// exposes the domain gauges — not just the RED/runtime foundation. This is the
// end-to-end wiring the metrics package unit tests can't prove on their own.
func TestBuildHandlerExposesDomainMetrics(t *testing.T) {
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/app.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	h, err := BuildHandler(context.Background(), st, slog.New(slog.DiscardHandler), Overrides{})
	if err != nil {
		t.Fatalf("BuildHandler: %v", err)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/metrics = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`loomarr_titles{state="wanted"}`, // zero-filled gauge proves the collector ran
		"loomarr_jobs{status=",
		"loomarr_active_sessions",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/metrics missing domain gauge %q (store collector not wired?)", want)
		}
	}
}
