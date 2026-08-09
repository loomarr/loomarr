package filler_test

import (
	"testing"

	"github.com/mantonx/loomarr/internal/filler"
)

// THE V29 gate (plan §6, §6.2): the coverage meter must agree with what actually airs.
//
// The mock's meter claims to come "from the same ladder reconcile uses" and does not — it
// recomputes its buckets inline with five inconsistent predicates. This test is what makes the
// claim true here: for every target, the rung Coverage reports must be the rung Assemble
// actually fills from. A meter that says "exact" while breaks resolve at "audience" is a
// confident wrong answer about why a channel sounds the way it does.
//
// Coverage and Assemble share `candidatePools` by construction, so this is a guard against
// someone re-implementing either side, not against today's code being wrong.
func TestCoverageMatchesTheRungAssembleFills(t *testing.T) {
	cat := sampleCatalog()

	cases := []struct {
		name   string
		window filler.Window
		policy filler.Policy
	}{
		{"exact era + audience", kidsWindow(1), filler.Policy{}},
		// 1985 kids: no exact-era clip, but c5 is same-decade → widened.
		{"no exact era, decade available", filler.Window{ChannelID: "ch1", Seed: 2, Era: filler.Year(1987), Audience: filler.Kids, GapMs: 120000, PodMax: 4}, filler.Policy{}},
		// 2015 kids: nothing in era or decade → any-era audience match.
		{"no era or decade, audience only", filler.Window{ChannelID: "ch1", Seed: 3, Era: filler.Year(2015), Audience: filler.Kids, GapMs: 120000, PodMax: 4}, filler.Policy{}},
		// A SPAN rather than a single year — the case that could not be expressed before V51f,
		// and the one where a decade-bucket widening would have collapsed into `exact`.
		{"era span", filler.Window{ChannelID: "ch1", Seed: 4, Era: filler.EraRange{From: 1985, To: 1992}, Audience: filler.Kids, GapMs: 120000, PodMax: 4}, filler.Policy{}},
		// An audience with nothing at all → the embedded bumper card.
		{"no material at all", filler.Window{ChannelID: "ch1", Seed: 5, Era: filler.Year(1992), Audience: filler.Audience("nobody"), GapMs: 120000, PodMax: 4}, filler.Policy{}},
		// Category narrowing shrinks the commercial pool before the ladder runs.
		{"category narrowed", filler.Window{ChannelID: "ch1", Seed: 6, Era: filler.Year(1992), Audience: filler.Kids, GapMs: 120000, PodMax: 4, Categories: []string{"toys"}}, filler.Policy{}},
		// Excluding every exact-era clip must push coverage DOWN a rung, exactly as it
		// pushes assembly down — this is where a coverage impl that ignored Excluded breaks.
		{"exclusions push down a rung", filler.Window{ChannelID: "ch1", Seed: 7, Era: filler.Year(1992), Audience: filler.Kids, GapMs: 120000, PodMax: 4, Excluded: []string{"c1.mp4", "c2.mp4", "c3.mp4", "c4.mp4"}}, filler.Policy{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report := filler.Coverage(cat, tc.window, tc.policy)
			// Assemble mutates `used`, so give it its own map — and give Coverage the
			// untouched catalog, which is what a caller would do.
			pod := filler.Assemble(cat, tc.window, tc.policy, nil)

			if report.Level != pod.MatchLevel {
				t.Errorf("coverage says %q, assembly filled from %q — the meter would lie",
					report.Level, pod.MatchLevel)
			}
		})
	}
}

// The rungs must be reported in ladder order, tightest first, because the UI reads them
// positionally — a reversed slice renders "your catalog is mostly loose matches" for a
// catalog that is mostly exact.
func TestCoverageRungsAreInLadderOrder(t *testing.T) {
	report := filler.Coverage(sampleCatalog(), kidsWindow(1), filler.Policy{})

	want := []filler.MatchLevel{filler.MatchExact, filler.MatchWidened, filler.MatchAudience}
	if len(report.Rungs) != len(want) {
		t.Fatalf("got %d rungs, want %d", len(report.Rungs), len(want))
	}
	for i, w := range want {
		if report.Rungs[i].Level != w {
			t.Errorf("rung %d is %q, want %q", i, report.Rungs[i].Level, w)
		}
	}
}

// ⚠ **Every rung is now always reported, and the test that asserted the opposite is gone with the
// field it tested.** `TestCoverageOmitsTheWidenedRungUnderEraStrict` proved that `EraStrict`
// removed the widened rung — true, but unreachable: no key, no env var, no policy field, set in
// tests and nowhere else. The absent-not-zero rule existed only to serve it. With the flag deleted
// a rung at 0 means what a reader assumes: nothing in the catalog reaches it.
func TestCoverageAlwaysReportsEveryRung(t *testing.T) {
	// A 2015 window has no exact-era and no widened clip in this catalog, so two rungs are
	// genuinely empty — precisely the case the old rule would have hidden.
	w := filler.Window{ChannelID: "ch1", Seed: 4, Era: filler.Year(2015), Audience: filler.Kids, GapMs: 120000, PodMax: 4}
	report := filler.Coverage(sampleCatalog(), w, filler.Policy{})

	if len(report.Rungs) != 3 {
		t.Fatalf("got %d rungs, want 3 (exact, widened, audience) — a rung must never be dropped", len(report.Rungs))
	}
	want := []filler.MatchLevel{filler.MatchExact, filler.MatchWidened, filler.MatchAudience}
	for i, lvl := range want {
		if report.Rungs[i].Level != lvl {
			t.Errorf("rung %d is %q, want %q", i, report.Rungs[i].Level, lvl)
		}
	}
	if report.Rungs[0].Clips != 0 || report.Rungs[1].Clips != 0 {
		t.Errorf("expected the 2015 exact/widened rungs to be empty, got %d/%d",
			report.Rungs[0].Clips, report.Rungs[1].Clips)
	}
}

// Total is the widest rung, not a sum — the rungs NEST (exact ⊆ widened ⊆ audience), so adding
// them counts one exact-era clip up to three times and reports a catalog three times its size.
func TestCoverageTotalDoesNotDoubleCountNestedRungs(t *testing.T) {
	report := filler.Coverage(sampleCatalog(), kidsWindow(1), filler.Policy{})

	// sampleCatalog has 4 kids commercials in 1992 + 1 kids commercial in 1985 = 5 eligible
	// for a kids window at any era. The late-night ad and the two bumpers are not commercials
	// for this audience.
	if report.Total != 5 {
		t.Errorf("Total = %d, want 5 (the widest rung); a sum would give %d",
			report.Total, sumRungs(report))
	}
	var widest int
	for _, r := range report.Rungs {
		if r.Clips > widest {
			widest = r.Clips
		}
	}
	if report.Total != widest {
		t.Errorf("Total = %d but the widest rung holds %d", report.Total, widest)
	}
}

// Duration eligibility is the POLICY's clip-length bounds (MinClipMs/MaxClipMs), applied when
// the pools are built. A clip outside them is not coverage: assembly will never pick it, so
// counting it would promise material that cannot air.
//
// ⚠ This is NOT the window's GapMs, and the distinction is easy to get backwards — I did, and
// the test above caught it. `durationEligible` reads policy only; the gap is enforced later, at
// fill time, as a running budget in `fillCommercials`. So a break too short for any clip still
// has full COVERAGE — the material exists and matches, it just does not fit this particular
// gap. Reporting 0 there would tell an operator to go find clips they already have.
func TestCoverageExcludesPolicyIneligibleClips(t *testing.T) {
	cat := sampleCatalog()
	w := kidsWindow(9)
	// The sample's kids commercials are 15s and 30s. A 20s floor leaves only the 30s ones.
	policy := filler.Policy{MinClipMs: 20000}

	report := filler.Coverage(cat, w, policy)
	pod := filler.Assemble(cat, w, policy, nil)

	if report.Level != pod.MatchLevel {
		t.Errorf("coverage %q vs assembly %q under a clip-length floor", report.Level, pod.MatchLevel)
	}
	// c1/c2/c3 (30s, 1992 kids) survive; c4 (15s) does not.
	if got, want := report.Rungs[0].Clips, 3; got != want {
		t.Errorf("exact rung has %d clips under a 20s floor, want %d", got, want)
	}

	// And the same window with no floor sees the 15s clip again — proving the floor is what
	// moved the number, not some unrelated filter.
	unfiltered := filler.Coverage(cat, w, filler.Policy{})
	if got, want := unfiltered.Rungs[0].Clips, 4; got != want {
		t.Errorf("exact rung has %d clips with no floor, want %d", got, want)
	}
}

// A gap too short for any clip does NOT reduce coverage — the material matches, it just does
// not fit this break. Assembly falls back to the bumper card; coverage still reports the pool.
// Conflating the two would have the meter tell an operator to acquire clips they already own.
func TestCoverageIsAboutMaterialNotThisBreaksGap(t *testing.T) {
	cat := sampleCatalog()
	tiny := filler.Window{ChannelID: "ch1", Seed: 10, Era: filler.Year(1992), Audience: filler.Kids, GapMs: 5000, PodMax: 4}

	report := filler.Coverage(cat, tiny, filler.Policy{})
	if report.Total == 0 {
		t.Error("Total = 0 for a short gap — coverage is about the catalog, not this break")
	}
	if report.Level != filler.MatchExact {
		t.Errorf("Level = %q, want exact: the clips match, they just do not fit", report.Level)
	}
}

func sumRungs(r filler.CoverageReport) int {
	var n int
	for _, rung := range r.Rungs {
		n += rung.Clips
	}
	return n
}
