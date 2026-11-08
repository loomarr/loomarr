package suggest_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/holidayvocab"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/proposals"
	"github.com/loomarr/loomarr/internal/proposals/suggest"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestPrepareApprovalMatchesCommittedContentWithoutGrantingAddedRoles(t *testing.T) {
	body := proposals.Proposal{Lineup: []proposals.ProposalItem{
		{MediaType: provision.Movie, TMDBID: 90000001, Name: "Drop", EditorialRole: proposals.EditorialCore},
		{MediaType: provision.Movie, TMDBID: 90000002, Name: "Keep", EditorialRole: proposals.EditorialDiscovery},
	}}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	p := store.Proposal{ID: "p1", JobID: "job1", Status: "submitted", Body: body}
	edit := &suggest.ApprovalEdit{DropKeys: []provision.Key{"movie:tmdb:90000001"}, Add: []proposals.ProposalItem{
		{MediaType: provision.Movie, TMDBID: 90000003, Name: "Review addition", EditorialRole: proposals.EditorialCore},
	}, Note: "More variety"}
	prepared, err := suggest.PrepareApproval(p, edit, holidayvocab.Default())
	if err != nil {
		t.Fatal(err)
	}
	decoded := prepared.Body
	if after, _ := json.Marshal(p.Body); prepared.Status != "submitted" || string(after) != string(raw) || edit.Add[0].EditorialRole != proposals.EditorialCore {
		t.Fatal("pure preparation changed status or caller-owned input")
	}
	if len(decoded.Lineup) != 1 || decoded.Lineup[0].EditorialRole != proposals.EditorialDiscovery || len(decoded.Acquisitions) != 1 || decoded.Acquisitions[0].EditorialRole != "" {
		t.Fatalf("retain recorded roles, but reject a role supplied by a review addition: %+v", decoded)
	}
	st := &testkit.ApprovalStore{}
	channels := &testkit.ApprovalChannels{}
	if _, err := suggest.NewApprover(st, channels, time.Now).Approve(context.Background(), p, edit, "admin"); err != nil {
		t.Fatal(err)
	}
	if len(st.Commits) != 1 || !reflect.DeepEqual(st.Commits[0].Proposal.Body, prepared.Body) || st.Commits[0].Proposal.ModSummary != prepared.ModSummary {
		t.Fatalf("outlook preparation and actual approval diverged: %+v", st.Commits)
	}
}

func TestPrepareApprovalDropsExcludedBackup(t *testing.T) {
	body := proposals.Proposal{
		Lineup: []proposals.ProposalItem{{MediaType: provision.Movie, TMDBID: 90000001, Name: "Keep"}},
		Alternates: []proposals.ProposalItem{
			{MediaType: provision.Movie, TMDBID: 90000002, Name: "Excluded backup"},
			{MediaType: provision.Movie, TMDBID: 90000003, Name: "Keep backup"},
		},
	}
	p := store.Proposal{ID: "p-backup", Status: "submitted", Body: body}
	prepared, err := suggest.PrepareApproval(p, &suggest.ApprovalEdit{
		DropKeys: []provision.Key{"movie:tmdb:90000002"},
	}, holidayvocab.Default())
	if err != nil {
		t.Fatal(err)
	}
	decoded := prepared.Body
	if len(decoded.Alternates) != 1 || decoded.Alternates[0].Name != "Keep backup" {
		t.Fatalf("alternates = %+v, want only the retained backup", decoded.Alternates)
	}
}

func TestPrepareApprovalPlacesOwnedAdditionsWithoutReacquiringThem(t *testing.T) {
	body := proposals.Proposal{Lineup: []proposals.ProposalItem{
		{MediaType: provision.Movie, TMDBID: 90000001, Name: "Existing", InLibrary: true, LibraryItemID: "library-1"},
	}}
	p := store.Proposal{ID: "p-owned-add", Status: "submitted", Body: body}
	edit := &suggest.ApprovalEdit{Add: []proposals.ProposalItem{
		{MediaType: provision.Movie, TMDBID: 90000002, Name: "Owned addition", InLibrary: true, LibraryItemID: "library-2"},
		{MediaType: provision.Movie, TMDBID: 90000003, Name: "Missing addition", InLibrary: true, LibraryItemID: "forged-library-id"},
		{MediaType: provision.Movie, TMDBID: 90000002, Name: "Duplicate owned addition", InLibrary: false},
		{MediaType: provision.Movie, TMDBID: 90000001, Name: "Duplicate existing title", InLibrary: false},
	}}
	resolver := &testkit.ApprovalAdditionResolver[proposals.ProposalItem]{Results: []testkit.ApprovalAdditionResolution[proposals.ProposalItem]{
		{Item: edit.Add[0], Owned: true},
		{Item: edit.Add[1], Owned: false},
		{Item: proposals.ProposalItem{MediaType: provision.Movie, TMDBID: 90000001, Name: "Existing", InLibrary: true, LibraryItemID: "library-1"}, Owned: true},
	}}
	resolvedEdit, err := suggest.ResolveApprovalEdit(context.Background(), edit, resolver)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := suggest.PrepareApproval(p, resolvedEdit, holidayvocab.Default())
	if err != nil {
		t.Fatal(err)
	}
	decoded := prepared.Body
	if len(decoded.Lineup) != 2 || decoded.Lineup[1].Name != "Owned addition" {
		t.Fatalf("lineup = %+v, want the owned review addition placed with available titles", decoded.Lineup)
	}
	if len(decoded.Acquisitions) != 1 || decoded.Acquisitions[0].Name != "Missing addition" {
		t.Fatalf("acquisitions = %+v, want only the missing review addition", decoded.Acquisitions)
	}
	if prepared.ModSummary != "added 2" {
		t.Fatalf("modification summary = %q, want only effective additions counted", prepared.ModSummary)
	}
	if calls := resolver.Calls(); len(calls) != 3 {
		t.Fatalf("resolver calls = %+v, want one per distinct added title", calls)
	}
}

func TestPrepareApprovalRejectsAnEmptyEffectiveChannel(t *testing.T) {
	body := proposals.Proposal{Acquisitions: []proposals.ProposalItem{
		{MediaType: provision.Movie, TMDBID: 90000001, Name: "Only title"},
	}}
	p := store.Proposal{ID: "p-empty", Status: "submitted", Body: body}
	_, err := suggest.PrepareApproval(p, &suggest.ApprovalEdit{
		DropKeys: []provision.Key{"movie:tmdb:90000001"},
	}, holidayvocab.Default())
	if !errors.Is(err, suggest.ErrEmptyApproval) {
		t.Fatalf("PrepareApproval error = %v, want ErrEmptyApproval", err)
	}
}
