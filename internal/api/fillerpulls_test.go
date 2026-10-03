package api_test

import (
	"context"
	"encoding/json"
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
