package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/loomarr/loomarr/internal/filler"
)

func TestApproveFillerPull_NoteIsAnAnnotationAndTargetsStayExact(t *testing.T) {
	srv, st, ff := newFillerServer(t)
	seedSource(t, st, "classic", "https://archive.org/details/classic", true)
	created := seedPull(t, st, "pull-note-exact-target", []filler.PullPlanRow{
		{SourceID: "classic", Provider: "archive", RemoteID: "reel-1", URL: "https://archive.org/details/reel-1"},
	})
	approved := decodePull(t, sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/pulls/"+created.ID+"/approve",
		`{"note":"reviewed by programming"}`, adminToken))

	if approved.Note != "reviewed by programming" {
		t.Fatalf("note = %q, want annotation preserved", approved.Note)
	}
	if len(ff.pullTargets) != 1 || ff.pullTargets[0].RemoteID != "reel-1" || ff.pullTargets[0].URL != "https://archive.org/details/reel-1" {
		t.Fatalf("ingest targets = %+v, want unchanged exact candidate", ff.pullTargets)
	}
}

// #749: the coverage gap a candidate was selected for is kept on the pending pull and reaches
// the approved download, so what the operator agreed to and what arrives share one record.
func TestApproveFillerPull_TargetsCarryTheGapTheirCandidateFills(t *testing.T) {
	srv, st, ff := newFillerServer(t)
	seedSource(t, st, "classic", "https://archive.org/details/classic", true)
	created := seedPull(t, st, "pull-gap-carried", []filler.PullPlanRow{
		{SourceID: "classic", Provider: "archive", RemoteID: "in-gap", URL: "https://archive.org/details/in-gap", Gap: "era:1990-1999"},
		{SourceID: "classic", Provider: "archive", RemoteID: "modern", URL: "https://archive.org/details/modern"},
	})
	gaps := map[string]string{}
	for _, row := range created.Plan {
		gaps[row.RemoteID] = row.Gap
	}
	if gaps["in-gap"] != "era:1990-1999" || gaps["modern"] != "" {
		t.Fatalf("pending plan gaps = %v, want only the in-gap item tagged", gaps)
	}
	decodePull(t, sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/pulls/"+created.ID+"/approve", `{}`, adminToken))
	approved := map[string]string{}
	for _, target := range ff.pullTargets {
		approved[target.RemoteID] = target.Gap
	}
	if approved["in-gap"] != "era:1990-1999" || approved["modern"] != "" || len(approved) != 2 {
		t.Fatalf("approved target gaps = %v, want the plan's gaps carried to ingest", approved)
	}
}

func TestApproveFillerPull_NoteIsAnAnnotationForLegacySourceLevelPlan(t *testing.T) {
	srv, st, ff := newFillerServer(t)
	seedSource(t, st, "classic", "https://archive.org/details/classic", true)
	pull := filler.Pull{
		ID: "legacy-pull", Status: filler.PullPending, Plan: []filler.PullPlanRow{{
			SourceID: "classic", Name: "Classic collection",
		}},
	}
	if err := st.UpsertPull(context.Background(), pull); err != nil {
		t.Fatal(err)
	}
	approved := decodePull(t, sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/pulls/legacy-pull/approve",
		`{"note":"legacy review annotation"}`, adminToken))

	if approved.Note != "legacy review annotation" {
		t.Fatalf("note = %q, want annotation preserved", approved.Note)
	}
	if len(ff.pullTargets) != 1 || ff.pullTargets[0].URL != "https://archive.org/details/classic" {
		t.Fatalf("legacy ingest targets = %+v, want registered source URL", ff.pullTargets)
	}
}
