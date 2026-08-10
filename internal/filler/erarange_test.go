package filler

import (
	"math/rand"
	"testing"
)

// ⚠ **Internal test on purpose**: the properties below are about `candidatePools`, the unexported
// builder `Assemble` and `Coverage` both call. Asserting them through the exported surface would
// re-derive the rungs in the test, which is the "meter that recomputes its own buckets" mistake
// `coverage.go` warns about — a confident wrong answer, one layer down.

// The exact rung is EXACTLY the clips inside the range — no more (a range that admits a clip it
// should not) and no fewer (the V51f bug: `To` discarded, so 1990–1999 behaved as 1990 alone).
func TestCandidatePools_ExactRungIsExactlyTheRange(t *testing.T) {
	rng := rand.New(rand.NewSource(51))

	for i := 0; i < 300; i++ {
		from := 1930 + rng.Intn(80)
		to := from + rng.Intn(30)
		era := EraRange{From: from, To: to}

		catalog := make([]Clip, 0, 40)
		for j := 0; j < 40; j++ {
			catalog = append(catalog, commercial("c"+string(rune('a'+j%26))+string(rune('a'+j/26)),
				1925+rng.Intn(95), Kids))
		}

		pools := candidatePools(catalog, Window{Era: era, Audience: Kids, GapMs: 120_000, PodMax: 4}, Policy{})
		exact := map[string]bool{}
		for _, c := range pools[0].clips {
			exact[c.ID()] = true
		}

		for _, c := range catalog {
			inRange := c.Era >= from && c.Era <= to
			if inRange && !exact[c.ID()] {
				t.Fatalf("era %+v: clip from %d is inside the range but missing from the exact rung", era, c.Era)
			}
			if !inRange && exact[c.ID()] {
				t.Fatalf("era %+v: clip from %d is outside the range but present in the exact rung", era, c.Era)
			}
		}
	}
}

// The widened rung is a decade either side of the range — and, being a fallback, is always a
// SUPERSET of exact. A rung that could be narrower than the one above it would make the ladder
// non-monotonic, so `fillCommercials` taking "the tightest non-empty pool" would stop meaning
// what it says.
func TestCandidatePools_WidenedIsADecadeEitherSideAndNests(t *testing.T) {
	rng := rand.New(rand.NewSource(52))

	for i := 0; i < 300; i++ {
		from := 1930 + rng.Intn(80)
		to := from + rng.Intn(30)
		era := EraRange{From: from, To: to}

		catalog := make([]Clip, 0, 40)
		for j := 0; j < 40; j++ {
			catalog = append(catalog, commercial("c"+string(rune('a'+j%26))+string(rune('a'+j/26)),
				1925+rng.Intn(95), Kids))
		}

		pools := candidatePools(catalog, Window{Era: era, Audience: Kids, GapMs: 120_000, PodMax: 4}, Policy{})
		exact, widened := map[string]bool{}, map[string]bool{}
		for _, c := range pools[0].clips {
			exact[c.ID()] = true
		}
		for _, c := range pools[1].clips {
			widened[c.ID()] = true
		}

		for _, c := range catalog {
			if inWidened := c.Era >= from-10 && c.Era <= to+10; inWidened != widened[c.ID()] {
				t.Fatalf("era %+v: clip from %d — widened membership %v, want %v",
					era, c.Era, widened[c.ID()], inWidened)
			}
			if exact[c.ID()] && !widened[c.ID()] {
				t.Fatalf("era %+v: clip from %d is exact but not widened — the ladder must nest", era, c.Era)
			}
		}
	}
}

// "Any era" collapses the era rungs rather than emptying them: with no era constraint the exact
// and widened rungs are the whole audience-matched pool. The alternative — treating an empty
// range as "matches nothing" — is the untagged-audience cliff wearing a different hat.
func TestCandidatePools_AnyEraFillsEveryRung(t *testing.T) {
	catalog := []Clip{
		commercial("a", 1975, Kids),
		commercial("b", 1992, Kids),
		commercial("c", 2011, Kids),
	}
	pools := candidatePools(catalog, Window{Era: EraRange{}, Audience: Kids, GapMs: 120_000, PodMax: 4}, Policy{})
	for _, p := range pools {
		if len(p.clips) != len(catalog) {
			t.Errorf("rung %s holds %d clips, want all %d under an open era range",
				p.level, len(p.clips), len(catalog))
		}
	}
}

// ⚠ A clip whose era Loomarr could not ground (year 0) satisfies NO range — the same shape as the
// audience rule, and for the same reason: "we could not tell" must never quietly count as "yes".
// It still reaches the audience rung, so it is not invisible, just not claimed as era-accurate.
func TestEraRange_UngroundedYearIsNeverInsideARange(t *testing.T) {
	if (EraRange{From: 1990, To: 1999}).Contains(0) {
		t.Error("a clip with no grounded era counted as inside 1990–1999")
	}
	if !(EraRange{}).Contains(0) {
		t.Error("a clip with no grounded era must still satisfy the open range")
	}
}
