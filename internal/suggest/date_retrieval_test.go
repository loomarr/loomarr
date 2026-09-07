package suggest

import (
	"context"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/testkit/catalogfixture"
)

func executionMeaning(axes ...DateExecutionAxis) ValidatedDateMeaning {
	return ValidatedDateMeaning{meaning: DateMeaning{Kind: DateMeaningConstraints}, windows: axes}
}

func TestProjectDiscoveryWindowsCoalescedMovieAndAiring(t *testing.T) {
	meaning := executionMeaning(
		DateExecutionAxis{Kind: DateAxisMovieRelease, Windows: []DateYearRange{{Start: 1990, End: 1999}}},
		DateExecutionAxis{Kind: DateAxisSeriesAiring, Windows: []DateYearRange{{Start: 1990, End: 1999}}},
	)
	got := projectDiscoveryWindows(catalog.DiscoveryQuery{MediaType: provision.Movie, Genres: []string{"Action"}}, meaning)
	if len(got) != 1 || got[0].YearFrom != 1990 || got[0].YearTo != 1999 || got[0].MediaType != provision.Movie {
		t.Fatalf("movie projection = %+v", got)
	}
	series := projectDiscoveryWindows(catalog.DiscoveryQuery{MediaType: provision.Series, Genres: []string{"Drama"}}, meaning)
	if len(series) != 1 || series[0].YearFrom != 0 || series[0].YearTo != 0 {
		t.Fatalf("airing became series premiere filter: %+v", series)
	}
}

func TestProjectDiscoveryWindowsKeepsOtherMediaUnconstrained(t *testing.T) {
	meaning := executionMeaning(DateExecutionAxis{Kind: DateAxisMovieRelease, Windows: []DateYearRange{{Start: 1990, End: 1994}, {Start: 2000, End: 2004}}})
	got := projectDiscoveryWindows(catalog.DiscoveryQuery{Genres: []string{"Comedy"}}, meaning)
	if len(got) != 3 || got[0].MediaType != provision.Movie || got[0].YearFrom != 1990 || got[1].YearFrom != 2000 || got[2].MediaType != provision.Series || got[2].YearFrom != 0 {
		t.Fatalf("mixed projection = %+v", got)
	}
}

func TestWorkLedgerReservesAllWindowsAtomically(t *testing.T) {
	ledger := &workLedger{total: 3, generation: 3}
	if !ledger.reserve(1) || ledger.total != 2 || ledger.generation != 2 {
		t.Fatalf("first tool charge = %+v", ledger)
	}
	if ledger.reserve(3) || ledger.total != 2 || ledger.generation != 2 {
		t.Fatalf("partial window reservation = %+v", ledger)
	}
	ledger.beginGeneration()
	if ledger.generation != maxToolRounds || ledger.total != 2 {
		t.Fatalf("generation reset changed invocation budget = %+v", ledger)
	}
}

func TestRunToolProjectsAcceptedMovieWindowsBeforeUnionDispatch(t *testing.T) {
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{{MediaType: provision.Movie, TMDBID: 1, Name: "Window Movie", Year: 1995}}}
	s := New(nil, catalog.New(nil, corpus), nil, 10)
	ledger := newWorkLedger()
	ledger.beginGeneration()
	if !ledger.reserve(1) {
		t.Fatal("initial model attempt could not be charged")
	}
	arguments := map[string]any{
		"genres": []any{"Drama"},
		"dateMeaning": map[string]any{
			"kind": "constraints", "anchors": []any{map[string]any{"field": "description", "start": 0, "end": 5}},
			"axes": []any{map[string]any{"kind": "movie_release", "combine": "any", "intervals": []any{map[string]any{"anchor": 0, "start": 1990, "end": 1999}}}},
		},
	}
	_, _, trace, valid, _ := s.runToolWithDateMeaning(context.Background(), llm.ToolCall{Name: catalogToolName, Arguments: arguments}, Intent{Description: "1990s"}, nil, nil, ledger)
	if !valid || trace.WindowsCompleted != 2 || trace.SourceQueriesDispatched != 2 {
		t.Fatalf("union trace = %+v valid=%v", trace, valid)
	}
	dispatches := corpus.Discoveries()
	if len(dispatches) != 2 || dispatches[0].Query.MediaType != provision.Movie || dispatches[0].Query.YearFrom != 1990 || dispatches[0].Query.YearTo != 1999 || dispatches[1].Query.MediaType != provision.Series || dispatches[1].Query.YearFrom != 0 {
		t.Fatalf("date dispatches = %+v", dispatches)
	}
}

func TestRunToolRejectsAllDateWindowsBeforeDispatchWhenGenerationCapacityIsShort(t *testing.T) {
	corpus := &catalogfixture.Corpus{}
	s := New(nil, catalog.New(nil, corpus), nil, 10)
	ledger := &workLedger{total: 2, generation: 2}
	if !ledger.reserve(1) { // every model attempt is charged before its arguments are decoded
		t.Fatal("attempt charge failed")
	}
	meaning := map[string]any{
		"kind": "constraints", "anchors": []any{map[string]any{"field": "description", "start": 0, "end": 12}},
		"axes": []any{map[string]any{"kind": "movie_release", "combine": "any", "intervals": []any{
			map[string]any{"anchor": 0, "start": 1990, "end": 1990}, map[string]any{"anchor": 0, "start": 2000, "end": 2000},
		}}},
	}
	_, candidates, trace, valid, _ := s.runToolWithDateMeaning(context.Background(), llm.ToolCall{Name: catalogToolName, Arguments: map[string]any{"genres": []any{"Drama"}, "dateMeaning": meaning}}, Intent{Description: "1990 or 2000"}, nil, nil, ledger)
	if !valid || trace.Terminal != FailureBudgetExhausted || candidates != nil || ledger.total != 1 || ledger.generation != 1 {
		t.Fatalf("capacity failure = trace:%+v candidates:%+v ledger:%+v valid:%v", trace, candidates, ledger, valid)
	}
	if got := corpus.Discoveries(); len(got) != 0 {
		t.Fatalf("partial date dispatches = %+v", got)
	}
}

func TestParseDiscoveryQueryRetiresRawEra(t *testing.T) {
	_, _, err := parseDiscoveryQuery(map[string]any{"genres": []any{"Drama"}, "era": "1990s", "dateMeaning": map[string]any{"kind": "none", "anchors": []any{}, "axes": []any{}}})
	if err == nil || !strings.Contains(err.Error(), "sole date authority") {
		t.Fatalf("raw era error = %v", err)
	}
}
