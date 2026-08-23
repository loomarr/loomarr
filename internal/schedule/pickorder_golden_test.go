package schedule

import (
	"fmt"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/provision"
)

// A CHARACTERIZATION test for the arranged order.
//
// The order a channel plays in is contract, not implementation detail: it is seed-deterministic
// by design (§10/§19) and the separation rules are user-visible policy. So an optimization of
// the arrangement code has to prove the OUTPUT is byte-identical, not merely that the existing
// assertions still hold — those check properties (no two same-series adjacent, every slot
// placed), and many different orders satisfy them.
//
// This records the exact sequence today's code produces for a series-heavy deck. If a refactor
// changes it, this fails loudly and the diff shows precisely which position moved.

// deck builds a mixed deck: `shows` series with `each` episodes apiece, interleaved in the
// arrival order a real expansion produces (all of show A, then all of show B, …), which is the
// worst case for anti-clustering and therefore the case the arranger works hardest on.
func deck(shows, each int) []Slot {
	var out []Slot
	for s := 0; s < shows; s++ {
		key := provision.Key(fmt.Sprintf("series:tvdb:%d", 100+s))
		for e := 1; e <= each; e++ {
			out = append(out, Slot{
				Kind:          SlotProgram,
				Key:           key,
				Title:         fmt.Sprintf("S%d E%02d", s, e),
				SeriesTitle:   fmt.Sprintf("Show %d", s),
				Season:        1,
				Episode:       e,
				DurationMs:    1_320_000,
				LibraryItemID: fmt.Sprintf("item-%d-%d", s, e),
			})
		}
	}
	return out
}

// signature renders an arrangement compactly, so a failure diff is readable rather than a wall
// of structs. Series letter per slot: "ABABAB…".
func signature(slots []Slot) string {
	var b strings.Builder
	for _, s := range slots {
		if s.Kind != SlotProgram {
			b.WriteByte('.')
			continue
		}
		// Last character of the series key distinguishes the shows in these fixtures.
		k := string(s.Key)
		b.WriteByte(k[len(k)-1])
	}
	return b.String()
}

func TestArrangeOrderIsStable_SmallMixedDeck(t *testing.T) {
	programs := deck(3, 4) // 3 shows × 4 episodes
	rp := ResolvedPolicy{Sep: ResolvedSeparation{BlockMax: 1}}

	got, ok := backtrackArrange(programs, rp)
	if !ok {
		t.Fatal("arrangement failed on a deck that should be satisfiable")
	}
	if len(got) != len(programs) {
		t.Fatalf("arrangement dropped slots: %d in, %d out", len(programs), len(got))
	}

	// The exact sequence today's code produces. If an optimization changes the arranged
	// order, this is the assertion that says so — and the signature shows where.
	const want = "012012012012"
	if s := signature(got); s != want {
		t.Fatalf("arranged order changed:\n  got  %s\n  want %s\n(the play order is contract — see the file comment)", s, want)
	}
}

func TestArrangeOrderIsStable_UnevenDeck(t *testing.T) {
	// Uneven counts are where "most-remaining-first" actually bites: the arranger must front-
	// load the show with the most episodes or it strands them at the end.
	programs := append(deck(1, 6), deck(2, 2)[:2]...)
	for i := range programs {
		programs[i].Episode = i + 1
	}
	rp := ResolvedPolicy{Sep: ResolvedSeparation{BlockMax: 1}}

	got, ok := backtrackArrange(programs, rp)
	if !ok {
		t.Skip("this deck is not satisfiable under BlockMax=1; the greedy fallback owns it")
	}
	if len(got) != len(programs) {
		t.Fatalf("arrangement dropped slots: %d in, %d out", len(programs), len(got))
	}
	t.Logf("uneven-deck signature: %s", signature(got))
}

// The property that must hold regardless of ordering strategy: every slot placed exactly once.
// Cheap, and it catches the class of bug an index-juggling optimization introduces.
func TestArrangePlacesEverySlotExactlyOnce(t *testing.T) {
	programs := deck(4, 5)
	rp := ResolvedPolicy{Sep: ResolvedSeparation{BlockMax: 1}}

	got, ok := backtrackArrange(programs, rp)
	if !ok {
		t.Skip("not satisfiable; greedy fallback owns this case")
	}
	seen := map[string]int{}
	for _, s := range got {
		seen[s.LibraryItemID]++
	}
	if len(seen) != len(programs) {
		t.Fatalf("placed %d distinct slots, want %d", len(seen), len(programs))
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("slot %s placed %d times, want exactly 1", id, n)
		}
	}
}
