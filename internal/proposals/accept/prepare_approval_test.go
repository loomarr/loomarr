package accept_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/platform/holidayvocab"
	"github.com/loomarr/loomarr/internal/platform/rating"
	"github.com/loomarr/loomarr/internal/platform/store/proposalstore"
	"github.com/loomarr/loomarr/internal/proposals"
	"github.com/loomarr/loomarr/internal/proposals/accept"
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
	p := proposalstore.Proposal{ID: "p1", JobID: "job1", Status: "submitted", Body: body}
	edit := &accept.ApprovalEdit{DropKeys: []provision.Key{"movie:tmdb:90000001"}, Add: []proposals.ProposalItem{
		{MediaType: provision.Movie, TMDBID: 90000003, Name: "Review addition", EditorialRole: proposals.EditorialCore},
	}, Note: "More variety"}
	prepared, err := accept.PrepareApproval(p, edit, holidayvocab.Default())
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
	if _, err := accept.NewApprover(st, channels, time.Now).Approve(context.Background(), p, edit, "admin"); err != nil {
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
	p := proposalstore.Proposal{ID: "p-backup", Status: "submitted", Body: body}
	prepared, err := accept.PrepareApproval(p, &accept.ApprovalEdit{
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
	p := proposalstore.Proposal{ID: "p-owned-add", Status: "submitted", Body: body}
	edit := &accept.ApprovalEdit{Add: []proposals.ProposalItem{
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
	resolvedEdit, err := accept.ResolveApprovalEdit(context.Background(), edit, resolver)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := accept.PrepareApproval(p, resolvedEdit, holidayvocab.Default())
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
	p := proposalstore.Proposal{ID: "p-empty", Status: "submitted", Body: body}
	_, err := accept.PrepareApproval(p, &accept.ApprovalEdit{
		DropKeys: []provision.Key{"movie:tmdb:90000001"},
	}, holidayvocab.Default())
	if !errors.Is(err, accept.ErrEmptyApproval) {
		t.Fatalf("PrepareApproval error = %v, want ErrEmptyApproval", err)
	}
}

// leftOutBody is a TV-Y7 proposal that left out an owned TV-G title, a missing TV-PG one, an
// unrated one (the unrated rule, not the limit) and a TV-MA one.
func leftOutBody() proposals.Proposal {
	body := proposals.Proposal{
		Lineup: []proposals.ProposalItem{{MediaType: provision.Movie, TMDBID: 90000001, Name: "Kept", OfficialRating: "TV-Y7"}},
		Refused: []proposals.RefusedPick{
			{Item: proposals.ProposalItem{MediaType: provision.Series, TVDBID: 90000002, Name: "Owned TV-G", OfficialRating: "TV-G", InLibrary: true, LibraryItemID: "library-2"}, Reason: "over_ceiling"},
			{Item: proposals.ProposalItem{MediaType: provision.Series, TVDBID: 90000003, Name: "Missing TV-PG", OfficialRating: "tv-pg"}, Reason: "over_ceiling"},
			{Item: proposals.ProposalItem{MediaType: provision.Movie, TMDBID: 90000004, Name: "Unrated"}, Reason: "over_ceiling"},
			{Item: proposals.ProposalItem{MediaType: provision.Movie, TMDBID: 90000005, Name: "Mature", OfficialRating: "TV-MA"}, Reason: "over_ceiling"},
		},
	}
	body.Policy.Audience.Ceiling = "TV-Y7"
	return body
}

// #3012: "Raise to TV-PG" on the card sets the limit and lets in what it now admits.
func TestPrepareApprovalRaisesTheAudienceLimitAndLetsTitlesIn(t *testing.T) {
	p := proposalstore.Proposal{ID: "p-raise", Status: "submitted", Body: leftOutBody()}
	prepared, err := accept.PrepareApproval(p, &accept.ApprovalEdit{Audience: "TV-PG"}, holidayvocab.Default())
	if err != nil {
		t.Fatal(err)
	}
	got := prepared.Body
	if got.Policy.Audience.Ceiling != "TV-PG" {
		t.Fatalf("ceiling = %q, want TV-PG", got.Policy.Audience.Ceiling)
	}
	if len(got.Lineup) != 2 || got.Lineup[1].Name != "Owned TV-G" {
		t.Fatalf("lineup = %+v, want the owned TV-G title back with the available titles", got.Lineup)
	}
	if len(got.Acquisitions) != 1 || got.Acquisitions[0].Name != "Missing TV-PG" {
		t.Fatalf("acquisitions = %+v, want the missing TV-PG title to acquire", got.Acquisitions)
	}
	if len(got.Refused) != 2 || got.Refused[0].Item.Name != "Unrated" || got.Refused[1].Item.Name != "Mature" {
		t.Fatalf("refused = %+v, want the unrated and TV-MA titles still left out", got.Refused)
	}
	if prepared.ModSummary != "raised the audience limit to TV-PG" {
		t.Fatalf("modification summary = %q", prepared.ModSummary)
	}
	if p.Body.Policy.Audience.Ceiling != "TV-Y7" || len(p.Body.Refused) != 4 || len(p.Body.Lineup) != 1 {
		t.Fatal("preparation changed the caller's proposal")
	}
}

// A drop in the same edit still applies to a title the raise let in.
func TestPrepareApprovalDropsATitleTheRaiseLetIn(t *testing.T) {
	p := proposalstore.Proposal{ID: "p-raise-drop", Status: "submitted", Body: leftOutBody()}
	prepared, err := accept.PrepareApproval(p, &accept.ApprovalEdit{
		Audience: "TV-PG",
		DropKeys: []provision.Key{"series:tvdb:90000003"},
	}, holidayvocab.Default())
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Body.Acquisitions) != 0 {
		t.Fatalf("acquisitions = %+v, want the dropped title gone", prepared.Body.Acquisitions)
	}
	if prepared.ModSummary != "dropped 1, raised the audience limit to TV-PG" {
		t.Fatalf("modification summary = %q", prepared.ModSummary)
	}
}

// The field only raises: no limit to raise, an unknown rating, the same one or a stricter one
// is refused rather than quietly changing what the channel is for.
func TestPrepareApprovalRefusesAnAudienceThatDoesNotRaise(t *testing.T) {
	noLimit := leftOutBody()
	noLimit.Policy.Audience.Ceiling = ""
	for name, tc := range map[string]struct {
		body     proposals.Proposal
		audience string
	}{
		"no limit":  {noLimit, "TV-PG"},
		"unknown":   {leftOutBody(), "XYZ"},
		"same":      {leftOutBody(), "TV-Y7"},
		"stricter":  {leftOutBody(), "TV-Y"},
		"same rank": {func() proposals.Proposal { b := leftOutBody(); b.Policy.Audience.Ceiling = "TV-G"; return b }(), "G"},
	} {
		t.Run(name, func(t *testing.T) {
			p := proposalstore.Proposal{ID: "p-no-raise", Status: "submitted", Body: tc.body}
			_, err := accept.PrepareApproval(p, &accept.ApprovalEdit{Audience: rating.Rating(tc.audience)}, holidayvocab.Default())
			if !errors.Is(err, accept.ErrAudienceNotRaised) {
				t.Fatalf("error = %v, want ErrAudienceNotRaised", err)
			}
		})
	}
}

// The card's button names the lowest limit that lets every rated left-out title in.
func TestAudienceToAdmitRefused(t *testing.T) {
	got, ok := accept.AudienceToAdmitRefused(leftOutBody())
	if !ok || got != "TV-MA" {
		t.Fatalf("AudienceToAdmitRefused = %q, %v; want TV-MA", got, ok)
	}
	if _, ok := accept.AudienceToAdmitRefused(proposals.Proposal{}); ok {
		t.Fatal("a proposal with nothing left out needs no raise")
	}
}
