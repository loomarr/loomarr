package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/fillerstore"
)

type pullBody struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	Note          string `json:"note"`
	EstimateClips int    `json:"estimateClips"`
	Plan          []struct {
		CandidateID string `json:"candidateId"`
		SourceID    string `json:"sourceId"`
		RemoteID    string `json:"remoteId"`
		URL         string `json:"url"`
		Dropped     bool   `json:"dropped"`
	} `json:"plan"`
	Rejected []struct {
		RemoteID    string `json:"remoteId"`
		Disposition string `json:"disposition"`
	} `json:"rejected"`
}

func decodePull(t *testing.T, res *http.Response) pullBody {
	t.Helper()
	var b pullBody
	if err := json.NewDecoder(res.Body).Decode(&b); err != nil {
		t.Fatal(err)
	}
	return b
}

func seedSource(t *testing.T, st fillerstore.Store, id, uri string, enabled bool) {
	t.Helper()
	ctx := context.Background()
	src := fillerstore.NewFillerSource(id, "archive", uri, id, time.Now().UTC())
	if err := st.UpsertFillerSource(ctx, src); err != nil {
		t.Fatal(err)
	}
	if !enabled {
		if err := st.SetFillerSourceEnabled(ctx, id, false); err != nil {
			t.Fatal(err)
		}
	}
}

// seedPull persists a pending pull directly, bypassing the removed propose-filler-pull endpoint
// (#1743): nothing calls it, so these tests seed the queue the way a pre-V66 or operator-composed
// pull would already be stored, and exercise approve/dismiss — the operations that remain.
func seedPull(t *testing.T, st fillerstore.Store, id string, plan []filler.PullPlanRow) filler.Pull {
	t.Helper()
	p := filler.Pull{ID: id, Status: filler.PullPending, CreatedAt: time.Now().UTC(), Plan: plan}
	if err := st.UpsertPull(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestApproveFillerPull_DropsOneCandidateWithoutDroppingItsSource(t *testing.T) {
	srv, st, ff := newFillerServer(t)
	seedSource(t, st, "classic", "https://archive.org/details/classic", true)
	created := seedPull(t, st, "pull-drop-one", []filler.PullPlanRow{
		{SourceID: "classic", Provider: "archive", RemoteID: "one", URL: "https://archive.org/details/one"},
		{SourceID: "classic", Provider: "archive", RemoteID: "two", URL: "https://archive.org/details/two"},
	})
	drop := created.Plan[0]
	res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/pulls/"+created.ID+"/approve",
		`{"dropCandidateIds":["`+drop.CandidateID()+`"]}`, adminToken)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if len(ff.ingested) != 1 || ff.ingested[0] == drop.URL {
		t.Fatalf("ingested = %v, should contain only the other candidate", ff.ingested)
	}
}

// The commit point. Approving is the ONLY path that enqueues, and it enqueues through the
// existing ingest job rather than a downloader of its own.
func TestApproveFillerPull_IsTheOnlyPathThatDownloads(t *testing.T) {
	srv, st, ff := newFillerServer(t)
	seedSource(t, st, "classic", "https://archive.org/details/classic", true)
	created := seedPull(t, st, "pull-only-download-path", []filler.PullPlanRow{{SourceID: "classic", Name: "Classic collection"}})
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

// A retry after the first decision is durable must not enqueue the same downloads twice.
// The concurrent boundary is covered separately with two requests held at the commit point.
func TestApproveFillerPull_CannotBeApprovedTwice(t *testing.T) {
	srv, st, ff := newFillerServer(t)
	seedSource(t, st, "classic", "https://archive.org/details/classic", true)
	created := seedPull(t, st, "pull-cannot-approve-twice", []filler.PullPlanRow{{SourceID: "classic", Name: "Classic collection"}})

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

func TestApproveFillerPull_RevalidatesCandidateAgainstOtherQueuedWork(t *testing.T) {
	srv, st, ff := newFillerServer(t)
	seedSource(t, st, "classic", "https://archive.org/details/classic", true)
	created := seedPull(t, st, "pull-revalidate", []filler.PullPlanRow{
		{SourceID: "classic", Provider: "archive", RemoteID: "same", URL: "https://archive.org/details/same"},
	})
	other := created
	other.ID = "other-pull"
	other.CreatedAt = other.CreatedAt.Add(time.Second)
	if err := st.UpsertPull(t.Context(), other); err != nil {
		t.Fatal(err)
	}

	res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/pulls/"+created.ID+"/approve", `{}`, adminToken)
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", res.StatusCode)
	}
	if len(ff.ingested) != 0 {
		t.Fatalf("revalidation still ingested %v", ff.ingested)
	}
}

// Dropping a row excludes it from the fetch AND is recorded on the pull. The record has to show
// what was proposed as well as what was agreed to, or "we approved this" loses the half that
// matters.
func TestApproveFillerPull_DroppedRowsAreExcludedButRecorded(t *testing.T) {
	srv, st, ff := newFillerServer(t)
	seedSource(t, st, "keep", "https://archive.org/details/keep", true)
	seedSource(t, st, "drop", "https://archive.org/details/drop", true)
	created := seedPull(t, st, "pull-dropped-rows", []filler.PullPlanRow{
		{SourceID: "keep", Name: "Keep"},
		{SourceID: "drop", Name: "Drop"},
	})

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
	created := seedPull(t, st, "pull-empty-commit", []filler.PullPlanRow{{SourceID: "classic", Name: "Classic collection"}})

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
	created := seedPull(t, st, "pull-source-disabled", []filler.PullPlanRow{{SourceID: "classic", Name: "Classic collection"}})

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

func TestApproveFillerPull_RefusesAProviderPausedSinceProposal(t *testing.T) {
	srv, st, ff := newFillerServer(t)
	seedSource(t, st, "classic", "https://archive.org/details/classic", true)
	created := seedPull(t, st, "pull-provider-paused", []filler.PullPlanRow{{SourceID: "classic", Name: "Classic collection"}})

	if err := st.SetFillerProviderEnabled(t.Context(), "archive", false); err != nil {
		t.Fatal(err)
	}

	res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/pulls/"+created.ID+"/approve", `{}`, adminToken)
	if res.StatusCode != http.StatusConflict {
		t.Errorf("status = %d, want 409", res.StatusCode)
	}
	if len(ff.ingested) != 0 {
		t.Errorf("fetched from a paused provider: %v", ff.ingested)
	}
}

// Dismissing records the decision and downloads nothing. The row is KEPT — the history answers
// what was declined, too.
func TestDismissFillerPull_RecordsAndDownloadsNothing(t *testing.T) {
	srv, st, ff := newFillerServer(t)
	seedSource(t, st, "classic", "https://archive.org/details/classic", true)
	created := seedPull(t, st, "pull-dismiss", []filler.PullPlanRow{{SourceID: "classic", Name: "Classic collection"}})

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
	created := seedPull(t, st, "pull-require-admin", []filler.PullPlanRow{{SourceID: "classic", Name: "Classic collection"}})

	for _, tc := range []struct{ method, path string }{
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

// Both requests pass the read-side guard before either reaches durable approval.
func TestApproveFillerPull_ConcurrentDecision(t *testing.T) {
	srv, st, ff := newFillerServer(t)
	seedSource(t, st, "classic", "https://archive.org/details/classic", true)
	created := seedPull(t, st, "pull-concurrent-decision", []filler.PullPlanRow{{SourceID: "classic", Name: "Classic collection"}})
	entered, release := make(chan struct{}, 2), make(chan struct{})
	ff.beforePull = func() { entered <- struct{}{}; <-release }
	results := make(chan int, 2)
	for range 2 {
		go func() {
			res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/pulls/"+created.ID+"/approve", `{"note":"concurrent approval"}`, adminToken)
			_ = res.Body.Close()
			results <- res.StatusCode
		}()
	}
	for range 2 {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			close(release)
			t.Fatal("both requests did not reach the approval decision boundary")
		}
	}
	close(release)
	statuses := map[int]int{}
	for range 2 {
		statuses[<-results]++
	}
	if statuses[http.StatusOK] != 1 || statuses[http.StatusConflict] != 1 {
		t.Fatalf("approval responses = %v, want one success and one conflict", statuses)
	}
	if len(ff.ingested) != 1 {
		t.Fatalf("downloads = %d, want one", len(ff.ingested))
	}
	runs, err := st.ListAcquisitionRuns(t.Context(), 10, time.Now().UTC())
	if err != nil || len(runs) != 1 || runs[0].PullID != created.ID || runs[0].Status != filler.AcquisitionQueued {
		t.Fatalf("durable runs = %+v (%v), want exactly one queued run for the pull", runs, err)
	}
	decision, err := st.GetPull(t.Context(), created.ID)
	if err != nil || decision.Status != filler.PullApproved || decision.Note != "concurrent approval" || decision.DecidedAt.IsZero() {
		t.Fatalf("committed decision = %+v (%v)", decision, err)
	}
}

func TestApproveFillerPull_HistoricalSourcePlan(t *testing.T) {
	srv, st, ff := newFillerServer(t)
	seedSource(t, st, "classic", "https://archive.org/details/classic", true)
	p := filler.Pull{ID: "historical-source-plan", Status: filler.PullPending, CreatedAt: time.Now().UTC(), Plan: []filler.PullPlanRow{{SourceID: "classic", Name: "Classic collection"}}}
	if err := st.UpsertPull(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/pulls/"+p.ID+"/approve", `{}`, adminToken)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("historical approval = %d", res.StatusCode)
	}
	if len(ff.ingested) != 1 || ff.ingested[0] != "https://archive.org/details/classic" {
		t.Fatalf("historical source target = %v", ff.ingested)
	}
}

func TestApproveFillerPull_ConcurrentDismissalWins(t *testing.T) {
	srv, st, ff := newFillerServer(t)
	seedSource(t, st, "classic", "https://archive.org/details/classic", true)
	created := seedPull(t, st, "pull-concurrent-dismissal", []filler.PullPlanRow{{SourceID: "classic", Name: "Classic collection"}})
	entered, release := make(chan struct{}), make(chan struct{})
	releaseApproval := sync.OnceFunc(func() { close(release) })
	defer releaseApproval()
	ff.beforePull = func() { close(entered); <-release }
	result := make(chan int, 1)
	go func() {
		res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/pulls/"+created.ID+"/approve", `{}`, adminToken)
		_ = res.Body.Close()
		result <- res.StatusCode
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("approval did not reach decision boundary")
	}
	res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/pulls/"+created.ID+"/dismiss", `{}`, adminToken)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("dismiss = %d", res.StatusCode)
	}
	releaseApproval()
	if status := <-result; status != http.StatusConflict {
		t.Fatalf("losing approval = %d", status)
	}
	if len(ff.ingested) != 0 {
		t.Fatalf("losing approval downloaded %v", ff.ingested)
	}
	runs, err := st.ListAcquisitionRuns(t.Context(), 10, time.Now().UTC())
	if err != nil || len(runs) != 0 {
		t.Fatalf("dismissed pull has runs: %+v (%v)", runs, err)
	}
}

// --- bulk approve (Refs #1659) ---

type bulkApproveFillerPullsBody struct {
	Approved int `json:"approved"`
	Results  []struct {
		ID    string `json:"id"`
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	} `json:"results"`
}

func decodeBulkApproveFillerPulls(t *testing.T, res *http.Response) bulkApproveFillerPullsBody {
	t.Helper()
	var b bulkApproveFillerPullsBody
	if err := json.NewDecoder(res.Body).Decode(&b); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestBulkApproveFillerPulls_AllSucceed(t *testing.T) {
	srv, st, ff := newFillerServer(t)
	seedSource(t, st, "classic", "https://archive.org/details/classic", true)
	p1 := seedPull(t, st, "pull-bulk-1", []filler.PullPlanRow{{SourceID: "classic", Name: "Classic collection"}})
	p2 := seedPull(t, st, "pull-bulk-2", []filler.PullPlanRow{{SourceID: "classic", Name: "Classic collection"}})

	res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/pulls/approve",
		`{"ids":["`+p1.ID+`","`+p2.ID+`"]}`, adminToken)
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("status = %d (%s), want 200", res.StatusCode, body)
	}
	out := decodeBulkApproveFillerPulls(t, res)
	if out.Approved != 2 {
		t.Errorf("approved = %d, want 2 (results: %+v)", out.Approved, out.Results)
	}
	for _, r := range out.Results {
		if !r.OK {
			t.Errorf("%s did not approve: %q", r.ID, r.Error)
		}
	}
	if len(ff.ingested) != 2 {
		t.Errorf("ingested %v, want 2 downloads — each id must reuse the single-approve path", ff.ingested)
	}
	for _, id := range []string{p1.ID, p2.ID} {
		p, err := st.GetPull(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		if p.Status != filler.PullApproved {
			t.Errorf("%s status = %q, want approved", id, p.Status)
		}
	}
}

// One already-decided id must not abort the rest, but the caller still has to learn which ids
// did not go through — mirrors TestBulkApprove_PartialFailureReportsPerID.
func TestBulkApproveFillerPulls_PartialFailureReportsPerID(t *testing.T) {
	srv, st, ff := newFillerServer(t)
	seedSource(t, st, "classic", "https://archive.org/details/classic", true)
	p1 := seedPull(t, st, "pull-bulk-partial-1", []filler.PullPlanRow{{SourceID: "classic", Name: "Classic collection"}})
	p2 := seedPull(t, st, "pull-bulk-partial-2", []filler.PullPlanRow{{SourceID: "classic", Name: "Classic collection"}})
	// p1 is already decided before the bulk call.
	if res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/pulls/"+p1.ID+"/approve", `{}`, adminToken); res.StatusCode != http.StatusOK {
		t.Fatalf("seed approve: %d", res.StatusCode)
	}

	res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/pulls/approve",
		`{"ids":["`+p1.ID+`","`+p2.ID+`"]}`, adminToken)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("bulk with partial failures = %d, want 200 (failures are data, not a request error)", res.StatusCode)
	}
	out := decodeBulkApproveFillerPulls(t, res)
	if out.Approved != 1 {
		t.Errorf("approved = %d, want 1 (results: %+v)", out.Approved, out.Results)
	}
	byID := map[string]bool{}
	for _, r := range out.Results {
		byID[r.ID] = r.OK
		if !r.OK && r.Error == "" {
			t.Errorf("%s failed with no reason", r.ID)
		}
	}
	if byID[p1.ID] || !byID[p2.ID] {
		t.Errorf("results = %+v; want %s failed, %s approved", out.Results, p1.ID, p2.ID)
	}
	if len(ff.ingested) != 2 {
		t.Errorf("ingested %v, want 2 (one from the seed approve of %s, one from the bulk approve of %s) — the already-decided pull must not re-download a second time", ff.ingested, p1.ID, p2.ID)
	}
}

func TestBulkApproveFillerPulls_UnknownID(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	seedSource(t, st, "classic", "https://archive.org/details/classic", true)
	p1 := seedPull(t, st, "pull-bulk-unknown", []filler.PullPlanRow{{SourceID: "classic", Name: "Classic collection"}})

	res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/pulls/approve",
		`{"ids":["`+p1.ID+`","does-not-exist"]}`, adminToken)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	out := decodeBulkApproveFillerPulls(t, res)
	if out.Approved != 1 {
		t.Errorf("approved = %d, want 1 (results: %+v)", out.Approved, out.Results)
	}
	byID := map[string]bool{}
	errs := map[string]string{}
	for _, r := range out.Results {
		byID[r.ID] = r.OK
		errs[r.ID] = r.Error
	}
	if !byID[p1.ID] {
		t.Errorf("results = %+v; want %s approved", out.Results, p1.ID)
	}
	if byID["does-not-exist"] || errs["does-not-exist"] == "" {
		t.Errorf("results = %+v; want does-not-exist to fail with a reason", out.Results)
	}
}

// §19: the gate is admin-only, and bulk is still the gate. A member must approve NOTHING.
func TestBulkApproveFillerPulls_MemberIsRejected(t *testing.T) {
	srv, st, ff := newFillerServer(t)
	seedSource(t, st, "classic", "https://archive.org/details/classic", true)
	created := seedPull(t, st, "pull-bulk-member", []filler.PullPlanRow{{SourceID: "classic", Name: "Classic collection"}})

	for _, tok := range []string{"", memberToken} {
		res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/pulls/approve", `{"ids":["`+created.ID+`"]}`, tok)
		if res.StatusCode != http.StatusUnauthorized && res.StatusCode != http.StatusForbidden {
			t.Errorf("bulk approve with token %q = %d, want 401/403", tok, res.StatusCode)
		}
		_ = res.Body.Close()
	}
	if len(ff.ingested) != 0 {
		t.Errorf("a rejected bulk approve downloaded %v", ff.ingested)
	}
}

func TestBulkApproveFillerPulls_EmptyIDsRejected(t *testing.T) {
	srv, _, _ := newFillerServer(t)
	res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/pulls/approve", `{"ids":[]}`, adminToken)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode < 400 || res.StatusCode >= 500 {
		t.Errorf("empty ids = %d, want a 4xx client error", res.StatusCode)
	}
}

// Consistent with bulk-approve-proposals in spirit (same per-id gate), but capped: a group of
// filler downloads can exceed the 24-25 used for interactive lookups, so this bounds request
// size without forcing the client to chunk (Refs #1659).
func TestBulkApproveFillerPulls_BatchSizeCap(t *testing.T) {
	srv, _, _ := newFillerServer(t)
	ids := make([]string, 0, 101)
	for i := 0; i < 101; i++ {
		ids = append(ids, fmt.Sprintf("pull-%d", i))
	}
	body, err := json.Marshal(struct {
		IDs []string `json:"ids"`
	}{IDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/pulls/approve", string(body), adminToken)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode < 400 || res.StatusCode >= 500 {
		t.Errorf("101 ids = %d, want a 4xx client error — the cap must reject before any approval runs", res.StatusCode)
	}
}
