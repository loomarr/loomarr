package suggest_test

import (
	"context"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/reference"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/testkit/catalogfixture"
)

type namedSetToolAvailabilityLLM struct {
	calls int
}

type referenceExistsValidator struct{}

func (referenceExistsValidator) Exists(context.Context, provision.MediaType, int) (bool, error) {
	return true, nil
}

func (m *namedSetToolAvailabilityLLM) Name() string { return "named-set-tool-availability" }

func (m *namedSetToolAvailabilityLLM) Chat(_ context.Context, _ []llm.Message, opts llm.ChatOptions) (llm.Response, error) {
	m.calls++
	if len(opts.Tools) > 0 {
		return testkit.ToolCallResponse("catalog_search", map[string]any{
			"mode": "collection", "media_type": "movie", "titles": []any{"Model Guess"},
			"dateMeaning": dateMeaningNone(),
		}), nil
	}
	return testkit.FinalResponse(`{"channelName":"Friday Night","rationale":"A verified named block.","dateMeaning":{"kind":"none","anchors":[],"axes":[]},"picks":[{"mediaType":"movie","key":"movie:tmdb:603","name":"The Matrix","rationale":"A verified constituent.","confidence":0.95}],"policy":{}}`), nil
}

func TestSuggestDiscoversNamedBlockSourceOnce(t *testing.T) {
	for _, request := range []string{"TGIF", "TGIF block", "Make a channel like TGIF", "a channel based on TGIF"} {
		t.Run(request, func(t *testing.T) {
			meaning := dateMeaningNone()
			corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{matrixCandidate()}}
			references := &testkit.ReferenceResolver{DiscoveryEvidence: reference.Evidence{
				URL: "https://lineups.example/frozen-block", Title: "Synthetic block roster", Excerpt: "The Matrix", TitleAnchors: []string{"The Matrix"},
			}}
			model := testkit.NewLLM(testkit.FinalResponse(finalWithDateMeaning(t, meaning)), testkit.FinalResponse(finalWithDateMeaning(t, meaning)))
			proposal, err := dateExecutionSuggester(model, corpus).WithReferences(references).Suggest(context.Background(), suggest.Intent{Description: request})
			if err != nil || len(proposal.Lineup) != 1 || proposal.Lineup[0].TMDBID != matrixCandidate().TMDBID {
				t.Fatalf("proposal=%+v err=%v", proposal, err)
			}
			if got := references.Discoveries(); len(got) != 1 || got[0] != "TGIF" || len(references.Calls()) != 0 {
				t.Fatalf("discovery=%v pasted=%v", got, references.Calls())
			}
			if model.Calls != 2 || !strings.Contains(model.LastMessages[1].Content, "UNTRUSTED REFERENCE DATA") {
				t.Fatalf("calls=%d; discovered source was not supplied to finalization", model.Calls)
			}
		})
	}
}

func TestSuggestNamedBlockStopsRetrievalAfterReferenceGrounding(t *testing.T) {
	corpus := &catalogfixture.Corpus{
		SearchFunc: func(_ context.Context, query string, _ int) ([]catalog.Candidate, error) {
			if query == "The Matrix" {
				return []catalog.Candidate{matrixCandidate()}, nil
			}
			return nil, nil
		},
	}
	references := &testkit.ReferenceResolver{DiscoveryEvidence: reference.Evidence{
		URL: "https://lineups.example/frozen-block", Title: "Synthetic block roster", Excerpt: "The Matrix", TitleAnchors: []string{"The Matrix"},
	}}
	model := &namedSetToolAvailabilityLLM{}

	proposal, err := suggest.New(model, catalog.New(nil, corpus), nil, 10).WithReferences(references).Suggest(context.Background(), suggest.Intent{Description: "TGIF"})
	if err != nil {
		t.Fatalf("suggestion failed after %d model calls and %d catalog searches: %v", model.calls, len(corpus.Searches()), err)
	}
	if len(proposal.Lineup) != 1 || proposal.Lineup[0].TMDBID != matrixCandidate().TMDBID {
		t.Fatalf("proposal=%+v, want the source-grounded constituent", proposal)
	}
	if model.calls != 2 {
		t.Fatalf("model calls=%d, want collection hypothesis followed by tool-free finalization", model.calls)
	}
	if got := references.Discoveries(); len(got) != 1 || got[0] != "TGIF" {
		t.Fatalf("discovery=%v, want one TGIF source resolution", got)
	}
}

func TestSuggestNamedBlockContinuesPastUnresolvedSourceAnchors(t *testing.T) {
	fullHouse := catalog.Candidate{MediaType: "series", TMDBID: 1001, Name: "Full House", Year: 1987, InLibrary: true}
	familyMatters := catalog.Candidate{MediaType: "series", TMDBID: 1002, Name: "Family Matters", Year: 1989, InLibrary: true}
	corpus := &catalogfixture.Corpus{
		SearchFunc: func(_ context.Context, query string, _ int) ([]catalog.Candidate, error) {
			switch query {
			case fullHouse.Name:
				return []catalog.Candidate{fullHouse}, nil
			case familyMatters.Name:
				return []catalog.Candidate{familyMatters}, nil
			default:
				return nil, nil
			}
		},
	}
	references := &testkit.ReferenceResolver{DiscoveryEvidence: reference.Evidence{
		URL: "https://lineups.example/tgif", Title: "TGIF", Excerpt: "A verified programming block.",
		TitleAnchors: []string{
			"Unavailable One", "Unavailable Two", "Unavailable Three", "Unavailable Four",
			"Unavailable Five", "Unavailable Six", "Unavailable Seven", "Unavailable Eight",
			fullHouse.Name, familyMatters.Name,
		},
	}}
	model := testkit.NewLLM(
		testkit.FinalResponse(`{"channelName":"Friday Night","rationale":"Interpreting the named block.","dateMeaning":{"kind":"none","anchors":[],"axes":[]},"picks":[],"policy":{}}`),
		testkit.FinalResponse(`{"channelName":"Friday Night","rationale":"A verified named block.","dateMeaning":{"kind":"none","anchors":[],"axes":[]},"picks":[{"mediaType":"series","key":"series:tmdb:1001","name":"Full House","rationale":"A defining constituent.","confidence":0.98},{"mediaType":"series","key":"series:tmdb:1002","name":"Family Matters","rationale":"A defining constituent.","confidence":0.98}],"policy":{}}`),
	)

	proposal, err := dateExecutionSuggester(model, corpus).WithReferences(references).Suggest(context.Background(), suggest.Intent{Description: "TGIF"})
	if err != nil {
		t.Fatalf("suggestion failed after %d source lookups: %v", len(corpus.Searches()), err)
	}
	if len(proposal.Lineup) != 2 {
		t.Fatalf("lineup=%+v, want later usable source members after early misses", proposal.Lineup)
	}
	if got := len(corpus.Searches()); got != 10 {
		t.Fatalf("source lookups=%d, want the bounded roster scanned through both usable members", got)
	}
}

func TestSuggestNamedBlockPreservesSourceMediaType(t *testing.T) {
	movie := catalog.Candidate{MediaType: "movie", TMDBID: 1001, Name: "Clueless", Year: 1995, InLibrary: true}
	series := catalog.Candidate{MediaType: "series", TMDBID: 1002, Name: "Clueless", Year: 1996}
	corpus := &catalogfixture.Corpus{
		SearchFunc: func(_ context.Context, query string, _ int) ([]catalog.Candidate, error) {
			if query == "Clueless" || query == "Clueless (TV series)" {
				return []catalog.Candidate{movie, series}, nil
			}
			return nil, nil
		},
	}
	references := &testkit.ReferenceResolver{DiscoveryEvidence: reference.Evidence{
		URL: "https://lineups.example/tgif", Title: "TGIF", Excerpt: "A verified programming block.",
		TitleAnchors: []string{"Clueless (TV series)"},
	}}
	model := testkit.NewLLM(
		testkit.FinalResponse(`{"channelName":"TGIF","rationale":"Interpreting the named block.","dateMeaning":{"kind":"none","anchors":[],"axes":[]},"picks":[],"policy":{}}`),
		testkit.FinalResponse(`{"channelName":"TGIF","rationale":"A verified named block.","dateMeaning":{"kind":"none","anchors":[],"axes":[]},"picks":[{"mediaType":"series","key":"series:tmdb:1002","name":"Clueless","rationale":"The source identifies the television series.","confidence":0.98}],"policy":{}}`),
	)

	proposal, err := suggest.New(model, catalog.New(nil, corpus), referenceExistsValidator{}, 10).
		WithReferences(references).
		Suggest(context.Background(), suggest.Intent{Description: "TGIF"})
	if err != nil {
		t.Fatalf("suggestion failed: %v", err)
	}
	if len(proposal.Lineup) != 0 || len(proposal.Acquisitions) != 1 || proposal.Acquisitions[0].MediaType != "series" {
		t.Fatalf("proposal=%+v, want the source-typed series rather than the owned namesake movie", proposal)
	}
}

func TestSuggestMissingNamedSourceDoesNotPromoteModelMemory(t *testing.T) {
	meaning := dateMeaningNone()
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{matrixCandidate()}}
	references := &testkit.ReferenceResolver{}
	model := testkit.NewLLM(testkit.FinalResponse(finalWithDateMeaning(t, meaning)), testkit.FinalResponse(finalWithDateMeaning(t, meaning)))
	proposal, err := dateExecutionSuggester(model, corpus).WithReferences(references).Suggest(context.Background(), suggest.Intent{Description: "TGIF block"})
	if err == nil || len(proposal.Lineup) != 0 || len(references.Discoveries()) != 1 {
		t.Fatalf("proposal=%+v err=%v discovery=%v", proposal, err, references.Discoveries())
	}
}
