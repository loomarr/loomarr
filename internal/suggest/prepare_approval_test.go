package suggest_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestPrepareApprovalMatchesCommittedContentWithoutGrantingAddedRoles(t *testing.T) {
	body := suggest.Proposal{Lineup: []suggest.ProposalItem{
		{MediaType: provision.Movie, TMDBID: 1, Name: "Drop", EditorialRole: suggest.EditorialCore},
		{MediaType: provision.Movie, TMDBID: 2, Name: "Keep", EditorialRole: suggest.EditorialDiscovery},
	}}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	p := store.Proposal{ID: "p1", JobID: "job1", Status: "submitted", ProposalJSON: string(raw)}
	edit := &suggest.ApprovalEdit{DropKeys: []provision.Key{"movie:tmdb:1"}, Add: []suggest.ProposalItem{
		{MediaType: provision.Movie, TMDBID: 3, Name: "Review addition", EditorialRole: suggest.EditorialCore},
	}, Note: "More variety"}
	prepared, decoded, err := suggest.PrepareApproval(p, edit)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Status != "submitted" || p.ProposalJSON != string(raw) || edit.Add[0].EditorialRole != suggest.EditorialCore {
		t.Fatal("pure preparation changed status or caller-owned input")
	}
	if len(decoded.Lineup) != 1 || decoded.Lineup[0].EditorialRole != suggest.EditorialDiscovery || len(decoded.Acquisitions) != 1 || decoded.Acquisitions[0].EditorialRole != "" {
		t.Fatalf("retain recorded roles, but reject a role supplied by a review addition: %+v", decoded)
	}
	st := &testkit.ApprovalStore{}
	channels := &testkit.ApprovalChannels{}
	if _, err := suggest.NewApprover(st, channels, time.Now).Approve(context.Background(), p, edit, "admin"); err != nil {
		t.Fatal(err)
	}
	if len(st.Commits) != 1 || st.Commits[0].Proposal.ProposalJSON != prepared.ProposalJSON || st.Commits[0].Proposal.ModSummary != prepared.ModSummary {
		t.Fatalf("outlook preparation and actual approval diverged: %+v", st.Commits)
	}
}

func TestPrepareApprovalDropsExcludedBackup(t *testing.T) {
	body := suggest.Proposal{
		Lineup: []suggest.ProposalItem{{MediaType: provision.Movie, TMDBID: 1, Name: "Keep"}},
		Alternates: []suggest.ProposalItem{
			{MediaType: provision.Movie, TMDBID: 2, Name: "Excluded backup"},
			{MediaType: provision.Movie, TMDBID: 3, Name: "Keep backup"},
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	p := store.Proposal{ID: "p-backup", Status: "submitted", ProposalJSON: string(raw)}
	_, decoded, err := suggest.PrepareApproval(p, &suggest.ApprovalEdit{
		DropKeys: []provision.Key{"movie:tmdb:2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Alternates) != 1 || decoded.Alternates[0].Name != "Keep backup" {
		t.Fatalf("alternates = %+v, want only the retained backup", decoded.Alternates)
	}
}

func TestPrepareApprovalRejectsAnEmptyEffectiveChannel(t *testing.T) {
	body := suggest.Proposal{Acquisitions: []suggest.ProposalItem{
		{MediaType: provision.Movie, TMDBID: 1, Name: "Only title"},
	}}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	p := store.Proposal{ID: "p-empty", Status: "submitted", ProposalJSON: string(raw)}
	_, _, err = suggest.PrepareApproval(p, &suggest.ApprovalEdit{
		DropKeys: []provision.Key{"movie:tmdb:1"},
	})
	if !errors.Is(err, suggest.ErrEmptyApproval) {
		t.Fatalf("PrepareApproval error = %v, want ErrEmptyApproval", err)
	}
}
