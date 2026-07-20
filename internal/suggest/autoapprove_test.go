package suggest_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/suggest"
)

// quotaStore is an in-memory stand-in for the slices AutoApprover needs. It is a fake,
// not a mock: the assertions are about what ends up in it, not about which calls it saw.
type quotaStore struct {
	users     map[string]store.User
	titles    map[provision.Key]provision.Record
	proposals map[string]store.Proposal
}

func newQuotaStore() *quotaStore {
	return &quotaStore{
		users:     map[string]store.User{},
		titles:    map[provision.Key]provision.Record{},
		proposals: map[string]store.Proposal{},
	}
}

func (q *quotaStore) GetUser(_ context.Context, id string) (store.User, error) {
	u, ok := q.users[id]
	if !ok {
		return store.User{}, store.ErrNotFound
	}
	return u, nil
}

func (q *quotaStore) GetTitle(_ context.Context, key provision.Key) (provision.Record, error) {
	r, ok := q.titles[key]
	if !ok {
		return provision.Record{}, store.ErrNotFound
	}
	return r, nil
}

func (q *quotaStore) UpsertTitle(_ context.Context, rec provision.Record) error {
	q.titles[rec.Key] = rec
	return nil
}

func (q *quotaStore) UpdateProposal(_ context.Context, p store.Proposal) error {
	q.proposals[p.ID] = p
	return nil
}

func (q *quotaStore) ListProposalsByCreator(_ context.Context, userID string) ([]store.Proposal, error) {
	var out []store.Proposal
	for _, p := range q.proposals {
		if p.CreatedBy == userID {
			out = append(out, p)
		}
	}
	return out, nil
}

// proposalWith builds a stored proposal whose acquisitions are the given TMDB movie ids.
func proposalWith(id, createdBy, status string, tmdbIDs ...int) store.Proposal {
	items := make([]suggest.ProposalItem, 0, len(tmdbIDs))
	for _, n := range tmdbIDs {
		items = append(items, suggest.ProposalItem{MediaType: "movie", TMDBID: n, Name: "Film"})
	}
	blob, _ := json.Marshal(suggest.Proposal{Acquisitions: items})
	return store.Proposal{ID: id, CreatedBy: createdBy, Status: status, ProposalJSON: string(blob)}
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
	return suggest.NewAutoApprover(
		st,
		func(context.Context) int { return defaultLimit },
		func() time.Time { return time.Unix(1_700_000_000, 0) },
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
}
