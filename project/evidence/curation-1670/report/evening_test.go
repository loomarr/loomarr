package main

import (
	"testing"
	"time"
)

// A household that watches 6 pm to 1:30 am turns its day over at 2 am, the first hour nobody
// watches after the evening; one that never watched has nothing to learn from.
func TestTheEveningCutIsTheQuietHourTheHouseholdsViewingShows(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	var viewings []Viewing
	for day := 10; day <= 12; day++ {
		viewings = append(viewings, Viewing{
			Started: time.Date(2026, 11, day, 18, 0, 0, 0, ny),
			Ended:   time.Date(2026, 11, day+1, 1, 30, 0, 0, ny),
		})
	}
	from, to := time.Date(2026, 11, 10, 0, 0, 0, 0, ny), time.Date(2026, 11, 14, 0, 0, 0, 0, ny)

	if hour, learned := eveningCut(viewings, from, to, ny); hour != 2 || !learned {
		t.Fatalf("cut = %02d:00 learned=%t, want 02:00 learned", hour, learned)
	}
	if hour, learned := eveningCut(nil, from, to, ny); hour != 0 || learned {
		t.Fatalf("with no viewing: cut = %02d:00 learned=%t, want midnight, not learned", hour, learned)
	}
}

// An evening runs from the cut to the same hour the next day: a 1 am airing belongs to the
// evening before, a long pause does not start a new evening, and 3 am does.
func TestAnEveningRunsFromTheCutToTheCut(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	local := func(day, hour int) ClipAiring {
		return ClipAiring{Hash: "ad", AiredAt: time.Date(2026, 11, day, hour, 0, 0, 0, ny).UTC()}
	}
	got := evenings([]ClipAiring{
		local(10, 18), local(10, 19), // the start of the evening of the 10th
		local(10, 23), local(11, 1), // after a four-hour pause, and past midnight: still the 10th
		local(11, 3), local(11, 20), // the 11th
	}, ny, 2)

	if len(got) != 2 || len(got[0]) != 4 || len(got[1]) != 2 {
		sizes := make([]int, len(got))
		for i, ev := range got {
			sizes[i] = len(ev)
		}
		t.Fatalf("evening sizes = %v, want [4 2]", sizes)
	}
	if evenings(nil, ny, 2) != nil {
		t.Fatal("no airings made an evening")
	}
}

func TestAnAdSeenOnTwoChannelsInOneEveningIsARepeat(t *testing.T) {
	s := newSnapshot(Channel{ID: "x"}, Channel{ID: "y"})
	s.Viewings = []Viewing{
		{ChannelID: "x", Started: at(10, 19, 0), Ended: at(10, 23, 30)},
		{ChannelID: "x", Started: at(11, 19, 0), Ended: at(11, 23, 30)},
	}
	s.FillerAirings = []ClipAiring{
		{ChannelID: "x", Hash: "ad", AiredAt: at(10, 20, 0)},
		{ChannelID: "y", Hash: "ad", AiredAt: at(10, 22, 0)}, // same evening, another channel
		{ChannelID: "x", Hash: "ad", AiredAt: at(11, 20, 0)}, // the next evening: once only
	}

	r := Compute(s, at(10, 0, 0), at(12, 0, 0), utc)

	if h := r.Household; h.Evenings != 2 || h.EveningsWithRepeat != 1 || h.EveningCutHour != 0 || !h.EveningCutLearned {
		t.Fatalf("evenings %d, with a repeat %d, cut %02d:00 learned=%t; want 2, 1, 00:00 learned",
			h.Evenings, h.EveningsWithRepeat, h.EveningCutHour, h.EveningCutLearned)
	}
}
