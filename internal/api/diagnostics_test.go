package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/diagnostics"
)

type diagnosticEventServiceFunc func(context.Context, diagnostics.EventQuery) (diagnostics.EventPage, error)

func (f diagnosticEventServiceFunc) Query(
	ctx context.Context, query diagnostics.EventQuery,
) (diagnostics.EventPage, error) {
	return f(ctx, query)
}

func TestDiagnosticEventsAreAdminOnlyAndBearerQueryable(t *testing.T) {
	wantPage := diagnostics.EventPage{Items: []diagnostics.EventView{{
		ID: "diag-1", OccurredAt: 10, ReceivedAt: 11, Level: diagnostics.LevelError,
		Source: diagnostics.SourceServer, Subsystem: "api", Event: "api.request_failed",
		RequestID: "req-1", Attributes: map[string]any{"status": float64(503)},
	}}, NextCursor: "opaque-next"}
	var mu sync.Mutex
	var queries []diagnostics.EventQuery
	service := diagnosticEventServiceFunc(func(_ context.Context, query diagnostics.EventQuery) (diagnostics.EventPage, error) {
		mu.Lock()
		queries = append(queries, query)
		mu.Unlock()
		return wantPage, nil
	})
	log := slog.New(slog.DiscardHandler)
	handler := api.Router(log, api.Options{Auth: testAuthorizer{}, Log: log, DiagnosticEvents: service})

	member := httptest.NewRequest(http.MethodGet, "/v1/diagnostics/events", nil)
	member.Header.Set("Authorization", "Bearer "+memberToken)
	memberResponse := httptest.NewRecorder()
	handler.ServeHTTP(memberResponse, member)
	if memberResponse.Code != http.StatusForbidden {
		t.Fatalf("member status = %d, want 403", memberResponse.Code)
	}

	path := "/v1/diagnostics/events?from=1&to=1000&limit=1&level=error&source=server" +
		"&subsystem=api&requestId=req-1&playbackSessionId=play-1&channelId=channel-1" +
		"&scheduleBlockId=block-1&jobId=job-1&processRunId=process-1&instanceId=instance-1&text=failed"
	admin := httptest.NewRequest(http.MethodGet, path, nil)
	admin.Header.Set("Authorization", "Bearer "+adminToken)
	adminResponse := httptest.NewRecorder()
	handler.ServeHTTP(adminResponse, admin)
	if adminResponse.Code != http.StatusOK {
		t.Fatalf("admin status = %d, want 200: %s", adminResponse.Code, adminResponse.Body.String())
	}
	if adminResponse.Header().Get("X-Next-Cursor") != wantPage.NextCursor {
		t.Fatalf("JSON next cursor header = %q", adminResponse.Header().Get("X-Next-Cursor"))
	}
	var jsonPage diagnostics.EventPage
	if err := json.NewDecoder(adminResponse.Body).Decode(&jsonPage); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(jsonPage, wantPage) {
		t.Fatalf("JSON page = %+v, want %+v", jsonPage, wantPage)
	}

	ndjson := httptest.NewRequest(http.MethodGet, path, nil)
	ndjson.Header.Set("Authorization", "Bearer "+adminToken)
	ndjson.Header.Set("Accept", "application/x-ndjson")
	ndjsonResponse := httptest.NewRecorder()
	handler.ServeHTTP(ndjsonResponse, ndjson)
	if ndjsonResponse.Code != http.StatusOK || ndjsonResponse.Header().Get("Content-Type") != "application/x-ndjson" {
		t.Fatalf("NDJSON response = %d %q", ndjsonResponse.Code, ndjsonResponse.Header().Get("Content-Type"))
	}
	if ndjsonResponse.Header().Get("X-Next-Cursor") != wantPage.NextCursor {
		t.Fatalf("NDJSON next cursor header = %q", ndjsonResponse.Header().Get("X-Next-Cursor"))
	}
	var ndjsonEvent diagnostics.EventView
	if err := json.Unmarshal([]byte(strings.TrimSpace(ndjsonResponse.Body.String())), &ndjsonEvent); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ndjsonEvent, wantPage.Items[0]) {
		t.Fatalf("NDJSON event = %+v, want JSON item %+v", ndjsonEvent, wantPage.Items[0])
	}
	agentHandler := api.Router(log, api.Options{
		Auth: api.NewTokenAuthorizer("agent-token"), Log: log, DiagnosticEvents: service,
	})
	agent := httptest.NewRequest(http.MethodGet, "/v1/diagnostics/events", nil)
	agent.Header.Set("Authorization", "Bearer agent-token")
	agentResponse := httptest.NewRecorder()
	agentHandler.ServeHTTP(agentResponse, agent)
	if agentResponse.Code != http.StatusOK {
		t.Fatalf("agent Bearer status = %d, want 200: %s", agentResponse.Code, agentResponse.Body.String())
	}

	mu.Lock()
	defer mu.Unlock()
	if len(queries) != 3 {
		t.Fatalf("queries = %d, want JSON, NDJSON, and agent Bearer", len(queries))
	}
	wantQuery := diagnostics.EventQuery{
		From: 1, To: 1000, Limit: 1, Level: diagnostics.LevelError, Source: diagnostics.SourceServer,
		Subsystem: "api", RequestID: "req-1", PlaybackSessionID: "play-1", ChannelID: "channel-1",
		ScheduleBlockID: "block-1", JobID: "job-1", ProcessRunID: "process-1",
		InstanceID: "instance-1", Text: "failed",
	}
	if !reflect.DeepEqual(queries[0], wantQuery) || !reflect.DeepEqual(queries[1], wantQuery) {
		t.Fatalf("parsed queries = %+v", queries)
	}
}

func TestHumaValidationProblemRetainsRequestCorrelation(t *testing.T) {
	sink := &diagnosticRecordSink{}
	recorder := diagnostics.New(sink, diagnostics.Options{BatchSize: 1, FlushInterval: time.Hour})
	log := slog.New(diagnostics.NewSlogHandler(slog.DiscardHandler, recorder))
	service := diagnosticEventServiceFunc(func(context.Context, diagnostics.EventQuery) (diagnostics.EventPage, error) {
		return diagnostics.EventPage{}, nil
	})
	handler := api.Router(log, api.Options{Auth: testAuthorizer{}, Log: log, DiagnosticEvents: service})
	request := httptest.NewRequest(http.MethodGet, "/v1/diagnostics/events?limit=201", nil)
	request.Header.Set("Authorization", "Bearer "+adminToken)
	request.Header.Set("X-Request-Id", "req-validation")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code < 400 || response.Code >= 500 {
		t.Fatalf("validation status = %d, want 4xx", response.Code)
	}
	var problem struct {
		Instance string `json:"instance"`
	}
	if err := json.NewDecoder(response.Body).Decode(&problem); err != nil {
		t.Fatal(err)
	}
	if problem.Instance != "req-validation" {
		t.Fatalf("validation instance = %q", problem.Instance)
	}
	if err := recorder.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	for _, record := range sink.records {
		if record.Event == "api.request_rejected" && record.RequestID == problem.Instance &&
			strings.Contains(record.AttributesJSON, `"instance":"req-validation"`) {
			return
		}
	}
	t.Fatalf("no correlated api.request_rejected event in %+v", sink.records)
}

func TestDiagnosticEventFiltersFailBeforeUnboundedRead(t *testing.T) {
	calls := 0
	service := diagnosticEventServiceFunc(func(context.Context, diagnostics.EventQuery) (diagnostics.EventPage, error) {
		calls++
		return diagnostics.EventPage{}, nil
	})
	log := slog.New(slog.DiscardHandler)
	handler := api.Router(log, api.Options{Auth: testAuthorizer{}, Log: log, DiagnosticEvents: service})
	request := httptest.NewRequest(http.MethodGet, "/v1/diagnostics/events?limit=201", nil)
	request.Header.Set("Authorization", "Bearer "+adminToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code < 400 || response.Code >= 500 {
		t.Fatalf("excessive limit status = %d, want safe 4xx", response.Code)
	}
	if calls != 0 {
		t.Fatalf("invalid query reached diagnostics module %d times", calls)
	}
}

type diagnosticRecordSink struct {
	mu      sync.Mutex
	records []diagnostics.Record
}

func (s *diagnosticRecordSink) AppendDiagnosticEvents(_ context.Context, records []diagnostics.Record) error {
	s.mu.Lock()
	s.records = append(s.records, records...)
	s.mu.Unlock()
	return nil
}

func TestAPIProblemRetainsRequestAndRFC7807InstanceCorrelation(t *testing.T) {
	sink := &diagnosticRecordSink{}
	recorder := diagnostics.New(sink, diagnostics.Options{BatchSize: 1, FlushInterval: time.Hour})
	log := slog.New(diagnostics.NewSlogHandler(slog.DiscardHandler, recorder))
	service := diagnosticEventServiceFunc(func(context.Context, diagnostics.EventQuery) (diagnostics.EventPage, error) {
		return diagnostics.EventPage{}, errors.New("database offline")
	})
	handler := api.Router(log, api.Options{Auth: testAuthorizer{}, Log: log, DiagnosticEvents: service})
	request := httptest.NewRequest(http.MethodGet, "/v1/diagnostics/events", nil)
	request.Header.Set("Authorization", "Bearer "+adminToken)
	request.Header.Set("X-Request-Id", "req-correlation")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: %s", response.Code, response.Body.String())
	}
	var problem struct {
		Instance string `json:"instance"`
	}
	if err := json.NewDecoder(response.Body).Decode(&problem); err != nil {
		t.Fatal(err)
	}
	if problem.Instance != "req-correlation" || response.Header().Get("X-Request-Id") != problem.Instance {
		t.Fatalf("response request correlation = header %q instance %q", response.Header().Get("X-Request-Id"), problem.Instance)
	}
	if err := recorder.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	for _, record := range sink.records {
		if record.Event != "api.request_failed" {
			continue
		}
		if record.RequestID != problem.Instance || !strings.Contains(record.AttributesJSON, `"instance":"req-correlation"`) {
			t.Fatalf("persisted correlation = %+v", record)
		}
		return
	}
	t.Fatalf("no api.request_failed event in %+v", sink.records)
}
