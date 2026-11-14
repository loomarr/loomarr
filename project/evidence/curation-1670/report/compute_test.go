package main

import (
	"fmt"
	"testing"
	"time"
)

var utc = time.UTC

func at(day, hour, minute int) time.Time { return time.Date(2026, 11, day, hour, minute, 0, 0, utc) }

// episodes lays n episodes of one show back to back on a channel from start, each runtime long.
func episodes(s *Snapshot, channel, key string, start time.Time, n int, runtime time.Duration, idPrefix string) {
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("%s-%d", idPrefix, i)
		s.Items[id] = Item{Kind: "episode", Name: id, Series: key, RuntimeMs: runtime.Milliseconds()}
		t := start.Add(time.Duration(i) * runtime)
		s.Scheduled = append(s.Scheduled, ProgramAiring{ChannelID: channel, ItemID: id, Key: key, AiredAt: t, RecordedAt: t})
	}
}

func newSnapshot(channels ...Channel) Snapshot {
	return Snapshot{Channels: channels, Items: map[string]Item{}, Clips: map[string]Clip{}}
}

// Overlapping ledger rows on one channel are one slot dealt several times: one aired, the rest
// were replaced, and the day counts as re-dealt.
func TestADealReplacedOnAirIsLeftOutAndItsDayCountsAsReDealt(t *testing.T) {
	s := newSnapshot(Channel{ID: "c", Number: 7, Lineup: []LineupTitle{{Key: "series:a"}}})
	// Day 10: an ordinary day, 48 half-hour episodes. Day 11: the same 48 episodes recorded three
	// times over within the day, as a ledger does when each reconcile commits a new deal.
	episodes(&s, "c", "series:a", at(10, 0, 0), 48, 30*time.Minute, "e")
	for pass := 0; pass < 3; pass++ {
		for i := 0; i < 48; i++ {
			id := fmt.Sprintf("e-%d", i)
			ts := at(11, 0, 0).Add(time.Duration(i)*30*time.Minute + time.Duration(pass)*time.Minute)
			s.Scheduled = append(s.Scheduled, ProgramAiring{ChannelID: "c", ItemID: id, Key: "series:a", AiredAt: ts, RecordedAt: ts})
		}
	}

	r := Compute(s, at(10, 0, 0), at(12, 0, 0), utc)

	c := r.Channels[0]
	if len(c.RedealDays) != 1 || c.RedealDays[0] != "2026-11-11" || c.ChannelDays != 2 {
		t.Fatalf("re-dealt days = %v of %d, want [2026-11-11] of 2", c.RedealDays, c.ChannelDays)
	}
	// The ledger cannot say which of day 11's deals ran, so only day 10 is measured.
	if c.Scheduled.Airings != 48 || c.ReplacedDeals != 96 {
		t.Fatalf("measured airings %d, replaced deals %d; want 48 (day 10 only) and 96 (two passes of day 11)",
			c.Scheduled.Airings, c.ReplacedDeals)
	}
}

// Many items dealt at one start fill one slot; a programme running a few minutes past its slot
// does not cost the next one its airing.
func TestOneSlotDealtManyTimesAiresOnceAndASlightOverrunKeepsTheNext(t *testing.T) {
	items := map[string]Item{
		"a": {RuntimeMs: (33 * time.Minute).Milliseconds()}, // runs three minutes into the next slot
		"b": {RuntimeMs: (30 * time.Minute).Milliseconds()},
		"c": {RuntimeMs: (30 * time.Minute).Milliseconds()},
	}
	row := func(item string, ts time.Time) ProgramAiring {
		return ProgramAiring{ChannelID: "ch", ItemID: item, AiredAt: ts, RecordedAt: ts}
	}
	kept, redealt := aired([]ProgramAiring{
		row("a", at(10, 20, 0)), row("b", at(10, 20, 0)), row("c", at(10, 20, 0)), // one anchor, three deals
		row("b", at(10, 20, 30)),
	}, items, at(10, 0, 0), at(11, 0, 0), utc)
	if len(kept) != 2 || !kept[1].AiredAt.Equal(at(10, 20, 30)) || !redealt["ch"]["2026-11-10"] {
		t.Fatalf("kept %+v, re-dealt %v; want one row at 20:00 and the 20:30 row", kept, redealt)
	}
}

func TestAShowsShareOfTheDayAndShowsPerDayReadWholeDaysOnly(t *testing.T) {
	s := newSnapshot(Channel{ID: "c", Lineup: []LineupTitle{{Key: "series:a"}, {Key: "series:b"}}})
	// Day 11 is whole: 30 episodes of a, 10 of b. Days 10 and 12 are partial and say nothing.
	episodes(&s, "c", "series:a", at(11, 0, 0), 30, 30*time.Minute, "a")
	episodes(&s, "c", "series:b", at(11, 15, 0), 10, 30*time.Minute, "b")
	episodes(&s, "c", "series:b", at(10, 22, 0), 2, 30*time.Minute, "edge")

	r := Compute(s, at(10, 20, 0), at(12, 2, 0), utc)

	c := r.Channels[0]
	if c.CleanDays != 1 || c.ShowsPerDay != 2 || c.MaxShowShare != 0.75 {
		t.Fatalf("clean days %d, shows/day %d, max share %v; want 1, 2, 0.75", c.CleanDays, c.ShowsPerDay, c.MaxShowShare)
	}
}

func TestALineupTitleWithNoStartInTheSpanIsReported(t *testing.T) {
	s := newSnapshot(Channel{ID: "c", Lineup: []LineupTitle{{Key: "series:a", Title: "A"}, {Key: "series:b", Title: "B"}}})
	episodes(&s, "c", "series:a", at(10, 0, 0), 4, 30*time.Minute, "a")

	r := Compute(s, at(10, 0, 0), at(11, 0, 0), utc)

	if got := r.Channels[0].NeverScheduled; len(got) != 1 || got[0] != "B" {
		t.Fatalf("never scheduled = %v, want [B]", got)
	}
}

func TestOnlyAGuardRelaxedAwayCountsAsRelaxedToNothing(t *testing.T) {
	halved := []Relaxation{{Kind: "episodeNoRepeat", From: "168h0m0s", To: "84h0m0s"}}
	if relaxedToNothing(halved) {
		t.Fatal("a halved no-repeat window is relaxed, not gone")
	}
	for _, gone := range []Relaxation{{Kind: "seriesMinGap", From: "2h0m0s", To: "0s"}, {Kind: "blockMax", From: "2", To: "unbounded"}} {
		if !relaxedToNothing(append(halved, gone)) {
			t.Fatalf("%s to %s is a guard relaxed away", gone.Kind, gone.To)
		}
	}
}

func TestAClipIsRefusedForAMissingFactAnUnchosenRoleOrAFarEra(t *testing.T) {
	nineties := Channel{FillerEra: []YearRange{{From: 1990, To: 1999}}}
	cases := []struct {
		name    string
		clip    Clip
		channel Channel
		want    bool
	}{
		{"in era", Clip{Role: "commercial", Era: 1994}, nineties, false},
		{"neighbouring decade", Clip{Role: "commercial", Era: 1985}, nineties, false},
		{"two decades off", Clip{Role: "commercial", Era: 2016}, nineties, true},
		{"unclassified", Clip{Role: "unclassified", Era: 1994}, nineties, true},
		{"no era", Clip{Role: "commercial"}, nineties, true},
		{"political, not chosen", Clip{Role: "political", Era: 1996}, nineties, true},
		{"political, chosen", Clip{Role: "political", Era: 1996}, Channel{FillerEra: nineties.FillerEra, FillerRoles: []string{"commercial", "political"}}, false},
		{"no channel era", Clip{Role: "commercial", Era: 1972}, Channel{}, false},
	}
	for _, tc := range cases {
		if got := refused(tc.clip, tc.channel); got != tc.want {
			t.Errorf("%s: refused = %t, want %t", tc.name, got, tc.want)
		}
	}
}

func TestTheSameEpisodeOnAnotherChannelWithinAWeekIsCounted(t *testing.T) {
	s := newSnapshot(Channel{ID: "x"}, Channel{ID: "y"})
	add := func(ch, item string, ts time.Time) {
		s.Items[item] = Item{Kind: "episode", RuntimeMs: (25 * time.Minute).Milliseconds()}
		s.Scheduled = append(s.Scheduled, ProgramAiring{ChannelID: ch, ItemID: item, Key: "series:s", AiredAt: ts, RecordedAt: ts})
	}
	add("x", "ep1", at(1, 20, 0))
	add("y", "ep1", at(3, 20, 0)) // two days later on another channel: counted
	add("x", "ep2", at(1, 20, 30))
	add("x", "ep2", at(4, 20, 30)) // the same channel: not this measure
	add("y", "ep3", at(1, 21, 0))
	add("x", "ep3", at(9, 21, 0)) // eight days later: rested

	r := Compute(s, at(1, 0, 0), at(12, 0, 0), utc)

	if got := r.Household.SameEpisodeTwoChannels; got != 1 {
		t.Fatalf("same episode on two channels within a week = %d, want 1", got)
	}
}

func TestAWatchedRowOnlyASecondsLongTuneInRecordedIsCounted(t *testing.T) {
	s := newSnapshot(Channel{ID: "c"})
	s.Viewings = []Viewing{
		{ChannelID: "c", Started: at(10, 20, 0), Ended: at(10, 20, 0).Add(6 * time.Second)},
		{ChannelID: "c", Started: at(10, 21, 0), Ended: at(10, 21, 20)},
	}
	s.Watched = []ProgramAiring{
		{ChannelID: "c", ItemID: "surfed", AiredAt: at(10, 19, 50), RecordedAt: at(10, 20, 0).Add(3 * time.Second)},
		{ChannelID: "c", ItemID: "watched", AiredAt: at(10, 21, 0), RecordedAt: at(10, 21, 0).Add(10 * time.Second)},
	}

	r := Compute(s, at(10, 0, 0), at(11, 0, 0), utc)

	if got := r.Household.ShortTuneInSeen; got != 1 {
		t.Fatalf("rows recorded only by a short tune-in = %d, want 1", got)
	}
}

func TestOneAiringRecordedTwiceSecondsApartIsADuplicate(t *testing.T) {
	watched := []ProgramAiring{
		{ChannelID: "c", ItemID: "e", AiredAt: at(10, 20, 0)},
		{ChannelID: "c", ItemID: "e", AiredAt: at(10, 20, 0).Add(30 * time.Second)},
		{ChannelID: "c", ItemID: "e", AiredAt: at(12, 20, 0)}, // a real re-airing two days later
	}
	rows, dup := watchedRows(watched, "c", at(10, 0, 0), at(13, 0, 0))
	if rows != 3 || dup != 1 {
		t.Fatalf("rows %d, duplicates %d; want 3, 1", rows, dup)
	}
}

func TestInEraCommercialsCountOnlyAirableOnesFromTheHomeCountry(t *testing.T) {
	s := newSnapshot()
	s.HomeCountry = "US"
	s.Clips = map[string]Clip{
		"a": {Role: "commercial", Era: 1994, Country: "US", Placement: "break_body"},
		"b": {Role: "commercial", Era: 1994, Country: "GB", Placement: "break_body"},
		"c": {Role: "commercial", Era: 1985, Country: "US", Placement: "break_body"},
		"d": {Role: "commercial", Era: 1994, Country: "US", Placement: "not_playable"},
		"e": {Role: "commercial", Era: 1994, Country: "US", Placement: "break_body", Held: true},
		"f": {Role: "psa", Era: 1994, Country: "US", Placement: "break_body"},
	}
	if got := inEraCommercials(s, Channel{FillerEra: []YearRange{{From: 1990, To: 1999}}}); got != 1 {
		t.Fatalf("in-era commercials = %d, want 1", got)
	}
}
