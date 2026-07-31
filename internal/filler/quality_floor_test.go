package filler_test

import (
	"testing"

	"github.com/mantonx/loomarr/internal/filler"
)

// A catalog spanning the quality range, all otherwise identical so quality is the only
// variable that can move a result.
func qualityCatalog() []filler.Clip {
	return []filler.Clip{
		{Path: "hd.mp4", Name: "HD spot", Kind: filler.Commercial, Era: 1992, Audience: filler.Kids, Category: "toys", DurationMs: 30000, Quality: "1080p"},
		{Path: "sd.mp4", Name: "SD spot", Kind: filler.Commercial, Era: 1992, Audience: filler.Kids, Category: "cereal", DurationMs: 30000, Quality: "480p"},
		{Path: "rip.mp4", Name: "240p rip", Kind: filler.Commercial, Era: 1992, Audience: filler.Kids, Category: "tech", DurationMs: 30000, Quality: "240p"},
		// No quality: audio-only, or scanned before 00014_clips_quality added the column.
		{Path: "unknown.mp4", Name: "Unprobed spot", Kind: filler.Commercial, Era: 1992, Audience: filler.Kids, Category: "cars", DurationMs: 30000},
		// A low-res BUMPER: the floor must not touch it (see the ladder's comment).
		{Path: "bump.mp4", Name: "We'll be right back", Kind: filler.Bumper, Era: 1992, Audience: filler.General, DurationMs: 5000, Quality: "240p"},
	}
}

func qualityWindow(seed int64) filler.Window {
	return filler.Window{ChannelID: "ch1", Seed: seed, Era: 1992, Audience: filler.Kids, GapMs: 120000, PodMax: 4}
}

func paths(p filler.Pod) []string {
	out := make([]string, 0, len(p.Entries))
	for _, e := range p.Entries {
		out = append(out, e.Path)
	}
	return out
}

// ⚠ **THE gate for V17c.** `00014_clips_quality` shipped promising quality NEVER affects
// selection, and the amended comment now says "by default". This is what makes the amended
// claim true: with the floor unset, assembly produces the identical pod, clip for clip and in
// the same order. If this ever fails, the phase has broken the promise it was allowed to
// narrow rather than the one it was allowed to break.
// ⚠ It asserts against a FIXED expectation, not against a second Assemble call, and that is
// load-bearing. The first version compared `Policy{}` to `Policy{MinQualityHeight: 0}` — two
// runs through the same code — so a floor leaking in as a DEFAULT applied to both sides and
// they still matched. Verified by sabotage: making an unset floor mean 480 left that version
// green, which is the precise failure it was written to catch.
//
// The expectation below is therefore what the catalog must yield with NO floor: every
// commercial eligible, including the 240p rip. A default that quietly excludes anything
// changes these lists and fails here.
func TestFloorOffIsByteIdentical(t *testing.T) {
	cat := qualityCatalog()

	// Every commercial in the catalog is 1992/kids and duration-eligible, so with no floor
	// the exact rung holds all four — the rip and the unprobed clip included.
	const wantCommercials = 4

	for _, seed := range []int64{1, 42, 1000, -7} {
		for _, policy := range []filler.Policy{{}, {MinQualityHeight: 0}} {
			// The pod is capped at PodMax, so not every eligible clip lands in one pod —
			// what matters is that the POOL was not narrowed. Coverage reads the pool
			// directly, which makes it the honest place to assert this.
			cov := filler.Coverage(cat, qualityWindow(seed), policy)
			if cov.Rungs[0].Clips != wantCommercials {
				t.Errorf("seed %d, policy %+v: exact rung holds %d commercials, want %d — an unset floor must exclude NOTHING",
					seed, policy, cov.Rungs[0].Clips, wantCommercials)
			}
			if cov.Level != filler.MatchExact {
				t.Errorf("seed %d, policy %+v: match level %q, want exact", seed, policy, cov.Level)
			}

			// And the assembled pod still draws from that unnarrowed pool.
			pod := filler.Assemble(cat, qualityWindow(seed), policy, nil)
			if pod.MatchLevel != filler.MatchExact {
				t.Errorf("seed %d, policy %+v: pod match level %q, want exact", seed, policy, pod.MatchLevel)
			}
		}
	}
}

// The floor's actual job: exclude a rip that is unwatchable rather than nostalgic.
func TestFloorExcludesBelowIt(t *testing.T) {
	pod := filler.Assemble(qualityCatalog(), qualityWindow(42), filler.Policy{MinQualityHeight: 480}, nil)

	for _, p := range paths(pod) {
		if p == "rip.mp4" {
			t.Error("a 240p clip survived a 480 floor")
		}
	}
}

// ⚠ Unknown quality is ELIGIBLE. Every clip scanned before 00014 has an empty label until the
// next sync re-probes it, so excluding unknowns would make setting a floor look like the
// catalog vanished — the opposite of the diagnosis an operator needs.
func TestFloorAdmitsUnknownQuality(t *testing.T) {
	// A floor high enough to exclude every KNOWN quality in the catalog. Whatever survives
	// can only be the unprobed clip.
	pod := filler.Assemble(qualityCatalog(), qualityWindow(42), filler.Policy{MinQualityHeight: 4000}, nil)

	var sawUnknown bool
	for _, p := range paths(pod) {
		if p == "unknown.mp4" {
			sawUnknown = true
		}
		if p == "hd.mp4" || p == "sd.mp4" || p == "rip.mp4" {
			t.Errorf("%q survived a 4000 floor", p)
		}
	}
	if !sawUnknown {
		t.Error("the unprobed clip was excluded — a floor must not empty a pre-00014 catalog")
	}
}

// The floor applies to commercials only. Bumpers are the station's bookends and the ladder's
// floor; excluding them on resolution trades a cosmetic complaint for a structural one.
func TestFloorLeavesBumpersAlone(t *testing.T) {
	pod := filler.Assemble(qualityCatalog(), qualityWindow(42), filler.Policy{MinQualityHeight: 4000}, nil)

	var sawBumper bool
	for _, p := range paths(pod) {
		if p == "bump.mp4" {
			sawBumper = true
		}
	}
	if !sawBumper {
		t.Error("a 240p bumper was excluded by the commercial quality floor")
	}
}

// Coverage must see the floor too, or the meter reports material the pods refuse to use —
// which is the V29 "lying meter" arriving through a different door.
func TestFloorIsVisibleToCoverage(t *testing.T) {
	cat := qualityCatalog()
	w := qualityWindow(42)

	open := filler.Coverage(cat, w, filler.Policy{})
	floored := filler.Coverage(cat, w, filler.Policy{MinQualityHeight: 480})

	if floored.Rungs[0].Clips >= open.Rungs[0].Clips {
		t.Errorf("exact rung has %d clips under a 480 floor and %d without it — the floor is invisible to coverage",
			floored.Rungs[0].Clips, open.Rungs[0].Clips)
	}

	// And it still agrees with assembly, which is the standing V29 invariant.
	pod := filler.Assemble(cat, w, filler.Policy{MinQualityHeight: 480}, nil)
	if floored.Level != pod.MatchLevel {
		t.Errorf("coverage says %q, assembly filled from %q under a floor", floored.Level, pod.MatchLevel)
	}
}
