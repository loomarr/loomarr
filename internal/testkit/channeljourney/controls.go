package channeljourney

import (
	"slices"
	"time"

	"github.com/loomarr/loomarr/internal/schedule"
)

// EligibilityControls changes a relevant fact or explicit season limit on fresh
// reference fixtures. A formerly excluded identity must now appear; the baseline
// Cases still prove exclusion under the original facts and approved policy.
func EligibilityControls() []Case {
	named := Cases()[0]
	named.Name += "_airing_year_control"
	named.Episodes["full-house"][0].Year = 1990
	named.Want = append([]string{"full-house-1989"}, named.Want...)
	named.Forbidden = slices.DeleteFunc(named.Forbidden, func(id string) bool { return id == "full-house-1989" })

	date := Cases()[1]
	date.Name += "_release_year_control"
	date.Proposal.Lineup[2].Year = 1997
	date.Want = append(date.Want, "later")
	date.Forbidden = []string{"romance"}

	genre := Cases()[1]
	genre.Name += "_genre_control"
	genre.Proposal.Lineup[3].Genres = []string{"Science Fiction"}
	genre.Want = append(genre.Want, "romance")
	genre.Forbidden = []string{"later"}

	unrated := Cases()[2]
	unrated.Name += "_classified_control"
	unrated.Proposal.Lineup[1].OfficialRating = "PG"
	unrated.Want = append(unrated.Want, "unclassified")
	unrated.Forbidden = []string{"mature"}

	rating := Cases()[2]
	rating.Name += "_rating_control"
	rating.Proposal.Lineup[2].OfficialRating = "PG"
	rating.Want = append(rating.Want, "mature")
	rating.Forbidden = []string{"unclassified"}

	seasons := Cases()[3]
	seasons.Name += "_season_limit_control"
	seasons.Proposal.Intent.Description = "The Simpsons seasons one through twelve in episode order"
	seasons.Proposal.Lineup[0].SeasonMax = 12
	seasons.Want = append(seasons.Want, "classic-s12e01")
	seasons.Forbidden = nil

	seasonal := Cases()[4]
	seasonal.Name += "_episode_evidence_control"
	seasonal.Episodes["simpsons"][1].Title = "Synthetic second Christmas celebration"
	// Both episodes now qualify, so supply the library resolver's season/episode order.
	slices.Reverse(seasonal.Episodes["simpsons"])
	seasonal.Want = []string{"ordinary", "christmas"}
	seasonal.Forbidden = nil
	return []Case{named, date, genre, unrated, rating, seasons, seasonal}
}

// SeparatedNamedBlock uses only the governed member identities from Cases. Six
// 22-minute episodes can satisfy a two-hour repeat window and 44-minute series gap.
// Want is the exact membership set; syndication determines its seeded order.
func SeparatedNamedBlock() Case {
	c := Cases()[0]
	c.Name = "named_block_separation"
	c.Proposal.Policy.Ordering = schedule.OrderSyndication
	c.Proposal.Policy.Separation = schedule.SeparationPolicy{
		EpisodeNoRepeat: schedule.Duration(2 * time.Hour),
		SeriesMinGap:    schedule.Duration(44 * time.Minute),
		BlockMax:        1,
	}
	for _, series := range []string{"full-house", "family-matters", "step-by-step"} {
		id := series + "-1994"
		c.Episodes[series] = append(c.Episodes[series], episode(id, "Synthetic later member episode", 4, 1, 1994))
		c.Want = append(c.Want, id)
	}
	return c
}

// SparseSingleShow keeps the explicit season exclusions with too little runtime
// to honor its requested repeat window. Reconciliation must explain relaxation.
func SparseSingleShow() Case {
	c := Cases()[3]
	c.Name = "single_show_sparse_repeat"
	c.Proposal.Policy.Separation.EpisodeNoRepeat = schedule.Duration(48 * time.Hour)
	return c
}

// SeparatedMovies retains the genre/date negatives and declares the two-hour
// repeat window its two available one-hour movies can actually satisfy.
func SeparatedMovies() Case {
	c := Cases()[1]
	c.Name = "genre_era_movie_repeat"
	c.Proposal.Policy.Separation.MovieNoRepeat = schedule.Duration(2 * time.Hour)
	return c
}
