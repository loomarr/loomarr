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
	if need := got.For(eighties, 2026); got.FromHistory || need != 80 {
		t.Fatalf("three weeks of history sized %+v (%d), want the default evening of 80", got, need)
	}
	// Each decade a channel draws from takes a default evening.
	if need := got.For([]filler.EraRange{{From: 1985, To: 1994}}, 2026); need != 160 {
		t.Fatalf("a two-decade channel needs %d, want two default evenings", need)
	}
	if need := filler.SizePool(nil, windows, 8*time.Hour, sizedAt).For(eighties, 2026); need != 107 {
		t.Fatalf("a spacing longer than the default evening needs %d, want its own 107", need)
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
	// A decade nobody watched needs nothing of its own: a channel there takes the spacing floor, and
	// one reaching into a watched decade takes that decade's evening.
	if need := got.For([]filler.EraRange{{From: 1975, To: 1989}}, 2026); need != 90 {
		t.Fatalf("a 1975-1989 channel needs %d, want the eighties' 90", need)
	}
	if need := got.For([]filler.EraRange{{From: 1950}}, 2026); need != 160 {
		t.Fatalf("a 1950-onwards channel needs %d, want the eighties' and nineties' evenings only", need)
	}
	if need := got.For(nil, 2026); need != 54 {
		t.Fatalf("an any-era channel needs %d, want the floor", need)
	}
}

// An evening on a channel spanning two decades is shared between them, not counted in each.
func TestSizePool_AChannelAcrossDecadesSharesItsEvening(t *testing.T) {
	windows := map[string][]filler.EraRange{"a": {{From: 1980, To: 1999}}, "b": {{From: 1970, To: 1999}}}
	airings := evening("a", sizedAt.Add(-5*7*24*time.Hour), 1)
	airings = append(airings, evening("a", sizedAt.Add(-48*time.Hour), 100)...)
	airings = append(airings, evening("b", sizedAt.Add(-24*time.Hour), 90)...)
	got := filler.SizePool(airings, windows, time.Hour, sizedAt)
	if need := got.For(windows["a"], 2026); need != 100 {
		t.Fatalf("the 1980-1999 channel needs %d after a 100-ad evening, want 100 (per decade %v)", need, got.PerDecade)
	}
	if need := got.For(windows["b"], 2026); need != 30+50+50 {
		t.Fatalf("the 1970-1999 channel needs %d, want its seventies' 30 and the longer evenings' 50 and 50", need)
	}
}
