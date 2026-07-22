package filler_test

import (
	"strconv"
	"testing"

	"github.com/mantonx/loomarr/internal/filler"
)

// catalog builds a small filler catalog for tests. n commercials of the given
// era/audience/category cycle, plus two bumpers.
func sampleCatalog() []filler.Clip {
	c := []filler.Clip{
		{TunarrProgramID: "b1", Name: "We'll be right back", Kind: filler.Bumper, Era: 1992, Audience: filler.General, DurationMs: 5000},
		{TunarrProgramID: "b2", Name: "Back to the show", Kind: filler.Bumper, Era: 1992, Audience: filler.General, DurationMs: 5000},
		// 1992 kids commercials across categories.
		{TunarrProgramID: "c1", Name: "Frosted Flakes", Kind: filler.Commercial, Era: 1992, Audience: filler.Kids, Category: "cereal", DurationMs: 30000},
		{TunarrProgramID: "c2", Name: "TMNT figures", Kind: filler.Commercial, Era: 1992, Audience: filler.Kids, Category: "toys", DurationMs: 30000},
		{TunarrProgramID: "c3", Name: "Sega Genesis", Kind: filler.Commercial, Era: 1992, Audience: filler.Kids, Category: "tech", DurationMs: 30000},
		{TunarrProgramID: "c4", Name: "Capri Sun", Kind: filler.Commercial, Era: 1992, Audience: filler.Kids, Category: "fast_food", DurationMs: 15000},
		// A 1985 (different decade) kids ad + a late-night ad (wrong audience).
		{TunarrProgramID: "c5", Name: "80s toy", Kind: filler.Commercial, Era: 1985, Audience: filler.Kids, Category: "toys", DurationMs: 30000},
		{TunarrProgramID: "c6", Name: "Beer ad", Kind: filler.Commercial, Era: 1992, Audience: filler.LateNight, Category: "cars", DurationMs: 30000},
	}
	return c
}

func kidsWindow(seed int64) filler.Window {
	return filler.Window{ChannelID: "ch1", Seed: seed, Era: 1992, Audience: filler.Kids, GapMs: 120000, PodMax: 4}
}

// Seeded-deterministic (§19): same catalog + same seed → identical pod.
func TestAssemble_Deterministic(t *testing.T) {
	cat := sampleCatalog()
	p1 := filler.Assemble(cat, kidsWindow(42), filler.Policy{}, nil)
	p2 := filler.Assemble(cat, kidsWindow(42), filler.Policy{}, nil)
	if len(p1.Entries) != len(p2.Entries) {
		t.Fatalf("non-deterministic length: %d vs %d", len(p1.Entries), len(p2.Entries))
	}
	for i := range p1.Entries {
		if p1.Entries[i].TunarrProgramID != p2.Entries[i].TunarrProgramID {
			t.Fatalf("non-deterministic at %d: %q vs %q", i, p1.Entries[i].TunarrProgramID, p2.Entries[i].TunarrProgramID)
		}
	}
	// A different seed should (very likely) reorder.
	p3 := filler.Assemble(cat, kidsWindow(99), filler.Policy{}, nil)
	same := len(p3.Entries) == len(p1.Entries)
	if same {
		for i := range p1.Entries {
			if p1.Entries[i].TunarrProgramID != p3.Entries[i].TunarrProgramID {
				same = false
				break
			}
		}
	}
	if same {
		t.Error("different seeds produced identical pods; seeding likely a no-op")
	}
}

// Era + audience matching (§10): exact-era kids ads are chosen; the wrong-audience
// (late_night) and wrong-decade (1985) clips are NOT in an exact match.
func TestAssemble_EraAudienceMatch(t *testing.T) {
	p := filler.Assemble(sampleCatalog(), kidsWindow(1), filler.Policy{EraStrict: true}, nil)
	if p.MatchLevel != filler.MatchExact {
		t.Fatalf("expected exact match, got %s", p.MatchLevel)
	}
	for _, e := range p.Entries {
		if e.TunarrProgramID == "c5" {
			t.Error("1985 clip placed in a strict 1992 pod (wrong era)")
		}
		if e.TunarrProgramID == "c6" {
			t.Error("late-night clip placed in a kids pod (wrong audience)")
		}
	}
}

// Category variety (§10): no two consecutive commercials share a category.
func TestAssemble_CategoryVariety(t *testing.T) {
	// Build a catalog where exact matches span 4 categories, so a full pod can be
	// varied.
	cat := sampleCatalog()
	p := filler.Assemble(cat, kidsWindow(7), filler.Policy{}, nil)
	catByID := map[string]string{}
	for _, c := range cat {
		catByID[c.TunarrProgramID] = c.Category
	}
	lastCat := ""
	for _, e := range p.Entries {
		if e.Kind != filler.Commercial {
			continue
		}
		c := catByID[e.TunarrProgramID]
		if c != "" && c == lastCat {
			t.Errorf("category %q played back-to-back (variety violated): pod %+v", c, ids(p))
		}
		lastCat = c
	}
}

// No-repeat within a window (§10): threading `used` across pods never re-plays a clip.
func TestAssemble_NoRepeatInWindow(t *testing.T) {
	cat := sampleCatalog()
	used := map[string]bool{}
	seen := map[string]int{}
	// Assemble several pods in one window.
	for i := 0; i < 3; i++ {
		w := kidsWindow(int64(100 + i))
		p := filler.Assemble(cat, w, filler.Policy{}, used)
		for _, e := range p.Entries {
			if e.TunarrProgramID != "" {
				seen[e.TunarrProgramID]++
			}
		}
	}
	for id, n := range seen {
		if n > 1 {
			t.Errorf("clip %s repeated %d times within the window (no-repeat violated)", id, n)
		}
	}
}

// Fallback ladder (§10): a window whose exact era has no ads widens; a window with
// NO matching ads at all degrades to the embedded bumper card — never empty.
func TestAssemble_FallbackLadder(t *testing.T) {
	cat := sampleCatalog()

	// A 1970s kids window: no exact/decade match, but kids ads exist → audience level.
	w70 := filler.Window{ChannelID: "ch", Seed: 5, Era: 1975, Audience: filler.Kids, GapMs: 120000, PodMax: 4}
	p70 := filler.Assemble(cat, w70, filler.Policy{}, nil)
	if p70.MatchLevel != filler.MatchAudience {
		t.Errorf("1975 kids window should fall to audience level, got %s", p70.MatchLevel)
	}
	// A late_night window with only one late-night ad still fills; an audience with
	// NO ads at all → bumper card.
	wNone := filler.Window{ChannelID: "ch", Seed: 5, Era: 1992, Audience: filler.Family, GapMs: 120000, PodMax: 4}
	pNone := filler.Assemble(catNoFamily(), wNone, filler.Policy{}, nil)
	if pNone.MatchLevel != filler.MatchBumperCard {
		t.Fatalf("a window with no matching ads should be bumper-card-only, got %s", pNone.MatchLevel)
	}
	// The pod is never empty — it has the fallback card.
	var hasCard bool
	for _, e := range pNone.Entries {
		if e.IsFallbackCard {
			hasCard = true
		}
	}
	if !hasCard {
		t.Error("bumper-card fallback missing — a break must never be dead air (§10)")
	}
}

// Density (§10): the pod respects PodMax.
func TestAssemble_RespectsPodMax(t *testing.T) {
	// Many eligible clips, PodMax=2 → at most 2 commercials.
	var cat []filler.Clip
	cat = append(cat, filler.Clip{TunarrProgramID: "bump", Kind: filler.Bumper, Era: 1992, Audience: filler.General, DurationMs: 5000})
	for i := 0; i < 10; i++ {
		cat = append(cat, filler.Clip{
			TunarrProgramID: "ad" + strconv.Itoa(i), Kind: filler.Commercial, Era: 1992,
			Audience: filler.Kids, Category: "cat" + strconv.Itoa(i), DurationMs: 30000,
		})
	}
	w := filler.Window{ChannelID: "ch", Seed: 3, Era: 1992, Audience: filler.Kids, GapMs: 600000, PodMax: 2}
	p := filler.Assemble(cat, w, filler.Policy{}, nil)
	commercials := 0
	for _, e := range p.Entries {
		if e.Kind == filler.Commercial {
			commercials++
		}
	}
	if commercials > 2 {
		t.Errorf("PodMax=2 exceeded: %d commercials", commercials)
	}
}

// --- per-channel selection (§10): categories / kinds / exclude / pin ---

// containsID reports whether the pod includes a clip id (any kind).
func containsID(p filler.Pod, id string) bool {
	for _, e := range p.Entries {
		if e.TunarrProgramID == id {
			return true
		}
	}
	return false
}

// A category selection narrows the commercial pool to just those categories — a "toys +
// cereal" channel never plays the tech/fast-food ads, even though they'd otherwise match.
func TestAssemble_CategorySelectionNarrowsPool(t *testing.T) {
	w := kidsWindow(7)
	w.Categories = []string{"toys", "cereal"}
	w.PodMax = 8
	p := filler.Assemble(sampleCatalog(), w, filler.Policy{}, nil)
	for _, e := range p.Entries {
		if e.Kind != filler.Commercial {
			continue
		}
		if e.TunarrProgramID != "c1" && e.TunarrProgramID != "c2" { // cereal, toys
			t.Errorf("category selection admitted %q (not toys/cereal)", e.TunarrProgramID)
		}
	}
	// The narrowing is real: the tech ad (c3) is excluded.
	if containsID(p, "c3") {
		t.Error("tech ad c3 played despite a toys+cereal category selection")
	}
}

// A kinds selection shapes the whole pod: "bumpers only" yields no commercials at all
// (the pod is bumpers), because kinds is applied catalog-wide before assembly.
func TestAssemble_KindsSelectionBumpersOnly(t *testing.T) {
	w := kidsWindow(7)
	w.Kinds = []string{"bumper"}
	p := filler.Assemble(sampleCatalog(), w, filler.Policy{}, nil)
	for _, e := range p.Entries {
		if e.Kind == filler.Commercial {
			t.Errorf("a bumpers-only selection still placed a commercial: %q", e.TunarrProgramID)
		}
	}
}

// Excluding a clip removes it from the pod entirely — it never appears as a commercial,
// even when it would otherwise be the tightest match.
func TestAssemble_ExcludeRemovesClip(t *testing.T) {
	w := kidsWindow(7)
	w.PodMax = 8
	w.Excluded = []string{"c2"} // exclude the TMNT toys ad
	p := filler.Assemble(sampleCatalog(), w, filler.Policy{}, nil)
	if containsID(p, "c2") {
		t.Error("an excluded clip (c2) still played")
	}
	// Other kids ads still play — exclude is surgical, not a blanket empty.
	if !containsID(p, "c1") && !containsID(p, "c3") {
		t.Error("exclude removed more than the one excluded clip")
	}
}

// Pinning forces a clip in even when it wouldn't otherwise rank — here a wrong-decade
// (1985) clip that the exact-1992 ladder would never pick.
func TestAssemble_PinForcesInOffLadderClip(t *testing.T) {
	w := kidsWindow(7)
	w.Pinned = []string{"c5"} // the 1985 toy ad — wrong decade for a 1992 exact match
	p := filler.Assemble(sampleCatalog(), w, filler.Policy{EraStrict: true}, nil)
	if !containsID(p, "c5") {
		t.Error("a pinned off-ladder clip (c5, 1985) was not forced into the 1992 pod")
	}
}

// Exclude WINS over pin: a clip that is both pinned and excluded is dropped (the safe
// default — exclude is the stronger, "never play this" intent).
func TestAssemble_ExcludeBeatsPin(t *testing.T) {
	w := kidsWindow(7)
	w.Pinned = []string{"c1"}
	w.Excluded = []string{"c1"}
	p := filler.Assemble(sampleCatalog(), w, filler.Policy{}, nil)
	if containsID(p, "c1") {
		t.Error("a clip both pinned and excluded played — exclude must win")
	}
}

// A pin still respects the pod budget: a long pin list can't produce an unbounded pod.
func TestAssemble_PinRespectsPodMax(t *testing.T) {
	var cat []filler.Clip
	for i := 0; i < 6; i++ {
		cat = append(cat, filler.Clip{
			TunarrProgramID: "pin" + strconv.Itoa(i), Kind: filler.Commercial,
			Era: 1992, Audience: filler.Kids, Category: "cat" + strconv.Itoa(i), DurationMs: 30000,
		})
	}
	w := filler.Window{ChannelID: "ch", Seed: 1, Era: 1992, Audience: filler.Kids, GapMs: 600000, PodMax: 2}
	w.Pinned = []string{"pin0", "pin1", "pin2", "pin3"} // 4 pins, PodMax 2
	p := filler.Assemble(cat, w, filler.Policy{}, nil)
	commercials := 0
	for _, e := range p.Entries {
		if e.Kind == filler.Commercial {
			commercials++
		}
	}
	if commercials > 2 {
		t.Errorf("pins exceeded PodMax=2: %d commercials", commercials)
	}
}

// An empty selection == the prior behaviour: the whole catalog, era/audience-matched.
// (Regression guard so the additive default never silently changes pods.)
func TestAssemble_EmptySelectionUnchanged(t *testing.T) {
	cat := sampleCatalog()
	base := filler.Assemble(cat, kidsWindow(42), filler.Policy{}, nil)
	// kidsWindow has no Categories/Kinds/Pinned/Excluded set — so it IS the empty
	// selection. Assert it still produces a real (non-fallback) pod with commercials.
	got := 0
	for _, e := range base.Entries {
		if e.Kind == filler.Commercial {
			got++
		}
	}
	if got == 0 {
		t.Error("empty selection produced no commercials — the additive default regressed")
	}
}

// --- helpers ---

func ids(p filler.Pod) []string {
	out := make([]string, len(p.Entries))
	for i, e := range p.Entries {
		out[i] = e.TunarrProgramID
	}
	return out
}

// catNoFamily returns a catalog with no family/general commercials (only kids +
// late_night), so a Family window finds nothing and hits the bumper card.
func catNoFamily() []filler.Clip {
	return []filler.Clip{
		{TunarrProgramID: "k1", Kind: filler.Commercial, Era: 1992, Audience: filler.Kids, Category: "toys", DurationMs: 30000},
		{TunarrProgramID: "l1", Kind: filler.Commercial, Era: 1992, Audience: filler.LateNight, Category: "cars", DurationMs: 30000},
	}
}
