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
