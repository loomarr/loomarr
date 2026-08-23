package suggest_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/suggest"
)

// quotaStore is an in-memory stand-in for the slices AutoApprover needs. It is a fake,
// not a mock: the assertions are about what ends up in it, not about which calls it saw.
type quotaStore struct {
	users     map[string]store.User
	titles    map[provision.Key]provision.Record
	proposals map[string]store.Proposal
	channels  map[string]store.Channel
	titleErr  error

	dataMu           sync.RWMutex
	quotaMu          sync.Mutex
	quotaStateMu     sync.Mutex
	quotaHeld        bool
	requireQuotaLock bool
	quotaLockErr     error
}

func newQuotaStore() *quotaStore {
	return &quotaStore{
		users:     map[string]store.User{},
		titles:    map[provision.Key]provision.Record{},
		proposals: map[string]store.Proposal{},
		channels:  map[string]store.Channel{},
	}
}

func (q *quotaStore) GetUser(_ context.Context, id string) (store.User, error) {
	q.dataMu.RLock()
	defer q.dataMu.RUnlock()
	u, ok := q.users[id]
	if !ok {
		return store.User{}, store.ErrNotFound
	}
	return u, nil
}

func (q *quotaStore) GetTitle(_ context.Context, key provision.Key) (provision.Record, error) {
	if err := q.requireHeldQuotaLock(); err != nil {
		return provision.Record{}, err
	}
	if q.titleErr != nil {
		return provision.Record{}, q.titleErr
	}
	q.dataMu.RLock()
	defer q.dataMu.RUnlock()
	r, ok := q.titles[key]
	if !ok {
		return provision.Record{}, store.ErrNotFound
	}
	return r, nil
}

func (q *quotaStore) NewestProposalByStatusForJob(_ context.Context, jobID, status string) (store.Proposal, error) {
	q.dataMu.RLock()
	defer q.dataMu.RUnlock()
	var newest store.Proposal
	for _, p := range q.proposals {
		if p.JobID == jobID && p.Status == status && (newest.ID == "" || p.CreatedAt.After(newest.CreatedAt)) {
			newest = p
		}
	}
	if newest.ID == "" {
		return store.Proposal{}, store.ErrNotFound
	}
	return newest, nil
}

func (q *quotaStore) CommitProposalApproval(ctx context.Context, commit store.ProposalApproval) (int, error) {
	var enqueued int
	err := q.withApprovalOrder(ctx, func() error {
		var err error
		enqueued, err = q.commitProposalApproval(commit)
		return err
	})
	return enqueued, err
}

func (q *quotaStore) CommitProposalApprovalGuarded(
	ctx context.Context,
	commit store.ProposalApproval,
	guard store.ProposalApprovalGuard,
) (int, error) {
	var enqueued int
	err := q.withApprovalOrder(ctx, func() error {
		if err := guard(ctx, q); err != nil {
			return err
		}
		var err error
		enqueued, err = q.commitProposalApproval(commit)
		return err
	})
	return enqueued, err
}

func (q *quotaStore) commitProposalApproval(commit store.ProposalApproval) (int, error) {
	q.dataMu.Lock()
	defer q.dataMu.Unlock()
	p, ok := q.proposals[commit.Proposal.ID]
	if !ok {
		return 0, store.ErrNotFound
	}
	if p.Status != "submitted" {
		return 0, store.ErrProposalNotSubmitted
	}
	q.proposals[p.ID] = commit.Proposal
	q.channels[commit.Channel.ID] = commit.Channel
	enqueued := 0
	for _, rec := range commit.Titles {
		if _, exists := q.titles[rec.Key]; exists {
			continue
		}
		q.titles[rec.Key] = rec
		if rec.State == provision.Wanted {
			enqueued++
		}
	}
	return enqueued, nil
}

func (q *quotaStore) ListProposalsByCreator(_ context.Context, userID string) ([]store.Proposal, error) {
	if err := q.requireHeldQuotaLock(); err != nil {
		return nil, err
	}
	q.dataMu.RLock()
	defer q.dataMu.RUnlock()
	var out []store.Proposal
	for _, p := range q.proposals {
		if p.CreatedBy == userID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (q *quotaStore) withApprovalOrder(ctx context.Context, fn func() error) error {
	if q.quotaLockErr != nil {
		return q.quotaLockErr
	}
	q.quotaMu.Lock()
	defer q.quotaMu.Unlock()

	q.quotaStateMu.Lock()
	q.quotaHeld = true
	q.quotaStateMu.Unlock()
	defer func() {
		q.quotaStateMu.Lock()
		q.quotaHeld = false
		q.quotaStateMu.Unlock()
	}()
	return fn()
}

func (q *quotaStore) requireHeldQuotaLock() error {
	q.quotaStateMu.Lock()
	defer q.quotaStateMu.Unlock()
	if q.requireQuotaLock && !q.quotaHeld {
		return errors.New("quota operation ran outside the requester lock")
	}
	return nil
}

func (q *quotaStore) quotaLockHeld() bool {
	q.quotaStateMu.Lock()
	defer q.quotaStateMu.Unlock()
	return q.quotaHeld
}

// proposalWith builds a stored proposal whose acquisitions are the given TMDB movie ids.
func proposalWith(id, createdBy, status string, tmdbIDs ...int) store.Proposal {
	items := make([]suggest.ProposalItem, 0, len(tmdbIDs))
	for _, n := range tmdbIDs {
		items = append(items, suggest.ProposalItem{MediaType: "movie", TMDBID: n, Name: "Film"})
	}
	blob, _ := json.Marshal(suggest.Proposal{Acquisitions: items})
	return store.Proposal{ID: id, JobID: "job-" + id, CreatedBy: createdBy, Status: status, ProposalJSON: string(blob)}
}

type quotaChannels struct {
	after func()
}

func (quotaChannels) PlanApprovedChannel(_ context.Context, p store.Proposal) (store.Channel, error) {
	ch := store.Channel{}
	ch.ID = "ch-" + p.ID
	ch.IntentRef = p.JobID
	return ch, nil
}

func (q quotaChannels) AfterApprovalCommitted(context.Context, string) {
	if q.after != nil {
		q.after()
	}
}

func movieKey(t *testing.T, tmdbID int) provision.Key {
	t.Helper()
	key, err := provision.Title{MediaType: provision.Movie, TMDBID: tmdbID}.Key()
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func autoApprover(st *quotaStore, defaultLimit int) *suggest.AutoApprover {
	return autoApproverWithChannels(st, defaultLimit, quotaChannels{})
}

func autoApproverWithChannels(st *quotaStore, defaultLimit int, channels quotaChannels) *suggest.AutoApprover {
	now := func() time.Time { return time.Unix(1_700_000_000, 0) }
	approver := suggest.NewApprover(st, channels, now)
	return suggest.NewAutoApprover(
		st,
		approver,
		func(context.Context) int { return defaultLimit },
		slog.New(slog.DiscardHandler),
	)
}

// §19's long-standing gate requirement: "auto_approve respects quota". It could not be
// written until now — quota was stored and enforced by nothing, and auto-approve did not
// exist at all.
func TestAutoApprove_RespectsQuota(t *testing.T) {
	ctx := context.Background()

	t.Run("approves while within the cap", func(t *testing.T) {
		st := newQuotaStore()
		st.users["grace"] = store.User{ID: "grace", AutoApprove: true, Quota: 3}
		p := proposalWith("p1", "grace", "submitted", 101, 102)
		st.proposals["p1"] = p

		d, err := autoApprover(st, 5).Consider(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		if !d.Approved {
			t.Fatalf("expected auto-approval within quota, got %+v", d)
		}
		if d.Enqueued != 2 {
			t.Errorf("enqueued %d, want 2", d.Enqueued)
		}
		// The audit must say a machine decided (§11).
		if got := st.proposals["p1"].ApprovedBy; got != suggest.AutoApprovedBy {
			t.Errorf("approvedBy = %q, want %q", got, suggest.AutoApprovedBy)
		}
		if st.titles[movieKey(t, 101)].State != provision.Wanted {
			t.Error("acquisition did not become a wanted title")
		}
		if got := st.channels["ch-p1"].IntentRef; got != p.JobID {
			t.Errorf("approved channel intentRef = %q, want %q", got, p.JobID)
		}
	})

	// The load-bearing case: over quota, the proposal must NOT acquire anything.
	t.Run("holds an over-cap proposal for an admin without enqueueing", func(t *testing.T) {
		st := newQuotaStore()
		st.users["grace"] = store.User{ID: "grace", AutoApprove: true, Quota: 2}
		// Two already pending from an earlier approved proposal.
		st.proposals["old"] = proposalWith("old", "grace", "approved", 201, 202)
		st.titles[movieKey(t, 201)] = provision.Record{Key: movieKey(t, 201), State: provision.Wanted}
		st.titles[movieKey(t, 202)] = provision.Record{Key: movieKey(t, 202), State: provision.Downloading}

		p := proposalWith("p2", "grace", "submitted", 301)
		st.proposals["p2"] = p

		d, err := autoApprover(st, 5).Consider(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		if d.Approved {
			t.Fatal("auto-approved over the cap — the gate did nothing")
		}
		if d.Reason == "" {
			t.Error("no reason recorded; 'why is this still queued?' must be answerable")
		}
		// Nothing spent, and the proposal is still available to an admin.
		if _, ok := st.titles[movieKey(t, 301)]; ok {
			t.Error("an over-quota proposal enqueued an acquisition")
		}
		if st.proposals["p2"].Status != "submitted" {
			t.Errorf("status = %q, want it left submitted for an admin (never denied)", st.proposals["p2"].Status)
		}
	})

	// A landed title stops counting: the cap is on PENDING acquisitions, so quota frees
	// up as content arrives rather than being a lifetime budget.
	t.Run("terminal titles no longer count against the cap", func(t *testing.T) {
		st := newQuotaStore()
		st.users["grace"] = store.User{ID: "grace", AutoApprove: true, Quota: 2}
		st.proposals["old"] = proposalWith("old", "grace", "approved", 201, 202)
		st.titles[movieKey(t, 201)] = provision.Record{Key: movieKey(t, 201), State: provision.Available}
		st.titles[movieKey(t, 202)] = provision.Record{Key: movieKey(t, 202), State: provision.Unavailable}

		p := proposalWith("p3", "grace", "submitted", 301)
		st.proposals["p3"] = p

		d, err := autoApprover(st, 5).Consider(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		if !d.Approved {
			t.Fatalf("landed titles should free the cap, got %+v", d)
		}
	})

	// A title someone else already caused costs this user nothing — otherwise a cap
	// would depend on unrelated activity.
	t.Run("an existing title is not charged again", func(t *testing.T) {
		st := newQuotaStore()
		st.users["grace"] = store.User{ID: "grace", AutoApprove: true, Quota: 1}
		st.proposals["old"] = proposalWith("old", "grace", "approved", 201)
		st.titles[movieKey(t, 201)] = provision.Record{Key: movieKey(t, 201), State: provision.Wanted}
		// The new proposal asks for a title that ALREADY exists (someone else's).
		st.titles[movieKey(t, 301)] = provision.Record{Key: movieKey(t, 301), State: provision.Wanted}

		p := proposalWith("p4", "grace", "submitted", 301)
		st.proposals["p4"] = p

		d, err := autoApprover(st, 5).Consider(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		if !d.Approved {
			t.Fatalf("a proposal costing nothing new should fit any cap, got %+v", d)
		}
	})

	t.Run("falls back to the default cap when the user's quota is 0", func(t *testing.T) {
		st := newQuotaStore()
		st.users["grace"] = store.User{ID: "grace", AutoApprove: true, Quota: 0}
		st.proposals["old"] = proposalWith("old", "grace", "approved", 201)
		st.titles[movieKey(t, 201)] = provision.Record{Key: movieKey(t, 201), State: provision.Wanted}

		p := proposalWith("p5", "grace", "submitted", 301)
		st.proposals["p5"] = p

		// Default of 1, already 1 pending → over.
		if d, _ := autoApprover(st, 1).Consider(ctx, p); d.Approved {
			t.Error("quota 0 must fall back to suggest.max_acquisitions, not mean unlimited")
		}
	})
}

func TestAutoApprove_ConcurrentProposalsShareTheRemainingQuota(t *testing.T) {
	ctx := context.Background()
	st := newQuotaStore()
	st.requireQuotaLock = true
	st.users["grace"] = store.User{ID: "grace", AutoApprove: true, Quota: 1}
	for _, p := range []store.Proposal{
		proposalWith("race-a", "grace", "submitted", 401),
		proposalWith("race-b", "grace", "submitted", 402),
	} {
		st.proposals[p.ID] = p
	}

	start := make(chan struct{})
	type outcome struct {
		decision suggest.Decision
		err      error
	}
	outcomes := make(chan outcome, 2)
	for _, id := range []string{"race-a", "race-b"} {
		p := st.proposals[id]
		go func() {
			<-start
			decision, err := autoApprover(st, 5).Consider(ctx, p)
			outcomes <- outcome{decision: decision, err: err}
		}()
	}
	close(start)

	approved := 0
	held := 0
	for range 2 {
		result := <-outcomes
		if result.err != nil {
			t.Fatalf("concurrent auto-approval failed: %v", result.err)
		}
		if result.decision.Approved {
			approved++
		} else if result.decision.Reason == "over the pending-acquisition cap" {
			held++
		}
	}
	if approved != 1 || held != 1 {
		t.Fatalf("concurrent decisions = %d approved, %d held; want one of each", approved, held)
	}

	wanted := 0
	for _, key := range []provision.Key{movieKey(t, 401), movieKey(t, 402)} {
		if rec, ok := st.titles[key]; ok && rec.State == provision.Wanted {
			wanted++
		}
	}
	if wanted != 1 {
		t.Fatalf("concurrent auto-approval inserted %d wanted titles, want 1", wanted)
	}
}

func TestAutoApprove_ReleasesQuotaOrderingBeforePostCommitWork(t *testing.T) {
	ctx := context.Background()
	st := newQuotaStore()
	st.users["grace"] = store.User{ID: "grace", AutoApprove: true, Quota: 1}
	p := proposalWith("post-commit", "grace", "submitted", 450)
	st.proposals[p.ID] = p

	afterCalled := false
	lockHeldDuringAfter := false
	auto := autoApproverWithChannels(st, 1, quotaChannels{after: func() {
		afterCalled = true
		lockHeldDuringAfter = st.quotaLockHeld()
	}})
	decision, err := auto.Consider(ctx, p)
	if err != nil || !decision.Approved {
		t.Fatalf("auto-approval = (%+v, %v), want approved", decision, err)
	}
	if !afterCalled {
		t.Fatal("post-commit channel work did not run")
	}
	if lockHeldDuringAfter {
		t.Fatal("post-commit channel work ran while requester ordering was still held")
	}
}

// Without the grant, nothing changes: every proposal waits for an admin (§7).
func TestAutoApprove_RequiresTheGrant(t *testing.T) {
	ctx := context.Background()
	st := newQuotaStore()
	st.users["grace"] = store.User{ID: "grace", AutoApprove: false, Quota: 99}
	p := proposalWith("p1", "grace", "submitted", 101)
	st.proposals["p1"] = p

	d, err := autoApprover(st, 99).Consider(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if d.Approved {
		t.Fatal("approved without the auto_approve grant")
	}
	if _, ok := st.titles[movieKey(t, 101)]; ok {
		t.Error("enqueued an acquisition without the grant")
	}
}

// Fail CLOSED. An unreadable user, or a disabled one, must not auto-approve — the safe
// direction for a gate that spends money.
func TestAutoApprove_FailsClosed(t *testing.T) {
	ctx := context.Background()

	t.Run("unknown requester", func(t *testing.T) {
		st := newQuotaStore()
		p := proposalWith("p1", "ghost", "submitted", 101)
		st.proposals["p1"] = p
		if d, _ := autoApprover(st, 99).Consider(ctx, p); d.Approved {
			t.Error("auto-approved for a user that could not be read")
		}
	})

	t.Run("disabled requester", func(t *testing.T) {
		st := newQuotaStore()
		st.users["grace"] = store.User{ID: "grace", AutoApprove: true, Quota: 99, Disabled: true}
		p := proposalWith("p1", "grace", "submitted", 101)
		st.proposals["p1"] = p
		if d, _ := autoApprover(st, 99).Consider(ctx, p); d.Approved {
			t.Error("auto-approved for a disabled user")
		}
	})

	t.Run("no requester at all", func(t *testing.T) {
		st := newQuotaStore()
		p := proposalWith("p1", "", "submitted", 101)
		if d, _ := autoApprover(st, 99).Consider(ctx, p); d.Approved {
			t.Error("auto-approved a proposal with no requester")
		}
	})

	t.Run("quota title read fails", func(t *testing.T) {
		st := newQuotaStore()
		st.users["grace"] = store.User{ID: "grace", AutoApprove: true, Quota: 99}
		st.proposals["old"] = proposalWith("old", "grace", "approved", 201)
		p := proposalWith("p1", "grace", "submitted", 101)
		st.proposals[p.ID] = p
		st.titleErr = errors.New("database unavailable")
		d, err := autoApprover(st, 99).Consider(ctx, p)
		if err == nil || d.Approved {
			t.Fatalf("quota read failure = (%+v, %v), want closed with error", d, err)
		}
		if st.proposals[p.ID].Status != "submitted" {
			t.Errorf("proposal status = %q, want submitted", st.proposals[p.ID].Status)
		}
	})

	t.Run("requester lock fails", func(t *testing.T) {
		st := newQuotaStore()
		st.users["grace"] = store.User{ID: "grace", AutoApprove: true, Quota: 99}
		p := proposalWith("p1", "grace", "submitted", 101)
		st.proposals[p.ID] = p
		st.quotaLockErr = errors.New("database lock unavailable")
		d, err := autoApprover(st, 99).Consider(ctx, p)
		if err == nil || d.Approved || d.Reason != "quota unavailable" {
			t.Fatalf("quota lock failure = (%+v, %v), want closed with quota-unavailable error", d, err)
		}
		if st.proposals[p.ID].Status != "submitted" {
			t.Errorf("proposal status = %q, want submitted", st.proposals[p.ID].Status)
		}
	})

	t.Run("approved audit is malformed", func(t *testing.T) {
		st := newQuotaStore()
		st.users["grace"] = store.User{ID: "grace", AutoApprove: true, Quota: 99}
		st.proposals["old"] = store.Proposal{ID: "old", CreatedBy: "grace", Status: "approved", ProposalJSON: `{`}
		p := proposalWith("p1", "grace", "submitted", 101)
		st.proposals[p.ID] = p
		d, err := autoApprover(st, 99).Consider(ctx, p)
		if err == nil || d.Approved {
			t.Fatalf("malformed approved audit = (%+v, %v), want closed with error", d, err)
		}
		if st.proposals[p.ID].Status != "submitted" {
			t.Errorf("proposal status = %q, want submitted", st.proposals[p.ID].Status)
		}
	})
}
