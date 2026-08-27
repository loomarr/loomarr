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
	"github.com/loomarr/loomarr/internal/store"
)

type diagnosticEventServiceFunc func(context.Context, diagnostics.EventQuery) (diagnostics.EventPage, error)

type diagnosticProcessService struct {
	query  func(context.Context, diagnostics.ProcessQuery) (diagnostics.ProcessPage, error)
	get    func(context.Context, string) (diagnostics.ProcessDetail, error)
	output func(context.Context, string) (diagnostics.ProcessOutput, error)
}

type diagnosticBundleService struct {
	preview func(context.Context, diagnostics.BundleSelection) (diagnostics.BundlePreview, error)
	build   func(context.Context, diagnostics.BundleSelection) (diagnostics.BundleResult, error)
}

func (s diagnosticBundleService) Preview(ctx context.Context, selection diagnostics.BundleSelection) (diagnostics.BundlePreview, error) {
	return s.preview(ctx, selection)
}

func (s diagnosticBundleService) Build(ctx context.Context, selection diagnostics.BundleSelection) (diagnostics.BundleResult, error) {
	return s.build(ctx, selection)
}

func (s diagnosticProcessService) Query(ctx context.Context, query diagnostics.ProcessQuery) (diagnostics.ProcessPage, error) {
	return s.query(ctx, query)
}

func (s diagnosticProcessService) Get(ctx context.Context, id string) (diagnostics.ProcessDetail, error) {
	return s.get(ctx, id)
}

func (s diagnosticProcessService) Output(ctx context.Context, id string) (diagnostics.ProcessOutput, error) {
	return s.output(ctx, id)
}

type clientDiagnosticServiceFunc func(context.Context, string, diagnostics.ClientBatch) (int, error)

func (f clientDiagnosticServiceFunc) Ingest(
	ctx context.Context, actorID string, batch diagnostics.ClientBatch,
) (int, error) {
	return f(ctx, actorID, batch)
}

type diagnosticUserAuthorizer struct{}

func (diagnosticUserAuthorizer) Authorize(r *http.Request) api.Role {
	role, _ := diagnosticUserAuthorizer{}.AuthorizeUser(r)
	return role
}

func (diagnosticUserAuthorizer) AuthorizeUser(r *http.Request) (api.Role, *store.User) {
	if strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") != memberToken {
		return api.RoleAnonymous, nil
	}
	user := store.User{ID: "member_1", Name: "Viewer", Role: store.RoleMember}
	return api.RoleMember, &user
}

func (f diagnosticEventServiceFunc) Query(
	ctx context.Context, query diagnostics.EventQuery,
) (diagnostics.EventPage, error) {
	return f(ctx, query)
}

type startupReportService struct {
	current diagnostics.StartupReport
	health  diagnostics.HealthReport
	items   []diagnostics.StartupReport
}

type healthRefreshFunc func(context.Context) (diagnostics.HealthReport, error)

func (f healthRefreshFunc) Refresh(ctx context.Context) (diagnostics.HealthReport, error) {
	return f(ctx)
}

func (s startupReportService) Current() diagnostics.StartupReport { return s.current }
func (s startupReportService) Health() diagnostics.HealthReport   { return s.health }
func (s startupReportService) Recent(context.Context, int) ([]diagnostics.StartupReport, error) {
	return s.items, nil
}

func TestStartupReportsAreAdminOnlyAndBearerQueryable(t *testing.T) {
	current := diagnostics.StartupReport{ID: "startup-current", Generation: 2, Version: "v1.2.3", State: diagnostics.StartupDegraded}
	service := startupReportService{current: current, items: []diagnostics.StartupReport{current}}
	log := slog.New(slog.DiscardHandler)
	handler := api.Router(log, api.Options{Auth: testAuthorizer{}, Log: log, StartupReports: service})

	member := httptest.NewRequest(http.MethodGet, "/v1/diagnostics/startup-reports", nil)
	member.Header.Set("Authorization", "Bearer "+memberToken)
	memberResponse := httptest.NewRecorder()
	handler.ServeHTTP(memberResponse, member)
	if memberResponse.Code != http.StatusForbidden {
		t.Fatalf("member status = %d, want 403", memberResponse.Code)
	}

	admin := httptest.NewRequest(http.MethodGet, "/v1/diagnostics/startup-reports?limit=10", nil)
	admin.Header.Set("Authorization", "Bearer "+adminToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, admin)
	if response.Code != http.StatusOK {
		t.Fatalf("admin status = %d: %s", response.Code, response.Body.String())
	}
	var body struct {
		Current diagnostics.StartupReport   `json:"current"`
		Items   []diagnostics.StartupReport `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Current.ID != current.ID || len(body.Items) != 1 || body.Items[0].State != diagnostics.StartupDegraded {
		t.Fatalf("startup response = %+v", body)
	}
}

func TestSupportBundlePreviewAndDownloadAreAdminOnlyAndUseSameSelection(t *testing.T) {
	var previewSelection, buildSelection diagnostics.BundleSelection
	service := diagnosticBundleService{
		preview: func(_ context.Context, selection diagnostics.BundleSelection) (diagnostics.BundlePreview, error) {
			previewSelection = selection
			return diagnostics.BundlePreview{Manifest: diagnostics.BundleManifest{FormatVersion: diagnostics.SupportBundleFormat, Counts: diagnostics.BundleCounts{Events: 2}}, EstimatedBytes: 512}, nil
		},
		build: func(_ context.Context, selection diagnostics.BundleSelection) (diagnostics.BundleResult, error) {
			buildSelection = selection
			return diagnostics.BundleResult{Manifest: diagnostics.BundleManifest{GeneratedAt: 1787572800000}, Content: []byte("PK bundle")}, nil
		},
	}
	log := slog.New(slog.DiscardHandler)
	handler := api.Router(log, api.Options{Auth: testAuthorizer{}, Log: log, DiagnosticBundles: service})
	body := `{"from":1,"to":2,"events":true,"processes":true,"processOutput":false,"channelId":"channel-1"}`
	member := httptest.NewRequest(http.MethodPost, "/v1/diagnostics/support-bundle/preview", strings.NewReader(body))
	member.Header.Set("Authorization", "Bearer "+memberToken)
	member.Header.Set("Content-Type", "application/json")
	memberResponse := httptest.NewRecorder()
	handler.ServeHTTP(memberResponse, member)
	if memberResponse.Code != http.StatusForbidden {
		t.Fatalf("member status = %d", memberResponse.Code)
	}

	preview := httptest.NewRequest(http.MethodPost, "/v1/diagnostics/support-bundle/preview", strings.NewReader(body))
	preview.Header.Set("Authorization", "Bearer "+adminToken)
	preview.Header.Set("Content-Type", "application/json")
	previewResponse := httptest.NewRecorder()
	handler.ServeHTTP(previewResponse, preview)
	if previewResponse.Code != http.StatusOK || previewSelection.ChannelID != "channel-1" {
		t.Fatalf("preview = %d %s, selection %#v", previewResponse.Code, previewResponse.Body.String(), previewSelection)
	}

	download := httptest.NewRequest(http.MethodPost, "/v1/diagnostics/support-bundle", strings.NewReader(body))
	download.Header.Set("Authorization", "Bearer "+adminToken)
	download.Header.Set("Content-Type", "application/json")
	downloadResponse := httptest.NewRecorder()
	handler.ServeHTTP(downloadResponse, download)
	if downloadResponse.Code != http.StatusOK || downloadResponse.Body.String() != "PK bundle" || buildSelection != previewSelection {
		t.Fatalf("download = %d %q, selection %#v", downloadResponse.Code, downloadResponse.Body.String(), buildSelection)
	}
	if downloadResponse.Header().Get("Content-Type") != "application/zip" || !strings.Contains(downloadResponse.Header().Get("Content-Disposition"), "loomarr-support-20260824T120000Z.zip") {
		t.Fatalf("download headers = %#v", downloadResponse.Header())
	}
}

func TestCurrentHealthIsAdminOnlyAndBearerQueryable(t *testing.T) {
	want := diagnostics.HealthReport{
		GenerationID: "startup-current", Generation: 2, Version: "v1.2.3",
		State: diagnostics.HealthDegraded, UpdatedAt: 123,
		Checks: []diagnostics.HealthCheck{{
			Key: "media_server", Label: "Media server", Mode: diagnostics.HealthCheckContinuous,
			Status: diagnostics.HealthWarning, Detail: "configured but unavailable",
		}},
	}
	service := startupReportService{health: want}
	log := slog.New(slog.DiscardHandler)
	handler := api.Router(log, api.Options{Auth: testAuthorizer{}, Log: log, StartupReports: service})

	member := httptest.NewRequest(http.MethodGet, "/v1/diagnostics/health", nil)
	member.Header.Set("Authorization", "Bearer "+memberToken)
	memberResponse := httptest.NewRecorder()
	handler.ServeHTTP(memberResponse, member)
	if memberResponse.Code != http.StatusForbidden {
		t.Fatalf("member status = %d, want 403", memberResponse.Code)
	}

	agentHandler := api.Router(log, api.Options{
		Auth: api.NewTokenAuthorizer("agent-token"), Log: log, StartupReports: service,
	})
	request := httptest.NewRequest(http.MethodGet, "/v1/diagnostics/health", nil)
	request.Header.Set("Authorization", "Bearer agent-token")
	response := httptest.NewRecorder()
	agentHandler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("agent status = %d: %s", response.Code, response.Body.String())
	}
	var got diagnostics.HealthReport
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("health response = %+v, want %+v", got, want)
	}
}

func TestCurrentHealthRefreshIsAdminOnlyAndUsesSharedRunner(t *testing.T) {
	want := diagnostics.HealthReport{GenerationID: "startup-current", State: diagnostics.HealthHealthy}
	calls := 0
	refresh := healthRefreshFunc(func(context.Context) (diagnostics.HealthReport, error) {
		calls++
		return want, nil
	})
	log := slog.New(slog.DiscardHandler)
	handler := api.Router(log, api.Options{Auth: testAuthorizer{}, Log: log, HealthRefresh: refresh})

	member := httptest.NewRequest(http.MethodPost, "/v1/diagnostics/health/refresh", nil)
	member.Header.Set("Authorization", "Bearer "+memberToken)
	memberResponse := httptest.NewRecorder()
	handler.ServeHTTP(memberResponse, member)
	if memberResponse.Code != http.StatusForbidden || calls != 0 {
		t.Fatalf("member response = %d, calls = %d", memberResponse.Code, calls)
	}

	admin := httptest.NewRequest(http.MethodPost, "/v1/diagnostics/health/refresh", nil)
	admin.Header.Set("Authorization", "Bearer "+adminToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, admin)
	if response.Code != http.StatusOK || calls != 1 {
		t.Fatalf("admin response = %d, calls = %d: %s", response.Code, calls, response.Body.String())
	}
	var got diagnostics.HealthReport
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("refresh response = %+v, want %+v", got, want)
	}
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

	path := "/v1/diagnostics/events?from=1&to=1000&limit=1&order=oldest&level=error&source=server&event=api.request_failed" +
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
	mu.Lock()
	if len(queries) != 1 || queries[0].Order != diagnostics.EventOrderOldest {
		t.Fatalf("event query order = %+v, want oldest", queries)
	}
	mu.Unlock()
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
		From: 1, To: 1000, Limit: 1, Order: diagnostics.EventOrderOldest,
		Level: diagnostics.LevelError, Source: diagnostics.SourceServer,
		Event:     "api.request_failed",
		Subsystem: "api", RequestID: "req-1", PlaybackSessionID: "play-1", ChannelID: "channel-1",
		ScheduleBlockID: "block-1", JobID: "job-1", ProcessRunID: "process-1",
		InstanceID: "instance-1", Text: "failed",
	}
	if !reflect.DeepEqual(queries[0], wantQuery) || !reflect.DeepEqual(queries[1], wantQuery) {
		t.Fatalf("parsed queries = %+v", queries)
	}
}

func TestDiagnosticProcessesAreAdminOnlyAndExposeBoundedOutput(t *testing.T) {
	wantPage := diagnostics.ProcessPage{Items: []diagnostics.ProcessRunView{{
		ID: "process-1", Purpose: "playout_program", ChannelID: "channel-1",
		StartedAt: 10, Status: diagnostics.ProcessFailed, DiscardedLines: 2,
	}}, NextCursor: "process-next"}
	wantDetail := diagnostics.ProcessDetail{Run: wantPage.Items[0], Progress: []diagnostics.ProcessProgressView{{
		OccurredAt: 11, Frame: 20, Speed: 0.9, OutTimeMS: 1_000,
	}}}
	var gotQuery diagnostics.ProcessQuery
	service := diagnosticProcessService{
		query: func(_ context.Context, query diagnostics.ProcessQuery) (diagnostics.ProcessPage, error) {
			gotQuery = query
			return wantPage, nil
		},
		get: func(_ context.Context, id string) (diagnostics.ProcessDetail, error) {
			if id != "process-1" {
				return diagnostics.ProcessDetail{}, diagnostics.ErrProcessNotFound
			}
			return wantDetail, nil
		},
		output: func(_ context.Context, id string) (diagnostics.ProcessOutput, error) {
			if id != "process-1" {
				return diagnostics.ProcessOutput{}, diagnostics.ErrProcessNotFound
			}
			return diagnostics.ProcessOutput{Content: []byte("redacted ffmpeg output\n"), DiscardedLines: 2, Truncated: true}, nil
		},
	}
	log := slog.New(slog.DiscardHandler)
	handler := api.Router(log, api.Options{Auth: testAuthorizer{}, Log: log, DiagnosticProcesses: service})

	member := httptest.NewRequest(http.MethodGet, "/v1/diagnostics/processes", nil)
	member.Header.Set("Authorization", "Bearer "+memberToken)
	memberResponse := httptest.NewRecorder()
	handler.ServeHTTP(memberResponse, member)
	if memberResponse.Code != http.StatusForbidden {
		t.Fatalf("member status = %d, want 403", memberResponse.Code)
	}

	list := httptest.NewRequest(http.MethodGet, "/v1/diagnostics/processes?from=1&to=1000&limit=25&status=failed&purpose=playout_program&channelId=channel-1&jobId=job-1", nil)
	list.Header.Set("Authorization", "Bearer "+adminToken)
	listResponse := httptest.NewRecorder()
	handler.ServeHTTP(listResponse, list)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", listResponse.Code, listResponse.Body.String())
	}
	var page diagnostics.ProcessPage
	if err := json.NewDecoder(listResponse.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(page, wantPage) {
		t.Fatalf("page = %#v, want %#v", page, wantPage)
	}
	wantQuery := diagnostics.ProcessQuery{From: 1, To: 1000, Limit: 25, Status: diagnostics.ProcessFailed, Purpose: "playout_program", ChannelID: "channel-1", JobID: "job-1"}
	if !reflect.DeepEqual(gotQuery, wantQuery) {
		t.Fatalf("query = %#v, want %#v", gotQuery, wantQuery)
	}

	detail := httptest.NewRequest(http.MethodGet, "/v1/diagnostics/processes/process-1", nil)
	detail.Header.Set("Authorization", "Bearer "+adminToken)
	detailResponse := httptest.NewRecorder()
	handler.ServeHTTP(detailResponse, detail)
	if detailResponse.Code != http.StatusOK {
		t.Fatalf("detail status = %d: %s", detailResponse.Code, detailResponse.Body.String())
	}
	var gotDetail diagnostics.ProcessDetail
	if err := json.NewDecoder(detailResponse.Body).Decode(&gotDetail); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotDetail, wantDetail) {
		t.Fatalf("detail = %#v, want %#v", gotDetail, wantDetail)
	}

	output := httptest.NewRequest(http.MethodGet, "/v1/diagnostics/processes/process-1/output", nil)
	output.Header.Set("Authorization", "Bearer "+adminToken)
	outputResponse := httptest.NewRecorder()
	handler.ServeHTTP(outputResponse, output)
	if outputResponse.Code != http.StatusOK || outputResponse.Body.String() != "redacted ffmpeg output\n" {
		t.Fatalf("output = %d %q", outputResponse.Code, outputResponse.Body.String())
	}
	if outputResponse.Header().Get("X-Diagnostic-Truncated") != "true" || outputResponse.Header().Get("X-Diagnostic-Discarded-Lines") != "2" {
		t.Fatalf("output metadata = %#v", outputResponse.Header())
	}
}

func TestClientDiagnosticsAcceptMemberAndDeriveActor(t *testing.T) {
	var gotActor string
	var gotBatch diagnostics.ClientBatch
	service := clientDiagnosticServiceFunc(func(_ context.Context, actorID string, batch diagnostics.ClientBatch) (int, error) {
		gotActor, gotBatch = actorID, batch
		return len(batch.Events), nil
	})
	log := slog.New(slog.DiscardHandler)
	handler := api.Router(log, api.Options{
		Auth: diagnosticUserAuthorizer{}, Log: log, ClientDiagnostics: service,
	})

	anonymous := httptest.NewRequest(http.MethodPost, "/v1/diagnostics/client-events", strings.NewReader(`{
		"source":"web","clientVersion":"dev","platform":"chromium","events":[]
	}`))
	anonymous.Header.Set("Content-Type", "application/json")
	anonymousResponse := httptest.NewRecorder()
	handler.ServeHTTP(anonymousResponse, anonymous)
	if anonymousResponse.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d, want 401", anonymousResponse.Code)
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/diagnostics/client-events", strings.NewReader(`{
		"source":"web","clientVersion":"v0.1.0-beta.4","platform":"chromium",
		"events":[{"event":"player.attached","occurredAt":1800000000000,
		"playbackSessionId":"play_1","channelId":"ch_1","transport":"hls_js"}]
	}`))
	request.Header.Set("Authorization", "Bearer "+memberToken)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("member status = %d, want 202: %s", response.Code, response.Body.String())
	}
	if gotActor != "member_1" || gotBatch.Source != diagnostics.SourceWeb || len(gotBatch.Events) != 1 {
		t.Fatalf("derived submission = actor %q batch %+v", gotActor, gotBatch)
	}
	var body struct {
		Accepted int `json:"accepted"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil || body.Accepted != 1 {
		t.Fatalf("response = %+v, err %v", body, err)
	}

	mobileRequest := httptest.NewRequest(http.MethodPost, "/v1/diagnostics/client-events", strings.NewReader(`{
		"source":"android_mobile","clientVersion":"0.0.1","platform":"android_mobile",
		"events":[{"event":"player.ready","occurredAt":1800000000000,
		"playbackSessionId":"play_mobile","channelId":"ch_1","transport":"native_hls"}]
	}`))
	mobileRequest.Header.Set("Authorization", "Bearer "+memberToken)
	mobileRequest.Header.Set("Content-Type", "application/json")
	mobileResponse := httptest.NewRecorder()
	handler.ServeHTTP(mobileResponse, mobileRequest)
	if mobileResponse.Code != http.StatusAccepted {
		t.Fatalf("Android mobile status = %d, want 202: %s", mobileResponse.Code, mobileResponse.Body.String())
	}
	if gotActor != "member_1" || gotBatch.Source != diagnostics.SourceAndroidMobile || gotBatch.Platform != "android_mobile" {
		t.Fatalf("Android mobile submission = actor %q batch %+v", gotActor, gotBatch)
	}
}

func TestClientDiagnosticsDeriveAPITokenIdentityAndMapModuleErrors(t *testing.T) {
	var actor string
	service := clientDiagnosticServiceFunc(func(_ context.Context, actorID string, _ diagnostics.ClientBatch) (int, error) {
		actor = actorID
		return 0, diagnostics.ErrClientRateLimited
	})
	log := slog.New(slog.DiscardHandler)
	handler := api.Router(log, api.Options{
		Auth: api.NewTokenAuthorizer("agent-token"), Log: log, ClientDiagnostics: service,
	})
	request := httptest.NewRequest(http.MethodPost, "/v1/diagnostics/client-events", strings.NewReader(`{
		"source":"android_tv","clientVersion":"0.1.0","platform":"shield_tv",
		"events":[{"event":"player.ready","occurredAt":1800000000000,
		"playbackSessionId":"play_1","channelId":"ch_1","transport":"media3"}]
	}`))
	request.Header.Set("Authorization", "Bearer agent-token")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("rate-limited status = %d, want 429: %s", response.Code, response.Body.String())
	}
	if actor != "api_token" {
		t.Fatalf("API-token actor = %q, want server-owned api_token", actor)
	}
}

func TestClientDiagnosticsRejectUnknownJSONBeforeModule(t *testing.T) {
	calls := 0
	service := clientDiagnosticServiceFunc(func(context.Context, string, diagnostics.ClientBatch) (int, error) {
		calls++
		return 1, nil
	})
	log := slog.New(slog.DiscardHandler)
	handler := api.Router(log, api.Options{
		Auth: diagnosticUserAuthorizer{}, Log: log, ClientDiagnostics: service,
	})
	request := httptest.NewRequest(http.MethodPost, "/v1/diagnostics/client-events", strings.NewReader(`{
		"source":"web","clientVersion":"dev","platform":"chromium",
		"events":[{"event":"client.unhandled_error","occurredAt":1800000000000,
		"surface":"root","errorClass":"type_error","cookie":"session=secret"}]
	}`))
	request.Header.Set("Authorization", "Bearer "+memberToken)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code < 400 || response.Code >= 500 {
		t.Fatalf("unknown-field status = %d, want 4xx: %s", response.Code, response.Body.String())
	}
	if calls != 0 {
		t.Fatalf("unknown field reached module %d times", calls)
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

func TestVerboseCaptureIsAdminOnlyBoundedAndStoppable(t *testing.T) {
	recorder := diagnostics.New(&diagnosticRecordSink{}, diagnostics.Options{})
	defer func() { _ = recorder.Close(t.Context()) }()
	log := slog.New(slog.DiscardHandler)
	handler := api.Router(log, api.Options{
		Auth: testAuthorizer{}, Log: log, DiagnosticCapture: recorder,
	})

	member := httptest.NewRequest(http.MethodPost, "/v1/diagnostics/verbose-capture", strings.NewReader(`{"durationMinutes":5}`))
	member.Header.Set("Authorization", "Bearer "+memberToken)
	member.Header.Set("Content-Type", "application/json")
	memberResponse := httptest.NewRecorder()
	handler.ServeHTTP(memberResponse, member)
	if memberResponse.Code != http.StatusForbidden {
		t.Fatalf("member status = %d, want 403", memberResponse.Code)
	}

	start := httptest.NewRequest(http.MethodPost, "/v1/diagnostics/verbose-capture", strings.NewReader(`{"durationMinutes":5,"subsystem":"player","channelId":"channel-1"}`))
	start.Header.Set("Authorization", "Bearer "+adminToken)
	start.Header.Set("Content-Type", "application/json")
	startResponse := httptest.NewRecorder()
	handler.ServeHTTP(startResponse, start)
	if startResponse.Code != http.StatusOK {
		t.Fatalf("start status = %d: %s", startResponse.Code, startResponse.Body.String())
	}
	var capture diagnostics.VerboseCapture
	if err := json.NewDecoder(startResponse.Body).Decode(&capture); err != nil {
		t.Fatal(err)
	}
	if !capture.Active || capture.Subsystem != "player" || capture.ChannelID != "channel-1" {
		t.Fatalf("capture = %#v", capture)
	}

	stop := httptest.NewRequest(http.MethodDelete, "/v1/diagnostics/verbose-capture", nil)
	stop.Header.Set("Authorization", "Bearer "+adminToken)
	stopResponse := httptest.NewRecorder()
	handler.ServeHTTP(stopResponse, stop)
	if stopResponse.Code != http.StatusOK || recorder.VerboseCapture().Active {
		t.Fatalf("stop status = %d, capture = %#v", stopResponse.Code, recorder.VerboseCapture())
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
