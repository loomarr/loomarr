// Package channeljourney holds the public synthetic approval-to-schedule fixtures
// for #1103. These are deterministic contracts, never live-provider evidence.
package channeljourney

import (
	"time"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/suggest"
)

const Version = "channel-reference-journeys-v1"

// Case joins an already-grounded Proposal to concrete expected playable items.
// Generation quality remains owned by the existing planner certification corpus.
type Case struct {
	Name      string
	Proposal  suggest.Proposal
	Episodes  map[string][]schedule.ResolvedProgram
	Want      []string
	Forbidden []string
}

func Clock() time.Time { return time.Date(2026, 12, 24, 18, 0, 0, 0, time.UTC) }

func movie(id int, name, libraryID string, year int, rating string) suggest.ProposalItem {
	return suggest.ProposalItem{MediaType: provision.Movie, TMDBID: id, Name: name,
		InLibrary: true, LibraryItemID: libraryID, Year: year, OfficialRating: rating}
}

func episode(id, title string, season, number, year int) schedule.ResolvedProgram {
	return schedule.ResolvedProgram{LibraryItemID: id, Title: title, Season: season,
		Episode: number, Year: year, DurationMs: 22 * 60 * 1000, OfficialRating: "TV-PG"}
}

// Cases returns fresh fixtures so mutations in one negative test cannot change
// another test's truth. Episode titles/metadata are synthetic, not historical claims.
func Cases() []Case {
	// These member identities and the lexical-neighbor negative mirror the shape of
	// TestSuggest_NamedCollectionAdmitsOnlyEnumeratedCatalogMembers (#968/#1022), with
	// invented demo-catalogue titles (#1592).
	named := suggest.Proposal{Intent: suggest.Intent{
		Description: "a named Friday-night family sitcom collection",
		MustInclude: []string{"Bowling Alley Blues", "Crumb Street Bakery", "Grand Oak Hotel"},
	}, Lineup: []suggest.ProposalItem{
		{MediaType: provision.Series, TVDBID: 90000762, Name: "Bowling Alley Blues", Year: 1987, InLibrary: true, LibraryItemID: "bowling-alley-blues"},
		{MediaType: provision.Series, TVDBID: 90000767, Name: "Crumb Street Bakery", Year: 1989, InLibrary: true, LibraryItemID: "crumb-street-bakery"},
		{MediaType: provision.Series, TVDBID: 90000760, Name: "Grand Oak Hotel", Year: 1991, InLibrary: true, LibraryItemID: "grand-oak-hotel"},
	}}
	named.Policy.Scope.Dates = &schedule.DateScope{SeriesAiring: []schedule.Range{{From: 1990, To: 1999}}}
	named.Policy.Ordering = schedule.OrderSequential
	genre := suggest.Proposal{Intent: suggest.Intent{Description: "nineties science fiction movies"}, Lineup: []suggest.ProposalItem{
		movie(90091001, "Synthetic Signal", "signal", 1992, "PG"),
		movie(90091002, "Synthetic Orbit", "orbit", 1998, "PG"),
		movie(90091003, "Synthetic Later", "later", 2003, "PG"),
	}}
	for i := range genre.Lineup {
		genre.Lineup[i].Genres = []string{"Science Fiction"}
	}
	otherGenre := movie(90091004, "Synthetic Romance", "romance", 1995, "PG")
	otherGenre.Genres = []string{"Romance"}
	genre.Lineup = append(genre.Lineup, otherGenre)
	genre.Policy.Scope.Genres.Include = []string{"Science Fiction"}
	genre.Policy.Scope.Dates = &schedule.DateScope{MovieRelease: []schedule.Range{{From: 1990, To: 1999}}}
	genre.Policy.Ordering = schedule.OrderSequential
	family := suggest.Proposal{Intent: suggest.Intent{Description: "family movie night"}, Lineup: []suggest.ProposalItem{
		movie(90092001, "Synthetic Family", "family", 2000, "G"),
		movie(90092002, "Synthetic Unclassified", "unclassified", 2000, ""),
		movie(90092003, "Synthetic Mature", "mature", 2000, "R"),
	}}
	family.Policy.Audience = schedule.AudiencePolicy{Ceiling: "TV-PG", Unrated: "exclude"}
	classic := suggest.Proposal{Intent: suggest.Intent{Description: "Kettle Kids seasons one through two in episode order"}, Lineup: []suggest.ProposalItem{
		{MediaType: provision.Series, TMDBID: 90000456, Name: "Kettle Kids", InLibrary: true, LibraryItemID: "kettle-kids", SeasonMin: 1, SeasonMax: 2},
	}}
	classic.Policy.Ordering = schedule.OrderSequential
	seasonal := suggest.Proposal{Intent: suggest.Intent{Description: "Christmas episodes of Kettle Kids already in my library"}, Lineup: []suggest.ProposalItem{
		{MediaType: provision.Series, TMDBID: 90000456, Name: "Kettle Kids", InLibrary: true, LibraryItemID: "kettle-kids"},
	}}
	seasonal.Policy.Ordering = schedule.OrderSequential
	seasonal.Policy.Seasonal = schedule.SeasonalPolicy{Mode: schedule.SeasonalExclusive, Holidays: []string{"christmas"}, OffSeason: schedule.OffSeasonDark}
	return []Case{
		{Name: "named_block", Proposal: named, Episodes: map[string][]schedule.ResolvedProgram{
			"bowling-alley-blues": {episode("bowling-alley-blues-1989", "Synthetic earlier episode", 1, 1, 1989), episode("bowling-alley-blues-1991", "Synthetic member episode", 2, 1, 1991)},
			"crumb-street-bakery": {episode("crumb-street-bakery-1992", "Synthetic member episode", 3, 1, 1992)},
			"grand-oak-hotel":     {episode("grand-oak-hotel-1993", "Synthetic member episode", 3, 1, 1993)},
		}, Want: []string{"bowling-alley-blues-1991", "crumb-street-bakery-1992", "grand-oak-hotel-1993"}, Forbidden: []string{"bowling-alley-blues-1989", "neighbor"}},
		{Name: "genre_era_movies", Proposal: genre, Want: []string{"signal", "orbit"}, Forbidden: []string{"later", "romance"}},
		{Name: "family", Proposal: family, Want: []string{"family"}, Forbidden: []string{"unclassified", "mature"}},
		{Name: "single_show", Proposal: classic, Episodes: map[string][]schedule.ResolvedProgram{
			// Library's EpisodeResolver supplies season/episode order.
			"kettle-kids": {episode("classic-s01e01", "Synthetic first", 1, 1, 1990), episode("classic-s02e02", "Synthetic second", 2, 2, 1991), episode("classic-s12e01", "Synthetic late", 12, 1, 2000)},
		}, Want: []string{"classic-s01e01", "classic-s02e02"}, Forbidden: []string{"classic-s12e01"}},
		{Name: "seasonal", Proposal: seasonal, Episodes: map[string][]schedule.ResolvedProgram{
			"kettle-kids": {episode("christmas", "Synthetic Christmas celebration", 2, 2, 1991), episode("ordinary", "Synthetic ordinary Tuesday", 2, 1, 1991)},
		}, Want: []string{"christmas"}, Forbidden: []string{"ordinary"}},
	}
}
