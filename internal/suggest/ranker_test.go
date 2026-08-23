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
