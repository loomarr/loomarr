package suggest_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/platform/llm"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/identitystore"
	"github.com/loomarr/loomarr/internal/proposals"
	"github.com/loomarr/loomarr/internal/proposals/quality"
	"github.com/loomarr/loomarr/internal/proposals/suggest"
	"github.com/loomarr/loomarr/internal/proposals/workflow"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/tmdb"
)

func newStore(t *testing.T) store.Store {
	t.Helper()
	return testkit.MigratedSQLiteStore(t)
}

func idGen() func() string {
	var n atomic.Int64
	return func() string { return "id-" + strconv.FormatInt(n.Add(1), 10) }
}

// marigoldMediaServer is the media-server mock with an owned film trio: its search
// answers "marigold" with Marigold Protocol (90000603), Marigold Protocol II
// (90000605) and Marigold Protocol III (90624860), in the real Emby /Items shape.
func marigoldMediaServer(t *testing.T) *testkit.MediaServer {
	ms := testkit.NewMediaServer(t)
	ms.SetSearchItems(
		testkit.SearchStub{Terms: []string{"marigold"}, LibraryItemID: "1000", Name: "Marigold Protocol", Type: "Movie", Year: 1999, TMDBID: 90000603},
		testkit.SearchStub{Terms: []string{"marigold"}, LibraryItemID: "1001", Name: "Marigold Protocol II", Type: "Movie", Year: 2003, TMDBID: 90000605},
		testkit.SearchStub{Terms: []string{"marigold"}, LibraryItemID: "1002", Name: "Marigold Protocol III", Type: "Movie", Year: 2021, TMDBID: 90624860},
	)
	return ms
}

func buildService(t *testing.T, st store.Store, llmMock llm.Provider) *suggest.Service {
	return buildServiceWith(t, st, workflow.New(st, idGen(), time.Now), llmMock)
}

// buildServiceWith is buildService over a caller-chosen durable workflow.
func buildServiceWith(t *testing.T, st suggest.ProposalStore, workflow suggest.DurableWorkflow, llmMock llm.Provider) *suggest.Service {
	ms := marigoldMediaServer(t)
	lib := library.New(library.Emby, ms.URL, ms.AdminToken, "dev-1")
	mt := testkit.NewTMDB(t)
	tm := tmdb.NewWithBase(mt.URL, "key")
	cat := catalog.New(lib, tm)
	sug := suggest.New(llmMock, cat, tm, 10)
	return suggest.NewService(st, workflow, sug, suggest.Config{Workers: 2, Timeout: time.Second, CacheTTL: time.Hour},
		time.Now, testkit.Logger())
}

// claimSignalWorkflow reports every Claim and hands out no work.
type claimSignalWorkflow struct {
	suggest.DurableWorkflow
	claimed chan struct{}
}

func (w *claimSignalWorkflow) Claim(context.Context, time.Time, time.Duration, int) ([]suggest.WorkflowWork, error) {
	select {
	case w.claimed <- struct{}{}:
	default:
	}
	return nil, nil
}

func TestWorkerClaimsImmediatelyOnStartup(t *testing.T) {
	st := &claimSignalWorkflow{claimed: make(chan struct{}, 1)}
	svc := suggest.NewService(nil, st, nil, suggest.Config{Workers: 1}, time.Now, testkit.Logger())
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go svc.Run(ctx)

	select {
	case <-st.claimed:
	case <-time.After(time.Second):
		t.Fatal("worker did not claim due jobs immediately on startup")
	}
}

// scopeFeedbackSource observes only the in-memory execution scope supplied at the
// public FeedbackSource seam. The integration journey owns store ordering,
// tombstones, deduplication, and event-to-signal projection coverage.
type scopeFeedbackSource struct {
	mu      sync.Mutex
	scopes  []string
	signals map[string][]suggest.FeedbackSignal
}

type failingFeedbackSource struct{ err error }

func (s failingFeedbackSource) Signals(context.Context, proposals.Intent) ([]suggest.FeedbackSignal, error) {
	return nil, s.err
}

func (s *scopeFeedbackSource) Signals(_ context.Context, intent proposals.Intent) ([]suggest.FeedbackSignal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scopes = append(s.scopes, intent.DiscoveryScopeID)
	return append([]suggest.FeedbackSignal(nil), s.signals[intent.DiscoveryScopeID]...), nil
}

func (s *scopeFeedbackSource) Scopes() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.scopes...)
}

type scopeLookupStore struct {
	store.Store
	override bool
	channel  store.Channel
	err      error
}

func (s *scopeLookupStore) GetChannelForJob(ctx context.Context, jobID string) (store.Channel, error) {
	if s.err != nil {
		return store.Channel{}, s.err
	}
	if s.override {
		return s.channel, nil
	}
	return s.Store.GetChannelForJob(ctx, jobID)
}

// Submit caches generated CONTENT from a successful job, never its request
// identity, owner, or decision state. Pending/failed jobs do not cache.
func TestSubmit_CachesOnlySuccessfulJobs(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	svc := buildService(t, st, testkit.NewLLM())
	intent := proposals.Intent{Description: "90s action"}

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
		Body: proposals.Proposal{Lineup: []proposals.ProposalItem{{Name: "Quiet Engines"}}}, CreatedAt: time.Now(), UpdatedAt: time.Now(),
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
		carols[0].ApprovedBy != "" || len(carols[0].Body.Lineup) != 1 || carols[0].Body.Lineup[0].Name != "Quiet Engines" {
		t.Fatalf("cached proposal lifecycle = %+v", carols)
	}

	// A different intent gets a fresh job regardless.
	id4, _ := svc.Submit(ctx, proposals.Intent{Description: "80s horror"}, "alice")
	if id4 == id1 {
		t.Error("different intent should not hit the cache")
	}
}

func TestSubmit_DistinctCaseSensitiveReferenceURLsDoNotShareSuccess(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	svc := buildService(t, st, testkit.NewLLM())
	upper := proposals.Intent{Description: "Use https://example.com/RosterA for this programming block"}
	lower := proposals.Intent{Description: "Use https://example.com/rostera for this programming block"}

	sourceID, err := svc.Submit(ctx, upper, "alice")
	if err != nil {
		t.Fatal(err)
	}
	job, err := st.GetJob(ctx, sourceID)
	if err != nil {
		t.Fatal(err)
	}
	job.Status = "done"
	if err := st.UpdateJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateProposal(ctx, store.Proposal{
		ID: "upper-reference-proposal", JobID: sourceID, Status: "submitted", CreatedBy: "alice",
		Body: proposals.Proposal{Lineup: []proposals.ProposalItem{{Name: "Roster A"}}}, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	newID, err := svc.Submit(ctx, lower, "bob")
	if err != nil {
		t.Fatal(err)
	}
	newJob, err := st.GetJob(ctx, newID)
	if err != nil {
		t.Fatal(err)
	}
	if newJob.Status != "queued" {
		t.Fatalf("distinct reference submission status = %q, want queued rather than cached done", newJob.Status)
	}
	proposals, err := st.ListProposalsByCreator(ctx, "bob")
	if err != nil {
		t.Fatal(err)
	}
	if len(proposals) != 0 {
		t.Fatalf("distinct reference submission cloned cached proposal: %+v", proposals)
	}
}

func TestSubmit_CaseSensitiveOrdinaryReferenceDoesNotCloneSuccess(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	svc := buildService(t, st, testkit.NewLLM())
	sourceID, err := svc.Submit(ctx, proposals.Intent{Description: "kettle kids"}, "alice")
	if err != nil {
		t.Fatal(err)
	}
	job, err := st.GetJob(ctx, sourceID)
	if err != nil {
		t.Fatal(err)
	}
	job.Status = "done"
	if err := st.UpdateJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateProposal(ctx, store.Proposal{ID: "kettle-proposal", JobID: sourceID, Status: "submitted", CreatedBy: "alice", Body: proposals.Proposal{Lineup: []proposals.ProposalItem{{Name: "Grand Oak Hotel"}}}, CreatedAt: time.Now(), UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	newID, err := svc.Submit(ctx, proposals.Intent{Description: "KETTLE KIDS"}, "bob")
	if err != nil {
		t.Fatal(err)
	}
	if newID == sourceID {
		t.Fatal("case-distinct ordinary reference reused successful job")
	}
	newJob, err := st.GetJob(ctx, newID)
	if err != nil {
		t.Fatal(err)
	}
	if newJob.Status != "queued" || newJob.CreatedBy != "bob" {
		t.Fatalf("new job = %+v, want queued job owned by bob", newJob)
	}
	if proposals, err := st.ListProposalsByCreator(ctx, "bob"); err != nil {
		t.Fatal(err)
	} else if len(proposals) != 0 {
		t.Fatalf("new submission cloned cached proposal: %+v", proposals)
	}
}

// A FAILED job (e.g. no grounded titles) must NOT wedge re-submits — retrying the
// same intent re-runs generation instead of returning the failed job.
func TestSubmit_FailedJobDoesNotCache(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	svc := buildService(t, st, testkit.NewLLM())
	intent := proposals.Intent{Description: "obscure intent"}

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
		catalogSearchResponse(map[string]any{"query": "marigold"}),
		finalResponseWithNone(`{"picks":[{"mediaType":"movie","key":"movie:tmdb:90000603","name":"Marigold Protocol"}]}`),
	)
	svc := buildService(t, st, llmMock)

	jobID, err := svc.Submit(context.Background(), proposals.Intent{Description: "sci-fi"}, "alice")
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

type submittedNotifier struct{ submitted chan store.Proposal }

func (n submittedNotifier) ProposalSubmitted(_ context.Context, p store.Proposal)  { n.submitted <- p }
func (submittedNotifier) ProposalApproved(context.Context, store.Proposal, string) {}
func (submittedNotifier) ProposalDenied(context.Context, store.Proposal)           {}

// A generated proposal reaching the approval queue tells the approvers a request is waiting,
// exactly as a built one does.
func TestWorker_GeneratedProposalNotifiesApprovers(t *testing.T) {
	st := newStore(t)
	notifier := submittedNotifier{submitted: make(chan store.Proposal, 1)}
	svc := buildService(t, st, testkit.NewLLM(
		catalogSearchResponse(map[string]any{"query": "marigold"}),
		finalResponseWithNone(`{"picks":[{"mediaType":"movie","key":"movie:tmdb:90000603","name":"Marigold Protocol"}]}`),
	)).WithProposalNotifier(notifier)
	jobID, err := svc.Submit(context.Background(), proposals.Intent{Description: "sci-fi"}, "alice")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go svc.Run(ctx)

	select {
	case p := <-notifier.submitted:
		if p.JobID != jobID || p.CreatedBy != "alice" || p.Status != "submitted" {
			t.Fatalf("notified proposal = %+v, want alice's submitted proposal for job %s", p, jobID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("approvers were never told a generated proposal is waiting")
	}
}

// A cache hit is a fresh request waiting for approval like any other, so it tells the approvers too.
func TestSubmit_CacheHitNotifiesApprovers(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	notifier := submittedNotifier{submitted: make(chan store.Proposal, 1)}
	svc := buildService(t, st, testkit.NewLLM()).WithProposalNotifier(notifier)
	intent := proposals.Intent{Description: "90s action"}

	source, err := svc.Submit(ctx, intent, "alice")
	if err != nil {
		t.Fatal(err)
	}
	j, _ := st.GetJob(ctx, source)
	j.Status = "done"
	if err := st.UpdateJob(ctx, j); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateProposal(ctx, store.Proposal{
		ID: "source-proposal", JobID: source, Status: "approved", CreatedBy: "alice", ApprovedBy: "admin",
		Body: proposals.Proposal{Lineup: []proposals.ProposalItem{{Name: "Quiet Engines"}}}, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	cached, err := svc.Submit(ctx, intent, "carol")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case p := <-notifier.submitted:
		if p.JobID != cached || p.CreatedBy != "carol" {
			t.Fatalf("notified proposal = %+v, want carol's cached proposal for job %s", p, cached)
		}
	default:
		t.Fatal("approvers were never told a cached request is waiting")
	}
}

// Approvers hear "a request is waiting" only when a person still has to act: a proposal an
// automatic grant already approved never sends it, so nobody gets a waiting notice followed by an
// approval notice for the same proposal.
func TestWorker_NotifiesApproversOnlyWhenAPersonMustAct(t *testing.T) {
	const onePick = `{"picks":[{"mediaType":"movie","key":"movie:tmdb:90000603","name":"Marigold Protocol"}]}`
	grace := identitystore.User{ID: "grace", AutoApprove: true, Quota: 100}
	tests := []struct {
		name       string
		user       identitystore.User
		picks      string
		curator    *recordingCurator
		planError  error
		start      func(ctx context.Context, svc *suggest.Service, st store.Store) error
		wantNotice bool
	}{
		{
			name: "auto-curated recurate that is approved", user: grace, picks: onePick,
			curator: &recordingCurator{decision: suggest.Decision{Approved: true}},
			start: func(ctx context.Context, svc *suggest.Service, st store.Store) error {
				if _, err := st.SaveChannel(ctx, store.Channel{Channel: schedule.Channel{
					ID: "channel-recurate", Number: 72, Name: "Recurate Channel", Status: schedule.StatusLive, IntentRef: "job-origin",
				}}); err != nil {
					return err
				}
				_, err := svc.Recurate(ctx, "channel-recurate", proposals.Intent{Description: "sci-fi"})
				return err
			},
		},
		{
			name: "auto-approved within quota", user: grace, picks: onePick,
			start: func(ctx context.Context, svc *suggest.Service, _ store.Store) error {
				_, err := svc.Submit(ctx, proposals.Intent{Description: "sci-fi"}, "grace")
				return err
			},
		},
		{
			// Two earlier approved acquisitions already spend the cap of one.
			name: "requester over quota", user: identitystore.User{ID: "grace", AutoApprove: true, Quota: 1}, picks: onePick,
			start: func(ctx context.Context, svc *suggest.Service, st store.Store) error {
				now := time.Now().UTC()
				if err := st.CreateJob(ctx, store.Job{
					ID: "job-spent", Kind: "suggest", Status: "done", CreatedBy: "grace", IntentJSON: `{"description":"earlier"}`,
					Deadline: now, CreatedAt: now, UpdatedAt: now,
				}); err != nil {
					return err
				}
				if err := st.CreateProposal(ctx, store.Proposal{
					ID: "spent", JobID: "job-spent", CreatedBy: "grace", Status: "approved", ApprovedBy: "admin",
					Body: proposals.Proposal{Acquisitions: []proposals.ProposalItem{
						{MediaType: provision.Movie, TMDBID: 90091001, Name: "A"},
						{MediaType: provision.Movie, TMDBID: 90091002, Name: "B"},
					}},
					CreatedAt: now, UpdatedAt: now,
				}); err != nil {
					return err
				}
				for _, id := range []int{90091001, 90091002} {
					key, err := provision.Title{MediaType: provision.Movie, TMDBID: id}.Key()
					if err != nil {
						return err
					}
					if err := st.UpsertTitle(ctx, provision.Record{Key: key, State: provision.Wanted}); err != nil {
						return err
					}
				}
				_, err := svc.Submit(ctx, proposals.Intent{Description: "sci-fi"}, "grace")
				return err
			},
			wantNotice: true,
		},
		{
			name: "refine is always admin-reviewed", user: grace, picks: onePick,
			start: func(ctx context.Context, svc *suggest.Service, _ store.Store) error {
				_, err := svc.Refine(ctx, "channel-1", proposals.Intent{Description: "sci-fi", RefineText: "more"}, "grace")
				return err
			},
			wantNotice: true,
		},
		{
			name: "auto-approve errors and the proposal stays submitted", user: grace, picks: onePick,
			planError: fmt.Errorf("cannot build local channel"),
			start: func(ctx context.Context, svc *suggest.Service, _ store.Store) error {
				_, err := svc.Submit(ctx, proposals.Intent{Description: "sci-fi"}, "grace")
				return err
			},
			wantNotice: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			st := newStore(t)
			if err := st.UpsertUser(ctx, tt.user); err != nil {
				t.Fatal(err)
			}
			notifier := submittedNotifier{submitted: make(chan store.Proposal, 4)}
			done := newDoneEmitter()
			curator := tt.curator
			if curator == nil {
				curator = &recordingCurator{}
			}
			approver := suggest.NewApprover(st, &testkit.ApprovalChannels{PlanError: tt.planError}, time.Now)
			svc := buildService(t, st, testkit.NewLLM(
				catalogSearchResponse(map[string]any{"query": "marigold"}),
				finalResponseWithNone(tt.picks),
			)).WithProposalNotifier(notifier).
				WithAutoApprove(suggest.NewAutoApprover(st, approver, func(context.Context) int { return 100 }, testkit.Logger())).
				WithAutoCurate(curator).
				WithProgressEmitter(done)
			if err := tt.start(ctx, svc, st); err != nil {
				t.Fatal(err)
			}
			runCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			go svc.Run(runCtx)
			// done is emitted after the notice and the automatic decision both settle.
			waitForDoneEvent(t, done)

			got := len(notifier.submitted)
			if tt.wantNotice && got != 1 || !tt.wantNotice && got != 0 {
				t.Fatalf("submitted notices = %d, want notice=%v", got, tt.wantNotice)
			}
		})
	}
}

// A built proposal follows the same rule as a generated one.
func TestSubmitBuilt_NotifiesApproversOnlyWhenAPersonMustAct(t *testing.T) {
	for _, tt := range []struct {
		name       string
		user       identitystore.User
		wantNotice bool
	}{
		{name: "auto-approved within quota", user: identitystore.User{ID: "grace", AutoApprove: true, Quota: 100}},
		{name: "no auto-approve grant", user: identitystore.User{ID: "grace"}, wantNotice: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			st := newStore(t)
			if err := st.UpsertUser(ctx, tt.user); err != nil {
				t.Fatal(err)
			}
			notifier := submittedNotifier{submitted: make(chan store.Proposal, 4)}
			approver := suggest.NewApprover(st, &testkit.ApprovalChannels{}, time.Now)
			svc := buildService(t, st, testkit.NewLLM()).WithProposalNotifier(notifier).
				WithAutoApprove(suggest.NewAutoApprover(st, approver, func(context.Context) int { return 100 }, testkit.Logger()))
			built := proposals.Proposal{
				ChannelName: "Marigold",
				Lineup:      []proposals.ProposalItem{{MediaType: "movie", Name: "Marigold Protocol", Year: 1999, TMDBID: 90000603, InLibrary: true}},
			}
			if _, err := svc.SubmitBuilt(ctx, proposals.Intent{Description: "Marigold"}, built, "grace"); err != nil {
				t.Fatal(err)
			}
			if got := len(notifier.submitted); got != btoi(tt.wantNotice) {
				t.Fatalf("submitted notices = %d, want notice=%v", got, tt.wantNotice)
			}
		})
	}
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestWorker_NoGroundedTitlesPersistsTypedFailure(t *testing.T) {
	st := newStore(t)
	flow := workflow.New(st, func() string { return "workflow-proposal-1" }, time.Now)
	// The first empty, no-tool answer receives the bounded grounding retry; a second
	// explicit empty answer proves the retry exhausted and preserves the typed failure.
	svc := buildServiceWith(t, st, flow, testkit.NewLLM(
		finalResponseWithNone(`{"picks":[]}`),
		finalResponseWithNone(`{"picks":[]}`),
	))
	jobID, err := svc.Submit(context.Background(), proposals.Intent{Description: "Classic Kettle Kids episodes"}, "alice")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go svc.Run(ctx)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		job, _ := st.GetJob(context.Background(), jobID)
		if job.Status == "failed" {
			if job.FailureCode != suggest.FailureCodeNoGroundedTitles {
				t.Fatalf("failure code = %q, want %q", job.FailureCode, suggest.FailureCodeNoGroundedTitles)
			}
			if job.FailureTraceJSON == "" || !strings.Contains(job.FailureTraceJSON, `"selection_empty"`) {
				t.Fatalf("failure trace = %q", job.FailureTraceJSON)
			}
			attempts, listErr := st.ListProposalJobAttempts(context.Background(), jobID)
			if listErr != nil || len(attempts) != 1 || attempts[0].FailureCode != suggest.FailureCodeNoGroundedTitles {
				t.Fatalf("failed Attempt history = %+v, %v", attempts, listErr)
			}
			journey, inspectErr := flow.Inspect(context.Background(), workflow.Viewer{UserID: "alice"}, jobID)
			if inspectErr != nil {
				t.Fatalf("Inspect failed Journey: %v", inspectErr)
			}
			if journey.Milestone != workflow.MilestoneFailed || journey.Failure == nil || journey.Failure.Code != workflow.FailureNoGroundedTitles || journey.Failure.Trace.Terminal != suggest.FailureSelectionEmpty {
				t.Fatalf("failed Journey = %+v", journey)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("job did not reach failed state")
}

func TestWorker_RecordsCommittedDiscoveryQualityStages(t *testing.T) {
	tests := []struct {
		name      string
		model     *testkit.LLM
		wantJob   string
		want      map[quality.Stage]quality.Outcome
		wantCands bool
	}{
		{
			name: "accepted grounded proposal",
			model: testkit.NewLLM(
				catalogSearchResponse(map[string]any{"query": "marigold"}),
				finalResponseWithNone(`{"picks":[{"mediaType":"movie","key":"movie:tmdb:90000603","name":"Marigold Protocol"}]}`),
			),
			wantJob: "done", wantCands: true,
			want: map[quality.Stage]quality.Outcome{
				quality.StageRetrieval:  quality.OutcomeSucceeded,
				quality.StageGeneration: quality.OutcomeSucceeded,
				quality.StageGrounding:  quality.OutcomeAccepted,
			},
		},
		{
			name: "empty retrieval abstains and rejects grounding",
			model: testkit.NewLLM(
				catalogSearchResponse(map[string]any{"query": "definitely absent"}),
				finalResponseWithNone(`{"picks":[]}`),
				finalResponseWithNone(`{"picks":[]}`),
			),
			wantJob: "failed",
			want: map[quality.Stage]quality.Outcome{
				quality.StageRetrieval:  quality.OutcomeEmpty,
				quality.StageGeneration: quality.OutcomeAbstained,
				quality.StageGrounding:  quality.OutcomeRejected,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := newStore(t)
			recorder := &testkit.QualityRecorder{}
			terminal := newDoneEmitter()
			svc := buildService(t, st, tt.model).
				WithProgressEmitter(terminal).
				WithQualityRecorder(recorder)
			jobID, err := svc.Submit(context.Background(), proposals.Intent{Description: "marigold"}, "alice")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go svc.Run(ctx)
			if tt.wantJob == "done" {
				waitForDoneEvent(t, terminal)
			} else {
				waitForFailedEvent(t, terminal)
			}

			job, err := st.GetJob(context.Background(), jobID)
			if err != nil || job.Status != tt.wantJob {
				t.Fatalf("terminal Job = %+v, %v; want status %q", job, err, tt.wantJob)
			}
			observations := recorder.Observations()
			if len(observations) != len(tt.want) {
				t.Fatalf("quality observations = %+v", observations)
			}
			seenKeys := make(map[string]bool, len(observations))
			for _, observation := range observations {
				if observation.Outcome != tt.want[observation.Stage] {
					t.Errorf("%s outcome = %q, want %q", observation.Stage, observation.Outcome, tt.want[observation.Stage])
				}
				if strings.Contains(observation.IdempotencyKey, jobID) || len(observation.IdempotencyKey) != 64 || seenKeys[observation.IdempotencyKey] {
					t.Errorf("idempotency key is not unique opaque SHA-256: %q", observation.IdempotencyKey)
				}
				seenKeys[observation.IdempotencyKey] = true
				if observation.Stage != quality.StageRetrieval && observation.CandidateCount != 0 {
					t.Errorf("%s candidate count = %d, want 0", observation.Stage, observation.CandidateCount)
				}
			}
			if tt.wantCands && observations[0].CandidateCount == 0 {
				t.Fatalf("successful retrieval did not record candidates: %+v", observations)
			}
		})
	}
}

func TestWorker_QualityRecordingFailureDoesNotFailCommittedProposal(t *testing.T) {
	st := newStore(t)
	recorder := &testkit.QualityRecorder{Err: errors.New("ledger unavailable")}
	terminal := newDoneEmitter()
	svc := buildService(t, st, testkit.NewLLM(
		catalogSearchResponse(map[string]any{"query": "marigold"}),
		finalResponseWithNone(`{"picks":[{"mediaType":"movie","key":"movie:tmdb:90000603","name":"Marigold Protocol"}]}`),
	)).WithProgressEmitter(terminal).WithQualityRecorder(recorder)
	jobID, err := svc.Submit(context.Background(), proposals.Intent{Description: "marigold"}, "alice")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go svc.Run(ctx)
	waitForDoneEvent(t, terminal)
	job, err := st.GetJob(context.Background(), jobID)
	if err != nil || job.Status != "done" {
		t.Fatalf("quality sink changed committed Job = %+v, %v", job, err)
	}
}

func TestWorker_DurableWorkflowNormalizesContextFailureTrace(t *testing.T) {
	for name, cause := range map[string]error{
		"raw cancellation": context.Canceled,
		"nested deadline":  fmt.Errorf("provider transport: %w", context.DeadlineExceeded),
	} {
		t.Run(name, func(t *testing.T) {
			st := newStore(t)
			flow := workflow.New(st, func() string { return "workflow-proposal-1" }, time.Now)
			ms := marigoldMediaServer(t)
			lib := library.New(library.Emby, ms.URL, ms.AdminToken, "dev-1")
			mt := testkit.NewTMDB(t)
			tm := tmdb.NewWithBase(mt.URL, "key")
			sug := suggest.New(testkit.NewLLM(), catalog.New(lib, tm), tm, 10).
				WithFeedback(failingFeedbackSource{err: cause})
			terminal := newDoneEmitter()
			svc := suggest.NewService(st, flow, sug, suggest.Config{Workers: 1, Timeout: time.Second, CacheTTL: time.Hour},
				time.Now, testkit.Logger()).
				WithProgressEmitter(terminal)

			jobID, err := svc.Submit(context.Background(), proposals.Intent{Description: "marigold"}, "alice")
			if err != nil {
				t.Fatal(err)
			}
			runCtx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go svc.Run(runCtx)
			waitForFailedEvent(t, terminal)

			job, err := st.GetJob(context.Background(), jobID)
			if err != nil {
				t.Fatal(err)
			}
			var trace proposals.DecisionTrace
			if err := json.Unmarshal([]byte(job.FailureTraceJSON), &trace); err != nil {
				t.Fatalf("context failure trace = %q: %v", job.FailureTraceJSON, err)
			}
			if job.FailureCode != suggest.FailureCodeGenerationFailed || trace.Version != suggest.DecisionTraceVersion || trace.Terminal != suggest.TerminalGenerationFailure {
				t.Fatalf("context failure was not normalized: code=%q trace=%+v", job.FailureCode, trace)
			}
		})
	}
}

func TestWorker_DurableRecurateRestoresChannelFeedbackScope(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	if _, err := st.SaveChannel(ctx, store.Channel{Channel: schedule.Channel{
		ID: "channel-a", Number: 71, Name: "Channel A", Status: schedule.StatusLive, IntentRef: "job-origin-a",
	}}); err != nil {
		t.Fatal(err)
	}
	feedback := &scopeFeedbackSource{signals: map[string][]suggest.FeedbackSignal{
		"":          {{Target: "movie:tmdb:90000603", Action: suggest.FeedbackNever}},
		"channel-a": {{Target: "movie:tmdb:90000603", Action: suggest.FeedbackSurprise}},
	}}
	llmMock := testkit.NewLLM(
		catalogSearchResponse(map[string]any{"query": "marigold"}),
		finalResponseWithNone(`{"picks":[{"mediaType":"movie","key":"movie:tmdb:90000603","name":"Marigold Protocol"}]}`),
	)
	flow := workflow.New(st, idGen(), time.Now)
	ms := marigoldMediaServer(t)
	mt := testkit.NewTMDB(t)
	tm := tmdb.NewWithBase(mt.URL, "key")
	suggester := suggest.New(llmMock,
		catalog.New(library.New(library.Emby, ms.URL, ms.AdminToken, "dev-1"), tm), tm, 10).
		WithFeedback(feedback)
	svc := suggest.NewService(st, flow, suggester,
		suggest.Config{Workers: 1, Timeout: time.Second, CacheTTL: time.Hour},
		time.Now, testkit.Logger())

	jobID, err := svc.Recurate(ctx, "channel-a", proposals.Intent{Description: "marigold"})
	if err != nil {
		t.Fatal(err)
	}
	requeued, err := st.GetJob(ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(requeued.IntentJSON, "channel-a") || strings.Contains(requeued.IntentJSON, "discoveryScope") {
		t.Fatalf("durable IntentJSON leaked execution scope: %s", requeued.IntentJSON)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go svc.Run(runCtx)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		job, getErr := st.GetJob(ctx, jobID)
		if getErr != nil {
			t.Fatal(getErr)
		}
		if job.Status == "done" {
			proposals, listErr := st.ListProposalsByStatus(ctx, "submitted")
			if listErr != nil || len(proposals) != 1 {
				t.Fatalf("submitted proposals = %+v, %v", proposals, listErr)
			}
			if scopes := feedback.Scopes(); len(scopes) != 1 || scopes[0] != "channel-a" {
				t.Fatalf("feedback scopes = %v, want [channel-a]", scopes)
			}
			return
		}
		if job.Status == "failed" {
			t.Fatalf("durable recurate failed without restored channel feedback: %s", job.LastError)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("durable recurate did not finish")
}

func TestWorker_DurableRecurateFailsClosedWithoutOneValidOwner(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*scopeLookupStore)
	}{
		{name: "missing"},
		{name: "lookup error", configure: func(st *scopeLookupStore) { st.err = errors.New("channel lookup unavailable") }},
		{name: "blank channel id", configure: func(st *scopeLookupStore) {
			st.override = true
			st.channel = store.Channel{Channel: schedule.Channel{ID: " ", IntentRef: "job-recurate"}}
		}},
		{name: "removed channel", configure: func(st *scopeLookupStore) {
			st.err = fmt.Errorf("%w: job names channel-gone", store.ErrJobChannelGone)
		}},
		{name: "detached channel", configure: func(st *scopeLookupStore) {
			st.override = true
			st.channel = store.Channel{Channel: schedule.Channel{
				ID: "channel-a", IntentRef: "job-recurate", Status: schedule.StatusDetached,
			}}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			base := newStore(t)
			serviceStore := &scopeLookupStore{Store: base}
			if tt.configure != nil {
				tt.configure(serviceStore)
			}
			now := time.Now().UTC()
			if err := base.CreateJob(ctx, store.Job{
				ID: "job-unrelated", Kind: "suggest", Status: "done", CreatedBy: "admin",
				IntentJSON: `{"description":"unrelated"}`, IntentHash: "unrelated",
				WorkflowVersion: workflow.WorkflowVersion1, CreatedAt: now, UpdatedAt: now,
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := base.SaveChannel(ctx, store.Channel{Channel: schedule.Channel{
				ID: "channel-unrelated", Number: 73, Name: "Unrelated Channel",
				Status: schedule.StatusLive, IntentRef: "job-unrelated",
			}}); err != nil {
				t.Fatal(err)
			}
			for _, event := range []store.DiscoveryFeedback{
				{ID: "household-never", ActorID: "admin", Scope: store.FeedbackHousehold,
					Target: "movie:tmdb:90000603", Action: store.FeedbackNever, CreatedAt: now},
				{ID: "unrelated-surprise", ActorID: "admin", Scope: store.FeedbackChannel, ScopeID: "channel-unrelated",
					Target: "movie:tmdb:90000603", Action: store.FeedbackSurprise, CreatedAt: now.Add(time.Second)},
			} {
				if err := base.AppendDiscoveryFeedback(ctx, event); err != nil {
					t.Fatal(err)
				}
			}
			if err := base.CreateJob(ctx, store.Job{
				ID: "job-recurate", Kind: "recurate", Status: "queued", CreatedBy: "admin",
				IntentJSON: `{"description":"marigold"}`, IntentHash: "marigold",
				WorkflowVersion: workflow.WorkflowVersion1, Deadline: now, CreatedAt: now, UpdatedAt: now,
			}); err != nil {
				t.Fatal(err)
			}
			llmMock := testkit.NewLLM(finalResponseWithNone(`{"picks":[]}`))
			feedback := &scopeFeedbackSource{signals: map[string][]suggest.FeedbackSignal{
				"":                  {{Target: "movie:tmdb:90000603", Action: suggest.FeedbackNever}},
				"channel-unrelated": {{Target: "movie:tmdb:90000603", Action: suggest.FeedbackSurprise}},
			}}
			ms := marigoldMediaServer(t)
			mt := testkit.NewTMDB(t)
			tm := tmdb.NewWithBase(mt.URL, "key")
			suggester := suggest.New(llmMock,
				catalog.New(library.New(library.Emby, ms.URL, ms.AdminToken, "dev-1"), tm), tm, 10).
				WithFeedback(feedback)
			flow := workflow.New(base, idGen(), time.Now)
			terminal := newDoneEmitter()
			svc := suggest.NewService(serviceStore, flow, suggester,
				suggest.Config{Workers: 1, Timeout: time.Second, CacheTTL: time.Hour},
				time.Now, testkit.Logger()).
				WithProgressEmitter(terminal)

			runCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			go svc.Run(runCtx)
			waitForFailedEvent(t, terminal)
			if scopes := feedback.Scopes(); len(scopes) != 0 {
				t.Fatalf("invalid recurate entered feedback/ranking pipeline with scopes %v", scopes)
			}
			job, err := base.GetJob(ctx, "job-recurate")
			if err != nil {
				t.Fatal(err)
			}
			if job.Status != "failed" || !strings.Contains(job.LastError, "resolve recurate channel") {
				t.Fatalf("ownerless recurate job = status %q error %q", job.Status, job.LastError)
			}
			if llmMock.Calls != 0 {
				t.Fatalf("ownerless recurate reached provider %d times", llmMock.Calls)
			}
			if requests := ms.Requests(); len(requests) != 0 {
				t.Fatalf("ownerless recurate reached Library: %+v", requests)
			}
			if requests := mt.Requests(); len(requests) != 0 {
				t.Fatalf("ownerless recurate reached TMDB: %+v", requests)
			}
			proposals, err := base.ListProposalsByCreator(ctx, "admin")
			if err != nil {
				t.Fatal(err)
			}
			if len(proposals) != 0 {
				t.Fatalf("invalid recurate persisted ranked proposals: %+v", proposals)
			}
		})
	}
}

func TestWorker_DurableFreshAndRefineStayHouseholdScoped(t *testing.T) {
	tests := []struct {
		name  string
		start func(context.Context, *suggest.Service, store.Store) (string, error)
	}{
		{name: "fresh", start: func(ctx context.Context, svc *suggest.Service, _ store.Store) (string, error) {
			return svc.Submit(ctx, proposals.Intent{Description: "marigold", DiscoveryScopeID: "channel-a"}, "admin")
		}},
		{name: "refine", start: func(ctx context.Context, svc *suggest.Service, st store.Store) (string, error) {
			return svc.Refine(ctx, "channel-a", proposals.Intent{Description: "marigold", DiscoveryScopeID: "channel-a"}, "admin")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			st := newStore(t)
			feedback := &scopeFeedbackSource{signals: map[string][]suggest.FeedbackSignal{
				"":          {{Target: "movie:tmdb:90000603", Action: suggest.FeedbackNever}},
				"channel-a": {{Target: "movie:tmdb:90000603", Action: suggest.FeedbackSurprise}},
			}}
			llmMock := testkit.NewLLM(
				catalogSearchResponse(map[string]any{"query": "marigold"}),
				finalResponseWithNone(`{"picks":[{"mediaType":"movie","key":"movie:tmdb:90000603","name":"Marigold Protocol"}]}`),
			)
			ms := marigoldMediaServer(t)
			mt := testkit.NewTMDB(t)
			tm := tmdb.NewWithBase(mt.URL, "key")
			suggester := suggest.New(llmMock,
				catalog.New(library.New(library.Emby, ms.URL, ms.AdminToken, "dev-1"), tm), tm, 10).
				WithFeedback(feedback)
			flow := workflow.New(st, func() string { return "job-fresh" }, time.Now)
			terminal := newDoneEmitter()
			svc := suggest.NewService(st, flow, suggester,
				suggest.Config{Workers: 1, Timeout: time.Second, CacheTTL: time.Hour},
				time.Now, testkit.Logger()).
				WithProgressEmitter(terminal)

			jobID, err := tt.start(ctx, svc, st)
			if err != nil {
				t.Fatal(err)
			}
			queued, err := st.GetJob(ctx, jobID)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(queued.IntentJSON, "channel-a") || strings.Contains(queued.IntentJSON, "discoveryScope") {
				t.Fatalf("%s durable IntentJSON leaked caller scope: %s", tt.name, queued.IntentJSON)
			}
			runCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			go svc.Run(runCtx)
			waitForFailedEvent(t, terminal)
			job, err := st.GetJob(ctx, jobID)
			if err != nil {
				t.Fatal(err)
			}
			if job.Status != "failed" || !strings.Contains(job.LastError, "no grounded titles") {
				t.Fatalf("%s used channel feedback: status=%q error=%q", tt.name, job.Status, job.LastError)
			}
			if scopes := feedback.Scopes(); len(scopes) != 1 || scopes[0] != "" {
				t.Fatalf("%s feedback scopes = %v, want household only", tt.name, scopes)
			}
		})
	}
}

// A hung LLM hits JOB_TIMEOUT and the job fails cleanly; the pool keeps draining
// OTHER jobs (§8 — one hung call can't starve the queue).
func TestWorker_HungLLMTimesOut_PoolKeepsDraining(t *testing.T) {
	st := newStore(t)
	// The first turn surfaces grounded candidates; the second hangs. This proves
	// timeout normalization preserves safe facts gathered before generation ended.
	slow := testkit.NewLLM(
		catalogSearchResponse(map[string]any{"query": "marigold"}),
		finalResponseWithNone(`{"channelName":"Late","picks":[]}`),
	)
	slow.OnChat = func() {
		if slow.Calls == 1 {
			slow.Delay = 2 * time.Second
		}
	}
	ms := marigoldMediaServer(t)
	lib := library.New(library.Emby, ms.URL, ms.AdminToken, "dev-1")
	mt := testkit.NewTMDB(t)
	tm := tmdb.NewWithBase(mt.URL, "key")
	cat := catalog.New(lib, tm)
	sug := suggest.New(slow, cat, tm, 10)
	terminal := newDoneEmitter()
	svc := suggest.NewService(st, workflow.New(st, idGen(), time.Now), sug,
		suggest.Config{Workers: 2, Timeout: 200 * time.Millisecond, CacheTTL: time.Hour},
		time.Now, testkit.Logger()).WithProgressEmitter(terminal)

	hungID, _ := svc.Submit(context.Background(), proposals.Intent{Description: "hangs"}, "alice")

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
	if j.FailureCode != suggest.FailureCodeGenerationFailed {
		t.Errorf("timeout failure code = %q, want bounded generic code", j.FailureCode)
	}
	var trace proposals.DecisionTrace
	if err := json.Unmarshal([]byte(j.FailureTraceJSON), &trace); err != nil {
		t.Fatalf("timeout failure trace = %q: %v", j.FailureTraceJSON, err)
	}
	if trace.Version != suggest.DecisionTraceVersion || trace.Terminal != suggest.TerminalGenerationFailure || len(trace.Candidates) == 0 {
		t.Fatalf("timeout failure trace did not preserve bounded candidate facts: %+v", trace)
	}
}

type lifecycleErrorStore struct {
	store.Store
	successErr    error
	failureErr    error
	successCalled chan struct{}
	failureCalled chan struct{}
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
	cause, failureCode, failureTraceJSON string,
	updatedAt time.Time,
) error {
	select {
	case s.failureCalled <- struct{}{}:
	default:
	}
	if s.failureErr != nil {
		return s.failureErr
	}
	return s.Store.CommitSuggestionFailure(ctx, jobID, expectedAttempt, cause, failureCode, failureTraceJSON, updatedAt)
}

func TestWorker_UndurableFailureEmitsNoTerminalEvent(t *testing.T) {
	ctx := context.Background()
	base := newStore(t)
	st := newLifecycleErrorStore(base)
	st.failureErr = errors.New("write unavailable")
	llmMock := testkit.NewLLM(
		finalResponseWithNone(`{"picks":[]}`),
		finalResponseWithNone(`{"picks":[]}`),
	)
	svc := buildService(t, st, llmMock)
	terminal := newDoneEmitter()
	svc.WithProgressEmitter(terminal)
	jobID, err := svc.Submit(ctx, proposals.Intent{Description: "Classic Kettle Kids episodes"}, "alice")
	if err != nil {
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
	job, err := base.GetJob(ctx, jobID)
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
}

func TestWorker_StaleSuccessDoesNotFailReplacement(t *testing.T) {
	ctx := context.Background()
	base := newStore(t)
	st := newLifecycleErrorStore(base)
	st.successErr = store.ErrJobNotRunning
	llmMock := testkit.NewLLM(
		catalogSearchResponse(map[string]any{"query": "marigold"}),
		finalResponseWithNone(`{"picks":[{"mediaType":"movie","key":"movie:tmdb:90000603","name":"Marigold Protocol"}]}`),
	)
	svc := buildService(t, st, llmMock)
	terminal := newDoneEmitter()
	svc.WithProgressEmitter(terminal)
	jobID, err := svc.Submit(ctx, proposals.Intent{Description: "sci-fi"}, "alice")
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
	case <-time.After(300 * time.Millisecond):
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
	a := proposals.Intent{Description: "  90s Action  ", MustInclude: []string{"Quiet Engines", "lemon tree heist"}}
	b := proposals.Intent{Description: "90s action", MustInclude: []string{"lemon tree heist", "Quiet Engines"}}
	if suggest.IntentHash(a) != suggest.IntentHash(b) {
		t.Error("case/whitespace/order differences should hash the same")
	}
	c := proposals.Intent{Description: "90s comedy"}
	if suggest.IntentHash(a) == suggest.IntentHash(c) {
		t.Error("different intents should hash differently")
	}
}

// A refine's change text + current lineup are part of the intent identity, so two
// refines of the same channel (or a refine vs the original) never collide in the cache.
func TestIntentHash_DistinguishesRefine(t *testing.T) {
	base := proposals.Intent{Description: "90s action"}
	refineA := base
	refineA.RefineText = "add more Ansel Brightwater"
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
	l1.CurrentLineup = []proposals.LineupContext{{Key: "movie:tmdb:90000603"}, {Key: "movie:tmdb:90000165"}}
	l2 := refineA
	l2.CurrentLineup = []proposals.LineupContext{{Key: "movie:tmdb:90000165"}, {Key: "movie:tmdb:90000603"}}
	if suggest.IntentHash(l1) != suggest.IntentHash(l2) {
		t.Error("current-lineup order must not change the hash")
	}
	if suggest.IntentHash(refineA) == suggest.IntentHash(l1) {
		t.Error("a different current lineup must change the hash")
	}
}

// Refine and recurate each submit a NEW job that names the channel. The request the channel was
// made from is never touched: it stays done, keeps its intent, and stays in its requester's list.
func TestRefineAndRecurate_SubmitNewChannelJobs(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	svc := buildService(t, st, testkit.NewLLM())

	if err := st.CreateJob(ctx, store.Job{
		ID: "job-orig", Kind: "suggest", Status: "done",
		IntentJSON: `{"description":"90s action"}`, CreatedBy: "alex",
	}); err != nil {
		t.Fatal(err)
	}

	refineID, err := svc.Refine(ctx, "ch-1", proposals.Intent{Description: "90s action", RefineText: "add more Ansel Brightwater"}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	recurateID, err := svc.Recurate(ctx, "ch-1", proposals.Intent{Description: "90s action", CurrentLineup: []proposals.LineupContext{{Key: "movie:tmdb:90000603"}}})
	if err != nil {
		t.Fatal(err)
	}
	if refineID == "job-orig" || recurateID == "job-orig" || refineID == recurateID {
		t.Fatalf("channel jobs reused an id: refine %q recurate %q", refineID, recurateID)
	}

	refine, err := st.GetJob(ctx, refineID)
	if err != nil {
		t.Fatal(err)
	}
	if refine.Kind != "suggest" || refine.Status != "queued" || refine.ChannelID != "ch-1" || refine.CreatedBy != "admin" {
		t.Errorf("refine job = kind %q status %q channel %q owner %q", refine.Kind, refine.Status, refine.ChannelID, refine.CreatedBy)
	}
	if !strings.Contains(refine.IntentJSON, "add more Ansel Brightwater") {
		t.Errorf("refine job intent = %q, want the refine text", refine.IntentJSON)
	}
	recurate, err := st.GetJob(ctx, recurateID)
	if err != nil {
		t.Fatal(err)
	}
	if recurate.Kind != "recurate" || recurate.Status != "queued" || recurate.ChannelID != "ch-1" || recurate.CreatedBy != "" {
		t.Errorf("recurate job = kind %q status %q channel %q owner %q; want system work with no requester",
			recurate.Kind, recurate.Status, recurate.ChannelID, recurate.CreatedBy)
	}

	orig, err := st.GetJob(ctx, "job-orig")
	if err != nil {
		t.Fatal(err)
	}
	if orig.Status != "done" || orig.Kind != "suggest" || orig.IntentJSON != `{"description":"90s action"}` || orig.CreatedBy != "alex" {
		t.Errorf("the channel's original request was modified: %+v", orig)
	}
	// Request lists: the requester still sees their own request, the admin sees their refine,
	// and the system's re-curation is in nobody's list.
	alex, err := st.ListProposalJobIDsByCreator(ctx, "alex", 10)
	if err != nil || len(alex) != 1 || alex[0] != "job-orig" {
		t.Errorf("requester's list = %v, %v; want [job-orig]", alex, err)
	}
	admin, err := st.ListProposalJobIDsByCreator(ctx, "admin", 10)
	if err != nil || len(admin) != 1 || admin[0] != refineID {
		t.Errorf("admin's list = %v, %v; want [%s]", admin, err, refineID)
	}
	all, err := st.ListProposalJobIDs(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range all {
		if id == recurateID {
			t.Errorf("re-curation job %s appears in the request list %v", recurateID, all)
		}
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
	if err := st.UpsertUser(ctx, identitystore.User{ID: "grace", AutoApprove: true, Quota: 100}); err != nil {
		t.Fatal(err)
	}

	llmMock := testkit.NewLLM(
		catalogSearchResponse(map[string]any{"query": "marigold"}),
		finalResponseWithNone(`{"picks":[{"mediaType":"movie","key":"movie:tmdb:90000603","name":"Marigold Protocol"}]}`),
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

	jobID, err := svc.Submit(ctx, proposals.Intent{Description: "sci-fi"}, "grace")
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
	if err := st.UpsertUser(ctx, identitystore.User{ID: "grace", AutoApprove: true, Quota: 100}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveChannel(ctx, store.Channel{Channel: schedule.Channel{
		ID: "channel-recurate", Number: 72, Name: "Recurate Channel",
		Status: schedule.StatusLive, IntentRef: "job-origin",
	}}); err != nil {
		t.Fatal(err)
	}
	// Even a re-curation job that somehow carries a requester with a grant stays out of that
	// grant: the durable kind, not the owner, is the authorization discriminator.
	now := time.Now().UTC()
	if err := st.CreateJob(ctx, store.Job{
		ID: "job-recurate", Kind: "recurate", Status: "queued", CreatedBy: "grace", ChannelID: "channel-recurate",
		IntentJSON: `{"description":"sci-fi","currentLineup":[{"key":"movie:tmdb:90000603"}]}`,
		Deadline:   now, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	llmMock := testkit.NewLLM(
		catalogSearchResponse(map[string]any{"query": "marigold"}),
		finalResponseWithNone(`{"picks":[{"mediaType":"movie","key":"movie:tmdb:90000603","name":"Marigold Protocol"}]}`),
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
	if err := st.UpsertUser(ctx, identitystore.User{ID: "grace", AutoApprove: true, Quota: 100}); err != nil {
		t.Fatal(err)
	}
	llmMock := testkit.NewLLM(
		catalogSearchResponse(map[string]any{"query": "marigold"}),
		finalResponseWithNone(`{"picks":[{"mediaType":"movie","key":"movie:tmdb:90000603","name":"Marigold Protocol"}]}`),
	)
	svc := buildService(t, st, llmMock)
	channels := &testkit.ApprovalChannels{PlanError: fmt.Errorf("cannot build local channel")}
	approver := suggest.NewApprover(st, channels, time.Now)
	svc = svc.WithAutoApprove(suggest.NewAutoApprover(
		st, approver, func(context.Context) int { return 100 }, testkit.Logger()))

	jobID, err := svc.Submit(ctx, proposals.Intent{Description: "sci-fi"}, "grace")
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

// A refine of a channel is reviewed by an admin in the refine panel. The refining person's
// auto-approve grant never applies to it, even when they hold one: the grant is for a member's own
// requests, and approving a lineup change unattended is the channel's auto-curate grant's job.
func TestWorker_ChannelRefineSkipsRequesterAutoApprove(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	if err := st.UpsertUser(ctx, identitystore.User{ID: "grace", AutoApprove: true, Quota: 100}); err != nil {
		t.Fatal(err)
	}
	llmMock := testkit.NewLLM(
		catalogSearchResponse(map[string]any{"query": "marigold"}),
		finalResponseWithNone(`{"picks":[{"mediaType":"movie","key":"movie:tmdb:90000603","name":"Marigold Protocol"}]}`),
	)
	channels := &testkit.ApprovalChannels{}
	approver := suggest.NewApprover(st, channels, time.Now)
	done := newDoneEmitter()
	svc := buildService(t, st, llmMock).
		WithAutoApprove(suggest.NewAutoApprover(st, approver, func(context.Context) int { return 100 }, testkit.Logger())).
		WithProgressEmitter(done)
	if _, err := svc.Refine(ctx, "channel-1", proposals.Intent{Description: "sci-fi", RefineText: "more"}, "grace"); err != nil {
		t.Fatal(err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go svc.Run(runCtx)
	waitForDoneEvent(t, done)
	if approved, _ := st.ListProposalsByStatus(ctx, "approved"); len(approved) != 0 {
		t.Fatalf("a refine went through the requester's auto-approve grant: %+v", approved)
	}
	if submitted, _ := st.ListProposalsByStatus(ctx, "submitted"); len(submitted) != 1 {
		t.Fatalf("submitted proposals = %d, want 1 left for the refine panel", len(submitted))
	}
	if len(channels.Plans) != 0 {
		t.Fatalf("auto-approver planned %d channels for a refine", len(channels.Plans))
	}
}
