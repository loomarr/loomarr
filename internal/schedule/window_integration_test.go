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
	// The dealt slice generally rotates (seed advanced by exactly one window index). We
	// don't over-assert exact contents, only that the boundary changed the seed path — the
	// digests should differ for a 48-episode deck dealt into 6h windows.
	if slotKeysDigest(cur) == slotKeysDigest(next) {
		t.Fatal("window boundary did not advance the deck (expected a rotated slice)")
	}
}
