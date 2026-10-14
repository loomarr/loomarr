package schedule

import (
	"fmt"
	"math/rand"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
)

// Internals of the per-pass deal (#1676): the seam repair, the chain walk and its bound, and what
// they cost.

// dealPool is n episodes of one series, durations varying around dur so the repair meets
// uneven runtimes the way a real library has.
func dealPool(n int, dur time.Duration, seed int64) []Slot {
	rng := rand.New(rand.NewSource(seed))
	slots := make([]Slot, n)
	for i := range slots {
		d := dur + time.Duration(rng.Intn(int(dur/3)))*time.Millisecond/time.Millisecond - dur/6
		slots[i] = Slot{
			Kind: SlotProgram, Key: provision.Key("series:tvdb:100"), SeriesTitle: "Show", Season: 1, Episode: i + 1,
			LibraryItemID: fmt.Sprintf("ep-%d", i), Title: fmt.Sprintf("Ep %d", i), DurationMs: d.Milliseconds(),
		}
	}
	return slots
}

// shuffleDealer resolves the policy the way the engine does: a pool of one show relaxes the
// series-spacing rules (§2), a pool of several does not, and the latter is what makes a deal costly.
func shuffleDealer(slots []Slot, window time.Duration) *deckDealer {
	shows := map[provision.Key]bool{}
	for _, s := range slots {
		shows[s.Key] = true
	}
	rp := ChannelPolicy{ProposalPolicy: ProposalPolicy{Ordering: "shuffle"}}.Resolved(Shuffle, len(shows) == 1)
	return newDeckDealer(slots, rp, 7, window)
}

// resetDealChains forgets every memoised chain, so the next walk is cold.
func resetDealChains() {
	chainsMu.Lock()
	defer chainsMu.Unlock()
	chains, chainLRU = map[uint64]*dealChain{}, nil
}

func startsOf(deck []Slot) map[string]int64 {
	out := map[string]int64{}
	var at int64
	for _, s := range deck {
		if s.IsProgram() {
			out[s.LibraryItemID] = at
			at += s.DurationMs
		}
	}
	return out
}

// Across every seam, a unit airs at least `spacing` after it last aired, measured from where it
// started in the pass before to where it starts in this one. The one exception is the first pass
// after an anchor of a chained walk (see chainBudget), which is repaired against its predecessor as
// dealt: it is skipped here, and TestDealChain_AnchorSeamIsTheOnlyRelaxation says why it is the only one.
func TestRepairSeam_EveryUnitRestsTheSpacing(t *testing.T) {
	for _, tc := range []struct {
		name   string
		slots  []Slot
		window time.Duration
	}{
		{"three windows a pass", dealPool(36, 2*time.Hour, 3), 24 * time.Hour},
		{"three and a third", dealPool(40, 2*time.Hour, 3), 24 * time.Hour},
		{"many windows a pass", dealPool(300, 30*time.Minute, 3), 24 * time.Hour},
		{"small pool", dealPool(14, 90*time.Minute, 3), 6 * time.Hour},
		{"several shows", multiPool(6, 26, 11*time.Minute), 24 * time.Hour},
		{"several shows, chained", multiPool(3, 20, 44*time.Minute), 24 * time.Hour},
		// A pool that fits one window loops inside it: TestFittingDeck_LoopedSeamRestsTheSpacing.
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetDealChains()
			d := shuffleDealer(tc.slots, tc.window)
			spacing := d.spacing.Milliseconds()
			total := programRuntime(d.slots)
			prev := d.deck(1)
			for p := int64(2); p < 70; p++ {
				cur := d.deck(p)
				before, after := startsOf(prev), startsOf(cur)
				prev = cur
				if len(after) != len(tc.slots) {
					t.Fatalf("pass %d holds %d units, want %d", p, len(after), len(tc.slots))
				}
				if !d.stable && p%d.bound == 0 {
					continue
				}
				for id, s := range after {
					if gap := total - before[id] + s; gap < spacing {
						t.Fatalf("pass %d: %s restarts %dms after it started, spacing is %dms", p, id, gap, spacing)
					}
				}
			}
		})
	}
}

// Only an anchor's seam may be short: the passes after it are repaired against the real deck.
func TestDealChain_AnchorSeamIsTheOnlyRelaxation(t *testing.T) {
	resetDealChains()
	d := shuffleDealer(dealPool(36, 2*time.Hour, 3), 24*time.Hour)
	if d.stable || d.bound >= 200 {
		t.Fatalf("fixture should be chained with a short bound, got stable=%v bound=%d", d.stable, d.bound)
	}
	spacing, total := d.spacing.Milliseconds(), programRuntime(d.slots)
	short, anchors := 0, 0
	for p := int64(1); p < 6*d.bound; p++ {
		if p%d.bound == 0 {
			anchors++
		}
		before, after := startsOf(d.deck(p-1)), startsOf(d.deck(p))
		for id, s := range after {
			if total-before[id]+s < spacing {
				if p%d.bound != 0 {
					t.Fatalf("pass %d (not an anchor, bound %d) came up short for %s", p, d.bound, id)
				}
				short++
				break
			}
		}
	}
	t.Logf("%d of %d anchor seams came up short (bound %d)", short, anchors, d.bound)
}

// A pass differs from the one before it, and the same channel at the same time deals the same.
func TestDealChain_PassesDifferAndRepeatExactly(t *testing.T) {
	resetDealChains()
	slots := dealPool(60, 30*time.Minute, 5)
	a := shuffleDealer(slots, 6*time.Hour)
	var first [][]string
	for p := int64(1); p < 12; p++ {
		first = append(first, idsOf(a.deck(p)))
		if p > 1 && slices.Equal(first[p-1], first[p-2]) {
			t.Fatalf("pass %d replays pass %d", p, p-1)
		}
	}
	resetDealChains()
	b := shuffleDealer(slots, 6*time.Hour)
	for p := int64(11); p >= 1; p-- { // a cold walk, taken backwards
		if got := idsOf(b.deck(p)); !slices.Equal(got, first[p-1]) {
			t.Fatalf("pass %d dealt differently on a cold walk", p)
		}
	}
}

// The memo only saves work: asking in any order, with the chain warm or cold, gives one answer.
func TestDealChain_MemoAgreesWithColdWalk(t *testing.T) {
	slots := dealPool(45, 40*time.Minute, 11)
	order := rand.New(rand.NewSource(2)).Perm(300)
	resetDealChains()
	warm := shuffleDealer(slots, 6*time.Hour)
	got := map[int64][]string{}
	for _, p := range order {
		got[int64(p)+1] = idsOf(warm.deck(int64(p) + 1))
	}
	for _, p := range []int64{1, 63, 64, 65, 128, 299, 300} {
		resetDealChains()
		if cold := idsOf(shuffleDealer(slots, 6*time.Hour).deck(p)); !slices.Equal(cold, got[p]) {
			t.Fatalf("pass %d: warm and cold walks disagree", p)
		}
	}
}

// A chained walk starts at the latest multiple of the pool's bound, so a channel older than that
// pays no more than that many deals, and a pass in a later segment never depends on how far the
// chain was walked before it.
func TestDealChain_WalkStartsAtTheAnchor(t *testing.T) {
	resetDealChains()
	d := shuffleDealer(dealPool(30, time.Hour, 1), 6*time.Hour) // 30h in 6h windows: five windows, chained
	if d.stable {
		t.Fatal("fixture should be in the chained regime")
	}
	far := 2*d.bound + 5
	d.deck(far)
	for p := range d.chain.decks {
		if p < 2*d.bound {
			t.Fatalf("pass %d was walked, below the anchor %d", p, 2*d.bound)
		}
	}
	resetDealChains()
	d2 := shuffleDealer(dealPool(30, time.Hour, 1), 6*time.Hour)
	d2.deck(d2.bound - 1)
	if !slices.Equal(idsOf(d2.deck(far)), idsOf(d.deck(far))) {
		t.Fatal("a pass past the bound depends on how far the chain was walked before it")
	}
}

func TestChainBound_ScalesDownWithThePool(t *testing.T) {
	for n, want := range map[int]int64{1: 2048, 27: 64, 156: 16, 500: 16, 100000: 16} {
		if got := chainBound(n); got != want {
			t.Errorf("chainBound(%d) = %d, want %d", n, got, want)
		}
		if got := chainBound(n); got%checkpointEvery != 0 {
			t.Errorf("chainBound(%d) = %d, not a multiple of %d", n, got, checkpointEvery)
		}
	}
}

// The tail-stable shortcut (two deals, any age) must give exactly the deck a full walk from pass 0
// gives: that equality is the claim the regime rests on, so it is checked, not assumed.
func TestDealChain_TailStableShortcutEqualsTheFullWalk(t *testing.T) {
	for _, tc := range []struct {
		name   string
		slots  []Slot
		window time.Duration
	}{
		{"kids 6x26 in 3h windows", multiPool(6, 26, 11*time.Minute), 3 * time.Hour},
		{"sitcom 5x100 in 24h windows", multiPool(5, 100, 22*time.Minute), 24 * time.Hour},
		{"a long pool, short windows", dealPool(120, 40*time.Minute, 9), 3 * time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetDealChains()
			fast := shuffleDealer(tc.slots, tc.window)
			if !fast.stable {
				t.Fatal("fixture should be tail-stable")
			}
			ref := shuffleDealer(tc.slots, tc.window)
			ref.stable, ref.bound, ref.chain = false, 1<<40, newDealChain() // the full walk, from pass 0
			for p := int64(1); p <= 24; p++ {
				if got, want := idsOf(fast.deck(p)), idsOf(ref.deck(p)); !slices.Equal(got, want) {
					t.Fatalf("pass %d: the shortcut dealt a different deck than the full walk", p)
				}
			}
		})
	}
}

// multiPool is `series` shows of `eps` episodes each, so the deal has separation to satisfy.
func multiPool(series, eps int, dur time.Duration) []Slot {
	var out []Slot
	for s := 0; s < series; s++ {
		for e := 0; e < eps; e++ {
			out = append(out, Slot{
				Kind: SlotProgram, Key: provision.Key(fmt.Sprintf("series:tvdb:%d", s)), SeriesTitle: fmt.Sprintf("Show %d", s),
				Season: 1, Episode: e + 1, LibraryItemID: fmt.Sprintf("s%d-e%d", s, e), Title: "t", DurationMs: dur.Milliseconds(),
			})
		}
	}
	return out
}

// Units already aired as of `since` are the previous window's, including the one that opened it.
func TestTradeRecent_CountsTheFilmThatOpenedYesterday(t *testing.T) {
	since := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	hour := int64(time.Hour / time.Millisecond)
	deck := []Slot{recSlot("1", "Opened yesterday", hour), recSlot("2", "Other", hour), recSlot("3", "Waiting", hour)}
	history := map[string]time.Time{
		"lib-1": since,                          // yesterday's first film aired at its window's opening
		"lib-3": since.Add(-3 * 24 * time.Hour), // and this one days ago
	}
	got := tradeRecent(deck, deck[:2], history, history, since, since.Add(24*time.Hour), time.Hour, 2*time.Hour)
	if titles := titlesOf(got); slices.Contains(titles, "Opened yesterday") {
		t.Fatalf("today's slice %v re-airs the film that opened yesterday", titles)
	}
}

func idsOf(deck []Slot) []string {
	out := make([]string, 0, len(deck))
	for _, s := range deck {
		out = append(out, s.LibraryItemID)
	}
	return out
}

// BenchmarkDealChain prices dealing the pass a channel of the given age is on: cold (a fresh
// process, or a lineup edit) and warm (every later call). The pools are the shapes the curation
// simulation uses, several series each, because what a deal costs depends on the separation it has
// to satisfy: a single-series pool deals in microseconds and would hide the cost. "chained" is the
// slowest regime (the two-window floor beats the tail-stable cap); the 6-year row is the worst case
// the bound allows.
func BenchmarkDealChain(b *testing.B) {
	for _, tc := range []struct {
		name   string
		slots  []Slot
		window time.Duration
		age    time.Duration
	}{
		{"1y/fits-one-window-6x6", multiPool(6, 6, 11*time.Minute), 24 * time.Hour, 365 * 24 * time.Hour},
		{"1y/kids-6x26", multiPool(6, 26, 11*time.Minute), 24 * time.Hour, 365 * 24 * time.Hour},
		{"1y/sitcom-5x100", multiPool(5, 100, 22*time.Minute), 24 * time.Hour, 365 * 24 * time.Hour},
		{"1y/chained-3x20-eps", multiPool(3, 20, 44*time.Minute), 24 * time.Hour, 365 * 24 * time.Hour},
		{"6y/chained-3x20-eps", multiPool(3, 20, 44*time.Minute), 24 * time.Hour, 6 * 365 * 24 * time.Hour},
		{"1y/chained-27-films", multiPool(27, 1, 110*time.Minute), 24 * time.Hour, 365 * 24 * time.Hour},
		{"6y/chained-27-films", multiPool(27, 1, 110*time.Minute), 24 * time.Hour, 6 * 365 * 24 * time.Hour},
		{"6y/chained-60-films-6h", multiPool(60, 1, 110*time.Minute), 6 * time.Hour, 6 * 365 * 24 * time.Hour},
	} {
		slots := tc.slots
		n := len(slots)
		ring := int64(tc.age / tc.window)
		stride := int64(max(programsPerWindow(slots, tc.window.Milliseconds()), 1))
		pass := ring * stride / int64(n)
		if programRuntime(slots) <= tc.window.Milliseconds() {
			pass = ring // a fitting pool: the window is the pass
		}
		b.Run(tc.name+"/cold", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				resetDealChains()
				shuffleDealer(slots, tc.window).deck(pass)
			}
		})
		b.Run(tc.name+"/warm", func(b *testing.B) {
			resetDealChains()
			shuffleDealer(slots, tc.window).deck(pass)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				shuffleDealer(slots, tc.window).deck(pass)
			}
		})
	}
}

// airing is one unit as playout airs it: the channel's window w opens at w*window, and what the
// window carries is looped (`elapsed % total`, as internal/playout/timeline.go does) until it closes.
type airing struct {
	id string
	at int64 // ms from the first window's open
}

// airLooped is windows [0, windows) as aired. A slot that starts at or after its window's close
// never airs: the next window's deck takes over.
func airLooped(d *deckDealer, window time.Duration, windows int64) []airing {
	var out []airing
	w := window.Milliseconds()
	for i := range windows {
		slice, _ := windowSliceDealt(d, window, i)
		var total int64
		for _, s := range slice {
			if s.IsProgram() {
				total += s.DurationMs
			}
		}
		for into := int64(0); into < w; {
			for _, s := range slice {
				if !s.IsProgram() || into >= w {
					continue
				}
				out = append(out, airing{s.LibraryItemID, i*w + into})
				into += s.DurationMs
			}
			if total == 0 {
				break
			}
		}
	}
	return out
}

func equalPool(n int, dur time.Duration) []Slot {
	slots := dealPool(n, dur, 1)
	for i := range slots {
		slots[i].DurationMs = dur.Milliseconds()
	}
	return slots
}

// A deck that fits one window loops inside it, so the unit that closes a window is wherever the
// loop stopped, not the deck's last. Across the seam no unit airs twice in a row, and none returns
// inside the repeat spacing.
func TestFittingDeck_LoopedSeamRestsTheSpacing(t *testing.T) {
	for _, tc := range []struct {
		name   string
		pool   []Slot
		window time.Duration
	}{
		{"5x1h", equalPool(5, time.Hour), 12 * time.Hour},
		{"5x1h", equalPool(5, time.Hour), 24 * time.Hour},
		{"7x50m", equalPool(7, 50*time.Minute), 24 * time.Hour},
		{"10x65m", equalPool(10, 65*time.Minute), 12 * time.Hour},
		{"10x65m", equalPool(10, 65*time.Minute), 24 * time.Hour},
		{"15x45m", equalPool(15, 45*time.Minute), 12 * time.Hour},
		{"15x45m", equalPool(15, 45*time.Minute), 24 * time.Hour},
		{"6 series x6x11m", multiPool(6, 6, 11*time.Minute), 24 * time.Hour},
	} {
		t.Run(fmt.Sprintf("%s/%v", tc.name, tc.window), func(t *testing.T) {
			resetDealChains()
			d := shuffleDealer(tc.pool, tc.window)
			if !d.whole {
				t.Fatalf("pool should fit the window")
			}
			aired := airLooped(d, tc.window, 60)
			last := map[string]int64{}
			for k, a := range aired {
				if k > 0 && aired[k-1].id == a.id {
					t.Fatalf("%s aired twice in a row at %v", a.id, time.Duration(a.at)*time.Millisecond)
				}
				if prev, ok := last[a.id]; ok && a.at/tc.window.Milliseconds() != prev/tc.window.Milliseconds() {
					if gap := a.at - prev; gap < d.spacing.Milliseconds() {
						t.Fatalf("%s returned %v after it aired, across the seam into window %d: under the %v spacing",
							a.id, time.Duration(gap)*time.Millisecond, a.at/tc.window.Milliseconds(), d.spacing)
					}
				}
				last[a.id] = a.at
			}
		})
	}
}

// A pool a little longer than a window (27 one-hour films, a 24h window) used to re-air a unit
// hours later: a window that skipped a duplicate read past its stride, and the next window started
// at its stride and aired the same units again. No unit may return inside the repeat spacing, and
// the spacing keeps a unit out of the window it already aired in.
func TestRingWindow_NoUnitReturnsInsideTheSpacing(t *testing.T) {
	resetDealChains()
	window := 24 * time.Hour
	d := shuffleDealer(equalPool(27, time.Hour), window)
	if d.spacing < window {
		t.Fatalf("spacing %v is shorter than the window: a unit could air twice in one", d.spacing)
	}
	aired := airLooped(d, window, 60) // 60 windows: 53 passes, inside one chained segment
	last := map[string]int64{}
	for _, a := range aired {
		if prev, ok := last[a.id]; ok {
			if gap := a.at - prev; gap < d.spacing.Milliseconds() {
				t.Fatalf("%s returned %v after it aired (window %d): under the %v spacing",
					a.id, time.Duration(gap)*time.Millisecond, a.at/window.Milliseconds(), d.spacing)
			}
		}
		last[a.id] = a.at
	}
}
