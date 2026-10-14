//go:build eval

package eval

import (
	"context"
	"slices"
	"strconv"
	"testing"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/suggest"
)

func TestV8ScheduledMovieDisjointWindowsExcludeTheGap(t *testing.T) {
	caseValue := v8ScheduledCase(t, "date-movie-disjoint")
	materializer, err := NewFrozenCertificationScheduleMaterializer()
	if err != nil {
		t.Fatal(err)
	}
	proposal := suggest.Proposal{Lineup: []suggest.ProposalItem{
		{MediaType: provision.Movie, TMDBID: 11003, Name: "Synthetic Early Window", Year: 1990, InLibrary: true, LibraryItemID: "fixture-11003", OfficialRating: "PG"},
		{MediaType: provision.Movie, TMDBID: 11004, Name: "Synthetic Late Window", Year: 2000, InLibrary: true, LibraryItemID: "fixture-11004", OfficialRating: "PG"},
		{MediaType: provision.Movie, TMDBID: 11005, Name: "Synthetic Gap Distractor", Year: 1996, InLibrary: true, LibraryItemID: "fixture-11005", OfficialRating: "PG"},
	}}

	t.Run("correct disjoint policy", func(t *testing.T) {
		proposal := proposal
		proposal.Policy.Scope.Dates = &schedule.DateScope{MovieRelease: []schedule.Range{{From: 1990, To: 1991}, {From: 2000, To: 2001}}}
		assertV8SchedulePass(t, caseValue, materializer, proposal)
	})
	for _, tc := range []struct {
		name  string
		dates *schedule.DateScope
	}{
		{name: "missing dates"},
		{name: "widened convex hull", dates: &schedule.DateScope{MovieRelease: []schedule.Range{{From: 1990, To: 2001}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proposal := proposal
			proposal.Policy.Scope.Dates = tc.dates
			assertV8ScheduleFailureWithForbidden(t, caseValue, materializer, proposal, "movie:tmdb:11005")
		})
	}
}

func TestV8ScheduledSeriesAiringWindowsFilterEpisodes(t *testing.T) {
	materializer, err := NewFrozenCertificationScheduleMaterializer()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		caseName  string
		tmdbID    int
		year      int
		premiere  schedule.Range
		libraryID string
	}{
		{name: "premiere and airing", caseName: "date-series-premiere-airing", tmdbID: 11006, year: 1980, premiere: schedule.Range{From: 1980, To: 1980}, libraryID: "fixture-11006"},
		{name: "pre era and airing", caseName: "date-series-pre-era-airing", tmdbID: 11007, year: 1985, premiere: schedule.Range{From: 1900, To: 1989}, libraryID: "fixture-11007"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caseValue := v8ScheduledCase(t, tc.caseName)
			proposal := suggest.Proposal{Lineup: []suggest.ProposalItem{{
				MediaType: provision.Series, TMDBID: tc.tmdbID, Name: "Synthetic series", Year: tc.year,
				InLibrary: true, LibraryItemID: tc.libraryID, OfficialRating: "TV-PG",
			}}}
			correct := proposal
			correct.Policy.Scope.Dates = &schedule.DateScope{
				SeriesPremiere: []schedule.Range{tc.premiere}, SeriesAiring: []schedule.Range{{From: 2000, To: 2000}},
			}
			assertV8SchedulePass(t, caseValue, materializer, correct)

			for _, bad := range []struct {
				name  string
				dates *schedule.DateScope
			}{
				{name: "missing airing", dates: &schedule.DateScope{SeriesPremiere: []schedule.Range{tc.premiere}}},
				{name: "widened airing", dates: &schedule.DateScope{SeriesPremiere: []schedule.Range{tc.premiere}, SeriesAiring: []schedule.Range{{From: 1999, To: 2001}}}},
			} {
				t.Run(bad.name, func(t *testing.T) {
					badProposal := proposal
					badProposal.Policy.Scope.Dates = bad.dates
					assertV8ScheduleFailureWithForbidden(t, caseValue, materializer, badProposal, seriesEpisodeIdentity(tc.tmdbID, 1))
				})
			}

			t.Run("airing placed on premiere excludes required episode", func(t *testing.T) {
				badProposal := proposal
				badProposal.Policy.Scope.Dates = &schedule.DateScope{SeriesPremiere: []schedule.Range{{From: 2000, To: 2000}}}
				assertV8ScheduleFailure(t, caseValue, materializer, badProposal)
			})
		})
	}
}

func v8ScheduledCase(t *testing.T, name string) Case {
	t.Helper()
	cases, err := CertificationCases()
	if err != nil {
		t.Fatal(err)
	}
	c := certificationCaseByName(t, cases, name)
	// These tests establish the scheduler's causal behavior. Date/proposal quality
	// scores belong to their dedicated tests; schedule expectations remain hard gates.
	c.ExpectedDateScope = nil
	c.ExpectedProposalKeys = nil
	return c
}

func assertV8SchedulePass(t *testing.T, c Case, materializer ScheduleMaterializer, proposal suggest.Proposal) {
	t.Helper()
	card := NewRunner(scriptedGenerator{proposal: proposal}, RunnerConfig{}).WithMaterializer(materializer).Run(context.Background(), []Case{c})
	if !card.Certified || card.Results[0].FailureStage != "" {
		t.Fatalf("schedule result = %+v", card.Results[0])
	}
}

func assertV8ScheduleFailure(t *testing.T, c Case, materializer ScheduleMaterializer, proposal suggest.Proposal) {
	t.Helper()
	card := NewRunner(scriptedGenerator{proposal: proposal}, RunnerConfig{}).WithMaterializer(materializer).Run(context.Background(), []Case{c})
	if card.Certified || card.Results[0].FailureStage != FailureStageSchedule || card.FailureCounts[FailureStageSchedule] != 1 {
		t.Fatalf("schedule failure result = %+v counts=%v", card.Results[0], card.FailureCounts)
	}
}

func assertV8ScheduleFailureWithForbidden(t *testing.T, c Case, materializer ScheduleMaterializer, proposal suggest.Proposal, forbidden string) {
	t.Helper()
	card := NewRunner(scriptedGenerator{proposal: proposal}, RunnerConfig{}).WithMaterializer(materializer).Run(context.Background(), []Case{c})
	result := card.Results[0]
	if !slices.Contains(result.ScheduledPrograms, forbidden) {
		t.Fatalf("scheduled programs = %v, want forbidden %q to prove the policy leak", result.ScheduledPrograms, forbidden)
	}
	if card.Certified || result.FailureStage != FailureStageSchedule || card.FailureCounts[FailureStageSchedule] != 1 {
		t.Fatalf("schedule failure result = %+v counts=%v", result, card.FailureCounts)
	}
}

func seriesEpisodeIdentity(tmdbID, episode int) string {
	return "series:tmdb:" + strconv.Itoa(tmdbID) + ":s01e0" + strconv.Itoa(episode)
}
