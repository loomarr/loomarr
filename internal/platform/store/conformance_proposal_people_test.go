package store

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/proposalstore"
)

// testProposalPeople pins who a job and a proposal name (#2793): a member by id, no one for the
// API token, an automatic decision as AutoApprovedBy or AutoCuratedBy, and no one once that member
// is deleted, while the decision keeps saying how it was made.
func testProposalPeople(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()

	tokenJob := sampleJob("job-token", "hash-token", now, now)
	tokenJob.CreatedBy = ""
	if err := s.CreateJob(ctx, tokenJob); err != nil {
		t.Fatal(err)
	}
	memberJob := sampleJob("job-1", "hash-1", now, now)
	memberJob.CreatedBy = "alice"
	if err := s.CreateJob(ctx, memberJob); err != nil {
		t.Fatal(err)
	}
	for _, p := range []Proposal{
		{ID: "p-person", JobID: "job-1", Status: "approved", CreatedBy: "alice", ApprovedBy: "admin"},
		{ID: "p-auto", JobID: "job-1", Status: "approved", CreatedBy: "alice", ApprovedBy: proposalstore.AutoApprovedBy},
		{ID: "p-curate", JobID: "job-1", Status: "approved", CreatedBy: "alice", ApprovedBy: proposalstore.AutoCuratedBy},
		{ID: "p-token", JobID: "job-token", Status: "submitted"},
		{ID: "p-stranger", JobID: "job-1", Status: "submitted", CreatedBy: "u-nobody"},
	} {
		p.CreatedAt, p.UpdatedAt = now, now
		if err := s.CreateProposal(ctx, p); err != nil {
			t.Fatalf("create %s: %v", p.ID, err)
		}
	}
	people := func(id string) (string, string) {
		t.Helper()
		p, err := s.GetProposal(ctx, id)
		if err != nil {
			t.Fatalf("get %s: %v", id, err)
		}
		return p.CreatedBy, p.ApprovedBy
	}
	for id, want := range map[string][2]string{
		"p-person":   {"alice", "admin"},
		"p-auto":     {"alice", proposalstore.AutoApprovedBy},
		"p-curate":   {"alice", proposalstore.AutoCuratedBy},
		"p-token":    {"", ""},
		"p-stranger": {"", ""},
	} {
		if by, decided := people(id); by != want[0] || decided != want[1] {
			t.Errorf("%s people = %q, %q; want %q, %q", id, by, decided, want[0], want[1])
		}
	}
	// The API token's own list is what it asked for, never everyone's.
	mine, err := s.ListProposalsByCreator(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(mine) != 2 {
		t.Errorf("the API token's proposals = %d, want its own 2 (no person named)", len(mine))
	}
	ids, err := s.ListProposalJobIDsByCreator(ctx, "", 10)
	if err != nil || len(ids) != 1 || ids[0] != "job-token" {
		t.Errorf("the API token's jobs = %v, %v; want [job-token]", ids, err)
	}
	// Feedback names its member the same way: the API token's is no one's.
	for _, actor := range []string{"alice", ""} {
		if err := s.AppendDiscoveryFeedback(ctx, DiscoveryFeedback{ID: "f-" + actor, ActorID: actor,
			Scope: FeedbackHousehold, Target: "movie:tmdb:90000603", Action: FeedbackKeep, CreatedAt: now}); err != nil {
			t.Fatalf("feedback from %q: %v", actor, err)
		}
	}
	feedback, err := s.ListDiscoveryFeedback(ctx, FeedbackFilter{Scope: FeedbackHousehold})
	if err != nil {
		t.Fatal(err)
	}
	actors := map[string]string{}
	for _, f := range feedback {
		actors[f.ID] = f.ActorID
	}
	if actors["f-alice"] != "alice" || actors["f-"] != "" || len(actors) != 2 {
		t.Errorf("feedback actors = %v, want alice and no one", actors)
	}

	sqlSt, ok := s.(*sqlStore)
	if !ok {
		t.Skip("needs the SQL store")
	}
	if _, err := sqlSt.db.ExecContext(ctx, sqlSt.ph(`DELETE FROM users WHERE id IN (?, ?)`), "alice", "admin"); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{"p-person": "", "p-auto": proposalstore.AutoApprovedBy} {
		if by, decided := people(id); by != "" || decided != want {
			t.Errorf("%s after its people were deleted = %q, %q; want no one, %q", id, by, decided, want)
		}
	}
	var via string
	if err := sqlSt.db.QueryRowContext(ctx, sqlSt.ph(`SELECT decided_via FROM proposals WHERE id = ?`), "p-person").Scan(&via); err != nil || via != "person" {
		t.Errorf("decided_via after the admin was deleted = %q, %v; want person", via, err)
	}
}
