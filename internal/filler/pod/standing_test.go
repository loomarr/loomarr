package pod_test

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/filler/pod"
)

var standingNow = seasonJul14

// plainAds is n year-round 1994 ads, plain00 up.
func plainAds(n int) []pod.Clip {
	var out []pod.Clip
	for i := range n {
		out = append(out, seasonAd(fmt.Sprintf("plain%02d", i), 1994))
	}
	return out
}

// airedTimes is an airing history in which plain<i> aired i times, long enough ago that no evening
// of it sizes the pool: the target is the default evening of 80.
func airedTimes(n int) []pod.Airing {
	var out []pod.Airing
	for i := range n {
		for range i {
			at := standingNow.Add(-90 * 24 * time.Hour)
			out = append(out, pod.Airing{ChannelID: "ch-a", ClipHash: fmt.Sprintf("plain%02d", i), AiredAt: at, RecordedAt: at})
		}
	}
	return out
}

func standingOf(t *testing.T, cat []pod.Clip, channels []pod.PoolChannel, airings []pod.Airing) pod.Standing {
	t.Helper()
	adapter := seasonAdapter(cat, nil, schedule.Calendar{Country: "US"}, standingNow)
	got, err := adapter.StandingFor(context.Background(), channels, airings, 4*time.Hour, standingNow)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

var nineties1 = pod.PoolChannel{ChannelID: "ch-a", Name: "Channel A", Number: 1, Selection: seasonSel}

// A channel keeps the evening target of clips (80 until the household has watched four weeks), its
// own era first and the least aired first; what no channel keeps is surplus, most aired first.
func TestStandingSurplusIsWhatNoChannelNeedsMostAiredFirst(t *testing.T) {
	got := standingOf(t, plainAds(85), []pod.PoolChannel{nineties1}, airedTimes(85))
	want := []string{"plain84", "plain83", "plain82", "plain81", "plain80"}
	if !slices.Equal(got.Surplus, want) || len(got.Idle) != 0 {
		t.Fatalf("standing = %+v, want only %v surplus", got, want)
	}

	if got := standingOf(t, plainAds(80), []pod.PoolChannel{nineties1}, airedTimes(80)); len(got.Surplus) != 0 {
		t.Fatalf("a channel with exactly its target has surplus %v, want none", got.Surplus)
	}
}

// A clip from the decade beside the channel's is kept after the clips in its own era, however
// little each has aired: the neighbouring one is the surplus.
func TestStandingKeepsTheNearerEraFirst(t *testing.T) {
	cat := append(plainAds(80), seasonAd("neighbour", 2004))
	got := standingOf(t, cat, []pod.PoolChannel{nineties1}, airedTimes(80))
	if !slices.Equal(got.Surplus, []string{"neighbour"}) {
		t.Fatalf("surplus = %v, want the 2004 ad on the widened rung, not one of the 80 in era", got.Surplus)
	}
}

// A clip further than a decade either side of every channel's era is idle: it fits no channel.
func TestStandingCallsAClipBeyondTheNeighbouringDecadeIdle(t *testing.T) {
	cat := append(plainAds(80), seasonAd("far", 1962))
	got := standingOf(t, cat, []pod.PoolChannel{nineties1}, airedTimes(80))
	if !slices.Equal(got.Idle, []string{"far"}) || len(got.Surplus) != 0 {
		t.Fatalf("standing = %+v, want only the 1962 ad idle", got)
	}
}

// A clip no channel can air is idle, even if it never aired; a pinned clip is the operator's word
// and a bookend a channel can air is not drawn from the commercial ladder, so neither is let go.
func TestStandingIdleIsWhatFitsNoChannel(t *testing.T) {
	late := seasonAd("late-night", 1994)
	late.Audience = pod.LateNight
	pinnedLate := seasonAd("pinned-late-night", 1994)
	pinnedLate.Audience = pod.LateNight
	cat := append(plainAds(3), late, pinnedLate)
	cat = append(cat, pod.Clip{Hash: "ident", Path: "ident.mp4", Role: pod.StationID, Era: 1994, Audience: pod.General, DurationMs: 5_000})
	channel := nineties1
	channel.Selection.Pinned = []string{"pinned-late-night"}

	got := standingOf(t, cat, []pod.PoolChannel{channel}, nil)
	if !slices.Equal(got.Idle, []string{"late-night"}) || len(got.Surplus) != 0 {
		t.Fatalf("standing = %+v, want only the late-night ad idle", got)
	}
}

// A holiday clip is kept through the off-season (decision 9 of #3838): it fits no channel in July,
// and is still neither idle nor surplus.
func TestStandingKeepsHolidayClipsThroughTheOffSeason(t *testing.T) {
	cat := append(plainAds(85), seasonAd("xmas00", 1994, "christmas"))
	got := standingOf(t, cat, []pod.PoolChannel{nineties1}, airedTimes(85))
	for _, hash := range slices.Concat(got.Idle, got.Surplus) {
		if hash == "xmas00" {
			t.Fatalf("an out-of-season Christmas ad is %+v, want it kept", got)
		}
	}
}

// A clip any channel needs is kept: the channels' needs are taken together, so one that is surplus
// to the 1990s channel is not surplus if a 1960s channel needs it.
func TestStandingAClipAnyChannelNeedsIsKept(t *testing.T) {
	sixties := pod.PoolChannel{ChannelID: "ch-b", Name: "Channel B", Number: 2,
		Selection: pod.Selection{Era: pod.EraRange{From: 1960, To: 1969}, Audience: pod.General}}
	cat := plainAds(85)
	for i := range 3 {
		cat = append(cat, seasonAd(fmt.Sprintf("sixties%d", i), 1965))
	}
	airings := airedTimes(85)
	for range 500 {
		at := standingNow.Add(-90 * 24 * time.Hour)
		airings = append(airings, pod.Airing{ChannelID: "ch-b", ClipHash: "sixties0", AiredAt: at, RecordedAt: at})
	}
	got := standingOf(t, cat, []pod.PoolChannel{nineties1, sixties}, airings)
	if !slices.Equal(got.Surplus, []string{"plain84", "plain83", "plain82", "plain81", "plain80"}) || len(got.Idle) != 0 {
		t.Fatalf("standing = %+v; want only the 1990s channel's five extra ads surplus, the most aired 1960s ad kept for its channel", got)
	}
}

// With no live channel nothing is said: an install whose channels are all paused has not decided its
// whole library is idle.
func TestStandingWithNoLiveChannelSaysNothing(t *testing.T) {
	if got := standingOf(t, plainAds(5), nil, nil); len(got.Idle)+len(got.Surplus) != 0 {
		t.Fatalf("standing = %+v, want none", got)
	}
}
