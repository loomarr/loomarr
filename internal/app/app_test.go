package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/diagnostics"
	"github.com/loomarr/loomarr/internal/events"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/testkit"
)

// TestEventEmitterPublishesToBus is the #11 seam: the composition-root emitter
// must turn a provisioning domain event into a `title` SSE frame on the bus, so
// /v1/events actually delivers state changes (before this wire, nothing ever
// called Publish and every SSE client waited forever). It also asserts the
// nil-engine path is safe (#10): an event emitted before the scheduler is wired
// still reaches the bus and never panics.
func TestEventEmitterPublishesToBus(t *testing.T) {
	t.Parallel()
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
		// ⚠ A TYPED payload, not a map — and the assertion is on the concrete type for a
		// reason beyond tidiness. huma names an SSE frame after its payload's Go type
		// (internal/api/events.go), so publishing anything else here would ship the frame
		// unnamed and every browser listener for "title" would silently stop firing.
		payload, ok := got.Payload.(api.TitleEvent)
		if !ok {
			t.Fatalf("payload type = %T, want api.TitleEvent", got.Payload)
		}
		if payload.Key != "movie:tmdb:603" || payload.State != "available" || payload.Name != "The Matrix" {
			t.Errorf("payload = %+v, want key/state/name for The Matrix available", payload)
		}
	default:
		t.Fatal("no event published to the bus — #11 seam still open (nothing reached the subscriber)")
	}
}

func TestStartupReportAndReadinessShareOneGenerationState(t *testing.T) {
	t.Setenv("API_TOKEN", "startup-admin-token")
	for _, tc := range []struct {
		name       string
		status     diagnostics.StartupCheckStatus
		state      diagnostics.StartupState
		health     diagnostics.HealthState
		readyCode  int
		readyValue bool
	}{
		{"ready", diagnostics.StartupPassed, diagnostics.StartupReady, diagnostics.HealthHealthy, http.StatusOK, true},
		{"blocked", diagnostics.StartupFailed, diagnostics.StartupBlocked, diagnostics.HealthUnhealthy, http.StatusServiceUnavailable, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := testkit.MigratedSQLiteStore(t)
			now := time.Unix(100, 0)
			startup := diagnostics.NewStartup(now, 1, "v1.2.3", []diagnostics.StartupCheck{
				{Key: diagnostics.StartupCheckDatabase, Label: "Database", Required: true},
			}, func() time.Time { return now })
			startup.Complete(diagnostics.StartupCheckDatabase, tc.status, tc.name, "", "")
			handler := buildTestApplication(t, st, Overrides{Startup: startup}).Handler()

			ready := httptest.NewRecorder()
			handler.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/v1/readyz", nil))
			if ready.Code != tc.readyCode {
				t.Fatalf("/readyz = %d, want %d: %s", ready.Code, tc.readyCode, ready.Body.String())
			}
			var readyBody struct {
				Ready bool `json:"ready"`
			}
			if err := json.NewDecoder(ready.Body).Decode(&readyBody); err != nil {
				t.Fatal(err)
			}

			reportRequest := httptest.NewRequest(http.MethodGet, "/v1/diagnostics/startup-reports?limit=1", nil)
			reportRequest.Header.Set("Authorization", "Bearer startup-admin-token")
			reportResponse := httptest.NewRecorder()
			handler.ServeHTTP(reportResponse, reportRequest)
			if reportResponse.Code != http.StatusOK {
				t.Fatalf("startup reports = %d: %s", reportResponse.Code, reportResponse.Body.String())
			}
			var reportBody struct {
				Current diagnostics.StartupReport `json:"current"`
			}
			if err := json.NewDecoder(reportResponse.Body).Decode(&reportBody); err != nil {
				t.Fatal(err)
			}
			if readyBody.Ready != tc.readyValue || reportBody.Current.State != tc.state {
				t.Fatalf("shared state disagreed: ready=%v report=%q", readyBody.Ready, reportBody.Current.State)
			}

			healthRequest := httptest.NewRequest(http.MethodGet, "/v1/diagnostics/health", nil)
			healthRequest.Header.Set("Authorization", "Bearer startup-admin-token")
			healthResponse := httptest.NewRecorder()
			handler.ServeHTTP(healthResponse, healthRequest)
			if healthResponse.Code != http.StatusOK {
				t.Fatalf("current health = %d: %s", healthResponse.Code, healthResponse.Body.String())
			}
			var health diagnostics.HealthReport
			if err := json.NewDecoder(healthResponse.Body).Decode(&health); err != nil {
				t.Fatal(err)
			}
			if health.State != tc.health || readyBody.Ready != (health.State != diagnostics.HealthUnhealthy && health.State != diagnostics.HealthStarting) {
				t.Fatalf("readiness and Current Health disagreed: ready=%v health=%q", readyBody.Ready, health.State)
			}
		})
	}
}

func TestHealthEmitterPublishesTypedInvalidation(t *testing.T) {
	t.Parallel()
	bus := events.NewBus()
	sub, unsubscribe := bus.Subscribe()
	defer unsubscribe()
	emit := &eventEmitter{bus: bus}
	emit.HealthChanged()
	select {
	case got := <-sub:
		if got.Type != "health" {
			t.Fatalf("event type = %q", got.Type)
		}
		if _, ok := got.Payload.(api.HealthEvent); !ok {
			t.Fatalf("payload type = %T, want api.HealthEvent", got.Payload)
		}
	default:
		t.Fatal("no Current Health invalidation reached the SSE bus")
	}
}

func TestStartupReportsSurviveGenerationReplacement(t *testing.T) {
	t.Setenv("API_TOKEN", "startup-admin-token")
	st := testkit.MigratedSQLiteStore(t)
	processStarted := time.Unix(100, 0)
	newReport := func(generation int) *diagnostics.Startup {
		now := processStarted.Add(time.Duration(generation) * time.Second)
		report := diagnostics.NewStartup(processStarted, generation, "v1.2.3", []diagnostics.StartupCheck{
			{Key: diagnostics.StartupCheckDatabase, Label: "Database", Required: true},
		}, func() time.Time { return now })
		report.Complete(diagnostics.StartupCheckDatabase, diagnostics.StartupPassed, "ready", "", "")
		return report
	}

	first, err := Build(t.Context(), st, slog.New(slog.DiscardHandler), Overrides{Startup: newReport(1)})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	second, err := Build(t.Context(), st, slog.New(slog.DiscardHandler), Overrides{Startup: newReport(2)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Shutdown(context.Background()) })

	request := httptest.NewRequest(http.MethodGet, "/v1/diagnostics/startup-reports?limit=10", nil)
	request.Header.Set("Authorization", "Bearer startup-admin-token")
	response := httptest.NewRecorder()
	second.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("startup reports = %d: %s", response.Code, response.Body.String())
	}
	var body struct {
		Items []diagnostics.StartupReport `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 2 || body.Items[0].Generation != 2 || body.Items[1].Generation != 1 {
		t.Fatalf("reports after replacement = %+v", body.Items)
	}
}

// Starting with no store is a SUPPORTED degraded mode, not a misconfiguration to crash
// on: main logs "running without a store (not ready)" and expects the server to keep
// serving so /readyz can report why. It didn't — with no store there is no settings
// service, and the first unguarded settings read panicked during Build. A
// container missing DATABASE_URL therefore crash-looped instead of answering the probe
// that would have explained the problem.
//
// The assertion is deliberately just "builds and answers": the value here is that the
// process stays alive long enough to tell the operator what's wrong.
func TestBuildWithoutStoreServesReadinessInsteadOfPanicking(t *testing.T) {
	t.Parallel()
	h := buildTestApplication(t, nil, Overrides{}).Handler()
	if h == nil {
		t.Fatal("Build returned a nil handler")
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("/readyz = %d, want 503 (not ready, but answering)", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "no store configured") {
		t.Errorf("/readyz body = %q, want the reason the operator needs", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("/healthz = %d, want 200 (the process is alive)", rec.Code)
	}
}

// Build must register the state-gauge collector (§17), so /metrics
// exposes the domain gauges — not just the RED/runtime foundation. This is the
// end-to-end wiring the metrics package unit tests can't prove on their own.
func TestBuildExposesDomainMetrics(t *testing.T) {
	t.Parallel()
	st := testkit.MigratedSQLiteStore(t)

	h := buildTestApplication(t, st, Overrides{}).Handler()

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
