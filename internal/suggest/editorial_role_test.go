package suggest

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/provision"
)

func TestGroundedProposalPersistsEditorialEvidenceButDropsUnsourcedPicks(t *testing.T) {
	intent := Intent{Description: "action movies", CurrentLineup: []LineupContext{{Key: "movie:tmdb:1", Name: "Retained"}}, Adjacent: []AdjacentContext{{Key: "movie:tmdb:2", Name: "Related", Votes: 3}}}
	meaning, err := ValidateDateMeaning(intent, &DateMeaning{Kind: DateMeaningNone})
	if err != nil {
		t.Fatal(err)
	}
	surfaced := map[provision.Key]catalog.Candidate{
		"movie:tmdb:1": {MediaType: provision.Movie, TMDBID: 1, Name: "Retained", InLibrary: true, Source: catalog.ScopeLibrary},
		"movie:tmdb:2": {MediaType: provision.Movie, TMDBID: 2, Name: "Related", InLibrary: true, Source: catalog.ScopeAdjacent},
		"movie:tmdb:3": {MediaType: provision.Movie, TMDBID: 3, Name: "Discovered", InLibrary: true, Source: catalog.ScopeLibrary},
	}
	out := finalOutput{Picks: []pick{{MediaType: "movie", TMDBID: 1}, {MediaType: "movie", TMDBID: 2}, {MediaType: "movie", TMDBID: 3}, {MediaType: "movie", TMDBID: 999}}}
	prop, err := (&Suggester{maxAcq: 3}).buildProposal(context.Background(), intent, out, surfaced, &DecisionTrace{}, meaning)
	if err != nil {
		t.Fatal(err)
	}
	if len(prop.Lineup) != 3 {
		t.Fatalf("fabricated pick must not reach a labeled proposal: %+v", prop.Lineup)
	}
	for index, role := range []EditorialRole{EditorialCore, EditorialAdjacent, EditorialDiscovery} {
		if prop.Lineup[index].EditorialRole != role {
			t.Fatalf("pick %d role = %q, want %q", index, prop.Lineup[index].EditorialRole, role)
		}
	}
}
