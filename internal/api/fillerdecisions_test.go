package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filleradmission"
	"github.com/loomarr/loomarr/internal/fillerdecision"
	"github.com/loomarr/loomarr/internal/store"
)

type decisionListBody[T any] struct {
	Rows  []T `json:"rows"`
	Total int `json:"total"`
}

type diagnosticWire struct {
	ID, ClipHash, Code string
	Retryable          bool
	Recovery           struct {
		Action, Mode, Destination string
		RetryAt                   *time.Time `json:"retryAt"`
	}
}

type activityWire struct {
	ID, ActionID, DecisionID, ClipHash, Kind string
}

func TestFillerDecisionProjectionsKeepAuditAndDiagnosticsOutOfIncoming(t *testing.T) {
	srv, st := newServer(t)
	seedDecisionAPI(t, st)

	res := do(t, srv, http.MethodGet, "/v1/filler/decisions/overview", memberToken, "")
	var overview struct {
		NextAction  string `json:"nextAction"`
		ActionCount int    `json:"actionCount"`
		Counts      struct{ UnresolvedReviews, Operational int }
	}
	decodeDecisionResponse(t, res, &overview)
	if overview.NextAction != "retry_processing" || overview.ActionCount != 1 ||
		overview.Counts.UnresolvedReviews != 1 || overview.Counts.Operational != 1 {
		t.Fatalf("overview = %+v", overview)
	}

	res = do(t, srv, http.MethodGet, "/v1/filler/decisions/diagnostics", memberToken, "")
	if res.StatusCode != http.StatusForbidden {
		_ = res.Body.Close()
		t.Fatalf("member diagnostics = %d, want 403", res.StatusCode)
	}
	_ = res.Body.Close()

	res = do(t, srv, http.MethodGet, "/v1/filler/decisions/diagnostics?limit=10", adminToken, "")
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("diagnostics status = %d: %s", res.StatusCode, raw)
	}
	if strings.Contains(string(raw), "provider-secret") || strings.Contains(string(raw), "/private/path") {
		t.Fatalf("diagnostics leaked provider/path detail: %s", raw)
	}
	var diagnostics decisionListBody[diagnosticWire]
	if err := json.Unmarshal(raw, &diagnostics); err != nil {
		t.Fatal(err)
	}
	if diagnostics.Total != 1 || diagnostics.Rows[0].Recovery.Destination != "/settings/ai" {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}

	res = do(t, srv, http.MethodGet, "/v1/filler/decisions/activity?limit=10", memberToken, "")
	raw, err = io.ReadAll(res.Body)
	_ = res.Body.Close()
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("activity = %d, %v", res.StatusCode, err)
	}
	if strings.Contains(string(raw), "applicationMode") || strings.Contains(string(raw), "shadow") {
		t.Fatalf("activity exposed retired rollout vocabulary: %s", raw)
	}
	var activity decisionListBody[activityWire]
	if err := json.Unmarshal(raw, &activity); err != nil {
		t.Fatal(err)
	}
	if activity.Total != 1 || activity.Rows[0].Kind != "review_requested" {
		t.Fatalf("activity = %+v", activity)
	}

	for _, request := range []struct{ method, path, body string }{
		{http.MethodGet, "/v1/filler/attention", ""},
		{http.MethodPost, "/v1/filler/attention/review-1/actions", `{"actionId":"retired","kind":"admit"}`},
	} {
		res = do(t, srv, request.method, request.path, adminToken, request.body)
		if res.StatusCode != http.StatusNotFound {
			_ = res.Body.Close()
			t.Fatalf("retired %s %s = %d, want 404", request.method, request.path, res.StatusCode)
		}
		_ = res.Body.Close()
	}
}

func TestFillerDiagnosticRecoveryRequiresAdminAndIsIdempotent(t *testing.T) {
	srv, st := newServer(t)
	at := time.Date(2026, 9, 12, 16, 0, 0, 0, time.UTC)
	if err := st.PutFillerDecision(t.Context(), fillerdecision.Record{
		ID: "retryable-hold", ClipHash: "clip-retry", EvidenceHash: "evidence-retry",
		EvidenceVersion: "e1", SchemaVersion: filleradmission.SchemaVersion,
		PolicyVersion: "p1", TaxonomyVersion: "t1", ApplicationMode: fillerdecision.ApplicationModeShadow,
		CreatedAt: at, Result: filleradmission.Result{Hold: &filleradmission.Hold{
			Code: filleradmission.HoldExtractionFailed, Retryable: true,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	body := `{"actionId":"diagnostic-retry-1","action":"retry"}`
	res := do(t, srv, http.MethodPost, "/v1/filler/decisions/diagnostics/retryable-hold/actions", memberToken, body)
	if res.StatusCode != http.StatusForbidden {
		_ = res.Body.Close()
		t.Fatalf("member recovery = %d, want 403", res.StatusCode)
	}
	_ = res.Body.Close()
	if _, found, err := st.FindFillerDiagnosticRecovery(t.Context(), "diagnostic-retry-1"); err != nil || found {
		t.Fatalf("member recovery persisted = %v, %v", found, err)
	}
	for range 2 {
		res = do(t, srv, http.MethodPost, "/v1/filler/decisions/diagnostics/retryable-hold/actions", adminToken, body)
		if res.StatusCode != http.StatusOK {
			raw, _ := io.ReadAll(res.Body)
			_ = res.Body.Close()
			t.Fatalf("admin recovery = %d: %s", res.StatusCode, raw)
		}
		_ = res.Body.Close()
	}
	request, found, err := st.FindFillerDiagnosticRecovery(t.Context(), "diagnostic-retry-1")
	if err != nil || !found || request.ActorID != "api-token" {
		t.Fatalf("recorded diagnostic recovery = %+v, %v, %v", request, found, err)
	}

	res = do(t, srv, http.MethodGet, "/v1/filler/decisions/diagnostics?limit=10", adminToken, "")
	var diagnostics decisionListBody[diagnosticWire]
	decodeDecisionResponse(t, res, &diagnostics)
	if diagnostics.Total != 1 || diagnostics.Rows[0].Recovery.Mode != "automatic_retry" || diagnostics.Rows[0].Recovery.RetryAt == nil {
		t.Fatalf("hold did not update to its retry schedule: %+v", diagnostics)
	}
}

func TestFillerDecisionProjectionsUseLatestOutcomeWithoutErasingHistory(t *testing.T) {
	srv, st := newServer(t)
	at := time.Date(2026, 8, 25, 5, 0, 0, 0, time.UTC)
	for _, record := range []fillerdecision.Record{
		{ID: "hold-old", ClipHash: "clip-recovered", EvidenceHash: "evidence-old", EvidenceVersion: "e1", SchemaVersion: 1,
			PolicyVersion: "p1", TaxonomyVersion: "t1", ApplicationMode: fillerdecision.ApplicationModeShadow, CreatedAt: at,
			Result: filleradmission.Result{Hold: &filleradmission.Hold{Code: filleradmission.HoldProviderUnavailable, Retryable: true}}},
		{ID: "admit-new", ClipHash: "clip-recovered", EvidenceHash: "evidence-new", EvidenceVersion: "e1", SchemaVersion: 1,
			PolicyVersion: "p1", TaxonomyVersion: "t1", ApplicationMode: fillerdecision.ApplicationModeShadow, CreatedAt: at.Add(time.Second),
			Result: filleradmission.Result{Decision: &filleradmission.Decision{Verdict: filleradmission.VerdictAdmit,
				ReasonCodes: []filleradmission.ReasonCode{filleradmission.ReasonEvidenceSatisfied}}}},
	} {
		if err := st.PutFillerDecision(t.Context(), record); err != nil {
			t.Fatal(err)
		}
	}

	res := do(t, srv, http.MethodGet, "/v1/filler/decisions/diagnostics?limit=10", adminToken, "")
	var diagnostics decisionListBody[diagnosticWire]
	decodeDecisionResponse(t, res, &diagnostics)
	if diagnostics.Total != 0 {
		t.Fatalf("recovered hold remained in diagnostics: %+v", diagnostics)
	}
	res = do(t, srv, http.MethodGet, "/v1/filler/decisions/activity?limit=10", memberToken, "")
	var activity decisionListBody[activityWire]
	decodeDecisionResponse(t, res, &activity)
	if activity.Total != 1 || activity.Rows[0].Kind != "automatic_admit" {
		t.Fatalf("latest projection erased or mislabeled history: %+v", activity)
	}
}

func seedDecisionAPI(t *testing.T, st store.Store) {
	t.Helper()
	at := time.Date(2026, 8, 25, 5, 0, 0, 0, time.UTC)
	if err := st.PutFillerDecision(t.Context(), fillerdecision.Record{
		ID: "review-1", ClipHash: "clip-review", EvidenceHash: "evidence-review", EvidenceVersion: "e1",
		SchemaVersion: 1, PolicyVersion: "p1", TaxonomyVersion: "t1", ApplicationMode: fillerdecision.ApplicationModeShadow,
		CreatedAt: at, Result: filleradmission.Result{Decision: &filleradmission.Decision{
			Verdict: filleradmission.VerdictReview, ReasonCodes: []filleradmission.ReasonCode{filleradmission.ReasonConflictRecordingDate},
			EvidenceRefs: []string{"filename-year", "spoken-year"}, ReviewQuestion: "Which date describes when this clip was recorded?",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.PutFillerDecision(t.Context(), fillerdecision.Record{
		ID: "hold-1", ClipHash: "clip-hold", EvidenceHash: "evidence-hold", EvidenceVersion: "e1",
		SchemaVersion: 1, PolicyVersion: "p1", TaxonomyVersion: "t1", ApplicationMode: fillerdecision.ApplicationModeShadow,
		CreatedAt: at.Add(time.Second), Result: filleradmission.Result{Hold: &filleradmission.Hold{
			Code: filleradmission.HoldProviderUnavailable, Detail: "provider-secret failed while opening /private/path", Retryable: true,
		}},
	}); err != nil {
		t.Fatal(err)
	}
}

func decodeDecisionResponse(t *testing.T, res *http.Response, target any) {
	t.Helper()
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(res.Body)
		t.Fatalf("status = %d: %s", res.StatusCode, raw)
	}
	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		t.Fatal(err)
	}
}
