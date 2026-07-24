package schedule_test

import (
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/schedule"
)

// Frozen clocks for seasonality assertions (§6): the container wall-clock drives
// which holiday window is active.
var (
	oct15 = time.Date(2026, time.October, 15, 12, 0, 0, 0, time.UTC)
	jul14 = time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC)
	dec20 = time.Date(2026, time.December, 20, 12, 0, 0, 0, time.UTC)
)

func seasonalChannel() schedule.Channel {
	return schedule.Channel{ID: "se", Name: "Seasonal", Number: 3, Strategy: schedule.Sequential}
}

// auto mode: an out-of-window seasonal item is BENCHED (the half viewers notice) —
// a Halloween special benched on Jul 14, aired on Oct 15.
func TestSeasonal_AutoBenchesOutOfWindow(t *testing.T) {
	entries := []schedule.LineupEntry{
		ratedEntry("movie:tmdb:1", "Halloween Spooktacular", "", 1994),
		ratedEntry("movie:tmdb:2", "Regular Show", "", 1994),
	}
	avail := mapAvail{"movie:tmdb:1": "l1", "movie:tmdb:2": "l2"}
	p := schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{Seasonal: schedule.SeasonalPolicy{Mode: schedule.SeasonalAuto}}}

	// July: the Halloween item is benched; the regular show survives.
	jul := schedule.ComputeDesiredAt(seasonalChannel(), entries, avail, schedule.PodFill, p, jul14)
	if hasKey(programKeys(jul), "movie:tmdb:1") {
		t.Error("a Halloween item must be benched in July (auto mode)")
	}
	if !hasKey(programKeys(jul), "movie:tmdb:2") {
		t.Error("a non-seasonal item must always air")
	}

	// October: the Halloween item airs (boosted, so it appears — possibly weighted up).
	oct := schedule.ComputeDesiredAt(seasonalChannel(), entries, avail, schedule.PodFill, p, oct15)
	if !hasKey(programKeys(oct), "movie:tmdb:1") {
		t.Error("a Halloween item must air in October (auto mode)")
	}
}

// off mode: no detection, no bench — a Halloween item airs year-round.
func TestSeasonal_OffMode_NoBench(t *testing.T) {
	entries := []schedule.LineupEntry{ratedEntry("movie:tmdb:1", "Halloween Special", "", 1994)}
	avail := mapAvail{"movie:tmdb:1": "l1"}
	p := schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{Seasonal: schedule.SeasonalPolicy{Mode: schedule.SeasonalOff}}}

	jul := schedule.ComputeDesiredAt(seasonalChannel(), entries, avail, schedule.PodFill, p, jul14)
	if !hasKey(programKeys(jul), "movie:tmdb:1") {
		t.Error("off mode must not bench anything, even out of window")
	}
}

// A zero clock disables seasonality entirely (the policy-free path / tests).
func TestSeasonal_ZeroClock_NoOp(t *testing.T) {
	entries := []schedule.LineupEntry{ratedEntry("movie:tmdb:1", "Christmas Special", "", 1994)}
	avail := mapAvail{"movie:tmdb:1": "l1"}
	p := schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{Seasonal: schedule.SeasonalPolicy{Mode: schedule.SeasonalAuto}}}

	d := schedule.ComputeDesiredAt(seasonalChannel(), entries, avail, schedule.PodFill, p, time.Time{})
	if !hasKey(programKeys(d), "movie:tmdb:1") {
		t.Error("a zero clock must disable seasonality (no bench)")
	}
}

// exclusive mode: in-window, ONLY seasonal content airs; out of window, the
// offSeason=dark fallback yields an empty program set (the channel goes dark).
func TestSeasonal_Exclusive(t *testing.T) {
	entries := []schedule.LineupEntry{
		ratedEntry("movie:tmdb:1", "Christmas Carol", "", 1994),
		ratedEntry("movie:tmdb:2", "Summer Blockbuster", "", 1994),
	}
	avail := mapAvail{"movie:tmdb:1": "l1", "movie:tmdb:2": "l2"}
	p := schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{Seasonal: schedule.SeasonalPolicy{
		Mode: schedule.SeasonalExclusive, Holidays: []string{"christmas"}, OffSeason: schedule.OffSeasonDark,
	}}}

	// December: only the Christmas item airs.
	dec := schedule.ComputeDesiredAt(seasonalChannel(), entries, avail, schedule.PodFill, p, dec20)
	keys := programKeys(dec)
	if !hasKey(keys, "movie:tmdb:1") || hasKey(keys, "movie:tmdb:2") {
		t.Errorf("exclusive in-window should air only seasonal content: got %v", keys)
	}

	// July (out of window, dark): no programs at all.
	jul := schedule.ComputeDesiredAt(seasonalChannel(), entries, avail, schedule.PodFill, p, jul14)
	if len(programKeys(jul)) != 0 {
		t.Errorf("exclusive+dark out of window should go dark: got %v", programKeys(jul))
	}
}
