package schedule_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
)

// mapAvail is a pure Availability for tests: keys present in the map are
// available at the mapped library item id.
type mapAvail map[provision.Key]string

func (m mapAvail) Resolve(k provision.Key) (string, int64, bool) {
	id, ok := m[k]
	return id, 0, ok // duration 0 ⇒ ComputeDesired falls back to the entry's own
}
func (m mapAvail) ResolveEpisodes(provision.Key) ([]schedule.ResolvedProgram, bool) {
	return nil, false // movie-only fake; series expansion tested via seriesAvail
}

// durAvail is an Availability that also supplies a resolved duration, to test
// that ComputeDesired prefers the freshly-resolved runtime.
type durAvail map[provision.Key]struct {
	id  string
	dur int64
}

func (m durAvail) Resolve(k provision.Key) (string, int64, bool) {
	v, ok := m[k]
	return v.id, v.dur, ok
}
func (m durAvail) ResolveEpisodes(provision.Key) ([]schedule.ResolvedProgram, bool) {
	return nil, false
}

// seriesAvail is an Availability that expands series keys into episode programs,
// to test §9 series expansion. Movie keys resolve normally; series keys return
// their mapped episodes.
type seriesAvail struct {
	movies map[provision.Key]struct {
		id  string
		dur int64
	}
	episodes map[provision.Key][]schedule.ResolvedProgram
}

func (s seriesAvail) Resolve(k provision.Key) (string, int64, bool) {
	if k.IsSeries() {
		return "", 0, false // series resolve via ResolveEpisodes
	}
	v, ok := s.movies[k]
	return v.id, v.dur, ok
}
func (s seriesAvail) ResolveEpisodes(k provision.Key) ([]schedule.ResolvedProgram, bool) {
	eps, ok := s.episodes[k]
	return eps, ok && len(eps) > 0
}

// newSeriesAvail builds a seriesAvail from a string-keyed episode map (convenience
// for policy tests that only exercise series expansion).
func newSeriesAvail(episodes map[string][]schedule.ResolvedProgram) seriesAvail {
	m := make(map[provision.Key][]schedule.ResolvedProgram, len(episodes))
	for k, v := range episodes {
		m[provision.Key(k)] = v
	}
	return seriesAvail{episodes: m}
}

func entry(key, title string) schedule.LineupEntry {
	return schedule.LineupEntry{Key: provision.Key(key), Title: title}
}

func seqChannel() schedule.Channel {
	return schedule.Channel{ID: "ch1", Name: "Test", Number: 5, Strategy: schedule.Sequential}
}

func TestComputeDesired_Sequential_AvailableBecomesProgram(t *testing.T) {
	entries := []schedule.LineupEntry{
		entry("movie:tmdb:1", "A"),
		entry("movie:tmdb:2", "B"),
	}
	avail := mapAvail{"movie:tmdb:1": "lib-A", "movie:tmdb:2": "lib-B"}

	got := schedule.ComputeDesired(seqChannel(), entries, avail, schedule.PodFill)

	if len(got.Slots) != 2 {
		t.Fatalf("want 2 slots, got %d", len(got.Slots))
	}
	if got.ProgramCount() != 2 {
		t.Fatalf("want 2 programs, got %d", got.ProgramCount())
	}
	// Order preserved for Sequential.
	if got.Slots[0].Title != "A" || got.Slots[1].Title != "B" {
		t.Fatalf("sequential order not preserved: %+v", got.Slots)
	}
	if got.Slots[0].LibraryItemID != "lib-A" {
		t.Fatalf("want lib-A, got %q", got.Slots[0].LibraryItemID)
	}
}

func TestComputeDesiredHighlightsSelectsRatedEpisodePoolBeforeOrdering(t *testing.T) {
	key := provision.Key("series:tmdb:456")
	episodes := make([]schedule.ResolvedProgram, 0, 8)
	for episode, rating := range []float64{6.1, 9.4, 6.3, 8.9, 6.0, 9.1, 6.2, 8.8} {
		episodes = append(episodes, schedule.ResolvedProgram{
			LibraryItemID:   fmt.Sprintf("ep-%d", episode+1),
			Title:           fmt.Sprintf("Episode %d", episode+1),
			DurationMs:      22 * 60 * 1000,
			Season:          1,
			Episode:         episode + 1,
			CommunityRating: rating,
		})
	}
	got := schedule.ComputeDesiredAt(seqChannel(), []schedule.LineupEntry{{
		Key: key, Title: "The Simpsons",
		EpisodeSelection: schedule.EpisodeSelection{Mode: schedule.EpisodeHighlights},
	}}, newSeriesAvail(map[string][]schedule.ResolvedProgram{string(key): episodes}), schedule.PodFill,
		schedule.ChannelPolicy{}, time.Time{})

	var ids []string
	for _, slot := range got.Slots {
		if slot.IsProgram() {
			ids = append(ids, slot.LibraryItemID)
		}
	}
	if want := []string{"ep-2", "ep-4", "ep-6", "ep-8"}; !slices.Equal(ids, want) {
		t.Fatalf("curated highlights = %v, want stable high-rated pool %v", ids, want)
	}
}

func TestComputeDesiredHolidaySelectionUsesEpisodeEvidenceAndKeepsPartsTogether(t *testing.T) {
	key := provision.Key("series:tmdb:456")
	episodes := []schedule.ResolvedProgram{
		{LibraryItemID: "ordinary", Title: "A Regular Tuesday", DurationMs: 1, Season: 2, Episode: 1},
		{LibraryItemID: "xmas-1", Title: "A Springfield Christmas (1)", DurationMs: 1, Season: 2, Episode: 2},
		{LibraryItemID: "xmas-2", Title: "A Springfield Christmas (2)", DurationMs: 1, Season: 2, Episode: 3},
		{LibraryItemID: "overview", Title: "Home for Winter", Overview: "The family meets Santa", DurationMs: 1, Season: 2, Episode: 4},
	}
	got := schedule.ComputeDesiredAt(seqChannel(), []schedule.LineupEntry{{
		Key: key, Title: "The Simpsons", EpisodeSelection: schedule.EpisodeSelection{
			Mode: schedule.EpisodeHoliday, Holidays: []string{"christmas"},
		},
	}}, newSeriesAvail(map[string][]schedule.ResolvedProgram{string(key): episodes}), schedule.PodFill,
		schedule.ChannelPolicy{}, time.Time{})

	var ids []string
	for _, slot := range got.Slots {
		if slot.IsProgram() {
			ids = append(ids, slot.LibraryItemID)
		}
	}
	if want := []string{"xmas-1", "xmas-2", "overview"}; !slices.Equal(ids, want) {
		t.Fatalf("holiday episode pool = %v, want %v", ids, want)
	}
}

func TestComputeDesiredEpisodeSelectionFallsBackWhenEvidenceIsSparse(t *testing.T) {
	key := provision.Key("series:tmdb:456")
	episodes := []schedule.ResolvedProgram{
		{LibraryItemID: "ep-1", Title: "One", DurationMs: 1, Season: 1, Episode: 1, CommunityRating: 9.5},
		{LibraryItemID: "ep-2", Title: "Two", DurationMs: 1, Season: 1, Episode: 2},
		{LibraryItemID: "ep-3", Title: "Three", DurationMs: 1, Season: 1, Episode: 3},
	}
	got := schedule.ComputeDesiredAt(seqChannel(), []schedule.LineupEntry{{
		Key: key, Title: "Series", EpisodeSelection: schedule.EpisodeSelection{Mode: schedule.EpisodeHighlights},
	}}, newSeriesAvail(map[string][]schedule.ResolvedProgram{string(key): episodes}), schedule.PodFill,
		schedule.ChannelPolicy{}, time.Time{})
	if got.ProgramCount() != 3 {
		t.Fatalf("sparse ratings produced %d programs, want safe full-pool fallback of 3", got.ProgramCount())
	}
}

// countKind counts slots of a given kind (helper for break-insertion tests).
func countKind(slots []schedule.Slot, k schedule.SlotKind) int {
	n := 0
	for _, s := range slots {
		if s.Kind == k {
			n++
		}
	}
	return n
}

// breakChannel is a sequential channel with a break-density set (breaks/hour).
func breakChannel(breaksPerHour int) schedule.Channel {
	ch := seqChannel()
	ch.BreaksPerHour = breaksPerHour
	return ch
}

// TestComputeDesired_InsertsBreaksByRuntime: with breaks-per-hour set, the
// scheduler interleaves SlotFiller break gaps between programs at ~60/N-minute
// runtime cadence (§10). 4/hr ⇒ a break ~every 15 min of accumulated runtime.
func TestComputeDesired_InsertsBreaksByRuntime(t *testing.T) {
	// Four 15-minute programs = 60 min total. At 4 breaks/hour (every 15 min),
	// breaks fall after prog 1, 2, 3 (not after the last — no trailing break).
	min15 := int64(15 * 60 * 1000)
	avail := durAvail{
		"movie:tmdb:1": {id: "l1", dur: min15},
		"movie:tmdb:2": {id: "l2", dur: min15},
		"movie:tmdb:3": {id: "l3", dur: min15},
		"movie:tmdb:4": {id: "l4", dur: min15},
	}
	entries := []schedule.LineupEntry{
		entry("movie:tmdb:1", "A"), entry("movie:tmdb:2", "B"),
		entry("movie:tmdb:3", "C"), entry("movie:tmdb:4", "D"),
	}

	got := schedule.ComputeDesired(breakChannel(4), entries, avail, schedule.PodFill)

	if got.ProgramCount() != 4 {
		t.Fatalf("want 4 programs, got %d", got.ProgramCount())
	}
	breaks := countKind(got.Slots, schedule.SlotFiller)
	if breaks != 3 {
		t.Fatalf("want 3 break gaps (after progs 1,2,3 of 4×15min at 4/hr), got %d", breaks)
	}
	// A break must never be the last slot (no trailing dead break) and never
	// adjacent to another break.
	for i, s := range got.Slots {
		if s.Kind == schedule.SlotFiller {
			if s.DurationMs != 5*60*1000 {
				t.Errorf("break duration = %dms, want default 5m", s.DurationMs)
			}
			if i == len(got.Slots)-1 {
				t.Errorf("break is the last slot (trailing break): %+v", got.Slots)
			}
			if i > 0 && got.Slots[i-1].Kind == schedule.SlotFiller {
				t.Errorf("two breaks back-to-back at %d", i)
			}
		}
	}
}

func TestComputeDesired_UsesChannelBreakDuration(t *testing.T) {
	ch := breakChannel(4)
	ch.BreakDurationMs = 90_000
	avail := durAvail{
		"movie:tmdb:1": {id: "l1", dur: 15 * 60 * 1000},
		"movie:tmdb:2": {id: "l2", dur: 15 * 60 * 1000},
	}
	got := schedule.ComputeDesired(ch, []schedule.LineupEntry{
		entry("movie:tmdb:1", "A"), entry("movie:tmdb:2", "B"),
	}, avail, schedule.PodFill)
	if len(got.Slots) != 3 || got.Slots[1].Kind != schedule.SlotFiller {
		t.Fatalf("slots = %+v, want program / break / program", got.Slots)
	}
	if got.Slots[1].DurationMs != 90_000 {
		t.Errorf("break duration = %dms, want channel override 90000", got.Slots[1].DurationMs)
	}
}

// TestDesiredCounts_BreaksAreNotPending is the P2 status-truth guarantee: a
// fully-acquired, break-heavy channel has PendingCount 0 (so it reads healthy),
// even though its total slot count is inflated by the commercial breaks. This is
// the exact case that used to misread as "pending-slots forever".
func TestDesiredCounts_BreaksAreNotPending(t *testing.T) {
	min15 := int64(15 * 60 * 1000)
	avail := durAvail{
		"movie:tmdb:1": {id: "l1", dur: min15},
		"movie:tmdb:2": {id: "l2", dur: min15},
		"movie:tmdb:3": {id: "l3", dur: min15},
		"movie:tmdb:4": {id: "l4", dur: min15},
	}
	entries := []schedule.LineupEntry{
		entry("movie:tmdb:1", "A"), entry("movie:tmdb:2", "B"),
		entry("movie:tmdb:3", "C"), entry("movie:tmdb:4", "D"),
	}
	got := schedule.ComputeDesired(breakChannel(4), entries, avail, schedule.PodFill)

	if got.ProgramCount() != 4 {
		t.Fatalf("want 4 programs, got %d", got.ProgramCount())
	}
	if got.BreakCount() != 3 {
		t.Fatalf("want 3 commercial breaks, got %d", got.BreakCount())
	}
	if got.PendingCount() != 0 {
		t.Fatalf("a fully-acquired break-heavy channel must have 0 pending; got %d (breaks miscounted as pending titles)", got.PendingCount())
	}
	if len(got.Slots) <= got.ProgramCount() {
		t.Fatalf("slotCount (%d) should exceed programCount (%d) because of breaks — proving raw slotCount is not a readiness signal", len(got.Slots), got.ProgramCount())
	}
}

// TestDesiredCounts_PodFillPlaceholderIsPending: a not-yet-available title
// pod-fills to a SlotFiller that KEEPS its Key (pending.go). That slot is a
// pending title, not a commercial break — the Key is what tells them apart.
func TestDesiredCounts_PodFillPlaceholderIsPending(t *testing.T) {
	min15 := int64(15 * 60 * 1000)
	avail := durAvail{"movie:tmdb:1": {id: "l1", dur: min15}} // :2 absent → pod-fill pending
	entries := []schedule.LineupEntry{entry("movie:tmdb:1", "A"), entry("movie:tmdb:2", "B")}

	got := schedule.ComputeDesired(breakChannel(0), entries, avail, schedule.PodFill)

	if got.ProgramCount() != 1 {
		t.Fatalf("want 1 program, got %d", got.ProgramCount())
	}
	if got.PendingCount() != 1 {
		t.Fatalf("want 1 pending (the pod-fill placeholder), got %d", got.PendingCount())
	}
	if got.BreakCount() != 0 {
		t.Fatalf("a keyed pod-fill placeholder must not count as a break; got %d breaks", got.BreakCount())
	}
}

// TestComputeDesired_ShortProgramsFewerBreaks: duration-aware — 22-min sitcoms
// accumulate slower, so 4×22min (88 min) at 4/hr (~every 15 min) yields breaks
// after progs where the running total crosses 15/30/45/60/75 min → fewer than
// one-per-program.
func TestComputeDesired_ShortProgramsFewerBreaks(t *testing.T) {
	min22 := int64(22 * 60 * 1000)
	avail := durAvail{
		"movie:tmdb:1": {id: "l1", dur: min22},
		"movie:tmdb:2": {id: "l2", dur: min22},
	}
	entries := []schedule.LineupEntry{entry("movie:tmdb:1", "A"), entry("movie:tmdb:2", "B")}

	got := schedule.ComputeDesired(breakChannel(4), entries, avail, schedule.PodFill)

	// After prog1 (22min > 15) → break. After prog2 (would be last) → no trailing.
	// So exactly 1 break between the two sitcoms.
	if b := countKind(got.Slots, schedule.SlotFiller); b != 1 {
		t.Fatalf("2×22min sitcoms at 4/hr → want 1 break, got %d (slots: %+v)", b, got.Slots)
	}
}

// TestComputeDesired_NoBreaksWhenDensityZero: breaks-per-hour 0 (or unset) → no
// break gaps, preserving the old behavior for channels that don't want ads.
func TestComputeDesired_NoBreaksWhenDensityZero(t *testing.T) {
	min15 := int64(15 * 60 * 1000)
	avail := durAvail{"movie:tmdb:1": {id: "l1", dur: min15}, "movie:tmdb:2": {id: "l2", dur: min15}}
	entries := []schedule.LineupEntry{entry("movie:tmdb:1", "A"), entry("movie:tmdb:2", "B")}

	got := schedule.ComputeDesired(breakChannel(0), entries, avail, schedule.PodFill)

	if b := countKind(got.Slots, schedule.SlotFiller); b != 0 {
		t.Errorf("breaks-per-hour 0 → want 0 breaks, got %d", b)
	}
	if len(got.Slots) != 2 {
		t.Errorf("want 2 slots (no breaks), got %d", len(got.Slots))
	}
}

// TestComputeDesired_ProgramCarriesResolvedDuration is the regression test for
// the smoke bug: a program slot must carry the duration the resolver supplies
// (Tunarr rejects a program with duration ≤ 0). A lineup entry has no runtime of
// its own — Availability.Resolve provides it from the library.
func TestComputeDesired_ProgramCarriesResolvedDuration(t *testing.T) {
	entries := []schedule.LineupEntry{entry("movie:tmdb:603", "The Matrix")}
	avail := durAvail{"movie:tmdb:603": {id: "lib-603", dur: 8_160_000}} // 136 min

	got := schedule.ComputeDesired(seqChannel(), entries, avail, schedule.PodFill)

	if got.Slots[0].DurationMs != 8_160_000 {
		t.Fatalf("program slot duration = %d, want 8160000 (resolved from library)", got.Slots[0].DurationMs)
	}
}

// TestComputeDesired_SeriesExpandsToEpisodes is the regression test for series
// support (§9): a series lineup entry must expand into one program slot per
// episode, each with the episode's own item id + duration — NOT one slot with the
// show's (unplayable, duration-0) id.
func TestComputeDesired_SeriesExpandsToEpisodes(t *testing.T) {
	entries := []schedule.LineupEntry{entry("series:tvdb:79169", "Seinfeld")}
	avail := seriesAvail{
		episodes: map[provision.Key][]schedule.ResolvedProgram{
			"series:tvdb:79169": {
				{LibraryItemID: "ep-1", Title: "The Pilot", DurationMs: 1380000},
				{LibraryItemID: "ep-2", Title: "The Stake Out", DurationMs: 1380000},
				{LibraryItemID: "ep-3", Title: "The Robbery", DurationMs: 1380000},
			},
		},
	}

	got := schedule.ComputeDesired(seqChannel(), entries, avail, schedule.PodFill)

	if got.ProgramCount() != 3 {
		t.Fatalf("series expanded to %d programs, want 3 episodes", got.ProgramCount())
	}
	// Sequential strategy preserves episode order, each with a real duration.
	for i, wantID := range []string{"ep-1", "ep-2", "ep-3"} {
		s := got.Slots[i]
		if s.Kind != schedule.SlotProgram || s.LibraryItemID != wantID {
			t.Errorf("slot %d = {%s,%s}, want program/%s", i, s.Kind, s.LibraryItemID, wantID)
		}
		if s.DurationMs <= 0 {
			t.Errorf("episode slot %d has duration %d, want > 0 (Tunarr rejects ≤ 0)", i, s.DurationMs)
		}
	}
}

// TestComputeDesired_SeasonRangeFiltersEpisodes covers the season-range intent
// filter (§9): a series entry with SeasonMin/SeasonMax expands to only the
// in-range episodes (e.g. "old-school Simpsons", seasons 1–2 of a longer run).
func TestComputeDesired_SeasonRangeFiltersEpisodes(t *testing.T) {
	e := entry("series:tvdb:456", "The Simpsons")
	e.SeasonMin, e.SeasonMax = 1, 2 // classic era only
	avail := seriesAvail{
		episodes: map[provision.Key][]schedule.ResolvedProgram{
			"series:tvdb:456": {
				{LibraryItemID: "s1e1", Title: "S1E1", DurationMs: 1320000, Season: 1},
				{LibraryItemID: "s2e1", Title: "S2E1", DurationMs: 1320000, Season: 2},
				{LibraryItemID: "s5e1", Title: "S5E1", DurationMs: 1320000, Season: 5},    // out of range
				{LibraryItemID: "s30e1", Title: "S30E1", DurationMs: 1320000, Season: 30}, // out of range
			},
		},
	}

	got := schedule.ComputeDesired(seqChannel(), []schedule.LineupEntry{e}, avail, schedule.PodFill)

	if got.ProgramCount() != 2 {
		t.Fatalf("season range 1–2 expanded to %d programs, want 2 (s1e1, s2e1)", got.ProgramCount())
	}
	for _, s := range got.Slots {
		if s.LibraryItemID == "s5e1" || s.LibraryItemID == "s30e1" {
			t.Errorf("out-of-range episode leaked in: %s", s.LibraryItemID)
		}
	}
}

// TestComputeDesired_SeasonRangeNoMatchIsPending: a range matching no in-library
// episodes yet → a pending slot (not an empty channel).
func TestComputeDesired_SeasonRangeNoMatchIsPending(t *testing.T) {
	e := entry("series:tvdb:456", "The Simpsons")
	e.SeasonMin, e.SeasonMax = 40, 50 // seasons we don't have
	avail := seriesAvail{
		episodes: map[provision.Key][]schedule.ResolvedProgram{
			"series:tvdb:456": {{LibraryItemID: "s1e1", DurationMs: 1320000, Season: 1}},
		},
	}
	got := schedule.ComputeDesired(seqChannel(), []schedule.LineupEntry{e}, avail, schedule.ComingSoon)
	if len(got.Slots) != 1 || got.Slots[0].Kind != schedule.SlotPending {
		t.Fatalf("out-of-range series = %+v, want one pending slot", got.Slots)
	}
}

// TestComputeDesired_SeriesNoEpisodesIsPending: an available series with no
// playable episodes yet degrades to a single pending slot (not zero programs, not
// a broken push) — episodes join on a later reconcile.
func TestComputeDesired_SeriesNoEpisodesIsPending(t *testing.T) {
	entries := []schedule.LineupEntry{entry("series:tvdb:1", "New Show")}
	avail := seriesAvail{episodes: map[provision.Key][]schedule.ResolvedProgram{}} // none yet

	got := schedule.ComputeDesired(seqChannel(), entries, avail, schedule.ComingSoon)

	if len(got.Slots) != 1 || got.Slots[0].Kind != schedule.SlotPending {
		t.Fatalf("series with no episodes = %+v, want one pending slot", got.Slots)
	}
}

// TestComputeDesired_ResolvedZeroFallsBackToEntry: if the resolver reports 0
// (server had no runtime), ComputeDesired falls back to the entry's own duration.
func TestComputeDesired_ResolvedZeroFallsBackToEntry(t *testing.T) {
	e := entry("movie:tmdb:1", "A")
	e.DurationMs = 5_400_000 // 90 min known on the entry
	avail := durAvail{"movie:tmdb:1": {id: "lib-A", dur: 0}}

	got := schedule.ComputeDesired(seqChannel(), []schedule.LineupEntry{e}, avail, schedule.PodFill)

	if got.Slots[0].DurationMs != 5_400_000 {
		t.Fatalf("duration = %d, want fallback to entry's 5400000", got.Slots[0].DurationMs)
	}
}

func TestComputeDesired_PendingPodFill_KeepsKeyAsFiller(t *testing.T) {
	entries := []schedule.LineupEntry{entry("movie:tmdb:9", "Coming")}
	got := schedule.ComputeDesired(seqChannel(), entries, mapAvail{}, schedule.PodFill)

	s := got.Slots[0]
	if s.Kind != schedule.SlotFiller {
		t.Fatalf("pod-fill: want SlotFiller, got %s", s.Kind)
	}
	if s.Key != "movie:tmdb:9" {
		t.Fatalf("pod-fill must preserve key for backfill, got %q", s.Key)
	}
	if s.LibraryItemID != "" {
		t.Fatalf("pending slot must have no library item, got %q", s.LibraryItemID)
	}
	if got.ProgramCount() != 0 {
		t.Fatalf("pending slot must not count as a program")
	}
}

func TestComputeDesired_PendingComingSoon_KeepsKeyAsPending(t *testing.T) {
	entries := []schedule.LineupEntry{entry("series:tvdb:7", "Later")}
	got := schedule.ComputeDesired(seqChannel(), entries, mapAvail{}, schedule.ComingSoon)

	s := got.Slots[0]
	if s.Kind != schedule.SlotPending {
		t.Fatalf("coming-soon: want SlotPending, got %s", s.Kind)
	}
	if s.Key != "series:tvdb:7" {
		t.Fatalf("coming-soon must preserve key, got %q", s.Key)
	}
}

func TestComputeDesired_Shuffle_DeterministicUnderSeed(t *testing.T) {
	entries := []schedule.LineupEntry{
		entry("movie:tmdb:1", "A"), entry("movie:tmdb:2", "B"),
		entry("movie:tmdb:3", "C"), entry("movie:tmdb:4", "D"),
		entry("movie:tmdb:5", "E"),
	}
	avail := mapAvail{
		"movie:tmdb:1": "1", "movie:tmdb:2": "2", "movie:tmdb:3": "3",
		"movie:tmdb:4": "4", "movie:tmdb:5": "5",
	}
	ch := schedule.Channel{ID: "ch", Name: "S", Number: 1, Strategy: schedule.Shuffle, Shuffle: schedule.ShuffleParams{Seed: 42}}

	first := schedule.ComputeDesired(ch, entries, avail, schedule.PodFill)
	second := schedule.ComputeDesired(ch, entries, avail, schedule.PodFill)

	// Same seed → identical order every reconcile (§9: guide doesn't scramble).
	for i := range first.Slots {
		if first.Slots[i].Title != second.Slots[i].Title {
			t.Fatalf("shuffle not deterministic at %d: %q vs %q", i, first.Slots[i].Title, second.Slots[i].Title)
		}
	}
	// A different seed should (very likely) reorder — guards against a no-op shuffle.
	other := ch
	other.Shuffle.Seed = 99
	diff := schedule.ComputeDesired(other, entries, avail, schedule.PodFill)
	same := true
	for i := range first.Slots {
		if first.Slots[i].Title != diff.Slots[i].Title {
			same = false
			break
		}
	}
	if same {
		t.Fatalf("different seeds produced identical order; shuffle likely a no-op")
	}
}

// ⚠ `TestPlaceAvailable_*`, `TestSubstitute_*` and `TestRevalidateAgainstLibrary_*` were
// DELETED with `schedule/backfill.go` (V41). The three pure lineup mutations they covered had
// no production callers: the live backfill path is `channels.Engine.OnAvailability` →
// `Reconcile`, which recomputes the whole DesiredLineup from scratch rather than mutating it in
// place — see `channels/backfill.go`, whose comment records that recompute supersedes the point
// mutations. Their only remaining callers were these tests, which is the shape §21 calls
// "built and imported by nothing": exported, covered, and unreachable.

func TestChannelValidate(t *testing.T) {
	cases := []struct {
		name    string
		ch      schedule.Channel
		wantErr bool
	}{
		{"ok", schedule.Channel{ID: "a", Name: "n", Number: 1, Strategy: schedule.Sequential}, false},
		{"no id", schedule.Channel{Name: "n", Number: 1, Strategy: schedule.Sequential}, true},
		{"no name", schedule.Channel{ID: "a", Number: 1, Strategy: schedule.Sequential}, true},
		{"bad number", schedule.Channel{ID: "a", Name: "n", Number: 0, Strategy: schedule.Sequential}, true},
		{"bad strategy", schedule.Channel{ID: "a", Name: "n", Number: 1, Strategy: "nope"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.ch.Validate()
			if tc.wantErr != (err != nil) {
				t.Fatalf("Validate() err=%v, wantErr=%v", err, tc.wantErr)
			}
		})
	}
}
