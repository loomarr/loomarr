package suggest

import (
	"slices"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/provision"
)

func TestRankGroundedCandidatesAppliesExplicitFeedbackDeterministically(t *testing.T) {
	candidates := []catalog.Candidate{
		{MediaType: provision.Movie, TMDBID: 1, Name: "Obvious Space Adventure", Genres: []string{"Science Fiction"}, InLibrary: true},
		{MediaType: provision.Movie, TMDBID: 2, Name: "Quiet Space Discovery", Genres: []string{"Science Fiction"}},
		{MediaType: provision.Movie, TMDBID: 3, Name: "Forbidden Space Film", Genres: []string{"Science Fiction"}},
		{MediaType: provision.Movie, TMDBID: 4, Name: "Grounded Mystery", Genres: []string{"Mystery"}},
	}
	signals := []FeedbackSignal{
		{Target: "movie:tmdb:1", Action: FeedbackLess},
		{Target: "movie:tmdb:3", Action: FeedbackNever},
		{Target: "movie:tmdb:2", Action: FeedbackSurprise},
	}
	first := RankGroundedCandidates("space discoveries", candidates, signals)
	second := RankGroundedCandidates("space discoveries", candidates, signals)
	if !slices.EqualFunc(first, second, func(a, b catalog.Candidate) bool { return a.TMDBID == b.TMDBID }) {
		t.Fatalf("identical inputs produced different ranking: %+v / %+v", first, second)
	}
	var ids []int
	for _, candidate := range first {
		ids = append(ids, candidate.TMDBID)
	}
	if slices.Contains(ids, 3) {
		t.Fatalf("never target survived ranking: %v", ids)
	}
	if len(ids) < 2 || ids[0] != 2 || ids[1] != 1 {
		t.Fatalf("feedback ranking = %v, want surprise discovery before demoted related anchor", ids)
	}
}

func TestRankGroundedCandidatesWithTracePublishesExactLexicographicTupleAndBound(t *testing.T) {
	candidates := make([]catalog.Candidate, 0, DecisionTraceMaxCandidates+1)
	for i := 0; i < DecisionTraceMaxCandidates+1; i++ {
		candidates = append(candidates, catalog.Candidate{MediaType: provision.Movie, TMDBID: i + 1, Name: "same", Genres: []string{"same"}})
	}
	got := RankGroundedCandidatesWithTrace("same", candidates, nil)
	if !got.Trace.Truncated || got.Trace.SurfacedTotal != DecisionTraceMaxCandidates+1 || len(got.Trace.Candidates) != DecisionTraceMaxCandidates {
		t.Fatalf("trace bounds = %+v", got.Trace)
	}
	if err := ValidateDecisionTrace(got.Trace); err != nil {
		t.Fatalf("ranker emitted trace rejected by shared validator: %v", err)
	}
	for i, item := range got.Trace.Candidates {
		if item.Rank.TieKey != item.Key || item.Rank.Relevance != 1 || item.Rank.Preference != 0 || item.Rank.Novelty != 1 {
			t.Fatalf("trace[%d] = %+v; tuple must be independently reconstructable", i, item)
		}
		if i > 0 && got.Trace.Candidates[i-1].Rank.TieKey >= item.Rank.TieKey {
			t.Fatalf("tie keys not stable: %+v", got.Trace.Candidates)
		}
	}
}

func TestRankGroundedCandidatesWithTraceClassifiesRetrievalEmpty(t *testing.T) {
	got := RankGroundedCandidatesWithTrace("nothing", nil, nil)
	if got.Trace.Version != DecisionTraceVersion || got.Trace.Terminal != ReasonRetrievalEmpty || got.Trace.SurfacedTotal != 0 || got.Trace.RecordedTotal != 0 {
		t.Fatalf("empty retrieval trace = %+v", got.Trace)
	}
}
