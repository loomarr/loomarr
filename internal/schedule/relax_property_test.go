package schedule

// Property tests for the relaxation ladder (programming-design §7). These are
// seeded, in-package plain-Go loops (math/rand with a fixed source) — NOT a
// framework — so any failure reproduces from the printed trial index / seed.
//
// They assert the SAFETY-CRITICAL invariants the ladder must hold for ANY policy:
//
//  1. TERMINATION + PROGRESS: relaxAndSlot's ladder is bounded (never loops), and
//     every applied ladderStep strictly progresses the policy (a monotone progress
//     metric strictly decreases at each accepted step).
//  2. NEVER-RELAXED (the most important property, §7): no ladder step ever mutates
//     Audience.Ceiling or explicit Scope (Series/Seasons). Feeding random policies
//     through relaxAndSlot / ladderStep leaves those fields identical.
//  3. halveFloor never drops below minNoRepeatFloor (24h) and is monotone
//     non-increasing.
//  4. widenEra never crosses a stated decade boundary and is idempotent at the edges.

import (
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
)

// ladderProgress is a monotone-decreasing scalar over the fields the ladder is
// allowed to relax (no-repeat windows, series gap/block, era width). Each accepted
// ladderStep must strictly decrease it — that's the "strictly progresses" guarantee
// that, together with the bound, proves termination. It mirrors the ladder's own
// order (windows, then gap/block, then era) but only needs to be *some* strictly
// decreasing measure, so we sum normalized contributions.
func ladderProgress(rp ResolvedPolicy) int64 {
	var p int64
	// No-repeat windows contribute their remaining slack above the floor (ms).
	if rp.Sep.EpisodeNoRepeat > minNoRepeatFloor {
		p += int64(rp.Sep.EpisodeNoRepeat-minNoRepeatFloor) / int64(time.Millisecond)
	}
	if rp.Sep.MovieNoRepeat > minNoRepeatFloor {
		p += int64(rp.Sep.MovieNoRepeat-minNoRepeatFloor) / int64(time.Millisecond)
	}
	// Gap / block relax toward 0.
	p += int64(rp.Sep.SeriesMinGap) / int64(time.Millisecond)
	p += int64(rp.Sep.BlockMax)
	// Era widens toward its decade edges; remaining widenable years shrink each step.
	if rp.Scope.Era != nil {
		e := *rp.Scope.Era
		if e.From > 0 {
			p += int64(e.From - (e.From/10)*10) // years above decade start
		}
		if e.To > 0 {
			p += int64(((e.To/10)*10 + 9) - e.To) // years below decade end
		}
	}
	return p
}

// randResolvedPolicy builds a random ResolvedPolicy exercising every ladder-relevant
// field: long/short no-repeat windows, series gap/block on or off, and an era that is
// sometimes set (with random decade placement) and sometimes nil. Audience ceiling and
// explicit scope (series/seasons) are set to non-zero sentinels so a mutation would be
// caught by the never-relaxed assertions.
func randResolvedPolicy(g *rand.Rand) ResolvedPolicy {
	ceilings := []Rating{"", "TV-Y", "TV-Y7", "TV-G", "TV-PG", "TV-14", "TV-MA", "PG-13", "R"}
	rp := ResolvedPolicy{
		Ceiling:  ceilings[g.Intn(len(ceilings))],
		Ordering: OrderSyndication,
		Sep: ResolvedSeparation{
			// Windows anywhere from below the floor up to ~90 days.
			EpisodeNoRepeat: time.Duration(g.Int63n(int64(90*24*time.Hour) + 1)),
			MovieNoRepeat:   time.Duration(g.Int63n(int64(90*24*time.Hour) + 1)),
			SeriesMinGap:    time.Duration(g.Int63n(int64(6 * time.Hour))),
			BlockMax:        g.Intn(6), // 0..5
		},
	}
	// Explicit scope: series + season window that MUST survive untouched.
	if g.Intn(2) == 0 {
		rp.Scope.Series = []provision.Key{
			provision.Key("series:tvdb:" + string(rune('1'+g.Intn(5)))),
			provision.Key("series:tvdb:" + string(rune('1'+g.Intn(5)))),
		}
	}
	if g.Intn(2) == 0 {
		lo := 1 + g.Intn(5)
		rp.Scope.Seasons = &Range{From: lo, To: lo + g.Intn(4)}
	}
	// Era: sometimes nil, sometimes a random window (possibly mid-decade so widening
	// has room, possibly already at the edges).
	switch g.Intn(3) {
	case 0:
		// nil era
	case 1:
		from := 1950 + g.Intn(70)
		rp.Scope.Era = &Range{From: from, To: from + g.Intn(20)}
	case 2:
		// era with a zero bound on one side (unbounded end)
		if g.Intn(2) == 0 {
			rp.Scope.Era = &Range{From: 1960 + g.Intn(50), To: 0}
		} else {
			rp.Scope.Era = &Range{From: 0, To: 1960 + g.Intn(50)}
		}
	}
	return rp
}

// snapshotScope deep-copies the never-relaxed fields so later comparison isn't
// fooled by aliasing (Era/Seasons are pointers; Series is a slice).
func snapshotScope(rp ResolvedPolicy) (Rating, []provision.Key, *Range, *Range) {
	var series []provision.Key
	if rp.Scope.Series != nil {
		series = append([]provision.Key(nil), rp.Scope.Series...)
	}
	var seasons, era *Range
	if rp.Scope.Seasons != nil {
		s := *rp.Scope.Seasons
		seasons = &s
	}
	if rp.Scope.Era != nil {
		e := *rp.Scope.Era
		era = &e
	}
	return rp.Ceiling, series, seasons, era
}

// TestLadderStep_TerminatesAndProgresses drives ladderStep to exhaustion on random
// policies and asserts: it stops within maxLadderSteps (bounded, no infinite loop),
// and every ok=true step STRICTLY decreases the progress metric (real progress).
func TestLadderStep_TerminatesAndProgresses(t *testing.T) {
	g := rand.New(rand.NewSource(0xF00D))
	for trial := 0; trial < 2000; trial++ {
		rp := randResolvedPolicy(g)
		cur := rp
		prev := ladderProgress(cur)
		steps := 0
		// A generous hard cap: far above maxLadderSteps. If ladderStep never returns
		// ok=false within this many iterations it is not terminating.
		const hardCap = 10000
		for i := 0; i < hardCap; i++ {
			relaxed, _, ok := ladderStep(cur)
			if !ok {
				break
			}
			steps++
			p := ladderProgress(relaxed)
			if p >= prev {
				t.Fatalf("trial %d step %d: progress did not strictly decrease (%d -> %d) for %+v",
					trial, steps, prev, p, cur)
			}
			prev = p
			cur = relaxed
			if steps > hardCap-1 {
				t.Fatalf("trial %d: ladderStep did not terminate within %d iterations", trial, hardCap)
			}
		}
	}
}

// TestRelaxAndSlot_LadderBounded asserts the ladder driver itself is bounded: the
// number of recorded relaxations never exceeds maxLadderSteps, for any policy and a
// pool deliberately too small to satisfy separation (forcing full descent).
func TestRelaxAndSlot_LadderBounded(t *testing.T) {
	g := rand.New(rand.NewSource(0xBEEF))
	for trial := 0; trial < 1000; trial++ {
		rp := randResolvedPolicy(g)
		// A tiny single-series pool with a huge no-repeat window can never satisfy the
		// window, so the ladder descends as far as it structurally can.
		pool := []Slot{
			progSlot("series:tvdb:X", "x1", 60000),
			progSlot("series:tvdb:X", "x2", 60000),
		}
		_, applied := relaxAndSlot(pool, rp, int64(trial))
		if len(applied) > maxLadderSteps {
			t.Fatalf("trial %d: ladder applied %d steps, exceeds bound %d: %+v",
				trial, len(applied), maxLadderSteps, applied)
		}
	}
}

// TestLadderStep_NeverRelaxesAudienceOrScope is the most important safety property
// (§7 "never relaxed"): stepping the ladder to exhaustion NEVER mutates the audience
// ceiling or the explicit scope (Series / Seasons). We snapshot those fields before,
// descend the whole ladder, and assert byte-identical after every intermediate step.
func TestLadderStep_NeverRelaxesAudienceOrScope(t *testing.T) {
	g := rand.New(rand.NewSource(0xCAFE))
	for trial := 0; trial < 2000; trial++ {
		rp := randResolvedPolicy(g)
		wantCeil, wantSeries, wantSeasons, _ := snapshotScope(rp)

		cur := rp
		for i := 0; i < maxLadderSteps+2; i++ {
			relaxed, _, ok := ladderStep(cur)
			if !ok {
				break
			}
			gotCeil, gotSeries, gotSeasons, _ := snapshotScope(relaxed)
			if gotCeil != wantCeil {
				t.Fatalf("trial %d step %d: audience ceiling mutated %q -> %q", trial, i, wantCeil, gotCeil)
			}
			if !reflect.DeepEqual(gotSeries, wantSeries) {
				t.Fatalf("trial %d step %d: explicit series scope mutated %v -> %v", trial, i, wantSeries, gotSeries)
			}
			if !reflect.DeepEqual(gotSeasons, wantSeasons) {
				t.Fatalf("trial %d step %d: explicit season scope mutated %v -> %v", trial, i, wantSeasons, gotSeasons)
			}
			cur = relaxed
		}
	}
}

// TestRelaxAndSlot_NeverRelaxesAudienceOrScope drives the SAME never-relaxed
// guarantee through the public ladder driver relaxAndSlot on a shortfall pool: the
// caller's audience ceiling and explicit scope are byte-identical before and after,
// no matter how far the ladder descended.
func TestRelaxAndSlot_NeverRelaxesAudienceOrScope(t *testing.T) {
	g := rand.New(rand.NewSource(0x5EED))
	for trial := 0; trial < 1000; trial++ {
		rp := randResolvedPolicy(g)
		wantCeil, wantSeries, wantSeasons, wantEra := snapshotScope(rp)

		pool := []Slot{
			progSlot("series:tvdb:Z", "z1", 60000),
			progSlot("series:tvdb:Z", "z2", 60000),
		}
		relaxAndSlot(pool, rp, int64(trial))

		// relaxAndSlot takes rp by value, so the caller's struct is untouched. Assert
		// the never-relaxed fields on the original are still exactly as snapshotted —
		// this proves nothing aliased through the pointers (Era/Seasons/Series) was
		// mutated in place.
		gotCeil, gotSeries, gotSeasons, gotEra := snapshotScope(rp)
		if gotCeil != wantCeil {
			t.Fatalf("trial %d: audience ceiling mutated %q -> %q", trial, wantCeil, gotCeil)
		}
		if !reflect.DeepEqual(gotSeries, wantSeries) {
			t.Fatalf("trial %d: explicit series scope mutated %v -> %v", trial, wantSeries, gotSeries)
		}
		if !reflect.DeepEqual(gotSeasons, wantSeasons) {
			t.Fatalf("trial %d: explicit season scope mutated %v -> %v", trial, wantSeasons, gotSeasons)
		}
		// Era MAY widen (it's a relaxable scope field), but only through a fresh pointer
		// — the caller's original Era value must not be mutated in place.
		if !reflect.DeepEqual(gotEra, wantEra) {
			t.Fatalf("trial %d: caller's era pointer mutated in place %v -> %v", trial, wantEra, gotEra)
		}
	}
}

// TestHalveFloor_NeverBelowFloor_Monotone asserts halveFloor's two contract points
// (§7 step 1): the result is never below minNoRepeatFloor (24h), and it is monotone
// non-increasing (halving never grows the window).
func TestHalveFloor_NeverBelowFloor_Monotone(t *testing.T) {
	g := rand.New(rand.NewSource(0x24))
	for trial := 0; trial < 5000; trial++ {
		// Inputs spanning below the floor up to ~1 year.
		d := time.Duration(g.Int63n(int64(365 * 24 * time.Hour)))
		out := halveFloor(d)
		if out < minNoRepeatFloor {
			t.Fatalf("trial %d: halveFloor(%s) = %s is below the 24h floor", trial, d, out)
		}
		if out > d && d >= minNoRepeatFloor {
			// For inputs already at/above the floor, halving must not increase the value.
			t.Fatalf("trial %d: halveFloor(%s) = %s grew the window (not monotone)", trial, d, out)
		}
	}
	// Boundary: exactly at and just above the floor clamp to the floor, never under.
	for _, d := range []time.Duration{
		0,
		minNoRepeatFloor - time.Nanosecond,
		minNoRepeatFloor,
		minNoRepeatFloor + time.Nanosecond,
		2 * minNoRepeatFloor, // halves to exactly the floor
		2*minNoRepeatFloor - time.Nanosecond,
	} {
		if got := halveFloor(d); got < minNoRepeatFloor {
			t.Fatalf("halveFloor(%s) = %s below floor", d, got)
		}
	}
	// Iterating halveFloor from any start converges to (and stays at) the floor —
	// this is the loop-safety property step 1 relies on.
	cur := 365 * 24 * time.Hour
	for i := 0; i < 100; i++ {
		next := halveFloor(cur)
		if next > cur {
			t.Fatalf("iterate %d: halveFloor grew %s -> %s", i, cur, next)
		}
		cur = next
	}
	if cur != minNoRepeatFloor {
		t.Fatalf("iterated halveFloor did not converge to the floor: got %s", cur)
	}
}

// TestWidenEra_NeverCrossesDecade_IdempotentAtEdges asserts widenEra's two contract
// points (§7 step 3): the widened window never crosses the decade boundary its bounds
// sit in, and widening is idempotent once each bound is at its decade edge.
func TestWidenEra_NeverCrossesDecade_IdempotentAtEdges(t *testing.T) {
	g := rand.New(rand.NewSource(0x1990))
	for trial := 0; trial < 5000; trial++ {
		// Random era with each bound sometimes 0 (unbounded).
		var from, to int
		if g.Intn(4) != 0 {
			from = 1900 + g.Intn(150)
		}
		if g.Intn(4) != 0 {
			to = 1900 + g.Intn(150)
		}
		in := Range{From: from, To: to}
		out := widenEra(in)

		if in.From > 0 {
			decadeStart := (in.From / 10) * 10
			if out.From < decadeStart {
				t.Fatalf("trial %d: widenEra(%+v).From=%d crossed decade start %d", trial, in, out.From, decadeStart)
			}
			if out.From > in.From {
				t.Fatalf("trial %d: widenEra(%+v).From=%d widened the WRONG way (should shrink or hold)", trial, in, out.From)
			}
		} else if out.From != 0 {
			t.Fatalf("trial %d: widenEra kept From unbounded->%d", trial, out.From)
		}

		if in.To > 0 {
			decadeEnd := (in.To/10)*10 + 9
			if out.To > decadeEnd {
				t.Fatalf("trial %d: widenEra(%+v).To=%d crossed decade end %d", trial, in, out.To, decadeEnd)
			}
			if out.To < in.To {
				t.Fatalf("trial %d: widenEra(%+v).To=%d widened the WRONG way (should grow or hold)", trial, in, out.To)
			}
		} else if out.To != 0 {
			t.Fatalf("trial %d: widenEra kept To unbounded->%d", trial, out.To)
		}

		// Idempotence at the edges: widening the OUTPUT again is a fixed point once
		// both bounds have reached their decade edges. Iterating must converge and then
		// hold (never oscillate) — bounded, so ladderStep step 3 can't loop.
		conv := out
		for i := 0; i < 20; i++ {
			nxt := widenEra(conv)
			if nxt == conv {
				break
			}
			conv = nxt
		}
		if again := widenEra(conv); again != conv {
			t.Fatalf("trial %d: widenEra not idempotent at edges: %+v -> %+v", trial, conv, again)
		}
		// At the fixed point each set bound must sit exactly on its decade edge.
		if conv.From > 0 && conv.From != (conv.From/10)*10 {
			t.Fatalf("trial %d: converged From=%d is not a decade start", trial, conv.From)
		}
		if conv.To > 0 && conv.To != (conv.To/10)*10+9 {
			t.Fatalf("trial %d: converged To=%d is not a decade end", trial, conv.To)
		}
	}

	// Explicit edge cases: a window already spanning a full decade is a fixed point.
	full := Range{From: 1990, To: 1999}
	if got := widenEra(full); got != full {
		t.Fatalf("widenEra(full decade %+v) should be idempotent, got %+v", full, got)
	}
	// A mid-decade window widens exactly ±eraWidenStep until it hits the edges.
	mid := Range{From: 1995, To: 1995}
	w1 := widenEra(mid)
	if w1.From != 1993 || w1.To != 1997 {
		t.Fatalf("widenEra(%+v) = %+v, want {1993 1997}", mid, w1)
	}
}
