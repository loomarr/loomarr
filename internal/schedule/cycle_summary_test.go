package schedule_test

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
)

// The Programming page tells a household what one pass through a channel means ("226 episodes,
// about 3½ days"). That sentence is only honest if Cycle counts the whole deck — not the
// rolling-window slice that happens to be airing — and counts only what can actually air.
func TestComputeDesired_CycleSummarySizesTheWholeDeck(t *testing.T) {
	const min = int64(60_000)
	eps := func(n int, dur int64) []schedule.ResolvedProgram {
		out := make([]schedule.ResolvedProgram, n)
		for i := range out {
			out[i] = schedule.ResolvedProgram{LibraryItemID: "e", Title: "ep", DurationMs: dur, Season: 1, Episode: i + 1}
		}
		return out
	}
	avail := seriesAvail{
		movies: map[provision.Key]struct {
			id  string
			dur int64
		}{"movie:tmdb:1": {id: "lib-m", dur: 100 * min}},
		episodes: map[provision.Key][]schedule.ResolvedProgram{
			"series:tvdb:1": eps(10, 20*min),
			"series:tvdb:2": eps(4, 30*min),
		},
	}
	entries := []schedule.LineupEntry{
		entry("series:tvdb:1", "Show One"),
		entry("series:tvdb:2", "Show Two"),
		entry("movie:tmdb:1", "A Movie"),
		entry("movie:tmdb:9", "Not Yet Available"), // pending: cannot air, so not counted
	}

	// A one-hour window slices the deck to a handful of programmes; the summary must not.
	ch := seqChannel()
	ch.DefaultWindow = time.Hour
	got := schedule.ComputeDesired(ch, entries, avail, schedule.PodFill)

	want := schedule.CycleSummary{Episodes: 14, Movies: 1, Series: 2, RuntimeMs: (10*20 + 4*30 + 100) * min}
	if got.Cycle != want {
		t.Fatalf("Cycle = %+v, want %+v", got.Cycle, want)
	}
}

// Each case below is a deck the raw lineup would size differently from the one that can air: the
// summary must come from the filtered, ordered deck, so a household is never told about episodes or
// films the channel will not play.
func TestComputeDesired_CycleSummaryCountsOnlyWhatCanAir(t *testing.T) {
	const min = int64(60_000)
	policy := func(p schedule.ProposalPolicy) schedule.ChannelPolicy {
		return schedule.ChannelPolicy{ProposalPolicy: p}
	}
	compute := func(entries []schedule.LineupEntry, avail seriesAvail, p schedule.ChannelPolicy, at time.Time) schedule.CycleSummary {
		return schedule.ComputeDesiredAt(seqChannel(), entries, avail, schedule.PodFill, p, at).Cycle
	}
	movies := func(ids ...string) seriesAvail {
		a := seriesAvail{movies: map[provision.Key]struct {
			id  string
			dur int64
		}{}}
		for _, id := range ids {
			a.movies[provision.Key(id)] = struct {
				id  string
				dur int64
			}{id: "lib-" + id, dur: 90 * min}
		}
		return a
	}

	t.Run("audience ceiling leaves over-ceiling films out", func(t *testing.T) {
		entries := []schedule.LineupEntry{
			ratedEntry("movie:tmdb:1", "Fine", "TV-14", 1994),
			ratedEntry("movie:tmdb:2", "Too Adult", "R", 1994),
			ratedEntry("movie:tmdb:3", "Also Too Adult", "R", 1995),
		}
		p := policy(schedule.ProposalPolicy{Audience: schedule.AudiencePolicy{Ceiling: "TV-14"}})
		got := compute(entries, movies("movie:tmdb:1", "movie:tmdb:2", "movie:tmdb:3"), p, time.Time{})
		if want := (schedule.CycleSummary{Movies: 1, RuntimeMs: 90 * min}); got != want {
			t.Fatalf("Cycle = %+v, want %+v", got, want)
		}
	})

	t.Run("season range counts only the chosen seasons", func(t *testing.T) {
		var eps []schedule.ResolvedProgram
		for s := 1; s <= 12; s++ {
			eps = append(eps, schedule.ResolvedProgram{LibraryItemID: "e", Title: "ep", DurationMs: 20 * min, Season: s, Episode: 1})
			eps = append(eps, schedule.ResolvedProgram{LibraryItemID: "e", Title: "ep", DurationMs: 20 * min, Season: s, Episode: 2})
		}
		avail := seriesAvail{episodes: map[provision.Key][]schedule.ResolvedProgram{"series:tvdb:1": eps}}
		e := entry("series:tvdb:1", "Long Runner")
		e.SeasonMin, e.SeasonMax = 1, 10
		got := compute([]schedule.LineupEntry{e}, avail, schedule.ChannelPolicy{}, time.Time{})
		if want := (schedule.CycleSummary{Episodes: 20, Series: 1, RuntimeMs: 20 * 20 * min}); got != want {
			t.Fatalf("Cycle = %+v, want %+v", got, want)
		}
	})

	t.Run("era scope leaves out-of-era films", func(t *testing.T) {
		entries := []schedule.LineupEntry{
			ratedEntry("movie:tmdb:1", "Nineties", "", 1994),
			ratedEntry("movie:tmdb:2", "Eighties", "", 1984),
			ratedEntry("movie:tmdb:3", "Aughts", "", 2004),
		}
		p := policy(schedule.ProposalPolicy{Scope: schedule.ScopePolicy{Dates: schedule.EraDates(schedule.Range{From: 1990, To: 1999})}})
		got := compute(entries, movies("movie:tmdb:1", "movie:tmdb:2", "movie:tmdb:3"), p, time.Time{})
		if want := (schedule.CycleSummary{Movies: 1, RuntimeMs: 90 * min}); got != want {
			t.Fatalf("Cycle = %+v, want %+v", got, want)
		}
	})

	t.Run("out-of-season title is benched", func(t *testing.T) {
		entries := []schedule.LineupEntry{
			ratedEntry("movie:tmdb:1", "Christmas Carol", "", 1994),
			ratedEntry("movie:tmdb:2", "Regular Show", "", 1994),
		}
		p := policy(schedule.ProposalPolicy{Seasonal: schedule.SeasonalPolicy{Mode: schedule.SeasonalAuto}})
		avail := movies("movie:tmdb:1", "movie:tmdb:2")
		if got, want := compute(entries, avail, p, jul14), (schedule.CycleSummary{Movies: 1, RuntimeMs: 90 * min}); got != want {
			t.Fatalf("July Cycle = %+v, want %+v", got, want)
		}
	})

	// A multi-part story airs as one slot per part, back to back; each part is its own file with its
	// own runtime, so it counts once per part. The rule: Episodes = parts that air.
	t.Run("multi-part episode counts once per part", func(t *testing.T) {
		eps := []schedule.ResolvedProgram{
			{LibraryItemID: "a", Title: "Regular", DurationMs: 20 * min, Season: 1, Episode: 1},
			{LibraryItemID: "b1", Title: "Big Day (1)", DurationMs: 20 * min, Season: 1, Episode: 2},
			{LibraryItemID: "b2", Title: "Big Day (2)", DurationMs: 20 * min, Season: 1, Episode: 3},
		}
		avail := seriesAvail{episodes: map[provision.Key][]schedule.ResolvedProgram{"series:tvdb:1": eps}}
		got := compute([]schedule.LineupEntry{entry("series:tvdb:1", "Show")}, avail, schedule.ChannelPolicy{}, time.Time{})
		if want := (schedule.CycleSummary{Episodes: 3, Series: 1, RuntimeMs: 60 * min}); got != want {
			t.Fatalf("Cycle = %+v, want %+v", got, want)
		}
	})
}

// Two different shows can share a name (a remake). They are two series on the channel.
func TestComputeDesired_CycleSummaryCountsSeriesByIdentityNotTitle(t *testing.T) {
	const min = int64(60_000)
	ep := func(id string) []schedule.ResolvedProgram {
		return []schedule.ResolvedProgram{{LibraryItemID: id, Title: "ep", DurationMs: 20 * min, Season: 1, Episode: 1}}
	}
	avail := seriesAvail{episodes: map[provision.Key][]schedule.ResolvedProgram{
		"series:tvdb:1": ep("a"), "series:tvdb:2": ep("b"),
	}}
	entries := []schedule.LineupEntry{entry("series:tvdb:1", "Battlestar Galactica"), entry("series:tvdb:2", "Battlestar Galactica")}
	got := schedule.ComputeDesired(seqChannel(), entries, avail, schedule.PodFill).Cycle
	if got.Series != 2 || got.Episodes != 2 {
		t.Fatalf("Cycle = %+v, want 2 series / 2 episodes", got)
	}
}
