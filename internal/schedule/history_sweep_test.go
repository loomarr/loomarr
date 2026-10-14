package schedule_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/schedule"
)

// Viewing history (#1939). Production always has it after the first day: each window opens with
// what the windows before it aired on record. These tests air a channel window after window through
// ComputeDesiredAt, loop a short window the way playout does (internal/playout/timeline.go,
// elapsed % total), and feed every airing back as history before the next window opens.

// historyAiring is one programme as it aired, in ms from the first window's open.
type historyAiring struct {
	id string
	at int64
}

// airWithHistory airs `windows` windows of a single-series channel of n episodes of the given
// durations (cycled), with or without the viewing history fed back.
func airWithHistory(n int, durs []time.Duration, window time.Duration, ordering schedule.OrderingMode, windows int, withHistory bool) []historyAiring {
	eps := manyEpisodes(n)
	for i := range eps {
		eps[i].DurationMs = durs[i%len(durs)].Milliseconds()
	}
	entries := []schedule.LineupEntry{entry("series:tvdb:100", "Show")}
	avail := newSeriesAvail(map[string][]schedule.ResolvedProgram{"series:tvdb:100": eps})
	policy := schedule.ChannelPolicy{Ordering: ordering}
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	w := window.Milliseconds()

	history := map[string]time.Time{}
	var out []historyAiring
	for i := range windows {
		opened := base.Add(time.Duration(i) * window)
		ch := schedule.Channel{ID: "c", Name: "c", Number: 1, Strategy: schedule.Shuffle, DefaultWindow: window,
			DealOrigin: base, WindowOpened: opened}
		if withHistory {
			ch.LastAired = map[string]time.Time{}
			for id, at := range history {
				ch.LastAired[id] = at
			}
		}
		var programs []schedule.Slot
		var total int64
		for _, s := range schedule.ComputeDesiredAt(ch, entries, avail, schedule.PodFill, policy, opened.Add(time.Minute)).Slots {
			if s.IsProgram() {
				programs = append(programs, s)
				total += s.DurationMs
			}
		}
		for into := int64(0); into < w && total > 0; {
			for _, s := range programs {
				if into >= w {
					break
				}
				at := int64(i)*w + into
				out = append(out, historyAiring{s.LibraryItemID, at})
				history[s.LibraryItemID] = base.Add(time.Duration(at) * time.Millisecond)
				into += s.DurationMs
			}
		}
	}
	return out
}

// airingStats is what a viewer notices: the same episode twice in a row, and the shortest time
// before any episode came back (overall, and across a window seam).
type airingStats struct {
	backToBack      int
	minGap, minSeam time.Duration
}

func statsOf(aired []historyAiring, window time.Duration) airingStats {
	st := airingStats{minGap: 1 << 62, minSeam: 1 << 62}
	w := window.Milliseconds()
	last := map[string]int64{}
	for k, a := range aired {
		if k > 0 && aired[k-1].id == a.id {
			st.backToBack++
		}
		if prev, ok := last[a.id]; ok {
			gap := time.Duration(a.at-prev) * time.Millisecond
			st.minGap = min(st.minGap, gap)
			if prev/w != a.at/w {
				st.minSeam = min(st.minSeam, gap)
			}
		}
		last[a.id] = a.at
	}
	return st
}

// The repeat spacing holds whether or not anyone watches: with the history fed back, no unit airs
// twice in a row and none comes back sooner than it does on the same channel with no history. Two
// regressions this guards (#1939): a pool a little wider than its window traded yesterday's last
// films into this morning (27 one-hour episodes in a 24h window came back after 4h, not 25h), and a
// deck that fits one window, re-dealt with today's history, repaired its seam against a deck that
// never aired and aired a unit back to back.
func TestHistory_NoUnitReturnsSoonerThanWithoutIt(t *testing.T) {
	for _, tc := range []struct {
		n      int
		dur    time.Duration
		window time.Duration
	}{
		// Decks that fit one window: each window loops a fresh deal.
		{5, time.Hour, 6 * time.Hour},
		{5, time.Hour, 12 * time.Hour},
		{5, time.Hour, 24 * time.Hour},
		{10, 65 * time.Minute, 24 * time.Hour},
		{15, 45 * time.Minute, 24 * time.Hour},
		// Pools wider than a window: the window tiles a ring of deals.
		{10, time.Hour, 6 * time.Hour},
		{15, time.Hour, 12 * time.Hour},
		{27, time.Hour, 24 * time.Hour},
		{40, time.Hour, 24 * time.Hour},
	} {
		for _, mode := range shuffled {
			t.Run(fmt.Sprintf("%dx%v/%v/%s", tc.n, tc.dur, tc.window, mode), func(t *testing.T) {
				durs := []time.Duration{tc.dur}
				windows := int(60 * 24 * time.Hour / tc.window)
				unwatched := statsOf(airWithHistory(tc.n, durs, tc.window, mode, windows, false), tc.window)
				watched := statsOf(airWithHistory(tc.n, durs, tc.window, mode, windows, true), tc.window)
				if watched.backToBack > 0 {
					t.Fatalf("with history, a unit aired twice in a row %d times", watched.backToBack)
				}
				if watched.minGap < unwatched.minGap {
					t.Fatalf("with history a unit came back after %v; with none, never sooner than %v", watched.minGap, unwatched.minGap)
				}
			})
		}
	}
}

// TestHistorySweep prints the #1939 sweep: pools 5-400, equal and 45-65 min units, 6/12/24h windows,
// shuffle and syndication, with and without history. SWEEP=1 runs it.
func TestHistorySweep(t *testing.T) {
	if os.Getenv("SWEEP") == "" {
		t.Skip("SWEEP=1 prints the sweep")
	}
	mixed := []time.Duration{45 * time.Minute, 65 * time.Minute, 52 * time.Minute, 58 * time.Minute, 49 * time.Minute}
	for _, n := range []int{5, 10, 15, 27, 40, 120, 400} {
		for _, durs := range [][]time.Duration{{time.Hour}, mixed} {
			for _, wh := range []int{6, 12, 24} {
				window := time.Duration(wh) * time.Hour
				for _, mode := range shuffled {
					var row string
					for _, hist := range []bool{false, true} {
						st := statsOf(airWithHistory(n, durs, window, mode, 60*24/wh, hist), window)
						row += fmt.Sprintf("  hist=%-5v b2b=%-3d minGap=%-6.1f minSeam=%-6.1f", hist, st.backToBack, st.minGap.Hours(), st.minSeam.Hours())
					}
					fmt.Printf("SW n=%-3d mixed=%-5v w=%-2dh %-11s%s\n", n, len(durs) > 1, wh, mode, row)
				}
			}
		}
	}
}
