package suggest_test

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/reference"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/testkit/catalogfixture"
	"github.com/loomarr/loomarr/internal/tmdb"
)

func TestSuggest_BlockEditorialEpochDoesNotRequireEpisodeDates(t *testing.T) {
	const description = "TGIF as it felt in the 1990s, without restricting episode dates."
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{matrixCandidate()}}
	references := &testkit.ReferenceResolver{DiscoveryEvidence: reference.Evidence{
		URL: "https://example.test/development/block-epoch", Title: "Synthetic reference",
		Excerpt: "One synthetic constituent", TitleAnchors: []string{"The Matrix"},
	}}
	model := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"mode": "collection", "media_type": "movie", "titles": []any{"The Matrix"}, "dateMeaning": dateMeaningNone()}),
		testkit.FinalResponse(finalWithDateMeaning(t, dateMeaningNone())),
	)
	proposal, err := dateExecutionSuggester(model, corpus).WithReferences(references).Suggest(context.Background(), suggest.Intent{Description: description})
	if err != nil || len(proposal.Lineup) != 1 || proposal.Policy.Scope.Dates != nil {
		t.Fatalf("editorial block reference became a playback restriction: proposal=%+v err=%v", proposal, err)
	}
}

func TestSuggest_BlockEpochDoesNotEraseSeparateDateRequirements(t *testing.T) {
	for name, intent := range map[string]suggest.Intent{
		"era field": {Description: "TGIF as it felt in the 1990s", Era: "1990s"},
		"airing":    {Description: "TGIF as it felt in the 1990s. Only episodes aired in the 2000s."},
		"release":   {Description: "TGIF as it felt in the 1990s, plus movies released in the 2000s."},
	} {
		t.Run(name, func(t *testing.T) {
			corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{matrixCandidate()}}
			model := testkit.NewLLM(testkit.FinalResponse(finalWithDateMeaning(t, dateMeaningNone())))
			proposal, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), intent)
			if err == nil || len(proposal.Lineup)+len(proposal.Acquisitions) != 0 || len(corpus.Searches()) != 0 {
				t.Fatalf("separate date requirement was erased: proposal=%+v err=%v", proposal, err)
			}
		})
	}
}

func TestSuggest_NegatedLibraryOnlyDoesNotDropOutsideCandidates(t *testing.T) {
	for _, description := range []string{
		"TGIF, not only shows already in my library.",
		"TGIF; I don't want only shows already in my library.",
	} {
		t.Run(description, func(t *testing.T) {
			owned := catalog.Candidate{MediaType: "series", TMDBID: 1, Name: "Owned Sitcom", InLibrary: true, Genres: []string{"Comedy"}}
			missing := catalog.Candidate{MediaType: "series", TMDBID: 2, Name: "Missing Sitcom", Genres: []string{"Comedy"}}
			corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{owned, missing}}
			references := &testkit.ReferenceResolver{DiscoveryEvidence: reference.Evidence{
				URL: "https://example.test/development/library-contrast", Title: "Synthetic reference",
				Excerpt: "Two synthetic sitcoms", TitleAnchors: []string{owned.Name, missing.Name},
			}}
			mock := testkit.NewTMDB(t)
			mock.AddSeries(2, missing.Name, 1996, []int{35}, "Synthetic sitcom")
			validator := tmdb.NewWithBase(mock.URL, "fixture")
			final := `{"picks":[{"mediaType":"series","key":"series:tmdb:1"},{"mediaType":"series","key":"series:tmdb:2"}],"dateMeaning":{"kind":"none","anchors":[],"axes":[]}}`
			model := testkit.NewLLM(
				testkit.ToolCallResponse("catalog_search", map[string]any{"mode": "collection", "media_type": "series", "titles": []any{owned.Name, missing.Name}, "dateMeaning": dateMeaningNone()}),
				testkit.FinalResponse(final),
			)
			proposal, err := suggest.New(model, catalog.New(nil, corpus), validator, 10).WithReferences(references).Suggest(context.Background(), suggest.Intent{Description: description})
			if err != nil || len(proposal.Lineup) != 1 || len(proposal.Acquisitions) != 1 || proposal.Acquisitions[0].TMDBID != missing.TMDBID {
				t.Fatalf("negated qualifier dropped a valid acquisition: proposal=%+v err=%v", proposal, err)
			}
		})
	}
}

func TestSuggest_LibraryOnlyDropsSourceCompletedAcquisitions(t *testing.T) {
	for _, description := range []string{
		"A TGIF channel, with no shows added from outside my library.",
		"TGIF using only shows already in my library.",
		"TGIF with no new acquisitions.",
		"TGIF; do not add anything new to my library.",
	} {
		t.Run(description, func(t *testing.T) {
			owned := catalog.Candidate{MediaType: "series", TMDBID: 1, Name: "Owned Sitcom", InLibrary: true, Genres: []string{"Comedy"}}
			missing := catalog.Candidate{MediaType: "series", TMDBID: 2, Name: "Missing Sitcom", Genres: []string{"Comedy"}}
			corpus := &catalogfixture.Corpus{SearchFunc: func(_ context.Context, title string, _ int) ([]catalog.Candidate, error) {
				if title == owned.Name {
					return []catalog.Candidate{owned}, nil
				}
				return []catalog.Candidate{missing}, nil
			}}
			references := &testkit.ReferenceResolver{DiscoveryEvidence: reference.Evidence{
				URL: "https://example.test/development/library-only", Title: "Synthetic reference",
				Excerpt: "Two synthetic sitcoms", TitleAnchors: []string{owned.Name, missing.Name},
			}}
			final := `{"picks":[{"mediaType":"series","key":"series:tmdb:1"},{"mediaType":"series","key":"series:tmdb:2"}],"dateMeaning":{"kind":"none","anchors":[],"axes":[]}}`
			model := testkit.NewLLM(
				testkit.ToolCallResponse("catalog_search", map[string]any{"mode": "collection", "media_type": "series", "titles": []any{owned.Name, missing.Name}, "dateMeaning": dateMeaningNone()}),
				testkit.FinalResponse(final),
			)
			proposal, err := dateExecutionSuggester(model, corpus).WithReferences(references).Suggest(context.Background(), suggest.Intent{Description: description})
			if err != nil || len(proposal.Lineup) != 1 || proposal.Lineup[0].TMDBID != owned.TMDBID || len(proposal.Acquisitions) != 0 || len(proposal.Alternates) != 0 {
				t.Fatalf("library-only reference restored outside picks: proposal=%+v err=%v", proposal, err)
			}
		})
	}
}
