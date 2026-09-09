package schedule_test

import (
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/testkit/libraryfixture"
)

func TestSeasonalExclusiveResolvesHolidaySeriesBeforeFiltering(t *testing.T) {
	key := provision.Key("series:tmdb:456")
	entries := []schedule.LineupEntry{{Key: key, Title: "The Simpsons", EpisodeSelection: schedule.EpisodeSelection{Mode: schedule.EpisodeHoliday, Holidays: []string{"christmas"}}}}
	avail := libraryfixture.Availability{Series: map[provision.Key]schedule.EpisodeResolution{key: {Programs: []schedule.ResolvedProgram{
		{LibraryItemID: "ordinary", Title: "An ordinary Tuesday", Season: 1, Episode: 1, DurationMs: 60000},
		{LibraryItemID: "christmas", Title: "A Christmas celebration", Season: 1, Episode: 2, DurationMs: 60000},
	}}}}
	policy := schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{Seasonal: schedule.SeasonalPolicy{
		Mode: schedule.SeasonalExclusive, Holidays: []string{"christmas"}, OffSeason: schedule.OffSeasonDark,
	}}}
	desired := schedule.ComputeDesiredAt(seasonalChannel(), entries, avail, schedule.PodFill, policy, dec20)
	if len(desired.Slots) != 1 || desired.Slots[0].LibraryItemID != "christmas" {
		t.Fatalf("exclusive holiday series schedule = %+v; exclusions = %+v", desired.Slots, desired.Excluded)
	}
}

func TestSeasonalExclusiveEpisodeEvidenceAndCalendar(t *testing.T) {
	key := provision.Key("series:tmdb:456")
	base := []schedule.ResolvedProgram{
		{LibraryItemID: "ordinary", Title: "Ordinary Tuesday", Season: 1, Episode: 1, DurationMs: 60000, OfficialRating: "TV-PG"},
		{LibraryItemID: "christmas", Title: "Christmas celebration", Season: 1, Episode: 2, DurationMs: 60000, OfficialRating: "TV-PG"},
		{LibraryItemID: "halloween", Title: "Halloween celebration", Season: 1, Episode: 3, DurationMs: 60000, OfficialRating: "TV-PG"},
		{LibraryItemID: "unknown", Season: 1, Episode: 4, DurationMs: 60000, OfficialRating: "TV-PG"},
		{LibraryItemID: "unsafe", Title: "Christmas celebration", Season: 1, Episode: 5, DurationMs: 60000, OfficialRating: "TV-MA"},
	}
	for _, tc := range []struct {
		name        string
		at          time.Time
		mode        schedule.SeasonalMode
		off         schedule.OffSeason
		unavailable bool
		want        []string
	}{
		{name: "December", at: dec20, mode: schedule.SeasonalExclusive, want: []string{"christmas"}},
		{name: "stale editorial evidence", at: dec20, mode: schedule.SeasonalExclusive, unavailable: true},
		{name: "July dark", at: jul14, mode: schedule.SeasonalExclusive, off: schedule.OffSeasonDark},
		{name: "July loop", at: jul14, mode: schedule.SeasonalExclusive, off: schedule.OffSeasonLoop, want: []string{"ordinary", "christmas", "halloween", "unknown"}},
		{name: "zero clock", mode: schedule.SeasonalExclusive, want: []string{"ordinary", "christmas", "halloween", "unknown"}},
		{name: "off unchanged", at: dec20, mode: schedule.SeasonalOff, want: []string{"ordinary", "christmas", "halloween", "unknown"}},
		{name: "auto unchanged", at: dec20, mode: schedule.SeasonalAuto, want: []string{"ordinary", "christmas", "halloween", "unknown"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entries := []schedule.LineupEntry{{Key: key, Title: "A regular series", OfficialRating: "TV-PG"}}
			avail := libraryfixture.Availability{Series: map[provision.Key]schedule.EpisodeResolution{key: {Programs: base, EditorialUnavailable: tc.unavailable}}}
			policy := schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{
				Audience: schedule.AudiencePolicy{Ceiling: "TV-PG"},
				Seasonal: schedule.SeasonalPolicy{Mode: tc.mode, Holidays: []string{"christmas"}, OffSeason: tc.off},
			}}
			d := schedule.ComputeDesiredAt(seasonalChannel(), entries, avail, schedule.PodFill, policy, tc.at)
			var got []string
			for _, s := range d.Slots {
				if s.IsProgram() {
					got = append(got, s.LibraryItemID)
				}
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("programs = %v, want %v", got, tc.want)
			}
			if d.PendingCount() != 0 {
				t.Fatal("seasonal exclusion falsely reported acquisition pending")
			}
			if !d.EligibleKeys[key] {
				t.Fatal("seasonal exclusion incorrectly removed library-availability evidence")
			}
		})
	}
}

func TestSeasonalExclusiveKeepsSupportedMultipartUnit(t *testing.T) {
	key := provision.Key("series:tmdb:456")
	entries := []schedule.LineupEntry{{Key: key, Title: "A regular series"}}
	avail := libraryfixture.Availability{Series: map[provision.Key]schedule.EpisodeResolution{key: {Programs: []schedule.ResolvedProgram{
		{LibraryItemID: "part-one", Title: "A winter visit (1)", Overview: "A Christmas gathering", Season: 1, Episode: 1, DurationMs: 60000},
		{LibraryItemID: "part-two", Title: "A winter visit (2)", Season: 1, Episode: 2, DurationMs: 60000},
		{LibraryItemID: "tagged", Title: "A reunion", Tags: []string{"Christmas"}, Season: 1, Episode: 3, DurationMs: 60000},
		{LibraryItemID: "ordinary", Title: "A normal day", Season: 1, Episode: 4, DurationMs: 60000},
	}}}}
	policy := schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{Seasonal: schedule.SeasonalPolicy{Mode: schedule.SeasonalExclusive, Holidays: []string{"christmas"}}}}
	d := schedule.ComputeDesiredAt(seasonalChannel(), entries, avail, schedule.PodFill, policy, dec20)
	var got []string
	for _, s := range d.Slots {
		got = append(got, s.LibraryItemID)
	}
	if !slices.Equal(got, []string{"part-one", "part-two", "tagged"}) {
		t.Fatalf("multipart schedule = %v", got)
	}
	if d.Slots[0].PartGroup == "" || d.Slots[0].PartGroup != d.Slots[1].PartGroup {
		t.Fatal("supported multipart unit lost its grouping")
	}
}

func TestSeasonalExclusiveHolidaySeriesTitleIsNotEpisodeEvidence(t *testing.T) {
	key := provision.Key("series:tmdb:456")
	entries := []schedule.LineupEntry{{Key: key, Title: "Christmas stories", EpisodeSelection: schedule.EpisodeSelection{Mode: schedule.EpisodeHoliday, Holidays: []string{"christmas"}}}}
	avail := libraryfixture.Availability{Series: map[provision.Key]schedule.EpisodeResolution{key: {Programs: []schedule.ResolvedProgram{
		{LibraryItemID: "ordinary", Title: "An ordinary Tuesday", Season: 1, Episode: 1, DurationMs: 60000},
	}}}}
	policy := schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{Seasonal: schedule.SeasonalPolicy{Mode: schedule.SeasonalExclusive, Holidays: []string{"christmas"}}}}
	d := schedule.ComputeDesiredAt(seasonalChannel(), entries, avail, schedule.PodFill, policy, dec20)
	if len(d.Slots) != 0 || d.PendingCount() != 0 {
		t.Fatalf("unknown holiday membership became playable/pending: %+v", d)
	}
	if len(d.Excluded.Items) != 1 || d.Excluded.Items[0].Reason != "out_of_season" {
		t.Fatalf("missing explicit seasonal exclusion: %+v", d.Excluded)
	}
}
