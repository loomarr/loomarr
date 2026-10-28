package schedule

import (
	"math/rand"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
)

// The season simulation of a syndicated multi-series channel took ~30s to preview in production
// (cycle preview of a four-series 1980s comedy channel, inside Halloween season). Two multipliers
// met: every deal of a pool the separation rules cannot satisfy burned the whole backtracking
// budget, and the chain walk re-dealt the same passes ~19 times each. These pin both.

// The simulation reads the pass before, the pass and the one after, window after window. Each pass
// must be dealt once, however the reads are ordered, and equal what a cold walk deals.
func TestDealChain_ReadingAroundOnePassDealsEachPassOnce(t *testing.T) {
	skipSerialUnderRace(t)
	resetDealChains()
	d := shuffleDealer(multiPool(3, 20, 44*time.Minute), 24*time.Hour)
	if d.stable || d.pinned || d.whole {
		t.Fatalf("pool is not in the chained regime (stable=%v pinned=%v whole=%v)", d.stable, d.pinned, d.whole)
	}
	const first, last = int64(1000), int64(1040)
	got := map[int64][]Slot{}
	for p := first; p <= last; p++ {
		for _, q := range []int64{p - 1, p, p + 1} {
			got[q] = d.deck(q)
		}
	}
	// Each segment is walked once from its anchor, and each anchor after the first deals its
	// warm-up passes first (warmUpPasses).
	anchor := (first - 1) - (first-1)%d.bound
	segments := (last+1)/d.bound - anchor/d.bound + 1
	budget := (last + 1 - anchor + 1) + segments*(d.warmUp+1)
	if int64(d.deals) > budget {
		t.Fatalf("dealt %d times, want at most %d (%d segments from anchor %d, warm-up %d): passes were re-dealt", d.deals, budget, segments, anchor, d.warmUp)
	}
	for _, p := range []int64{first - 1, first + 7, last, last + 1} {
		resetDealChains()
		fresh := shuffleDealer(multiPool(3, 20, 44*time.Minute), 24*time.Hour)
		if !slices.Equal(fresh.deck(p), got[p]) {
			t.Fatalf("pass %d differs from a cold walk's", p)
		}
	}
}

func sepPolicy(blockMax int, gap time.Duration) ResolvedPolicy {
	var rp ResolvedPolicy
	rp.Sep.BlockMax, rp.Sep.SeriesMinGap = blockMax, gap
	return rp
}

func TestSeparationUnsatisfiable(t *testing.T) {
	const ep = 22 * time.Minute
	for _, tc := range []struct {
		name string
		pool []Slot
		rp   ResolvedPolicy
		want bool
	}{
		{"four shows under a 2h gap", multiPool(4, 30, ep), sepPolicy(2, 2*time.Hour), true},
		{"four shows under a 1h gap fit", multiPool(4, 30, ep), sepPolicy(2, time.Hour), false},
		{"eight shows under a 2h gap fit", multiPool(8, 30, ep), sepPolicy(2, 2*time.Hour), false},
		{"one show cannot avoid runs", multiPool(1, 5, ep), sepPolicy(2, 0), true},
		{"blockMax 2 over a majority show", append(multiPool(1, 7, ep), multiPool(2, 1, ep)[1:]...), sepPolicy(2, 0), true},
		{"no rules", multiPool(4, 30, ep), sepPolicy(0, 0), false},
	} {
		if got := separationUnsatisfiable(tc.pool, tc.rp); got != tc.want {
			t.Errorf("%s: separationUnsatisfiable = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// Whenever the count says no order exists, the search must agree: skipping it is only the same
// answer if it could never have found one. Small pools, so the search is exhaustive.
func TestSeparationUnsatisfiable_NeverRulesOutASolvablePool(t *testing.T) {
	rng := rand.New(rand.NewSource(2027))
	var ruledOut int
	for range 4000 {
		n := 2 + rng.Intn(6)
		shows := 1 + rng.Intn(4)
		pool := make([]Slot, n)
		for i := range pool {
			pool[i] = Slot{
				Kind: SlotProgram, Key: provision.Key("series:tvdb:" + string(rune('a'+rng.Intn(shows)))),
				LibraryItemID: string(rune('A' + i)), DurationMs: int64(10+rng.Intn(50)) * 60_000,
			}
		}
		rp := sepPolicy(rng.Intn(4), time.Duration(rng.Intn(4))*45*time.Minute)
		if !separationUnsatisfiable(pool, rp) {
			continue
		}
		ruledOut++
		if _, ok := backtrackArrange(pool, rp); ok {
			t.Fatalf("ruled out a pool the search solves: %+v under %+v", pool, rp.Sep)
		}
	}
	if ruledOut == 0 {
		t.Fatal("the generator never produced an unsatisfiable pool; the test proves nothing")
	}
}

// A pool the rules cannot satisfy must reach the greedy order without spending the search budget.
// The budget is mostly rejected attempts, which allocate nothing, so it is measured as time, but
// against the greedy pass over the same pool rather than a fixed bound: skipping the search makes
// the repair cost the greedy pass (ratio ~1), spending it ~4x; the bound is 2.5x, whatever the machine.
func TestSeparationRepair_UnsatisfiablePoolSkipsTheSearch(t *testing.T) {
	skipSerialUnderRace(t)
	pool := multiPool(4, 60, 22*time.Minute)
	rp := sepPolicy(2, 2*time.Hour)
	if !slices.Equal(separationRepair(pool, rp), greedyArrange(pool, rp)) {
		t.Fatal("an unsatisfiable pool must come out in the greedy order")
	}
	best := func(f func()) time.Duration {
		fastest := time.Hour
		for range 15 {
			start := time.Now()
			f()
			fastest = min(fastest, time.Since(start))
		}
		return fastest
	}
	repair := best(func() { separationRepair(pool, rp) })
	greedy := best(func() { greedyArrange(pool, rp) })
	if repair > 5*greedy/2 {
		t.Fatalf("repair took %v against %v for the greedy pass: the search ran its budget (%d attempts) on a pool it cannot solve", repair, greedy, backtrackBudget)
	}
}
