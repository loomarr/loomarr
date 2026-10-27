package filler_test

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/clipcatalog"
	"github.com/loomarr/loomarr/internal/filler"
)

var (
	sizedAt  = time.Date(2026, 10, 27, 23, 0, 0, 0, time.UTC)
	eighties = []filler.EraRange{{From: 1980, To: 1989}}
	nineties = []filler.EraRange{{From: 1990, To: 1999}}
)

// evening airs n ads on a channel from start, one every four and a half minutes.
func evening(channel string, start time.Time, n int) []clipcatalog.Airing {
	out := make([]clipcatalog.Airing, n)
	for i := range out {
		at := start.Add(time.Duration(i) * 270 * time.Second)
		out[i] = clipcatalog.Airing{ChannelID: channel, ClipHash: "c", AiredAt: at, RecordedAt: at}
	}
	return out
}

func TestSizePool_ADefaultEveningUntilTheHouseholdHasFourWeeks(t *testing.T) {
	windows := map[string][]filler.EraRange{"a": eighties}
	got := filler.SizePool(evening("a", sizedAt.Add(-27*24*time.Hour), 120), windows, 4*time.Hour, sizedAt)
	if got.FromHistory || got.Floor != 80 {
		t.Fatalf("three weeks of history sized %+v, want the default evening of 80", got)
	}
	if got := filler.SizePool(nil, windows, 8*time.Hour, sizedAt); got.Floor != 107 {
		t.Fatalf("a spacing longer than the default evening sized %+v, want its own 107", got)
	}
}

// Rotation rests an ad household-wide, so one evening across two channels of one era needs both
// channels' ads; a silence of over two hours starts another evening.
func TestSizePool_EachDecadeTakesItsLongestEvening(t *testing.T) {
	windows := map[string][]filler.EraRange{"a": eighties, "b": eighties, "c": nineties}
	start := sizedAt.Add(-5 * 7 * 24 * time.Hour)
	var airings []clipcatalog.Airing
	airings = append(airings, evening("a", start, 10)...) // the history's first night
	night := sizedAt.Add(-3 * 24 * time.Hour)
	airings = append(airings, evening("a", night, 40)...)
	airings = append(airings, evening("b", night.Add(40*270*time.Second), 50)...)
	airings = append(airings, evening("c", night.Add(3*time.Hour), 70)...)     // still the same evening
	airings = append(airings, evening("a", sizedAt.Add(-24*time.Hour), 60)...) // a shorter one
	airings = append(airings, evening("gone", night, 500)...)                  // a deleted channel
	airings = append(airings, evening("a", sizedAt.Add(-9*7*24*time.Hour), 200)...)

	got := filler.SizePool(airings, windows, 4*time.Hour, sizedAt)
	if !got.FromHistory || got.Floor != 54 {
		t.Fatalf("sized %+v, want from history with the 4h spacing floor of 54", got)
	}
	if got.PerDecade[1980] != 90 || got.PerDecade[1990] != 70 {
		t.Fatalf("per decade = %v, want 90 for the eighties and 70 for the nineties", got.PerDecade)
	}
	if need := got.For(eighties, 2026); need != 90 {
		t.Fatalf("an eighties channel needs %d, want 90", need)
	}
	// A decade nobody watched still takes the spacing floor; a window spans each decade it touches.
	if need := got.For([]filler.EraRange{{From: 1975, To: 1989}}, 2026); need != 54+90 {
		t.Fatalf("a 1975-1989 channel needs %d, want the seventies' floor and the eighties' 90", need)
	}
	if need := got.For([]filler.EraRange{{From: 2015}}, 2026); need != 2*54 {
		t.Fatalf("a 2015-onwards channel needs %d, want two decades at the floor", need)
	}
	if need := got.For(nil, 2026); need != 54 {
		t.Fatalf("an any-era channel needs %d, want one window at the floor", need)
	}
}
