package schedule

import (
	"fmt"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
)

// relaxAndSlot skips re-placing the cycle when a ladder step changed nothing placement reads
// (see placementInputsChanged). That is a performance change that MUST NOT alter behaviour:
// the play order is contract, and so is the applied-relaxation list the UI surfaces.
//
// This pins it by differential testing — the optimized path against a reference that re-places
// on every step (the original algorithm, kept below), across 180 deck/policy shapes. Both the
// arrangement and the applied list must match exactly.
//
// Sabotage-verified: forcing placementInputsChanged to always return false fails this with
// "ARRANGEMENT differs", so it genuinely guards the invariant rather than passing by luck.
func TestRelaxAndSlotMatchesAlwaysReplace(t *testing.T) {
	for _, shows := range []int{1, 2, 3, 4, 6} {
		for _, each := range []int{3, 12, 25} {
			for _, block := range []int{0, 1, 2, 3} {
				for _, gap := range []time.Duration{0, time.Hour, 2 * time.Hour} {
					programs := equivDeck(shows, each)
					rp := ResolvedPolicy{
						Sep: ResolvedSeparation{
							BlockMax: block, SeriesMinGap: gap,
							EpisodeNoRepeat: 168 * time.Hour, MovieNoRepeat: 168 * time.Hour,
						},
						Ordering: OrderSyndication,
					}
					gotSlots, gotApplied := relaxAndSlot(programs, rp, 42)
					wantSlots, wantApplied := refRelaxAndSlot(programs, rp, 42)

					if equivSig(gotSlots) != equivSig(wantSlots) {
						t.Fatalf("shows=%d each=%d block=%d gap=%v: ARRANGEMENT differs\n got %s\nwant %s",
							shows, each, block, gap, equivSig(gotSlots), equivSig(wantSlots))
					}
					if len(gotApplied) != len(wantApplied) {
						t.Fatalf("shows=%d each=%d block=%d gap=%v: applied count %d vs %d",
							shows, each, block, gap, len(gotApplied), len(wantApplied))
					}
					for i := range gotApplied {
						if gotApplied[i] != wantApplied[i] {
							t.Fatalf("applied[%d] %+v vs %+v", i, gotApplied[i], wantApplied[i])
						}
					}
				}
			}
		}
	}
}

// refRelaxAndSlot is the ORIGINAL algorithm, kept verbatim as the oracle: re-place after every
// ladder step. It is deliberately a duplicate rather than a shared helper — a reference
// implementation that shares code with the thing it checks stops being a reference.
func refRelaxAndSlot(slots []Slot, rp ResolvedPolicy, seed int64) ([]Slot, []AppliedRelaxation) {
	ordered := slotByPolicy(slots, rp, seed)
	if !separationUnsatisfied(ordered, rp) {
		return ordered, nil
	}
	var applied []AppliedRelaxation
	cur := rp
	for step := 0; step < maxLadderSteps; step++ {
		relaxed, note, ok := ladderStep(cur)
		if !ok {
			break
		}
		cur = relaxed
		applied = append(applied, note)
		ordered = slotByPolicy(slots, cur, seed)
		if !separationUnsatisfied(ordered, cur) {
			break
		}
	}
	return ordered, applied
}

// equivDeck builds a deck of `shows` series with `each` episodes apiece, in the arrival order a
// real expansion produces (all of show A, then all of B) — the worst case for anti-clustering.
func equivDeck(shows, each int) []Slot {
	var out []Slot
	for s := 0; s < shows; s++ {
		key := provision.Key(fmt.Sprintf("series:tvdb:%d", 100+s))
		for e := 1; e <= each; e++ {
			out = append(out, Slot{
				Kind: SlotProgram, Key: key,
				Title: fmt.Sprintf("S%d E%02d", s, e), SeriesTitle: fmt.Sprintf("Show %d", s),
				Season: 1, Episode: e, DurationMs: 1_320_000,
				LibraryItemID: fmt.Sprintf("item-%d-%d", s, e),
			})
		}
	}
	return out
}

// equivSig renders an arrangement as one character per slot, so a failure diff is readable.
func equivSig(slots []Slot) string {
	b := make([]byte, 0, len(slots))
	for _, s := range slots {
		k := string(s.Key)
		b = append(b, k[len(k)-1])
	}
	return string(b)
}
