package schedule_test

import (
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
)

// A multi-episode series so a rolling window has something to truncate: 48 half-hour
// episodes = 24h of runtime. A 6h window keeps ~12; the whole deck is 48.
func manyEpisodes(n int) []schedule.ResolvedProgram {
	out := make([]schedule.ResolvedProgram, n)
	for i := range out {
		out[i] = schedule.ResolvedProgram{
			LibraryItemID: "ep-" + itoa(i),
			Title:         "Episode " + itoa(i),
			DurationMs:    30 * 60 * 1000, // 30 min
			Season:        1,
		}
	}
	return out
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

func windowChannel() schedule.Channel {
	return schedule.Channel{
		ID: "st", Name: "Trek", Number: 7,
		Strategy: schedule.Shuffle, Shuffle: schedule.ShuffleParams{Seed: 99},
		DefaultWindow: 6 * time.Hour,
	}
}

func slotKeysDigest(d schedule.DesiredLineup) string {
	s := ""
	for _, sl := range d.Slots {
		s += string(sl.Kind) + "|" + sl.LibraryItemID + "|" + itoa(int(sl.DurationMs)) + ";"
	}
	return s
}

// THE THRASH GUARD (plan Risk #2): two `now` values inside the SAME rolling window must
// produce a byte-identical desired lineup, so the 10-minute channel sweep re-derives the
// exact same slots and pushes NOTHING to Tunarr. The window index folds into the deck seed;
// it must be constant across a window.
func TestComputeDesiredAt_IdempotentWithinWindow(t *testing.T) {
	entries := []schedule.LineupEntry{entry("series:tvdb:100", "Trek")}
	avail := newSeriesAvail(map[string][]schedule.ResolvedProgram{
		"series:tvdb:100": manyEpisodes(48),
	})

	base := time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC)
	a := schedule.ComputeDesiredAt(windowChannel(), entries, avail, schedule.PodFill, schedule.ChannelPolicy{}, base.Add(1*time.Hour))
	b := schedule.ComputeDesiredAt(windowChannel(), entries, avail, schedule.PodFill, schedule.ChannelPolicy{}, base.Add(5*time.Hour))

	if slotKeysDigest(a) != slotKeysDigest(b) {
		t.Fatalf("desired differs within a window (thrash):\n a=%s\n b=%s", slotKeysDigest(a), slotKeysDigest(b))
	}
}

// The window actually truncates: a 6h window over 24h of episodes materializes ~6h of
// programs, NOT all 48. (The exact count depends on break interleaving, so assert it's a
// manageable horizon well under the full run — the original "800 episodes" complaint.)
func TestComputeDesiredAt_WindowTruncates(t *testing.T) {
	entries := []schedule.LineupEntry{entry("series:tvdb:100", "Trek")}
	avail := newSeriesAvail(map[string][]schedule.ResolvedProgram{
		"series:tvdb:100": manyEpisodes(48),
	})
	now := time.Date(2026, 7, 23, 3, 0, 0, 0, time.UTC)

	d := schedule.ComputeDesiredAt(windowChannel(), entries, avail, schedule.PodFill, schedule.ChannelPolicy{}, now)
	if d.ProgramCount() >= 48 {
		t.Fatalf("window did not truncate: %d programs (want a 6h slice, well under 48)", d.ProgramCount())
	}
	if d.ProgramCount() == 0 {
		t.Fatal("window truncated to nothing (never dark)")
	}
}

// Advancing across a window boundary re-deals: the next window's slice is (in general)
// different from the current one — the deck rotates so viewers don't see the same 6h loop
// forever. EligibleKeys stays the SAME (the library supplies the same series either way),
// which is what keeps the boundary crossing from reading as drift.
func TestComputeDesiredAt_WindowBoundaryAdvancesButStaysEligible(t *testing.T) {
	entries := []schedule.LineupEntry{entry("series:tvdb:100", "Trek")}
	avail := newSeriesAvail(map[string][]schedule.ResolvedProgram{
		"series:tvdb:100": manyEpisodes(48),
	})
	base := time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC)

	cur := schedule.ComputeDesiredAt(windowChannel(), entries, avail, schedule.PodFill, schedule.ChannelPolicy{}, base.Add(1*time.Hour))
	next := schedule.ComputeDesiredAt(windowChannel(), entries, avail, schedule.PodFill, schedule.ChannelPolicy{}, base.Add(7*time.Hour)) // next 6h window

	// The series key stays eligible across the boundary (no false drift).
	seriesKey := provision.Key("series:tvdb:100")
	if !cur.EligibleKeys[seriesKey] || !next.EligibleKeys[seriesKey] {
		t.Fatal("series should stay eligible across a window boundary")
	}
	// The dealt slice generally rotates (the window index advances the slice offset). We
	// don't over-assert exact contents, only that the boundary changed the slice — the
	// digests should differ for a 48-episode deck dealt into 6h windows.
	if slotKeysDigest(cur) == slotKeysDigest(next) {
		t.Fatal("window boundary did not advance the deck (expected a rotated slice)")
	}
}

// THE ROTATION FIX (the "1980s Action Heroes repeats the same films" defect): a movie pool
// bigger than the window must ROTATE THROUGH THE WHOLE CATALOG over consecutive windows —
// not loop a fixed prefix that starves the tail. 15 films × 2h = 30h; a 24h window holds 12,
// so over ceil(30h/24h)=2 windows EVERY film must air at least once. This is the regression
// guard for the exact bug: before the fix, `truncateToWindow` kept films [0..12) every window
// and films 12,13,14 never aired.
func TestComputeDesiredAt_MoviePoolRotatesThroughWholeCatalog(t *testing.T) {
	const nFilms = 15
	avail := durAvail{}
	var entries []schedule.LineupEntry
	for i := 0; i < nFilms; i++ {
		key := provision.Key("movie:tmdb:" + itoa(1000+i))
		avail[key] = struct {
			id  string
			dur int64
		}{id: "lib-" + itoa(i), dur: 2 * 60 * 60 * 1000} // 2h each
		entries = append(entries, schedule.LineupEntry{Key: key, Title: "Film " + itoa(i)})
	}
	ch := schedule.Channel{ID: "action", Name: "80s Action", Number: 3, Strategy: schedule.Sequential, DefaultWindow: 24 * time.Hour}
	// Sequential ordering is the case that starved before the fix (a stable order + a fixed
	// prefix). syndication behaves the same way; shuffle also rotates now.
	pol := schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{Ordering: "syndication"}}
	base := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)

	seen := map[string]bool{}
	// Walk a full cycle of 24h windows (2 windows cover 30h of content). Sample each window
	// once, well inside it, to collect what airs.
	for w := int64(0); w < 2; w++ {
		at := base.Add(time.Duration(w)*24*time.Hour + time.Hour)
		d := schedule.ComputeDesiredAt(ch, entries, avail, schedule.PodFill, pol, at)
		for _, s := range d.Slots {
			if s.IsProgram() {
				seen[s.Title] = true
			}
		}
	}
	if len(seen) != nFilms {
		t.Fatalf("only %d/%d films aired across a full cycle — the tail is starved (the rotation bug)", len(seen), nFilms)
	}
}

// A single window over the movie pool must be a manageable slice (~12 films of 24h), not the
// whole 15-film run — the window still bounds the horizon; rotation just moves WHICH slice.
func TestComputeDesiredAt_MovieWindowIsBounded(t *testing.T) {
	avail := durAvail{}
	var entries []schedule.LineupEntry
	for i := 0; i < 15; i++ {
		key := provision.Key("movie:tmdb:" + itoa(2000+i))
		avail[key] = struct {
			id  string
			dur int64
		}{id: "lib-" + itoa(i), dur: 2 * 60 * 60 * 1000}
		entries = append(entries, schedule.LineupEntry{Key: key, Title: "Film " + itoa(i)})
	}
	ch := schedule.Channel{ID: "a", Number: 3, Strategy: schedule.Sequential, DefaultWindow: 24 * time.Hour}
	d := schedule.ComputeDesiredAt(ch, entries, avail, schedule.PodFill, schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{Ordering: "syndication"}}, time.Date(2026, 7, 24, 1, 0, 0, 0, time.UTC))
	progs := d.ProgramCount()
	if progs < 1 || progs >= 15 {
		t.Fatalf("window materialized %d films; want a bounded slice (>=1, <15 — the whole 30h run isn't one window)", progs)
	}
}
