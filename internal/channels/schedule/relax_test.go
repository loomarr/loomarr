package schedule_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/library/provision"
)

// relaxShows builds a series channel whose shows have the given episode counts, every episode
// `each` long, and returns its relaxation record under the separation policy.
func relaxShows(t *testing.T, counts []int, each time.Duration, sep schedule.SeparationPolicy) []schedule.AppliedRelaxation {
	t.Helper()
	var entries []schedule.LineupEntry
	episodes := map[string][]schedule.ResolvedProgram{}
	for i, n := range counts {
		key := fmt.Sprintf("series:tvdb:%d", 90000001+i)
		entries = append(entries, schedule.LineupEntry{Key: provision.Key(key), Title: key})
		for e := range n {
			episodes[key] = append(episodes[key], schedule.ResolvedProgram{
				LibraryItemID: fmt.Sprintf("%d-%d", i, e), Title: key, DurationMs: each.Milliseconds(),
				Season: 1, Episode: e + 1,
			})
		}
	}
	p := schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{Ordering: schedule.OrderSyndication, Separation: sep}}
	ch := schedule.Channel{ID: "r", Name: "R", Number: 1, Strategy: schedule.Shuffle, Shuffle: schedule.ShuffleParams{Seed: 3}}
	return schedule.ComputeDesiredAt(ch, entries, newSeriesAvail(episodes), schedule.PodFill, p, time.Time{}).Applied
}

// The install's channels all had episodeNoRepeat cut to 24 h because their series gap could not
// be kept (#4140): the ladder shortened the no-repeat window first whatever had breached. Here the
// cycle airs 200 h, longer than the week the window asks for, so only the gap relaxes, and only to
// the 5 h one show holding half the episodes allows.
func TestRelax_AGapShortfallKeepsTheNoRepeatWindow(t *testing.T) {
	got := relaxShows(t, []int{20, 10, 10}, 5*time.Hour, schedule.SeparationPolicy{SeriesMinGap: schedule.Duration(12 * time.Hour)})
	want := []schedule.AppliedRelaxation{{Kind: "seriesMinGap", From: "12h0m0s", To: "5h0m0s"}}
	if !slices.Equal(got, want) {
		t.Fatalf("applied %+v, want %+v", got, want)
	}
}

// Twenty episodes of one show among five of another cannot run fewer than four in a row: the
// back-to-back cap rises to four, not to unbounded. A run of two always breaches a gap, so the gap
// goes first, and to zero only because nothing wider holds.
func TestRelax_ABlockShortfallRisesToTheShortestRunThatHolds(t *testing.T) {
	got := relaxShows(t, []int{20, 5}, 7*time.Hour, schedule.SeparationPolicy{BlockMax: 1})
	want := []schedule.AppliedRelaxation{
		{Kind: "seriesMinGap", From: "2h0m0s", To: "0s"},
		{Kind: "blockMax", From: "1", To: "4"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("applied %+v, want %+v", got, want)
	}
}

// A cycle shorter than its no-repeat window repeats once per loop, so the record says how often:
// the window becomes the cycle's runtime rather than a floor the channel still cannot keep.
func TestRelax_AShortCycleRecordsItsOwnRuntime(t *testing.T) {
	got := relaxShows(t, []int{2}, time.Minute, schedule.SeparationPolicy{EpisodeNoRepeat: schedule.Duration(720 * time.Hour)})
	want := []schedule.AppliedRelaxation{{Kind: "episodeNoRepeat", From: "720h0m0s", To: "2m0s"}}
	if !slices.Equal(got, want) {
		t.Fatalf("applied %+v, want %+v", got, want)
	}
}

// Pool growth un-relaxes automatically (§7): four one-hour shows honor a 2 h no-repeat window and
// the default gap, so nothing is recorded.
func TestRelax_UnRelaxesWhenPoolRecovers(t *testing.T) {
	got := relaxShows(t, []int{1, 1, 1, 1}, time.Hour, schedule.SeparationPolicy{EpisodeNoRepeat: schedule.Duration(2 * time.Hour)})
	if len(got) != 0 {
		t.Errorf("a healthy pool should apply NO relaxation, got %+v", got)
	}
}

// The ladder NEVER relaxes audience/scope (§7): even after full descent on a tiny
// pool, a TV-MA item stays off a kids channel and an out-of-era item stays out.
// (Complements TestEnforce_TVMA_NeverUnderKidsCeiling_EvenAfterRelaxation with the
// scope side.)
func TestRelax_NeverRelaxesScope(t *testing.T) {
	entries := []schedule.LineupEntry{
		ratedEntry("movie:tmdb:90000001", "In Era", "", 1994),
		ratedEntry("movie:tmdb:90000002", "Out Of Era", "", 1980),
	}
	avail := mapAvail{"movie:tmdb:90000001": "l1", "movie:tmdb:90000002": "l2"}
	p := schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{Scope: schedule.ScopePolicy{Dates: schedule.EraDates(schedule.Range{From: 1990, To: 1999})}, Separation: schedule.SeparationPolicy{EpisodeNoRepeat: schedule.Duration(720 * time.Hour)}}}
	d := schedule.ComputeDesiredAt(policyChannel(), entries, avail, schedule.PodFill, p, time.Time{})
	if hasKey(programKeys(d), "movie:tmdb:90000002") {
		t.Error("scope (era) must NEVER be relaxed to admit an out-of-era title")
	}
	for _, a := range d.Applied {
		if a.Kind == "era" {
			t.Errorf("a channel short of titles stays inside its line, got %+v", a)
		}
	}
}
