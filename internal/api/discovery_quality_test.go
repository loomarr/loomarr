package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/quality"
)

func TestDiscoveryQualityExportIsAdminOnlyAndOmitsReceipts(t *testing.T) {
	srv, st := newServer(t)
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	snapshot := quality.RunSnapshot{
		ID: "run-1", SchemaVersion: 1, CorpusVersion: "corpus-1", RequestedModel: "model-1",
		Provider: quality.ProviderOllama, BudgetProfile: "default", ApplicationVersion: "v1", CreatedAt: now,
	}
	if err := st.PutQualityRunSnapshot(t.Context(), snapshot); err != nil {
		t.Fatal(err)
	}
	const receipt = "private-receipt-key"
	if err := st.RecordQualityObservation(t.Context(), quality.Observation{
		IdempotencyKey: receipt, At: now, Stage: quality.StageRetrieval, Outcome: quality.OutcomeSucceeded,
		Duration: 250 * time.Millisecond, CandidateCount: 7, RunSnapshotID: snapshot.ID,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.MaintainQualityLedger(t.Context(), now.Add(31*24*time.Hour)); err != nil {
		t.Fatal(err)
	}

	member := do(t, srv, http.MethodGet, "/v1/system/discovery-quality/export", memberToken, "")
	defer member.Body.Close()
	if member.StatusCode != http.StatusForbidden {
		t.Fatalf("member status = %d, want 403", member.StatusCode)
	}

	admin := do(t, srv, http.MethodGet, "/v1/system/discovery-quality/export", adminToken, "")
	defer admin.Body.Close()
	if admin.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(admin.Body)
		t.Fatalf("admin status = %d, want 200: %s", admin.StatusCode, body)
	}
	body, err := io.ReadAll(admin.Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), receipt) {
		t.Fatalf("export exposed observation receipt: %s", body)
	}
	var export quality.Export
	if err := json.Unmarshal(body, &export); err != nil {
		t.Fatal(err)
	}
	if len(export.Aggregates) != 1 || export.Aggregates[0].CandidateCount != 7 {
		t.Fatalf("aggregates = %+v, want one retrieval aggregate", export.Aggregates)
	}
	if len(export.RunSnapshots) != 1 || export.RunSnapshots[0].ID != snapshot.ID {
		t.Fatalf("run snapshots = %+v, want %q", export.RunSnapshots, snapshot.ID)
	}
	if got := admin.Header.Get("Content-Disposition"); got != `attachment; filename="loomarr-discovery-quality.json"` {
		t.Fatalf("Content-Disposition = %q", got)
	}
}
