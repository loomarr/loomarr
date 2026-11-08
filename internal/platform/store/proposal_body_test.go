package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/proposals"
	"github.com/loomarr/loomarr/internal/provision"
)

// The store is the one place a proposal's JSON is encoded and decoded: callers write and read a
// typed proposals.Proposal and never see the column.
func TestProposalBodyIsDecodedOnceAtTheStore(t *testing.T) {
	s := newSQLiteStore(t).(*sqlStore)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()

	want := proposals.Proposal{
		Intent:      proposals.Intent{Description: "90s action", MustExclude: []string{"Cats"}},
		ChannelName: "Action Hour",
		Lineup:      []proposals.ProposalItem{{MediaType: provision.Movie, TMDBID: 90000001, Name: "Round Trip Movie", InLibrary: true}},
		Retired:     []provision.Key{"movie:tmdb:90000002"},
	}
	if err := s.CreateProposal(ctx, Proposal{ID: "p-typed", JobID: "j", Status: "submitted", CreatedBy: "u", Body: want, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetProposal(ctx, "p-typed")
	if err != nil {
		t.Fatal(err)
	}
	if got.Body.ChannelName != want.ChannelName || got.Body.Intent.Description != "90s action" ||
		len(got.Body.Lineup) != 1 || got.Body.Lineup[0].TMDBID != 90000001 || len(got.Body.Retired) != 1 {
		t.Fatalf("body did not round-trip as a typed value: %+v", got.Body)
	}
	listed, err := s.ListProposalsByCreator(ctx, "u")
	if err != nil || len(listed) != 1 || listed[0].Body.ChannelName != "Action Hour" {
		t.Fatalf("list decoded %+v, err %v", listed, err)
	}
}

// A proposal the store cannot read is an error, never a silently empty one: the caller would
// approve or bind a lineup it never saw. The schema refuses a column that isn't JSON at all, so
// what is left to refuse on read is JSON that isn't a proposal.
func TestProposalBodyThatIsNotAProposalIsAnError(t *testing.T) {
	s := newSQLiteStore(t).(*sqlStore)
	ctx := context.Background()
	insert := func(id, body string) error {
		_, err := s.db.ExecContext(ctx, s.ph(`INSERT INTO proposals (id, job_id, status, created_by, decided_by, deny_reason, mod_summary, note, proposal_json, approved_at, created_at, updated_at)
			VALUES (?, 'j', 'approved', 'u', '', '', '', '', ?, 0, 0, 0)`), id, body)
		return err
	}
	for _, notJSON := range []string{"", "{"} {
		if err := insert("p-"+notJSON, notJSON); err == nil {
			t.Fatalf("proposal_json %q was stored", notJSON)
		}
	}
	if err := insert("p-bad", "[1]"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetProposal(ctx, "p-bad"); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("GetProposal(malformed) = %v, want a decode error", err)
	}
	if _, err := s.ListProposalsByCreator(ctx, "u"); err == nil {
		t.Fatal("ListProposalsByCreator decoded a malformed row")
	}
}
