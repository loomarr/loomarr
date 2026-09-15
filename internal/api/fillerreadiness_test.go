package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/filler"
)

func TestFillerReadinessReturnsOneServerOwnedActionAndItsEvidence(t *testing.T) {
	srv, _, ff := newFillerServer(t)
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	ff.readiness = filler.ProjectReadiness(filler.ReadinessInput{
		Fetch: filler.FetchStatus{Enabled: true, CatalogClips: 12, MaxCatalog: 500},
		Pipeline: filler.PipelineOverview{
			Runnable: 2, NeedsDecision: 3, Ready: 9, Complete: 2, Rejected: 4, Recoverable: 1,
		},
		Pool: filler.PoolReport{
			Clips: 12, BreakBody: 10, Eligible: 8,
			Channels: []filler.ChannelCoverage{{
				ChannelID: "ch-7", Name: "Saturday Morning", Number: 7,
				Report: filler.CoverageReport{
					Level: filler.MatchAudience, Total: 6, DurationMs: 180_000, Categories: 3, Brands: 4,
				},
			}},
		},
		Runs: []filler.AcquisitionRun{{
			ID: "acq-1", Trigger: filler.AcquisitionPull, PullID: "pull-1",
			Status: filler.AcquisitionSuccess, Requested: 4, Fetched: 3,
			StartedAt: now.Add(-time.Minute), CompletedAt: now, UpdatedAt: now,
			Outcome:   filler.AcquisitionOutcome{Enrolled: 3, Preparing: 1, Ready: 2},
			Artifacts: filler.AcquisitionArtifactOutcome{Consumed: 3},
		}},
		// Two repair rows are higher-priority actionable work than the recoverable pipeline row.
		Repairs: filler.AcquisitionRepairSummary{Count: 2, LatestReason: "latest retained repair"},
	})

	res := sourceReq(t, http.MethodGet, srv.URL+"/v1/filler/readiness", "", memberToken)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var body api.FillerReadinessDTO
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.NextAction != "retry_acquisition" || body.ActionCount != 2 {
		t.Fatalf("next action = %q (%d), want the two repair actions", body.NextAction, body.ActionCount)
	}
	if body.Pipeline.NeedsDecision != 3 || body.Pipeline.Ready != 9 || body.Pipeline.Complete != 2 ||
		body.Pipeline.Rejected != 4 || body.Pipeline.Recoverable != 1 {
		t.Fatalf("pipeline ownership collapsed: %+v", body.Pipeline)
	}
	if len(body.Pool.Channels) != 1 || body.Pool.Channels[0].DurationMs != 180_000 || body.Pool.Channels[0].Brands != 4 {
		t.Fatalf("channel coverage = %+v, want duration and variety", body.Pool.Channels)
	}
	if len(body.Acquisitions) != 1 || body.Acquisitions[0].PullID != "pull-1" || body.Acquisitions[0].Outcome.Ready != 2 {
		t.Fatalf("acquisition trace = %+v", body.Acquisitions)
	}
	if body.Acquisitions[0].Artifacts.Consumed != 3 {
		t.Fatalf("acquisition artifact outcome = %+v", body.Acquisitions[0].Artifacts)
	}
	if body.Repairs.Count != 2 || body.Repairs.LatestReason != "latest retained repair" {
		t.Fatalf("repair summary = %+v", body.Repairs)
	}
}

func TestFillerAcquisitionReturnsDurableReconnectState(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	now := time.Date(2026, 9, 13, 14, 0, 0, 0, time.UTC)
	if err := st.UpsertAcquisitionRun(context.Background(), filler.AcquisitionRun{
		ID: "acq-found-clip", Trigger: filler.AcquisitionSource, SourceID: "archive:classic",
		Status: filler.AcquisitionError, Requested: 1, Failed: 1, Error: "provider timed out",
		StartedAt: now.Add(-time.Minute), CompletedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	if res := sourceReq(t, http.MethodGet, srv.URL+"/v1/filler/acquisitions/acq-found-clip", "", memberToken); res.StatusCode != http.StatusForbidden {
		_ = res.Body.Close()
		t.Fatalf("member acquisition read = %d, want 403", res.StatusCode)
	} else {
		_ = res.Body.Close()
	}

	res := sourceReq(t, http.MethodGet, srv.URL+"/v1/filler/acquisitions/acq-found-clip", "", adminToken)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("acquisition read = %d, want 200", res.StatusCode)
	}
	var body api.FillerAcquisitionRunDTO
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.ID != "acq-found-clip" || body.SourceID != "archive:classic" || body.Status != "error" || body.Error != "provider timed out" {
		t.Fatalf("acquisition reconnect state = %+v", body)
	}
}
