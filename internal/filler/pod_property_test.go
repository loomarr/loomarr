package filler_test

import (
	"math/rand"
	"reflect"
	"strconv"
	"testing"

	"github.com/mantonx/loomarr/internal/filler"
)

// This file adds SEEDED property tests over Assemble (pod.go) + the fallback
// ladder (ladder.go), complementing the six worked examples in pod_test.go. Each
// test loops over many randomly-generated catalogs / windows / seeds drawn from a
// single fixed math/rand source, so a failure reproduces exactly (§19). These are
// plain seeded Go loops, not a property framework — no new dependency.
//
// Invariants asserted (all from §10):
//   - DETERMINISM   — same catalog + same Window.Seed → byte-identical Pod.
//   - NEVER DEAD AIR — a Pod always has ≥1 entry; when no commercial fits, it is
//     MatchBumperCard with FallbackCard present (pod.go:120-123).
//   - NO-REPEAT     — no TunarrProgramID appears twice in a single Pod.
//   - DENSITY       — commercial entries ≤ PodMax (pod.go/ladder.go cap PodMax on
//     commercials; bumpers are bookends, the fallback card is the floor).

// --- randomized catalog / window builders (seeded, hermetic) ---

var (
	propEras      = []int{1975, 1982, 1985, 1990, 1992, 1994, 1999, 2003}
	propAudiences = []filler.Audience{filler.Kids, filler.Family, filler.General, filler.LateNight}
	propCategs    = []string{"toys", "cereal", "cars", "tech", "fast_food", "movie_trailer", ""}
	propKinds     = []filler.Kind{filler.Commercial, filler.Bumper, filler.StationID}
	propDurations = []int64{5000, 10000, 15000, 20000, 30000, 45000, 60000}
)

// randomCatalog builds a catalog of 0..maxN clips with unique program IDs, using
// the supplied rng so the whole catalog is reproducible from the outer seed.
func randomCatalog(rng *rand.Rand, maxN int) []filler.Clip {
	n := rng.Intn(maxN + 1)
	cat := make([]filler.Clip, 0, n)
	for i := 0; i < n; i++ {
		cat = append(cat, filler.Clip{
			// ⚠ Hash is the IDENTITY (Clip.ID()) pod assembly keys pinned/excluded/no-repeat on, and it
			// is DISTINCT from Path — a clip's hash is not its location (§10 V38c/V45a). Omitting it made
			// every clip share the empty-string key, so hash-keyed no-repeat collided on "" and passed
			// for the wrong reason ([[loomarr-fixture-collapsed-keys]]). Derive, never equate.
			Hash:       "h" + strconv.Itoa(i),
			Path:       "p" + strconv.Itoa(i),
			Name:       "clip-" + strconv.Itoa(i),
			Kind:       propKinds[rng.Intn(len(propKinds))],
			Era:        propEras[rng.Intn(len(propEras))],
			Audience:   propAudiences[rng.Intn(len(propAudiences))],
			Category:   propCategs[rng.Intn(len(propCategs))],
			DurationMs: propDurations[rng.Intn(len(propDurations))],
		})
	}
	return cat
}

// randomWindow builds a Window with a random target, gap and PodMax. windowSeed is
// the Window.Seed that Assemble uses internally; it is independent of the rng that
// shapes the catalog/window so the two don't correlate.
func randomWindow(rng *rand.Rand, windowSeed int64) filler.Window {
	return filler.Window{
		ChannelID: "ch",
		Seed:      windowSeed,
		Era:       randomEraRange(rng),
		Audience:  propAudiences[rng.Intn(len(propAudiences))],
		GapMs:     int64(30000 + rng.Intn(20)*30000), // 30s .. ~10min
		PodMax:    1 + rng.Intn(6),                   // 1..6
	}
}

// randomEraRange picks one of the THREE shapes an operator can express since V51f — any era, a
// single year, or a span — rather than the single year the field used to be limited to.
//
// ⚠ It replaces `EraStrict` (retired-ok) as this generator's era-varying dimension. That flag was reachable
// only from tests, so randomising it explored a branch no install could ever be in; randomising
// the range explores the branch every install is in.
func randomEraRange(rng *rand.Rand) filler.EraRange {
	switch rng.Intn(3) {
	case 0:
		return filler.EraRange{}
	case 1:
		return filler.Year(propEras[rng.Intn(len(propEras))])
	default:
		a, b := propEras[rng.Intn(len(propEras))], propEras[rng.Intn(len(propEras))]
		if a > b {
			a, b = b, a
		}
		return filler.EraRange{From: a, To: b}
	}
}

func randomPolicy(rng *rand.Rand) filler.Policy {
	p := filler.Policy{}
	// Occasionally bound clip durations, to exercise durationEligible pruning.
	if rng.Intn(3) == 0 {
		p.MinClipMs = 10000
		p.MaxClipMs = 45000
	}
	return p
}

func commercialCount(p filler.Pod) int {
	n := 0
	for _, e := range p.Entries {
		if e.Kind == filler.Commercial {
			n++
		}
	}
	return n
}

func hasFallbackCard(p filler.Pod) bool {
	for _, e := range p.Entries {
		if e.IsFallbackCard {
			return true
		}
	}
	return false
}

// TestProp_Determinism asserts §19 seeded-determinism as a property: across many
// random catalogs/windows/policies, calling Assemble twice with the SAME inputs
// (and the same Window.Seed) yields a byte-identical Pod (reflect.DeepEqual on the
// whole struct, not just IDs). Fresh `used` maps each call so the two runs are
// truly independent.
func TestProp_Determinism(t *testing.T) {
	rng := rand.New(rand.NewSource(0xD37E12))
	for i := 0; i < 400; i++ {
		cat := randomCatalog(rng, 14)
		w := randomWindow(rng, rng.Int63())
		pol := randomPolicy(rng)

		p1 := filler.Assemble(cat, w, pol, map[string]bool{})
		p2 := filler.Assemble(cat, w, pol, map[string]bool{})
		if !reflect.DeepEqual(p1, p2) {
			t.Fatalf("iter %d: Assemble not deterministic for identical inputs\n a=%+v\n b=%+v\n window=%+v policy=%+v",
				i, p1, p2, w, pol)
		}
	}
}

// TestProp_DeterminismNilVsEmptyUsed asserts the documented `used == nil` handling
// (pod.go:87-89): a nil used map and a fresh empty map must produce the same Pod.
func TestProp_DeterminismNilVsEmptyUsed(t *testing.T) {
	rng := rand.New(rand.NewSource(0x5EED))
	for i := 0; i < 200; i++ {
		cat := randomCatalog(rng, 12)
		w := randomWindow(rng, rng.Int63())
		pol := randomPolicy(rng)

		pNil := filler.Assemble(cat, w, pol, nil)
		pEmpty := filler.Assemble(cat, w, pol, map[string]bool{})
		if !reflect.DeepEqual(pNil, pEmpty) {
			t.Fatalf("iter %d: nil used != empty used\n nil=%+v\n empty=%+v", i, pNil, pEmpty)
		}
	}
}

// TestProp_NeverDeadAir asserts §10 "never dead air": every Pod has ≥1 entry, and
// the terminal ladder rung is well-formed — when Assemble placed no commercial the
// pod MUST be MatchBumperCard AND carry the embedded FallbackCard (pod.go:120-123).
// The random catalogs include ones with no commercials at all, wrong audience, or
// duration-pruned-to-empty, which is exactly where this rung fires.
func TestProp_NeverDeadAir(t *testing.T) {
	rng := rand.New(rand.NewSource(0xDEADA12))
	for i := 0; i < 500; i++ {
		cat := randomCatalog(rng, 12)
		w := randomWindow(rng, rng.Int63())
		pol := randomPolicy(rng)

		p := filler.Assemble(cat, w, pol, map[string]bool{})

		if len(p.Entries) == 0 {
			t.Fatalf("iter %d: empty pod — dead air (§10)\n catalog=%+v\n window=%+v policy=%+v", i, cat, w, pol)
		}
		if commercialCount(p) == 0 {
			// No commercial fit → the ladder must have bottomed out at the card.
			if p.MatchLevel != filler.MatchBumperCard {
				t.Fatalf("iter %d: no commercials placed but MatchLevel=%s (want bumper_card)\n pod=%+v\n window=%+v",
					i, p.MatchLevel, p, w)
			}
			if !hasFallbackCard(p) {
				t.Fatalf("iter %d: no commercials placed and no FallbackCard — break would be dead air\n pod=%+v", i, p)
			}
		}
		// The fallback card is only added on the no-commercial path; when commercials
		// were placed there must be no dead-air card masquerading as content.
		if commercialCount(p) > 0 && hasFallbackCard(p) {
			t.Fatalf("iter %d: FallbackCard present alongside %d commercials (card is the no-match floor only)\n pod=%+v",
				i, commercialCount(p), p)
		}
	}
}

// TestProp_NoRepeatWithinPod asserts §10 no-repeat: within a single Pod no real
// TunarrProgramID appears twice. (The empty-ID fallback card is exempt — it has no
// program identity.) pod_test.go already covers no-repeat ACROSS pods threading
// `used`; this covers the within-pod guarantee for a single Assemble call.
func TestProp_NoRepeatWithinPod(t *testing.T) {
	rng := rand.New(rand.NewSource(0x0FF1CE))
	for i := 0; i < 500; i++ {
		cat := randomCatalog(rng, 16)
		w := randomWindow(rng, rng.Int63())
		pol := randomPolicy(rng)

		p := filler.Assemble(cat, w, pol, map[string]bool{})
		seen := map[string]bool{}
		for _, e := range p.Entries {
			if e.Path == "" {
				continue
			}
			if seen[e.Path] {
				t.Fatalf("iter %d: program %q appears twice in one pod (no-repeat violated)\n pod=%+v",
					i, e.Path, p)
			}
			seen[e.Path] = true
		}
	}
}

// TestProp_Density asserts §10 density: the number of COMMERCIAL entries never
// exceeds Window.PodMax. PodMax caps commercials (ladder.go), not total entries —
// bumpers are bookends (≤2) and the fallback card is the floor — so this is the
// true invariant. When PodMax<=0 the code defaults the cap to 4 (ladder.go:81-83),
// which randomWindow never triggers (PodMax is 1..6); a dedicated case covers it.
func TestProp_Density(t *testing.T) {
	rng := rand.New(rand.NewSource(0xDE151))
	for i := 0; i < 500; i++ {
		cat := randomCatalog(rng, 20)
		w := randomWindow(rng, rng.Int63())
		pol := randomPolicy(rng)

		p := filler.Assemble(cat, w, pol, map[string]bool{})
		if got := commercialCount(p); got > w.PodMax {
			t.Fatalf("iter %d: %d commercials > PodMax=%d (density violated)\n pod=%+v",
				i, got, w.PodMax, p)
		}
	}
}

// TestProp_DensityDefaultPodMax pins the documented default (ladder.go:81-83): a
// window with PodMax<=0 caps commercials at 4. Built from a rich all-eligible
// exact-match catalog so the cap, not scarcity, is what bounds the count.
func TestProp_DensityDefaultPodMax(t *testing.T) {
	var cat []filler.Clip
	cat = append(cat, filler.Clip{Hash: "hbump", Path: "bump", Kind: filler.Bumper, Era: 1992, Audience: filler.General, DurationMs: 5000})
	for i := 0; i < 12; i++ {
		cat = append(cat, filler.Clip{
			// Hash distinct from Path — the identity pod assembly keys on (§10 V45a).
			Hash: "had" + strconv.Itoa(i),
			Path: "ad" + strconv.Itoa(i), Kind: filler.Commercial, Era: 1992,
			Audience: filler.Kids, Category: "c" + strconv.Itoa(i), DurationMs: 20000,
		})
	}
	for seed := int64(0); seed < 40; seed++ {
		w := filler.Window{ChannelID: "ch", Seed: seed, Era: filler.Year(1992), Audience: filler.Kids, GapMs: 3600000, PodMax: 0}
		p := filler.Assemble(cat, w, filler.Policy{}, map[string]bool{})
		if got := commercialCount(p); got > 4 {
			t.Fatalf("seed %d: PodMax<=0 should default the commercial cap to 4, got %d\n pod=%+v", seed, got, p)
		}
	}
}

// TestProp_NoRepeatAcrossWindow asserts the threaded-`used` no-repeat guarantee
// over a whole window: assembling many pods against one shared `used` map never
// re-plays a real program. Complements pod_test.go's fixed 3-pod example with a
// randomized many-pod sweep.
func TestProp_NoRepeatAcrossWindow(t *testing.T) {
	rng := rand.New(rand.NewSource(0xACC0))
	for i := 0; i < 120; i++ {
		cat := randomCatalog(rng, 18)
		pol := randomPolicy(rng)
		used := map[string]bool{}
		seen := map[string]int{}
		pods := 2 + rng.Intn(6)
		for k := 0; k < pods; k++ {
			w := randomWindow(rng, rng.Int63())
			p := filler.Assemble(cat, w, pol, used)
			for _, e := range p.Entries {
				if e.Path != "" {
					seen[e.Path]++
				}
			}
		}
		for id, n := range seen {
			if n > 1 {
				t.Fatalf("iter %d: program %q played %d times across the window (no-repeat violated)", i, id, n)
			}
		}
	}
}
