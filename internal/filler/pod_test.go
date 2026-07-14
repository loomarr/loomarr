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
		{LibraryItemID: "b1", Name: "We'll be right back", Kind: filler.Bumper, Era: 1992, Audience: filler.General, DurationMs: 5000},
		{LibraryItemID: "b2", Name: "Back to the show", Kind: filler.Bumper, Era: 1992, Audience: filler.General, DurationMs: 5000},
		// 1992 kids commercials across categories.
		{LibraryItemID: "c1", Name: "Frosted Flakes", Kind: filler.Commercial, Era: 1992, Audience: filler.Kids, Category: "cereal", DurationMs: 30000},
		{LibraryItemID: "c2", Name: "TMNT figures", Kind: filler.Commercial, Era: 1992, Audience: filler.Kids, Category: "toys", DurationMs: 30000},
		{LibraryItemID: "c3", Name: "Sega Genesis", Kind: filler.Commercial, Era: 1992, Audience: filler.Kids, Category: "tech", DurationMs: 30000},
		{LibraryItemID: "c4", Name: "Capri Sun", Kind: filler.Commercial, Era: 1992, Audience: filler.Kids, Category: "fast_food", DurationMs: 15000},
		// A 1985 (different decade) kids ad + a late-night ad (wrong audience).
		{LibraryItemID: "c5", Name: "80s toy", Kind: filler.Commercial, Era: 1985, Audience: filler.Kids, Category: "toys", DurationMs: 30000},
		{LibraryItemID: "c6", Name: "Beer ad", Kind: filler.Commercial, Era: 1992, Audience: filler.LateNight, Category: "cars", DurationMs: 30000},
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
		if p1.Entries[i].LibraryItemID != p2.Entries[i].LibraryItemID {
			t.Fatalf("non-deterministic at %d: %q vs %q", i, p1.Entries[i].LibraryItemID, p2.Entries[i].LibraryItemID)
		}
	}
	// A different seed should (very likely) reorder.
	p3 := filler.Assemble(cat, kidsWindow(99), filler.Policy{}, nil)
	same := len(p3.Entries) == len(p1.Entries)
	if same {
		for i := range p1.Entries {
			if p1.Entries[i].LibraryItemID != p3.Entries[i].LibraryItemID {
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
		if e.LibraryItemID == "c5" {
			t.Error("1985 clip placed in a strict 1992 pod (wrong era)")
		}
		if e.LibraryItemID == "c6" {
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
		catByID[c.LibraryItemID] = c.Category
	}
	lastCat := ""
	for _, e := range p.Entries {
		if e.Kind != filler.Commercial {
			continue
		}
		c := catByID[e.LibraryItemID]
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
			if e.LibraryItemID != "" {
				seen[e.LibraryItemID]++
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
	cat = append(cat, filler.Clip{LibraryItemID: "bump", Kind: filler.Bumper, Era: 1992, Audience: filler.General, DurationMs: 5000})
	for i := 0; i < 10; i++ {
		cat = append(cat, filler.Clip{
			LibraryItemID: "ad" + strconv.Itoa(i), Kind: filler.Commercial, Era: 1992,
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

// --- helpers ---

func ids(p filler.Pod) []string {
	out := make([]string, len(p.Entries))
	for i, e := range p.Entries {
		out[i] = e.LibraryItemID
	}
	return out
}

// catNoFamily returns a catalog with no family/general commercials (only kids +
// late_night), so a Family window finds nothing and hits the bumper card.
func catNoFamily() []filler.Clip {
	return []filler.Clip{
		{LibraryItemID: "k1", Kind: filler.Commercial, Era: 1992, Audience: filler.Kids, Category: "toys", DurationMs: 30000},
		{LibraryItemID: "l1", Kind: filler.Commercial, Era: 1992, Audience: filler.LateNight, Category: "cars", DurationMs: 30000},
	}
}
