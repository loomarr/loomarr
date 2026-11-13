package suggest

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/library/titlesearch"
	"github.com/loomarr/loomarr/internal/proposals"
)

func TestGroundedProposalPersistsEditorialEvidenceButDropsUnsourcedPicks(t *testing.T) {
	intent := Intent{Intent: proposals.Intent{Description: "action movies", CurrentLineup: []proposals.LineupContext{{Key: "movie:tmdb:90000001", Name: "Retained"}}, Adjacent: []proposals.AdjacentContext{{Key: "movie:tmdb:90000002", Name: "Related", Votes: 3}}}}
	meaning, err := ValidateDateMeaning(intent, &DateMeaning{Kind: DateMeaningNone})
	if err != nil {
		t.Fatal(err)
	}
	surfaced := map[provision.Key]titlesearch.Candidate{
		"movie:tmdb:90000001": {MediaType: provision.Movie, TMDBID: 90000001, Name: "Retained", InLibrary: true, Source: titlesearch.ScopeLibrary},
		"movie:tmdb:90000002": {MediaType: provision.Movie, TMDBID: 90000002, Name: "Related", InLibrary: true, Source: titlesearch.ScopeAdjacent},
		"movie:tmdb:90000003": {MediaType: provision.Movie, TMDBID: 90000003, Name: "Discovered", InLibrary: true, Source: titlesearch.ScopeLibrary},
	}
	out := finalOutput{Picks: []pick{{Key: "movie:tmdb:90000001"}, {Key: "movie:tmdb:90000002"}, {Key: "movie:tmdb:90000003"}, {Key: "movie:tmdb:90000999"}}}
	prop, err := (&Suggester{maxAcq: 3}).buildProposal(context.Background(), intent, out, surfaced, &proposals.DecisionTrace{}, meaning)
	if err != nil {
		t.Fatal(err)
	}
	if len(prop.Lineup) != 3 {
		t.Fatalf("fabricated pick must not reach a labeled proposal: %+v", prop.Lineup)
	}
	for index, role := range []proposals.EditorialRole{proposals.EditorialCore, proposals.EditorialAdjacent, proposals.EditorialDiscovery} {
		if prop.Lineup[index].EditorialRole != role {
			t.Fatalf("pick %d role = %q, want %q", index, prop.Lineup[index].EditorialRole, role)
		}
	}
}
