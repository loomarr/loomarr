package schedule

import (
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
)

func movieEntry(key string, year, collectionID int) LineupEntry {
	return LineupEntry{Key: provision.Key(key), Title: key, Year: year, CollectionID: collectionID}
}

// assignFranchiseGroups: movies sharing a TMDB collection group together in RELEASE-YEAR
// order; a collection with <2 present films isn't grouped; series never group.
func TestAssignFranchiseGroups(t *testing.T) {
	entries := []LineupEntry{
		// Indiana Jones (collection 84), deliberately out of release order + interleaved.
		movieEntry("movie:tmdb:87", 1984, 84),                                  // Temple of Doom
		movieEntry("movie:tmdb:210577", 2001, 0),                               // a standalone
		movieEntry("movie:tmdb:85", 1981, 84),                                  // Raiders (FIRST film, no shared title)
		movieEntry("movie:tmdb:89", 1989, 84),                                  // Last Crusade
		movieEntry("movie:tmdb:999", 1990, 555),                                // lone film of another franchise
		LineupEntry{Key: "series:tvdb:1", Title: "A Series", CollectionID: 84}, // series ignored
	}
	tags := assignFranchiseGroups(entries)

	// The three Indy films share one group, indexed by release year (Raiders 1 → Temple 2 → Crusade 3).
	g := tags["movie:tmdb:85"]
	if g.group == "" {
		t.Fatal("Raiders should be in a franchise group")
	}
	if tags["movie:tmdb:87"].group != g.group || tags["movie:tmdb:89"].group != g.group {
		t.Error("all three Indy films must share one group")
	}
	if tags["movie:tmdb:85"].index != 1 || tags["movie:tmdb:87"].index != 2 || tags["movie:tmdb:89"].index != 3 {
		t.Errorf("release order wrong: Raiders=%d Temple=%d Crusade=%d",
			tags["movie:tmdb:85"].index, tags["movie:tmdb:87"].index, tags["movie:tmdb:89"].index)
	}
	// A standalone, a lone-franchise film, and a series are NOT grouped.
	if _, ok := tags["movie:tmdb:210577"]; ok {
		t.Error("standalone must not group")
	}
	if _, ok := tags["movie:tmdb:999"]; ok {
		t.Error("a lone film of a franchise must not group (nothing to keep it with)")
	}
	if _, ok := tags["series:tvdb:1"]; ok {
		t.Error("a series must never franchise-group")
	}
}

// The end-to-end guarantee: a franchise stays adjacent + in release order through the
// shuffle deck (collapse/expand), never split by an unrelated film — the exact bug reported
// (Temple → Crusade → [Top Gun] → Raiders).
func TestFranchiseStaysTogetherUnderShuffle(t *testing.T) {
	entries := []LineupEntry{
		movieEntry("movie:tmdb:85", 1981, 84), // Raiders
		movieEntry("movie:tmdb:87", 1984, 84), // Temple
		movieEntry("movie:tmdb:89", 1989, 84), // Crusade
		movieEntry("movie:tmdb:t1", 1986, 0),  // Top Gun (standalone)
		movieEntry("movie:tmdb:t2", 1989, 0),  // Batman (standalone)
	}
	tags := assignFranchiseGroups(entries)
	// Build movie slots carrying the franchise tags (as resolveEntry would).
	slots := make([]Slot, 0, len(entries))
	for _, e := range entries {
		s := Slot{Kind: SlotProgram, Key: e.Key, LibraryItemID: string(e.Key), Title: string(e.Key), DurationMs: 90 * 60 * 1000}
		if tag, ok := tags[e.Key]; ok {
			s.PartGroup, s.PartIndex = tag.group, tag.index
		}
		slots = append(slots, s)
	}
	rp := ChannelPolicy{ProposalPolicy: ProposalPolicy{Ordering: OrderShuffle}}.Resolved(Shuffle, false)
	for seed := int64(0); seed < 50; seed++ {
		collapsed, expand := collapseGroups(slots)
		ordered, _ := slotWithRelaxation(collapsed, rp, seed)
		ordered = expandGroups(ordered, expand)
		// Find the three Indy films; they must be at consecutive positions, in release order.
		pos := map[string]int{}
		for i, s := range ordered {
			pos[string(s.Key)] = i
		}
		r, tm, c := pos["movie:tmdb:85"], pos["movie:tmdb:87"], pos["movie:tmdb:89"]
		if tm != r+1 || c != r+2 {
			t.Fatalf("seed %d: franchise split/out-of-order: Raiders=%d Temple=%d Crusade=%d", seed, r, tm, c)
		}
	}
}

// The rotating window must never split a franchise (or two-parter) at the window seam:
// because windowSlice runs on the COLLAPSED deck (a franchise is one super-slot), it's kept
// whole or excluded whole, at every window offset. Guards the plan Risk #2 for the window.
func TestFranchiseNeverSplitByWindowSeam(t *testing.T) {
	entries := []LineupEntry{
		movieEntry("movie:tmdb:85", 1981, 84), // Raiders  ┐
		movieEntry("movie:tmdb:87", 1984, 84), // Temple   │ Indy franchise (collection 84)
		movieEntry("movie:tmdb:89", 1989, 84), // Crusade  ┘
		movieEntry("movie:tmdb:s1", 1986, 0),
		movieEntry("movie:tmdb:s2", 1988, 0),
		movieEntry("movie:tmdb:s3", 1987, 0),
	}
	tags := assignFranchiseGroups(entries)
	slots := make([]Slot, 0, len(entries))
	for _, e := range entries {
		s := Slot{Kind: SlotProgram, Key: e.Key, LibraryItemID: string(e.Key), Title: string(e.Key), DurationMs: 2 * 60 * 60 * 1000} // 2h each
		if tag, ok := tags[e.Key]; ok {
			s.PartGroup, s.PartIndex = tag.group, tag.index
		}
		slots = append(slots, s)
	}
	rp := ChannelPolicy{ProposalPolicy: ProposalPolicy{Ordering: OrderSyndication}}.Resolved(Sequential, false)
	collapsed, expand := collapseGroups(slots)
	ordered, _ := slotWithRelaxation(collapsed, rp, 7)
	indy := map[string]bool{"movie:tmdb:85": true, "movie:tmdb:87": true, "movie:tmdb:89": true}
	// At every window offset (5h window, so the franchise's 6h block straddles a seam if split),
	// the Indy films appear either ALL or NONE — never a lone member orphaned by truncation.
	for idx := int64(0); idx < int64(len(entries))+2; idx++ {
		sliced := windowSlice(ordered, 5*time.Hour, idx)
		sliced = expandGroups(sliced, expand)
		n := 0
		for _, s := range sliced {
			if indy[string(s.Key)] {
				n++
			}
		}
		if n != 0 && n != 3 {
			t.Fatalf("index %d: franchise split by the window seam — %d/3 Indy films present", idx, n)
		}
	}
}

func TestPartMarker(t *testing.T) {
	cases := []struct {
		title    string
		wantBase string
		wantPart int
		wantOK   bool
	}{
		{"All Good Things... (2)", "All Good Things...", 2, true},
		{"Encounter at Farpoint (1)", "Encounter at Farpoint", 1, true},
		{"The Best of Both Worlds, Part 1", "The Best of Both Worlds,", 1, true},
		{"Chain of Command Part II", "Chain of Command", 2, true},
		{"Unification Pt. 2", "Unification", 2, true},
		{"Time's Arrow (10)", "Time's Arrow", 10, true},
		// Not part markers:
		{"The Inner Light", "", 0, false},
		{"Cause and Effect", "", 0, false},
		{"Ship in a Bottle (Reprise)", "", 0, false}, // non-numeric parenthetical
		{"Part of the Journey", "", 0, false},        // "part" not a trailing marker
	}
	for _, c := range cases {
		t.Run(c.title, func(t *testing.T) {
			base, part, ok := partMarker(c.title)
			if ok != c.wantOK || part != c.wantPart || (ok && base != c.wantBase) {
				t.Errorf("partMarker(%q) = (%q,%d,%v), want (%q,%d,%v)",
					c.title, base, part, ok, c.wantBase, c.wantPart, c.wantOK)
			}
		})
	}
}

func rp(title string, season, ep, epEnd int) ResolvedProgram {
	return ResolvedProgram{
		LibraryItemID: title, Title: title, DurationMs: 45 * 60 * 1000,
		Season: season, Episode: ep, EpisodeEnd: epEnd,
	}
}

func TestAssignPartGroups_TitleSuffix(t *testing.T) {
	progs := []ResolvedProgram{
		rp("The Inner Light", 5, 25, 0),
		rp("Time's Arrow (1)", 5, 26, 0),
		rp("Time's Arrow (2)", 6, 1, 0), // note: (2) is next season's opener — still consecutive by our list order
		rp("Realm of Fear", 6, 2, 0),
	}
	assignPartGroups("series:tvdb:71470", progs)

	// The standalone episodes get no group.
	if progs[0].PartGroup != "" || progs[3].PartGroup != "" {
		t.Errorf("standalone episodes should have no PartGroup: %q, %q", progs[0].PartGroup, progs[3].PartGroup)
	}
	// A cross-season two-parter is NOT grouped: the season differs, so our same-season guard
	// (deliberately conservative) leaves them ungrouped rather than mis-glue across a season.
	if progs[1].PartGroup != "" || progs[2].PartGroup != "" {
		t.Errorf("cross-season parts must not group (same-season guard): %q, %q", progs[1].PartGroup, progs[2].PartGroup)
	}
}

func TestAssignPartGroups_SameSeasonConsecutive(t *testing.T) {
	progs := []ResolvedProgram{
		rp("Chain of Command (1)", 6, 10, 0),
		rp("Chain of Command (2)", 6, 11, 0),
		rp("Ship in a Bottle", 6, 12, 0),
	}
	assignPartGroups("series:tvdb:71470", progs)

	if progs[0].PartGroup == "" || progs[0].PartGroup != progs[1].PartGroup {
		t.Fatalf("the two parts should share one group: %q vs %q", progs[0].PartGroup, progs[1].PartGroup)
	}
	if progs[0].PartIndex != 1 || progs[1].PartIndex != 2 {
		t.Errorf("part indices wrong: %d, %d", progs[0].PartIndex, progs[1].PartIndex)
	}
	if progs[2].PartGroup != "" {
		t.Error("the standalone episode must not be grouped")
	}
}

func TestAssignPartGroups_NonConsecutiveRejected(t *testing.T) {
	// Same base + part markers but NOT consecutive episodes → not grouped (guards against
	// unrelated "Part 1" episodes elsewhere in a season).
	progs := []ResolvedProgram{
		rp("Trials and Tribble-ations (1)", 5, 6, 0),
		rp("Something Else", 5, 7, 0),
		rp("Trials and Tribble-ations (2)", 5, 8, 0),
	}
	assignPartGroups("series:tvdb:72073", progs)
	for i, p := range progs {
		if p.PartGroup != "" {
			t.Errorf("non-consecutive parts must not group (idx %d got %q)", i, p.PartGroup)
		}
	}
}

func TestAssignPartGroups_IndexNumberEnd(t *testing.T) {
	// A single file spanning episodes 25–26 (IndexNumberEnd) is tagged atomic on its own.
	progs := []ResolvedProgram{rp("The Menagerie", 1, 25, 26)}
	assignPartGroups("series:tvdb:77526", progs)
	if progs[0].PartGroup == "" || progs[0].PartIndex != 1 {
		t.Errorf("single-file span should be tagged: group=%q idx=%d", progs[0].PartGroup, progs[0].PartIndex)
	}
}

func TestCollapseExpand_RoundTripAndAtomicDuration(t *testing.T) {
	slots := []Slot{
		{Kind: SlotProgram, Key: "k", Title: "Standalone A", DurationMs: 1000},
		{Kind: SlotProgram, Key: "k", Title: "Two-Parter (1)", DurationMs: 1000, PartGroup: "g1", PartIndex: 1},
		{Kind: SlotProgram, Key: "k", Title: "Two-Parter (2)", DurationMs: 1000, PartGroup: "g1", PartIndex: 2},
		{Kind: SlotProgram, Key: "k", Title: "Standalone B", DurationMs: 1000},
	}
	collapsed, expand := collapseGroups(slots)

	// The group collapses to ONE super-slot with the combined duration.
	if len(collapsed) != 3 {
		t.Fatalf("collapsed len = %d, want 3 (A, group, B)", len(collapsed))
	}
	if collapsed[1].PartGroup != "g1" || collapsed[1].DurationMs != 2000 {
		t.Errorf("super-slot wrong: group=%q dur=%d (want g1/2000)", collapsed[1].PartGroup, collapsed[1].DurationMs)
	}
	// Expand restores the real parts, adjacent + in order.
	out := expandGroups(collapsed, expand)
	if len(out) != 4 || out[1].Title != "Two-Parter (1)" || out[2].Title != "Two-Parter (2)" {
		t.Errorf("expand didn't restore ordered parts: %+v", titles(out))
	}
}

// The end-to-end guarantee: even with a shuffle strategy + a tight rolling window, a
// two-parter stays adjacent + in-order and is never split by the window.
func TestComputeDesired_MultiPartStaysTogether(t *testing.T) {
	// A same-season consecutive two-parter ("Gambit") plus standalones, so the shuffle has
	// room to scatter the parts if it could — but collapse/expand keep them atomic.
	seriesEps := []ResolvedProgram{
		rp("Standalone 1", 6, 1, 0),
		rp("Gambit (1)", 7, 4, 0),
		rp("Gambit (2)", 7, 5, 0),
		rp("Standalone 2", 7, 6, 0),
	}
	assignPartGroups("series:tvdb:71470", seriesEps)
	slots := make([]Slot, 0, len(seriesEps))
	for _, e := range seriesEps {
		slots = append(slots, Slot{Kind: SlotProgram, Key: "series:tvdb:71470", LibraryItemID: e.LibraryItemID, Title: e.Title, DurationMs: e.DurationMs, PartGroup: e.PartGroup, PartIndex: e.PartIndex})
	}

	// Run collapse → shuffle deck → expand over many seeds; parts must always be adjacent + ordered.
	rp2 := ChannelPolicy{ProposalPolicy: ProposalPolicy{Ordering: OrderShuffle}}.Resolved(Shuffle, false)
	for seed := int64(0); seed < 50; seed++ {
		collapsed, expand := collapseGroups(slots)
		ordered, _ := slotWithRelaxation(collapsed, rp2, seed)
		ordered = expandGroups(ordered, expand)
		assertPartsAdjacent(t, ordered, "Gambit (1)", "Gambit (2)", seed)
	}
}

// The window must keep a two-parter WHOLE: because a group collapses to one super-slot with
// the combined runtime, truncateToWindow either keeps both parts or drops both — never a
// lone Part 2. Here a 1h window over [group(2×45m=90m), standalone(45m)]: the group alone
// exceeds the window, so it's kept whole (≥1 program floor) and expands to both parts.
func TestMultiPart_WindowKeepsPairWhole(t *testing.T) {
	slots := []Slot{
		{Kind: SlotProgram, Key: "k", LibraryItemID: "p1", Title: "Two-Parter (1)", DurationMs: 45 * 60 * 1000, PartGroup: "g", PartIndex: 1},
		{Kind: SlotProgram, Key: "k", LibraryItemID: "p2", Title: "Two-Parter (2)", DurationMs: 45 * 60 * 1000, PartGroup: "g", PartIndex: 2},
		{Kind: SlotProgram, Key: "k", LibraryItemID: "s", Title: "Standalone", DurationMs: 45 * 60 * 1000},
	}
	collapsed, expand := collapseGroups(slots)
	ordered := windowSlice(collapsed, time.Hour, 0) // 1h window; group (90m) exceeds it → kept whole
	ordered = expandGroups(ordered, expand)

	// Both parts present (never a lone Part 2), adjacent + in order.
	assertPartsAdjacent(t, ordered, "Two-Parter (1)", "Two-Parter (2)", 0)
}

func titles(slots []Slot) []string {
	out := make([]string, len(slots))
	for i, s := range slots {
		out[i] = s.Title
	}
	return out
}

func assertPartsAdjacent(t *testing.T, slots []Slot, p1, p2 string, seed int64) {
	t.Helper()
	i1, i2 := -1, -1
	for i, s := range slots {
		if s.Title == p1 {
			i1 = i
		}
		if s.Title == p2 {
			i2 = i
		}
	}
	if i1 < 0 || i2 < 0 {
		t.Fatalf("seed %d: parts missing (%q@%d, %q@%d) in %v", seed, p1, i1, p2, i2, titles(slots))
	}
	if i2 != i1+1 {
		t.Errorf("seed %d: parts not adjacent+ordered: %q@%d, %q@%d in %v", seed, p1, i1, p2, i2, titles(slots))
	}
}
