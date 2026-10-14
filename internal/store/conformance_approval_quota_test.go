package store

import (
	"context"
	"errors"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
)

var errTestApprovalQuotaFull = errors.New("test approval quota is full")

// testProposalAutoApprovalQuotaConcurrent exercises the guarded approval seam in
// the one conformance suite shared by SQLite and Postgres.
func testProposalAutoApprovalQuotaConcurrent(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	testProposalAutoApprovalQuotaAcrossStores(t, s, s)
}

// testProposalAutoApprovalQuotaAcrossStores also powers the Postgres-only
// independent-pool proof. Seeding and observation use the first Store; each
// contender commits through the Store supplied for that goroutine.
func testProposalAutoApprovalQuotaAcrossStores(t *testing.T, first, second Store) {
	t.Helper()
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()
	for _, p := range []Proposal{
		{ID: "quota-race-a", JobID: "quota-job-a", Status: "submitted", CreatedBy: "member", ProposalJSON: `{}`, CreatedAt: now, UpdatedAt: now},
		{ID: "quota-race-b", JobID: "quota-job-b", Status: "submitted", CreatedBy: "member", ProposalJSON: `{}`, CreatedAt: now, UpdatedAt: now},
	} {
		if err := first.CreateProposal(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	// If requester ordering is absent, both transaction-bound guards can observe
	// zero approved proposals before either commits. The bounded rendezvous makes
	// that broken interleaving deterministic without deadlocking the serialized path.
	arrived := make(chan struct{}, 2)
	proceed := make(chan struct{})
	go func() {
		<-arrived
		select {
		case <-arrived:
		case <-time.After(250 * time.Millisecond):
		}
		close(proceed)
	}()

	start := make(chan struct{})
	errs := make(chan error, 2)
	stores := []Store{first, second}
	for i, id := range []string{"quota-race-a", "quota-race-b"} {
		go func() {
			<-start
			_, err := stores[i].CommitProposalApprovalGuarded(ctx, testApprovalCommit(id, "member", 501+i, 160+i, now),
				func(guardCtx context.Context, view ProposalApprovalReader) error {
					proposals, err := view.ListProposalsByCreator(guardCtx, "member")
					if err != nil {
						return err
					}
					used := 0
					for _, proposal := range proposals {
						if proposal.Status == "approved" {
							used++
						}
					}
					arrived <- struct{}{}
					<-proceed
					if used >= 1 {
						return errTestApprovalQuotaFull
					}
					return nil
				})
			errs <- err
		}()
	}
	close(start)

	approved, held := 0, 0
	for range 2 {
		err := <-errs
		switch {
		case err == nil:
			approved++
		case errors.Is(err, errTestApprovalQuotaFull):
			held++
		default:
			t.Fatalf("quota-serialized approval: %v", err)
		}
	}
	if approved != 1 || held != 1 {
		t.Fatalf("concurrent decisions = %d approved, %d held; want one of each", approved, held)
	}
	assertProposalDecisionCounts(t, first, "member", 1, 1)
}

func testProposalApprovalManualParticipatesInRequesterOrdering(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()
	seedApprovalProposals(t, s, now, "ordered-auto", "ordered-manual")

	guardEntered := make(chan struct{})
	releaseGuard := make(chan struct{})
	guardedDone := make(chan error, 1)
	go func() {
		_, err := s.CommitProposalApprovalGuarded(ctx, testApprovalCommit("ordered-auto", "member", 601, 170, now),
			func(context.Context, ProposalApprovalReader) error {
				close(guardEntered)
				<-releaseGuard
				return nil
			})
		guardedDone <- err
	}()
	<-guardEntered

	manualStarted := make(chan struct{})
	manualDone := make(chan error, 1)
	go func() {
		close(manualStarted)
		_, err := s.CommitProposalApproval(ctx, testApprovalCommit("ordered-manual", "member", 602, 171, now))
		manualDone <- err
	}()
	<-manualStarted
	select {
	case err := <-manualDone:
		t.Fatalf("manual approval bypassed requester ordering: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseGuard)
	if err := <-guardedDone; err != nil {
		t.Fatalf("guarded approval: %v", err)
	}
	if err := <-manualDone; err != nil {
		t.Fatalf("manual approval after ordering: %v", err)
	}
	assertProposalDecisionCounts(t, s, "member", 2, 0)
}

func testProposalApprovalCanceledWaiterReleasesOrdering(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()
	seedApprovalProposals(t, s, now, "cancel-holder", "cancel-waiter", "cancel-reacquire")

	holderEntered := make(chan struct{})
	releaseHolder := make(chan struct{})
	holderDone := make(chan error, 1)
	go func() {
		_, err := s.CommitProposalApprovalGuarded(ctx, testApprovalCommit("cancel-holder", "member", 701, 180, now),
			func(context.Context, ProposalApprovalReader) error {
				close(holderEntered)
				<-releaseHolder
				return nil
			})
		holderDone <- err
	}()
	<-holderEntered

	waitCtx, cancel := context.WithCancel(ctx)
	waiterStarted := make(chan struct{})
	waiterDone := make(chan error, 1)
	var waiterGuardEntered atomic.Bool
	go func() {
		close(waiterStarted)
		_, err := s.CommitProposalApprovalGuarded(waitCtx, testApprovalCommit("cancel-waiter", "member", 702, 181, now),
			func(context.Context, ProposalApprovalReader) error {
				waiterGuardEntered.Store(true)
				return nil
			})
		waiterDone <- err
	}()
	<-waiterStarted
	select {
	case err := <-waiterDone:
		t.Fatalf("waiter returned before cancellation: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	cancel()
	if err := <-waiterDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled waiter = %v, want context.Canceled", err)
	}
	if waiterGuardEntered.Load() {
		t.Fatal("canceled waiter entered the guarded quota read")
	}

	close(releaseHolder)
	if err := <-holderDone; err != nil {
		t.Fatalf("holder approval: %v", err)
	}
	if got, err := s.GetProposal(ctx, "cancel-waiter"); err != nil || got.Status != "submitted" {
		t.Fatalf("canceled waiter proposal = (%+v, %v), want submitted", got, err)
	}
	if _, err := s.CommitProposalApprovalGuarded(ctx, testApprovalCommit("cancel-reacquire", "member", 703, 182, now),
		func(context.Context, ProposalApprovalReader) error { return nil }); err != nil {
		t.Fatalf("reacquire requester ordering after cancellation: %v", err)
	}
	assertProposalDecisionCounts(t, s, "member", 2, 1)
}

func seedApprovalProposals(t *testing.T, s Store, now time.Time, ids ...string) {
	t.Helper()
	for _, id := range ids {
		if err := s.CreateProposal(context.Background(), Proposal{
			ID: id, JobID: "job-" + id, Status: "submitted", CreatedBy: "member",
			ProposalJSON: `{}`, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func testApprovalCommit(id, creator string, tmdbID, channelNumber int, now time.Time) ProposalApproval {
	proposal := Proposal{
		ID: id, JobID: "job-" + id, Status: "approved", CreatedBy: creator,
		ApprovedBy: "auto", ProposalJSON: `{}`, ApprovedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	return ProposalApproval{
		Proposal: proposal,
		Titles: []provision.Record{{
			Key:   provision.Key("movie:tmdb:" + strconv.Itoa(tmdbID)),
			Title: provision.Title{MediaType: provision.Movie, TMDBID: tmdbID},
			State: provision.Wanted, Deadline: now,
		}},
		Channel: approvalChannel("ch-"+id, proposal.JobID, channelNumber),
	}
}

func assertProposalDecisionCounts(t *testing.T, s Store, creator string, wantApproved, wantSubmitted int) {
	t.Helper()
	proposals, err := s.ListProposalsByCreator(context.Background(), creator)
	if err != nil {
		t.Fatal(err)
	}
	approved, submitted := 0, 0
	for _, proposal := range proposals {
		switch proposal.Status {
		case "approved":
			approved++
		case "submitted":
			submitted++
		}
	}
	if approved != wantApproved || submitted != wantSubmitted {
		t.Fatalf("proposal states = %d approved, %d submitted; want %d approved, %d submitted",
			approved, submitted, wantApproved, wantSubmitted)
	}
}
