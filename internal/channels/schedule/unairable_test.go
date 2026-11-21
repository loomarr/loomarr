package schedule_test

import (
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/platform/rating"
)

// On the install four lineup titles never aired in six days and nothing said why (#4140): one
// was not in the library, one asked for seasons the library does not hold, one pointed at a
// library item that no longer exists. Every lineup title that contributes nothing to the cycle
// is now reported with its reason; a title that airs, or rests for its season, is not.
func TestComputeDesired_ReportsTitlesThatCannotAir(t *testing.T) {
	episodes := func(prefix string, seasons ...int) []schedule.ResolvedProgram {
		var out []schedule.ResolvedProgram
		for _, s := range seasons {
			for e := 1; e <= 3; e++ {
				out = append(out, schedule.ResolvedProgram{
					LibraryItemID: prefix + string(rune('0'+s)) + string(rune('0'+e)), Title: prefix,
					DurationMs: (22 * time.Minute).Milliseconds(), Season: s, Episode: e, Year: 1990,
				})
			}
		}
		return out
	}
	avail := seriesAvail{
		movies: map[provision.Key]struct {
			id  string
			dur int64
		}{
			"movie:tmdb:90000001": {"film-1", (95 * time.Minute).Milliseconds()},
			"movie:tmdb:90000003": {"film-3", (95 * time.Minute).Milliseconds()},
		},
		episodes: map[provision.Key][]schedule.ResolvedProgram{
			"series:tvdb:90000011": episodes("airs", 1, 2),
			"series:tvdb:90000012": episodes("early", 1, 2, 3),
			"series:tvdb:90000014": episodes("mature", 1),
		},
	}
	mature := episodes("mature", 1)
	for i := range mature {
		mature[i].OfficialRating = rating.Normalize("TV-MA")
	}
	avail.episodes["series:tvdb:90000014"] = mature

	entries := []schedule.LineupEntry{
		{Key: "series:tvdb:90000011", Title: "Airs", Year: 1990},
		{Key: "series:tvdb:90000012", Title: "Early Seasons Only", Year: 1990, SeasonMin: 5, SeasonMax: 8},
		{Key: "series:tvdb:90000013", Title: "Not Downloaded", Year: 1990},
		{Key: "series:tvdb:90000014", Title: "Too Mature", Year: 1990, OfficialRating: rating.Normalize("TV-PG")},
		{Key: "movie:tmdb:90000001", Title: "In Era", Year: 1992},
		{Key: "movie:tmdb:90000002", Title: "Missing Film", Year: 1993},
		{Key: "movie:tmdb:90000003", Title: "Out Of Era", Year: 1975},
	}
	p := schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{
		Scope:    schedule.ScopePolicy{Dates: schedule.EraDates(schedule.Range{From: 1985, To: 1999})},
		Audience: schedule.AudiencePolicy{Ceiling: rating.Normalize("TV-14")},
	}}
	d := schedule.ComputeDesiredAt(policyChannel(), entries, avail, schedule.PodFill, p, time.Time{})

	want := []schedule.UnairableTitle{
		{Key: "series:tvdb:90000012", Title: "Early Seasons Only", Reason: schedule.UnairableNoEpisodesInSeasons},
		{Key: "series:tvdb:90000013", Title: "Not Downloaded", Reason: schedule.UnairableNotInLibrary},
		{Key: "series:tvdb:90000014", Title: "Too Mature", Reason: schedule.UnairableOverCeiling},
		{Key: "movie:tmdb:90000002", Title: "Missing Film", Reason: schedule.UnairableNotInLibrary},
		{Key: "movie:tmdb:90000003", Title: "Out Of Era", Reason: schedule.UnairableOutOfScope},
	}
	if !slices.Equal(d.Unairable, want) {
		t.Fatalf("unairable\n got %+v\nwant %+v", d.Unairable, want)
	}
}

// Every title airing leaves nothing to report.
func TestComputeDesired_NothingToReportWhenEveryTitleAirs(t *testing.T) {
	entries := []schedule.LineupEntry{entry("movie:tmdb:90000001", "A"), entry("movie:tmdb:90000002", "B")}
	d := schedule.ComputeDesiredAt(policyChannel(), entries, mapAvail{"movie:tmdb:90000001": "a", "movie:tmdb:90000002": "b"}, schedule.PodFill, schedule.ChannelPolicy{}, time.Time{})
	if len(d.Unairable) != 0 {
		t.Fatalf("unairable %+v, want none", d.Unairable)
	}
}
