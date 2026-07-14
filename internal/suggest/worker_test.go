package suggest_test

import (
	"context"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/catalog"
	"github.com/mantonx/loomarr/internal/library"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/suggest"
	"github.com/mantonx/loomarr/internal/testkit"
	"github.com/mantonx/loomarr/internal/tmdb"
)

func newStore(t *testing.T) store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+filepath.Join(t.TempDir(), "s.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func idGen() func() string {
	var n int64
	return func() string { return "id-" + strconv.FormatInt(atomic.AddInt64(&n, 1), 10) }
}

func buildService(t *testing.T, st store.Store, llmMock *testkit.LLM) *suggest.Service {
	ms := testkit.NewMediaServer(t)
	lib := library.New(library.Emby, ms.URL, ms.AdminToken, "dev-1")
	mt := testkit.NewTMDB(t)
	tm := tmdb.NewWithBase(mt.URL, "key")
	cat := catalog.New(lib, tm, nil)
	sug := suggest.New(llmMock, cat, tm, 10)
	return suggest.NewService(st, sug, suggest.Config{Workers: 2, Timeout: time.Second, CacheTTL: time.Hour},
		idGen(), time.Now, testkit.Logger())
}

// Submit caches by intent hash — but ONLY a SUCCESSFUL (done) prior job (§8).
// A duplicate intent within TTL reuses a completed job; a still-running or failed
// job does not (so the operator can retry a failed intent).
func TestSubmit_CachesOnlySuccessfulJobs(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	svc := buildService(t, st, testkit.NewLLM())
	intent := suggest.Intent{Description: "90s action"}

	id1, err := svc.Submit(ctx, intent, "alice")
	if err != nil {
		t.Fatal(err)
	}
	// Before the job completes, a duplicate intent does NOT hit the cache (only a
	// `done` job caches) — so it enqueues a fresh job rather than reusing a
	// pending/failed one.
	id2, _ := svc.Submit(ctx, intent, "bob")
	if id2 == id1 {
		t.Error("a not-yet-completed job must not be a cache hit")
	}

	// Mark job id1 done → NOW an identical intent reuses it.
	j, _ := st.GetJob(ctx, id1)
	j.Status = "done"
	if err := st.UpdateJob(ctx, j); err != nil {
		t.Fatal(err)
	}
	id3, err := svc.Submit(ctx, intent, "carol")
	if err != nil {
		t.Fatal(err)
	}
	if id3 != id1 {
		t.Errorf("identical intent should reuse the DONE job: %s vs %s", id3, id1)
	}

	// A different intent gets a fresh job regardless.
	id4, _ := svc.Submit(ctx, suggest.Intent{Description: "80s horror"}, "alice")
	if id4 == id1 {
		t.Error("different intent should not hit the cache")
	}
}

// A FAILED job (e.g. no grounded titles) must NOT wedge re-submits — retrying the
// same intent re-runs generation instead of returning the failed job.
func TestSubmit_FailedJobDoesNotCache(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	svc := buildService(t, st, testkit.NewLLM())
	intent := suggest.Intent{Description: "obscure intent"}

	id1, _ := svc.Submit(ctx, intent, "alice")
	j, _ := st.GetJob(ctx, id1)
	j.Status = "failed"
	j.LastError = "no grounded titles found for this intent"
	if err := st.UpdateJob(ctx, j); err != nil {
		t.Fatal(err)
	}

	id2, err := svc.Submit(ctx, intent, "alice") // retry the same intent
	if err != nil {
		t.Fatal(err)
	}
	if id2 == id1 {
		t.Error("a failed job must not be cached — the retry should enqueue a fresh job")
	}
}

// A submitted job is claimed, run, and its proposal lands in the approval queue
// (submitted) — persisted, so it survives a restart (§8).
func TestWorker_RunsJobAndPersistsProposal(t *testing.T) {
	st := newStore(t)
	llmMock := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"query": "matrix"}),
		testkit.FinalResponse(`{"picks":[{"mediaType":"movie","tmdbId":603,"name":"The Matrix"}]}`),
	)
	svc := buildService(t, st, llmMock)

	jobID, err := svc.Submit(context.Background(), suggest.Intent{Description: "sci-fi"}, "alice")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go svc.Run(ctx)

	// Wait for the job to complete (poll the store — the source of truth).
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		j, _ := st.GetJob(context.Background(), jobID)
		if j.Status == "done" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	j, _ := st.GetJob(context.Background(), jobID)
	if j.Status != "done" {
		t.Fatalf("job did not complete: status=%s err=%s", j.Status, j.LastError)
	}
	// The proposal is in the approval queue.
	queue, _ := st.ListProposalsByStatus(context.Background(), "submitted")
	if len(queue) != 1 {
		t.Fatalf("approval queue = %d, want 1", len(queue))
	}
	if queue[0].CreatedBy != "alice" {
		t.Errorf("proposal created_by = %q, want alice", queue[0].CreatedBy)
	}
}

// A hung LLM hits JOB_TIMEOUT and the job fails cleanly; the pool keeps draining
// OTHER jobs (§8 — one hung call can't starve the queue).
func TestWorker_HungLLMTimesOut_PoolKeepsDraining(t *testing.T) {
	st := newStore(t)
	// A slow LLM that exceeds the (short) timeout.
	slow := testkit.NewLLM(testkit.ToolCallResponse("catalog_search", map[string]any{"query": "x"}))
	slow.Delay = 2 * time.Second
	ms := testkit.NewMediaServer(t)
	lib := library.New(library.Emby, ms.URL, ms.AdminToken, "dev-1")
	mt := testkit.NewTMDB(t)
	tm := tmdb.NewWithBase(mt.URL, "key")
	cat := catalog.New(lib, tm, nil)
	sug := suggest.New(slow, cat, tm, 10)
	svc := suggest.NewService(st, sug, suggest.Config{Workers: 2, Timeout: 200 * time.Millisecond, CacheTTL: time.Hour},
		idGen(), time.Now, testkit.Logger())

	hungID, _ := svc.Submit(context.Background(), suggest.Intent{Description: "hangs"}, "alice")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go svc.Run(ctx)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		j, _ := st.GetJob(context.Background(), hungID)
		if j.Status == "failed" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	j, _ := st.GetJob(context.Background(), hungID)
	if j.Status != "failed" {
		t.Fatalf("hung job should fail on timeout, got status=%s", j.Status)
	}
	if j.LastError == "" {
		t.Error("failed job should record the timeout error")
	}
}

func TestIntentHash_NormalizesEquivalentIntents(t *testing.T) {
	a := suggest.Intent{Description: "  90s Action  ", MustInclude: []string{"Speed", "die hard"}}
	b := suggest.Intent{Description: "90s action", MustInclude: []string{"die hard", "Speed"}}
	if suggest.IntentHash(a) != suggest.IntentHash(b) {
		t.Error("case/whitespace/order differences should hash the same")
	}
	c := suggest.Intent{Description: "90s comedy"}
	if suggest.IntentHash(a) == suggest.IntentHash(c) {
		t.Error("different intents should hash differently")
	}
}
