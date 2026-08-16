package suggest_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/catalog"
	"github.com/mantonx/loomarr/internal/library"
	"github.com/mantonx/loomarr/internal/llm"
	"github.com/mantonx/loomarr/internal/provision"
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

func buildService(t *testing.T, st suggest.ProposalStore, llmMock *testkit.LLM) *suggest.Service {
	ms := testkit.NewMediaServer(t)
	lib := library.New(library.Emby, ms.URL, ms.AdminToken, "dev-1")
	mt := testkit.NewTMDB(t)
	tm := tmdb.NewWithBase(mt.URL, "key")
	cat := catalog.New(lib, tm)
	sug := suggest.New(llmMock, cat, tm, 10)
	return suggest.NewService(st, sug, suggest.Config{Workers: 2, Timeout: time.Second, CacheTTL: time.Hour},
		idGen(), time.Now, testkit.Logger())
}

// Submit caches generated CONTENT from a successful job, never its request
// identity, owner, or decision state. Pending/failed jobs do not cache.
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

	// Mark job id1 done with an already-approved proposal. A later identical
	// request may reuse the payload, but must get a fresh caller-owned lifecycle.
	j, _ := st.GetJob(ctx, id1)
	j.Status = "done"
	if err := st.UpdateJob(ctx, j); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateProposal(ctx, store.Proposal{
		ID: "source-proposal", JobID: id1, Status: "approved", CreatedBy: "alice", ApprovedBy: "admin",
		ProposalJSON: `{"lineup":[{"name":"Speed"}]}`, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	id3, err := svc.Submit(ctx, intent, "carol")
	if err != nil {
		t.Fatal(err)
	}
	if id3 == id1 {
		t.Fatal("a cache hit must create a fresh job id")
	}
	clonedJob, err := st.GetJob(ctx, id3)
	if err != nil {
		t.Fatal(err)
	}
	if clonedJob.Status != "done" || clonedJob.CreatedBy != "carol" {
		t.Fatalf("cached job lifecycle = %+v", clonedJob)
	}
	carols, err := st.ListProposalsByCreator(ctx, "carol")
	if err != nil {
		t.Fatal(err)
	}
	if len(carols) != 1 || carols[0].JobID != id3 || carols[0].Status != "submitted" ||
		carols[0].ApprovedBy != "" || carols[0].ProposalJSON != `{"lineup":[{"name":"Speed"}]}` {
		t.Fatalf("cached proposal lifecycle = %+v", carols)
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
	cat := catalog.New(lib, tm)
	sug := suggest.New(slow, cat, tm, 10)
	terminal := newDoneEmitter()
	svc := suggest.NewService(st, sug, suggest.Config{Workers: 2, Timeout: 200 * time.Millisecond, CacheTTL: time.Hour},
		idGen(), time.Now, testkit.Logger()).WithProgressEmitter(terminal)

	hungID, _ := svc.Submit(context.Background(), suggest.Intent{Description: "hangs"}, "alice")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go svc.Run(ctx)

	waitForFailedEvent(t, terminal)
	j, _ := st.GetJob(context.Background(), hungID)
	if j.Status != "failed" {
		t.Fatalf("hung job should fail on timeout, got status=%s", j.Status)
	}
	if j.LastError == "" {
		t.Error("failed job should record the timeout error")
	}
	if j.FailureCode != "timed_out" {
		t.Fatalf("hung job failure code = %q, want timed_out", j.FailureCode)
	}
}

type lifecycleErrorStore struct {
	store.Store
	successErr        error
	failureErr        error
	successCalled     chan struct{}
	failureCalled     chan struct{}
	failureCode       string
	failureDiagnostic string
}

func newLifecycleErrorStore(st store.Store) *lifecycleErrorStore {
	return &lifecycleErrorStore{
		Store: st, successCalled: make(chan struct{}, 1), failureCalled: make(chan struct{}, 1),
	}
}

func (s *lifecycleErrorStore) CommitSuggestionSuccess(
	ctx context.Context,
	jobID string,
	expectedAttempt int,
	p store.Proposal,
	updatedAt time.Time,
) error {
	select {
	case s.successCalled <- struct{}{}:
	default:
	}
	if s.successErr != nil {
		return s.successErr
	}
	return s.Store.CommitSuggestionSuccess(ctx, jobID, expectedAttempt, p, updatedAt)
}

func (s *lifecycleErrorStore) CommitSuggestionFailure(
	ctx context.Context,
	jobID string,
	expectedAttempt int,
	code string,
	diagnostic string,
	updatedAt time.Time,
) error {
	s.failureCode = code
	s.failureDiagnostic = diagnostic
	select {
	case s.failureCalled <- struct{}{}:
	default:
	}
	if s.failureErr != nil {
		return s.failureErr
	}
	return s.Store.CommitSuggestionFailure(ctx, jobID, expectedAttempt, code, diagnostic, updatedAt)
}

type failingProvider struct {
	err error
}

func (p failingProvider) Name() string { return "failing-provider" }

func (p failingProvider) Chat(context.Context, []llm.Message, llm.ChatOptions) (llm.Response, error) {
	return llm.Response{}, p.err
}

func buildFailingService(t *testing.T, st suggest.ProposalStore, provider llm.Provider) *suggest.Service {
	t.Helper()
	ms := testkit.NewMediaServer(t)
	lib := library.New(library.Emby, ms.URL, ms.AdminToken, "dev-1")
	mt := testkit.NewTMDB(t)
	tm := tmdb.NewWithBase(mt.URL, "key")
	cat := catalog.New(lib, tm)
	sug := suggest.New(provider, cat, tm, 10)
	return suggest.NewService(st, sug, suggest.Config{Workers: 1, Timeout: time.Second, CacheTTL: time.Hour},
		idGen(), time.Now, testkit.Logger())
}

func TestWorker_FailuresPersistBoundedCodesAndPrivateDiagnostics(t *testing.T) {
	providerDetail := `upstream said "account api-key suffix 1234"`
	tests := []struct {
		name       string
		provider   llm.Provider
		wantCode   string
		wantDetail string
	}{
		{
			name: "no grounded titles",
			provider: testkit.NewLLM(testkit.FinalResponse(
				`{"picks":[{"mediaType":"movie","tmdbId":999999,"name":"Invented"}]}`,
			)),
			wantCode: "no_grounded_titles",
		},
		{
			name: "provider transport unavailable",
			provider: failingProvider{err: &url.Error{
				Op: "Post", URL: "https://llm.invalid/v1/chat", Err: syscall.ECONNREFUSED,
			}},
			wantCode:   "provider_unavailable",
			wantDetail: "connection refused",
		},
		{
			name:       "provider service unavailable",
			provider:   failingProvider{err: errors.New("openai chat: status 503")},
			wantCode:   "provider_unavailable",
			wantDetail: "status 503",
		},
		{
			name:       "provider authentication rejection is not availability",
			provider:   failingProvider{err: errors.New("openai chat: status 401")},
			wantCode:   "generation_failed",
			wantDetail: "status 401",
		},
		{
			name:       "unclassified provider response",
			provider:   failingProvider{err: errors.New(providerDetail)},
			wantCode:   "generation_failed",
			wantDetail: providerDetail,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			base := newStore(t)
			st := newLifecycleErrorStore(base)
			svc := buildFailingService(t, st, tt.provider)
			terminal := newDoneEmitter()
			svc.WithProgressEmitter(terminal)
			jobID, err := svc.Submit(ctx, suggest.Intent{Description: "sci-fi"}, "alice")
			if err != nil {
				t.Fatal(err)
			}

			runCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			go svc.Run(runCtx)
			waitForFailedEvent(t, terminal)

			if st.failureCode != tt.wantCode {
				t.Fatalf("failure code = %q, want %q (diagnostic %q)",
					st.failureCode, tt.wantCode, st.failureDiagnostic)
			}
			if st.failureCode == "" {
				t.Fatal("failure committed without a stable code")
			}
			if strings.Contains(st.failureCode, providerDetail) {
				t.Fatalf("safe failure code leaked provider detail: %q", st.failureCode)
			}
			if tt.wantDetail != "" && !strings.Contains(st.failureDiagnostic, tt.wantDetail) {
				t.Fatalf("private diagnostic = %q, want detail %q", st.failureDiagnostic, tt.wantDetail)
			}
			job, err := base.GetJob(ctx, jobID)
			if err != nil {
				t.Fatal(err)
			}
			if job.Status != "failed" {
				t.Fatalf("job status = %q, want failed", job.Status)
			}
		})
	}
}

func TestWorker_UndurableFailureEmitsNoTerminalEvent(t *testing.T) {
	ctx := context.Background()
	base := newStore(t)
	st := newLifecycleErrorStore(base)
	st.failureErr = errors.New("write unavailable")
	llmMock := testkit.NewLLM()
	svc := buildService(t, st, llmMock)
	terminal := newDoneEmitter()
	svc.WithProgressEmitter(terminal)
	now := time.Now()
	if err := st.CreateJob(ctx, store.Job{
		ID: "bad-intent", Kind: "suggest", Status: "queued", IntentJSON: "{", IntentHash: "bad",
		CreatedBy: "alice", Deadline: now, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go svc.Run(runCtx)
	select {
	case <-st.failureCalled:
	case <-time.After(5 * time.Second):
		t.Fatal("worker never attempted to persist the failure")
	}
	if st.failureCode != "generation_failed" || !strings.Contains(st.failureDiagnostic, "bad intent json") {
		t.Fatalf("failure persistence attempt = code %q diagnostic %q",
			st.failureCode, st.failureDiagnostic)
	}
	job, err := base.GetJob(ctx, "bad-intent")
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "running" || job.Attempts != 1 {
		t.Fatalf("undurable failure changed authoritative state: %+v", job)
	}
	select {
	case <-terminal.failed:
		t.Fatal("worker emitted failed without a durable failed transition")
	default:
	}
	if llmMock.Calls != 0 {
		t.Fatalf("malformed intent reached the model %d times", llmMock.Calls)
	}
}

func TestWorker_StaleSuccessDoesNotFailReplacement(t *testing.T) {
	ctx := context.Background()
	base := newStore(t)
	st := newLifecycleErrorStore(base)
	st.successErr = store.ErrJobNotRunning
	llmMock := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"query": "matrix"}),
		testkit.FinalResponse(`{"picks":[{"mediaType":"movie","tmdbId":603,"name":"The Matrix"}]}`),
	)
	svc := buildService(t, st, llmMock)
	terminal := newDoneEmitter()
	svc.WithProgressEmitter(terminal)
	jobID, err := svc.Submit(ctx, suggest.Intent{Description: "sci-fi"}, "alice")
	if err != nil {
		t.Fatal(err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go svc.Run(runCtx)
	select {
	case <-st.successCalled:
	case <-time.After(5 * time.Second):
		t.Fatal("worker never attempted to persist success")
	}
	select {
	case <-st.failureCalled:
		t.Fatal("lost success CAS was incorrectly routed through failure persistence")
	default:
	}
	select {
	case <-terminal.done:
		t.Fatal("worker emitted done without a durable success transition")
	case <-terminal.failed:
		t.Fatal("worker emitted failed for a stale execution")
	default:
	}
	job, err := base.GetJob(ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "running" || job.Attempts != 1 {
		t.Fatalf("stale success changed authoritative state: %+v", job)
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

// A refine's change text + current lineup are part of the intent identity, so two
// refines of the same channel (or a refine vs the original) never collide in the cache.
func TestIntentHash_DistinguishesRefine(t *testing.T) {
	base := suggest.Intent{Description: "90s action"}
	refineA := base
	refineA.RefineText = "add more Schwarzenegger"
	refineB := base
	refineB.RefineText = "make it kid-friendly"

	if suggest.IntentHash(base) == suggest.IntentHash(refineA) {
		t.Error("a refine must hash differently from the plain intent")
	}
	if suggest.IntentHash(refineA) == suggest.IntentHash(refineB) {
		t.Error("two different refine texts must hash differently")
	}
	// The current lineup also contributes (order-independent).
	l1 := refineA
	l1.CurrentLineup = []suggest.LineupContext{{Key: "movie:tmdb:603"}, {Key: "movie:tmdb:165"}}
	l2 := refineA
	l2.CurrentLineup = []suggest.LineupContext{{Key: "movie:tmdb:165"}, {Key: "movie:tmdb:603"}}
	if suggest.IntentHash(l1) != suggest.IntentHash(l2) {
		t.Error("current-lineup order must not change the hash")
	}
	if suggest.IntentHash(refineA) == suggest.IntentHash(l1) {
		t.Error("a different current lineup must change the hash")
	}
}

// Refine re-queues an EXISTING job in place (the channel's IntentRef) — same id, new
// intent, status back to queued so the worker re-runs it. This is what lets the refined
// proposal bind back to the same channel.
func TestRefine_ReQueuesExistingJob(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	svc := buildService(t, st, testkit.NewLLM())

	// A completed original suggestion job.
	if err := st.CreateJob(ctx, store.Job{
		ID: "job-orig", Kind: "suggest", Status: "done",
		IntentJSON: `{"description":"90s action"}`, CreatedBy: "alex",
	}); err != nil {
		t.Fatal(err)
	}

	refine := suggest.Intent{Description: "90s action", RefineText: "add more Schwarzenegger"}
	got, err := svc.Refine(ctx, "job-orig", refine)
	if err != nil {
		t.Fatal(err)
	}
	if got != "job-orig" {
		t.Errorf("Refine returned %q, want the same job id job-orig", got)
	}
	// The SAME job is now re-queued with the refine intent (not a new job).
	j, _ := st.GetJob(ctx, "job-orig")
	if j.Status != "queued" {
		t.Errorf("refined job status = %q, want queued (re-run)", j.Status)
	}
	if j.Kind != "suggest" {
		t.Errorf("manual refine kind = %q, want suggest", j.Kind)
	}
	if !strings.Contains(j.IntentJSON, "add more Schwarzenegger") {
		t.Errorf("refined job intent = %q, want the refine text swapped in", j.IntentJSON)
	}
}

func TestRecurate_ReQueuesWithServerOwnedOrigin(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	svc := buildService(t, st, testkit.NewLLM())
	if err := st.CreateJob(ctx, store.Job{
		ID: "job-orig", Kind: "suggest", Status: "done", IntentJSON: `{"description":"90s action"}`,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Recurate(ctx, "job-orig", suggest.Intent{Description: "90s action", CurrentLineup: []suggest.LineupContext{{Key: "movie:tmdb:603"}}}); err != nil {
		t.Fatal(err)
	}
	j, err := st.GetJob(ctx, "job-orig")
	if err != nil {
		t.Fatal(err)
	}
	if j.Kind != "recurate" || j.Status != "queued" {
		t.Fatalf("recurated job = kind %q status %q", j.Kind, j.Status)
	}
}

// Refine on a missing job errors (the caller — refineChannel — guards IntentRef, but the
// service must be honest if handed a bad id).
func TestRefine_MissingJob(t *testing.T) {
	st := newStore(t)
	svc := buildService(t, st, testkit.NewLLM())
	if _, err := svc.Refine(context.Background(), "nope", suggest.Intent{}); err == nil {
		t.Error("refine on a missing job should error")
	}
}

type recordingCurator struct {
	calls    int
	decision suggest.Decision
	err      error
}

type doneEmitter struct {
	done   chan struct{}
	failed chan struct{}
}

func newDoneEmitter() *doneEmitter {
	return &doneEmitter{done: make(chan struct{}, 1), failed: make(chan struct{}, 1)}
}

func (e *doneEmitter) SuggestionPhase(_ string, phase string, _ int) {
	var target chan struct{}
	switch phase {
	case "done":
		target = e.done
	case "failed":
		target = e.failed
	}
	if target != nil {
		select {
		case target <- struct{}{}:
		default:
		}
	}
}

func waitForFailedEvent(t *testing.T, e *doneEmitter) {
	t.Helper()
	select {
	case <-e.failed:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for durable failed event")
	}
}

func waitForDoneEvent(t *testing.T, e *doneEmitter) {
	t.Helper()
	select {
	case <-e.done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for durable done event")
	}
}

func (c *recordingCurator) Consider(context.Context, store.Proposal) (suggest.Decision, error) {
	c.calls++
	return c.decision, c.err
}

// TestWorker_AutoApproveCommitsChannel proves the worker reaches the same deep approval gate as
// manual approval: its observable outcome is one approved proposal with its local channel, not a
// second best-effort binder call whose failure could strand the proposal.
func TestWorker_AutoApproveCommitsChannel(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)

	// A user holding the auto-approve grant, comfortably within quota.
	if err := st.UpsertUser(ctx, store.User{ID: "grace", AutoApprove: true, Quota: 100}); err != nil {
		t.Fatal(err)
	}

	llmMock := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"query": "matrix"}),
		testkit.FinalResponse(`{"picks":[{"mediaType":"movie","tmdbId":603,"name":"The Matrix"}]}`),
	)
	svc := buildService(t, st, llmMock)
	channels := &testkit.ApprovalChannels{}
	approver := suggest.NewApprover(st, channels, time.Now)
	curator := &recordingCurator{decision: suggest.Decision{Approved: true}}
	done := newDoneEmitter()
	svc = svc.WithAutoApprove(suggest.NewAutoApprover(
		st, approver, func(context.Context) int { return 100 }, testkit.Logger())).
		WithAutoCurate(curator).
		WithProgressEmitter(done)

	jobID, err := svc.Submit(ctx, suggest.Intent{Description: "sci-fi"}, "grace")
	if err != nil {
		t.Fatal(err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go svc.Run(runCtx)

	// The durable job may become done just before the optional approval decision,
	// but the terminal event is emitted only after that synchronous attempt settles.
	waitForDoneEvent(t, done)
	j, _ := st.GetJob(ctx, jobID)
	if j.Status != "done" {
		t.Fatalf("job did not complete: status=%s err=%s", j.Status, j.LastError)
	}

	// The proposal must have been auto-approved (not left in the queue)...
	approved, _ := st.ListProposalsByStatus(ctx, "approved")
	if len(approved) != 1 {
		t.Fatalf("approved proposals = %d, want 1 (auto-approved within quota)", len(approved))
	}
	if approved[0].JobID != jobID {
		t.Fatalf("approved proposal's job = %q, want %q", approved[0].JobID, jobID)
	}

	if len(channels.Plans) != 1 {
		t.Fatalf("channel plans = %d, want 1", len(channels.Plans))
	}
	if channels.Plans[0].ID != approved[0].ID {
		t.Errorf("planned proposal = %q, want %q", channels.Plans[0].ID, approved[0].ID)
	}
	if channels.Plans[0].Status != "approved" || channels.Plans[0].ApprovedBy == "" {
		t.Errorf("planner got an ineffective proposal (status=%q approvedBy=%q)",
			channels.Plans[0].Status, channels.Plans[0].ApprovedBy)
	}
	ch, err := st.GetChannel(ctx, "ch_"+approved[0].ID)
	if err != nil {
		t.Fatalf("approved proposal has no committed channel: %v", err)
	}
	if ch.IntentRef != jobID {
		t.Errorf("channel intentRef = %q, want %q", ch.IntentRef, jobID)
	}
	if len(channels.Committed) != 1 || channels.Committed[0] != ch.ID {
		t.Errorf("post-commit channels = %+v, want %q", channels.Committed, ch.ID)
	}
	if curator.calls != 0 {
		t.Errorf("ordinary user proposal reached auto-curate grant %d times", curator.calls)
	}
}

func TestWorker_RecurateSkipsRequesterAutoApprove(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	if err := st.UpsertUser(ctx, store.User{ID: "grace", AutoApprove: true, Quota: 100}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateJob(ctx, store.Job{
		ID: "job-recurate", Kind: "suggest", Status: "done", CreatedBy: "grace",
		IntentJSON: `{"description":"sci-fi"}`,
	}); err != nil {
		t.Fatal(err)
	}
	llmMock := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"query": "matrix"}),
		testkit.FinalResponse(`{"picks":[{"mediaType":"movie","tmdbId":603,"name":"The Matrix"}]}`),
	)
	svc := buildService(t, st, llmMock)
	channels := &testkit.ApprovalChannels{}
	approver := suggest.NewApprover(st, channels, time.Now)
	curator := &recordingCurator{decision: suggest.Decision{Reason: "below auto-curate quality bar"}}
	done := newDoneEmitter()
	svc = svc.WithAutoApprove(suggest.NewAutoApprover(
		st, approver, func(context.Context) int { return 100 }, testkit.Logger())).
		WithAutoCurate(curator).
		WithProgressEmitter(done)
	if _, err := svc.Recurate(ctx, "job-recurate", suggest.Intent{
		Description: "sci-fi", CurrentLineup: []suggest.LineupContext{{Key: "movie:tmdb:603"}},
	}); err != nil {
		t.Fatal(err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go svc.Run(runCtx)
	waitForDoneEvent(t, done)
	if curator.calls != 1 {
		t.Fatalf("auto-curate calls = %d, want 1", curator.calls)
	}
	approved, _ := st.ListProposalsByStatus(ctx, "approved")
	if len(approved) != 0 {
		t.Fatalf("requester grant bypassed auto-curate thresholds: %+v", approved)
	}
	submitted, _ := st.ListProposalsByStatus(ctx, "submitted")
	if len(submitted) != 1 {
		t.Fatalf("submitted proposals = %d, want 1 for review", len(submitted))
	}
	if len(channels.Plans) != 0 {
		t.Fatalf("requester auto-approver planned %d channels for a recurate job", len(channels.Plans))
	}
}

// A local channel-plan failure is an approval failure. The generation job still completes and
// leaves the proposal for an admin, but neither approval nor acquisitions may leak through.
func TestWorker_AutoApprovePlanFailureLeavesProposalSubmitted(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	if err := st.UpsertUser(ctx, store.User{ID: "grace", AutoApprove: true, Quota: 100}); err != nil {
		t.Fatal(err)
	}
	llmMock := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"query": "matrix"}),
		testkit.FinalResponse(`{"picks":[{"mediaType":"movie","tmdbId":603,"name":"The Matrix"}]}`),
	)
	svc := buildService(t, st, llmMock)
	channels := &testkit.ApprovalChannels{PlanError: fmt.Errorf("cannot build local channel")}
	approver := suggest.NewApprover(st, channels, time.Now)
	svc = svc.WithAutoApprove(suggest.NewAutoApprover(
		st, approver, func(context.Context) int { return 100 }, testkit.Logger()))

	jobID, err := svc.Submit(ctx, suggest.Intent{Description: "sci-fi"}, "grace")
	if err != nil {
		t.Fatal(err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go svc.Run(runCtx)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		j, _ := st.GetJob(ctx, jobID)
		if j.Status == "done" || j.Status == "failed" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	j, _ := st.GetJob(ctx, jobID)
	if j.Status != "done" {
		t.Fatalf("a failed auto-approval must leave the proposal for review, not fail generation: status=%s err=%s", j.Status, j.LastError)
	}
	approved, _ := st.ListProposalsByStatus(ctx, "approved")
	if len(approved) != 0 {
		t.Fatalf("plan failure approved %d proposals, want 0", len(approved))
	}
	submitted, _ := st.ListProposalsByStatus(ctx, "submitted")
	if len(submitted) != 1 {
		t.Fatalf("submitted proposals = %d, want 1 for admin review", len(submitted))
	}
	if titles, _ := st.ListTitlesByState(ctx, provision.Wanted); len(titles) != 0 {
		t.Fatalf("plan failure enqueued %d titles, want 0", len(titles))
	}
	if all, _ := st.ListChannels(ctx); len(all) != 0 {
		t.Fatalf("plan failure created %d channels, want 0", len(all))
	}
}
