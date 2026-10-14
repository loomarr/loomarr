package schedule_test

import (
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/schedule"
)

// Shuffle seed + per-pass reshuffle (#1676). A channel's deck must (a) differ from another
// channel's with the same lineup, (b) be dealt afresh each time the rolling window wraps the
// catalog, and (c) not repeat a unit across that seam within the episode repeat spacing.

const passEpisodeMin = 30 * time.Minute

// airedWindows concatenates the program ids of windows [from, to) of a 6h-window channel. The
// windows tile the deck, so the concatenation is the airing sequence.
func airedWindows(t *testing.T, ch schedule.Channel, ordering schedule.OrderingMode, eps, from, to int) []string {
	t.Helper()
	entries := []schedule.LineupEntry{entry("series:tvdb:90000100", "Show")}
	avail := newSeriesAvail(map[string][]schedule.ResolvedProgram{"series:tvdb:90000100": manyEpisodes(eps)})
	policy := schedule.ChannelPolicy{Ordering: ordering}
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	var out []string
	for w := from; w < to; w++ {
		at := base.Add(time.Duration(w)*6*time.Hour + time.Hour)
		d := schedule.ComputeDesiredAt(ch, entries, avail, schedule.PodFill, policy, at)
		for _, s := range d.Slots {
			if s.IsProgram() {
				out = append(out, s.LibraryItemID)
			}
		}
	}
	return out
}

// passChannel is a channel born at the first window airedWindows reads, so that window opens pass 0
// of its deck: passes are counted from the channel's origin, and the tests below read whole passes.
func passChannel(id string) schedule.Channel {
	return schedule.Channel{ID: id, Name: id, Number: 1, Strategy: schedule.Shuffle, DefaultWindow: 6 * time.Hour,
		DealOrigin: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
}

var shuffled = []schedule.OrderingMode{schedule.OrderShuffle, schedule.OrderSyndication}

func TestShuffle_ChannelsWithIdenticalLineupsDiffer(t *testing.T) {
	for _, ord := range shuffled {
		a := airedWindows(t, passChannel("channel-a"), ord, 48, 0, 4)
		b := airedWindows(t, passChannel("channel-b"), ord, 48, 0, 4)
		if slices.Equal(a, b) {
			t.Fatalf("%s: two channels with the same lineup aired the same order (shared seed)", ord)
		}
	}
}

func TestShuffle_EachPassIsAFreshPermutation(t *testing.T) {
	for _, eps := range []int{48, 50} { // 50 does not divide by the 12-episode window: a window straddles the seam
		for _, ord := range shuffled {
			seq := airedWindows(t, passChannel("c"), ord, eps, 0, 3*eps/12+1)
			p1, p2 := seq[:eps], seq[eps:2*eps]
			if slices.Equal(p1, p2) {
				t.Fatalf("%s/%d: pass 2 replayed pass 1's order", ord, eps)
			}
			a, b := slices.Clone(p1), slices.Clone(p2)
			slices.Sort(a)
			slices.Sort(b)
			if !slices.Equal(a, b) || len(slices.Compact(a)) != eps {
				t.Fatalf("%s/%d: a pass must air every episode exactly once", ord, eps)
			}
		}
	}
}

func TestShuffle_NoRepeatInsideSpacingAcrossTheSeam(t *testing.T) {
	// 7d default spacing is far beyond a 24h pool, so it caps at a third of the pool: 16 episodes.
	const spacing = 16
	for _, eps := range []int{48, 50} {
		for _, ord := range shuffled {
			seq := airedWindows(t, passChannel("c"), ord, eps, 0, 6*eps/12)
			last := map[string]int{}
			for i, id := range seq {
				if j, ok := last[id]; ok && i-j < spacing {
					t.Fatalf("%s/%d: %s aired at %d and again at %d, inside the %d-episode spacing", ord, eps, id, j, i, spacing)
				}
				last[id] = i
			}
		}
	}
}

// A catalog that fits one window has no ring to wrap, so it used to air in the same order in every
// window (a 4h pool in 6h windows: the same four hours, every window). There the window is the pass.
func TestShuffle_ACatalogThatFitsOneWindowIsDealtAnewEachWindow(t *testing.T) {
	const eps = 8 // 4h of the 6h window
	for _, ord := range shuffled {
		seq := airedWindows(t, passChannel("c"), ord, eps, 0, 6)
		windows := make([][]string, 0, 6)
		for w := 0; w < 6; w++ {
			windows = append(windows, seq[w*eps:(w+1)*eps])
		}
		for w := 1; w < len(windows); w++ {
			if slices.Equal(windows[w], windows[w-1]) {
				t.Fatalf("%s: window %d replayed window %d's order", ord, w, w-1)
			}
		}
		for w, ids := range windows {
			if sorted := slices.Sorted(slices.Values(ids)); len(slices.Compact(sorted)) != eps {
				t.Fatalf("%s: window %d does not air every episode exactly once: %v", ord, w, ids)
			}
		}
	}
}

// Viewing must not turn a fitting catalog back into a replay (recency ordered it oldest-aired-first,
// which is yesterday's order), and the window's order must not move while it airs (#1674): the
// history that arrives mid-window changes nothing, and the seam to the window before still rests.
func TestShuffle_AFittingCatalogIgnoresHistoryAndRestsAcrossWindows(t *testing.T) {
	const eps = 8        // 30-minute episodes: 4h. A third of the pool less two episodes of slack is 60m,
	const spacingEps = 2 // so the repeat spacing is 2 of them
	entries := []schedule.LineupEntry{entry("series:tvdb:90000100", "Show")}
	avail := newSeriesAvail(map[string][]schedule.ResolvedProgram{"series:tvdb:90000100": manyEpisodes(eps)})
	policy := schedule.ChannelPolicy{Ordering: schedule.OrderSyndication}
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	history := map[string]time.Time{}
	var prev []string
	for w := 0; w < 10; w++ {
		opened := base.Add(time.Duration(w) * 6 * time.Hour)
		airedAt := func(at time.Time, seen map[string]time.Time) []string {
			ch := passChannel("c")
			ch.WindowOpened, ch.LastAired = opened, seen
			var ids []string
			for _, s := range schedule.ComputeDesiredAt(ch, entries, avail, schedule.PodFill, policy, at).Slots {
				if s.IsProgram() {
					ids = append(ids, s.LibraryItemID)
				}
			}
			return ids
		}
		ids := airedAt(opened.Add(time.Hour), history)
		if len(ids) != eps {
			t.Fatalf("window %d airs %d episodes, want %d", w, len(ids), eps)
		}
		// Everything watched since the window opened arrives mid-window: the order must not move.
		midway := map[string]time.Time{}
		for id, at := range history {
			midway[id] = at
		}
		for i, id := range ids {
			midway[id] = opened.Add(time.Duration(i) * passEpisodeMin)
		}
		if again := airedAt(opened.Add(3*time.Hour), midway); !slices.Equal(ids, again) {
			t.Fatalf("window %d re-ordered itself when viewing was recorded: %v then %v", w, ids, again)
		}
		if prev != nil {
			if slices.Equal(ids, prev) {
				t.Fatalf("window %d replays window %d's order", w, w-1)
			}
			for y, id := range ids {
				if x := slices.Index(prev, id); eps-x+y < spacingEps {
					t.Fatalf("window %d: %s was %d from the end of the window before and is %d from the start of this one: under %d episodes apart",
						w, id, eps-x, y, spacingEps)
				}
			}
		}
		for id, at := range midway { // the next window opens with this one's viewing on record
			history[id] = at
		}
		prev = ids
	}
}

func TestSequential_AFittingCatalogKeepsItsOrderEachWindow(t *testing.T) {
	seq := airedWindows(t, passChannel("c"), schedule.OrderSequential, 8, 0, 4)
	for i, id := range seq {
		if want := "ep-" + itoa(i%8); id != want {
			t.Fatalf("position %d aired %s, want %s", i, id, want)
		}
	}
}

func TestShuffle_DeterministicForChannelAndTime(t *testing.T) {
	for _, ord := range shuffled {
		a := airedWindows(t, passChannel("c"), ord, 50, 3, 9)
		b := airedWindows(t, passChannel("c"), ord, 50, 3, 9)
		if !slices.Equal(a, b) {
			t.Fatalf("%s: same (channel, at) produced different orders", ord)
		}
	}
}

func TestSequential_StaysInOrderAcrossPasses(t *testing.T) {
	seq := airedWindows(t, passChannel("c"), schedule.OrderSequential, 48, 0, 8)
	for i, id := range seq {
		if want := "ep-" + itoa(i%48); id != want {
			t.Fatalf("position %d aired %s, want %s", i, id, want)
		}
	}
}
