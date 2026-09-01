package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/store"
)

type pullBody struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	Note          string `json:"note"`
	EstimateClips int    `json:"estimateClips"`
	Plan          []struct {
		SourceID string `json:"sourceId"`
		Dropped  bool   `json:"dropped"`
	} `json:"plan"`
}

func decodePull(t *testing.T, res *http.Response) pullBody {
	t.Helper()
	var b pullBody
	if err := json.NewDecoder(res.Body).Decode(&b); err != nil {
		t.Fatal(err)
	}
	return b
}

func seedSource(t *testing.T, st store.Store, id, uri string, enabled bool) {
	t.Helper()
	ctx := context.Background()
	src := store.NewFillerSource(id, "archive", uri, id, time.Now().UTC())
	if err := st.UpsertFillerSource(ctx, src); err != nil {
		t.Fatal(err)
	}
	if !enabled {
		if err := st.SetFillerSourceEnabled(ctx, id, false); err != nil {
			t.Fatal(err)
		}
	}
}

// ⚠ THE safety property. §10's rule is "the machine proposes, a human commits", and this is what
// makes the first half true: proposing writes a row and downloads NOTHING.
func TestProposeFillerPull_DownloadsNothing(t *testing.T) {
	srv, st, ff := newFillerServer(t)
	seedSource(t, st, "classic", "https://archive.org/details/classic", true)

	res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/pulls", `{}`, adminToken)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	body := decodePull(t, res)

	if body.Status != "pending" {
		t.Errorf("status = %q, want pending", body.Status)
	}
	if len(ff.ingested) != 0 {
		t.Errorf("proposing downloaded %v — the gate exists so that this cannot happen", ff.ingested)
	}
	pulls, err := st.ListPulls(context.Background(), filler.PullPending)
	if err != nil || len(pulls) != 1 {
		t.Fatalf("store holds %d pending pulls (%v), want 1", len(pulls), err)
	}
}

// The mock writes an empty state for this precondition; it belongs on the server, because a pull
// composed from a switched-off source is one that can never run, and finding that out AFTER a
// human approved it is the worst moment.
func TestProposeFillerPull_RefusedWhenEverySourceIsOff(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	seedSource(t, st, "classic", "https://archive.org/details/classic", false)

	res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/pulls", `{}`, adminToken)
	if res.StatusCode != http.StatusConflict {
		t.Errorf("status = %d, want 409", res.StatusCode)
	}
	if pulls, _ := st.ListPulls(context.Background(), ""); len(pulls) != 0 {
		t.Errorf("wrote %d pulls that could never run", len(pulls))
	}
}

// The commit point. Approving is the ONLY path that enqueues, and it enqueues through the
// existing ingest job rather than a downloader of its own.
func TestApproveFillerPull_IsTheOnlyPathThatDownloads(t *testing.T) {
	srv, st, ff := newFillerServer(t)
	seedSource(t, st, "classic", "https://archive.org/details/classic", true)

	created := decodePull(t, sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/pulls", `{}`, adminToken))
	if len(ff.ingested) != 0 {
		t.Fatalf("downloaded before approval: %v", ff.ingested)
	}

	res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/pulls/"+created.ID+"/approve",
		`{"note":"no local dealers, no PSAs"}`, adminToken)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	body := decodePull(t, res)
	if body.Status != "approved" {
		t.Errorf("status = %q, want approved", body.Status)
	}
	if body.Note != "no local dealers, no PSAs" {
		t.Errorf("note = %q — the operator's narrowing instruction was lost", body.Note)
	}
	if len(ff.ingested) != 1 || ff.ingested[0] != "https://archive.org/details/classic" {
		t.Errorf("ingested %v, want the source's uri once", ff.ingested)
	}
	if ff.pullID != created.ID || len(ff.pullTargets) != 1 || ff.pullTargets[0].SourceID != "classic" || ff.pullTargets[0].Kind != "archive" {
		t.Errorf("pull attribution = id %q targets %+v, want approved pull and exact source", ff.pullID, ff.pullTargets)
	}
}

// A double-click, a retried request, or a second admin on the same queue must not enqueue the
// same downloads twice.
func TestApproveFillerPull_CannotBeApprovedTwice(t *testing.T) {
	srv, st, ff := newFillerServer(t)
	seedSource(t, st, "classic", "https://archive.org/details/classic", true)
	created := decodePull(t, sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/pulls", `{}`, adminToken))

	if res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/pulls/"+created.ID+"/approve", `{}`, adminToken); res.StatusCode != http.StatusOK {
		t.Fatalf("first approve: %d", res.StatusCode)
	}
	if res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/pulls/"+created.ID+"/approve", `{}`, adminToken); res.StatusCode != http.StatusConflict {
		t.Errorf("second approve = %d, want 409", res.StatusCode)
	}
	if len(ff.ingested) != 1 {
		t.Errorf("enqueued %d times, want 1 — an approved pull must not re-fetch", len(ff.ingested))
	}
}

// Dropping a row excludes it from the fetch AND is recorded on the pull. The record has to show
// what was proposed as well as what was agreed to, or "we approved this" loses the half that
// matters.
func TestApproveFillerPull_DroppedRowsAreExcludedButRecorded(t *testing.T) {
	srv, st, ff := newFillerServer(t)
	seedSource(t, st, "keep", "https://archive.org/details/keep", true)
	seedSource(t, st, "drop", "https://archive.org/details/drop", true)
	created := decodePull(t, sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/pulls", `{}`, adminToken))
	if len(created.Plan) != 2 {
		t.Fatalf("plan has %d rows, want 2", len(created.Plan))
	}

	body := decodePull(t, sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/pulls/"+created.ID+"/approve",
		`{"dropSourceIds":["drop"]}`, adminToken))

	if len(ff.ingested) != 1 || ff.ingested[0] != "https://archive.org/details/keep" {
		t.Errorf("ingested %v, want only the kept source", ff.ingested)
	}
	var sawDropped bool
	for _, r := range body.Plan {
		if r.SourceID == "drop" {
			sawDropped = r.Dropped
		}
	}
	if !sawDropped {
		t.Error("the dropped row is gone from the record — the audit must show what was proposed too")
	}
}

// Approving with everything dropped is refused, not recorded as an approval that fetched
// nothing: in the history those two are indistinguishable.
func TestApproveFillerPull_RefusesAnEmptyCommit(t *testing.T) {
	srv, st, ff := newFillerServer(t)
	seedSource(t, st, "classic", "https://archive.org/details/classic", true)
	created := decodePull(t, sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/pulls", `{}`, adminToken))

	res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/pulls/"+created.ID+"/approve",
		`{"dropSourceIds":["classic"]}`, adminToken)
	if res.StatusCode != http.StatusConflict {
		t.Errorf("status = %d, want 409", res.StatusCode)
	}
	if len(ff.ingested) != 0 {
		t.Errorf("ingested %v for an empty commit", ff.ingested)
	}
}

// ⚠ Re-checked at the COMMIT point. A source can be switched off while a pull sits in the queue,
// and approving into it would fetch from something the operator turned off.
func TestApproveFillerPull_RefusesASourceDisabledSinceProposal(t *testing.T) {
	srv, st, ff := newFillerServer(t)
	seedSource(t, st, "classic", "https://archive.org/details/classic", true)
	created := decodePull(t, sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/pulls", `{}`, adminToken))

	if err := st.SetFillerSourceEnabled(context.Background(), "classic", false); err != nil {
		t.Fatal(err)
	}

	res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/pulls/"+created.ID+"/approve", `{}`, adminToken)
	if res.StatusCode != http.StatusConflict {
		t.Errorf("status = %d, want 409", res.StatusCode)
	}
	if len(ff.ingested) != 0 {
		t.Errorf("fetched from a switched-off source: %v", ff.ingested)
	}
}

// Dismissing records the decision and downloads nothing. The row is KEPT — the history answers
// what was declined, too.
func TestDismissFillerPull_RecordsAndDownloadsNothing(t *testing.T) {
	srv, st, ff := newFillerServer(t)
	seedSource(t, st, "classic", "https://archive.org/details/classic", true)
	created := decodePull(t, sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/pulls", `{}`, adminToken))

	body := decodePull(t, sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/pulls/"+created.ID+"/dismiss", `{}`, adminToken))
	if body.Status != "dismissed" {
		t.Errorf("status = %q, want dismissed", body.Status)
	}
	if len(ff.ingested) != 0 {
		t.Errorf("dismissing downloaded %v", ff.ingested)
	}
	if pulls, _ := st.ListPulls(context.Background(), filler.PullDismissed); len(pulls) != 1 {
		t.Errorf("dismissed pulls = %d, want 1 — a decided pull is kept, not deleted", len(pulls))
	}
}

// §19 negatives. These routes decide what gets downloaded, so a member must not reach any of
// them — least of all approve.
func TestFillerPullRoutes_RequireAdmin(t *testing.T) {
	srv, st, ff := newFillerServer(t)
	seedSource(t, st, "classic", "https://archive.org/details/classic", true)
	created := decodePull(t, sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/pulls", `{}`, adminToken))

	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/v1/filler/pulls"},
		{http.MethodGet, "/v1/filler/pulls"},
		{http.MethodPost, "/v1/filler/pulls/" + created.ID + "/approve"},
		{http.MethodPost, "/v1/filler/pulls/" + created.ID + "/dismiss"},
	} {
		if res := sourceReq(t, tc.method, srv.URL+tc.path, `{}`, ""); res.StatusCode == http.StatusOK {
			t.Errorf("%s %s succeeded with no credential", tc.method, tc.path)
		}
	}
	if len(ff.ingested) != 0 {
		t.Errorf("an unauthenticated caller caused a download: %v", ff.ingested)
	}
}
