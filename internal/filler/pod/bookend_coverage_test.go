package pod_test

import (
	"testing"

	"github.com/loomarr/loomarr/internal/filler/pod"
)

// A channel's coverage counts the bookends its own audience ceiling admits, so a catalog full of
// late-night idents does not read as covered for a kids channel.
func TestCoverageCountsTheBookendsTheChannelAdmits(t *testing.T) {
	bookend := func(id string, role pod.Role, aud pod.Audience) pod.Clip {
		return pod.Clip{Hash: id, Path: id, Role: role, Era: 1992, Audience: aud, DurationMs: 5_000}
	}
	catalog := []pod.Clip{
		bookend("kids-bumper", pod.Bumper, pod.Kids),
		bookend("family-ident", pod.StationID, pod.Family),
		bookend("general-bumper", pod.Bumper, pod.General),
		bookend("late-ident", pod.StationID, pod.LateNight),
		{Hash: "undated", Path: "undated", Role: pod.Bumper, Audience: pod.Kids, DurationMs: 5_000},
	}
	for _, tc := range []struct {
		channel pod.Audience
		want    int
	}{
		{pod.Kids, 2}, {pod.Family, 3}, {pod.General, 3}, {pod.LateNight, 4},
	} {
		w := pod.Window{ChannelID: "c", Era: pod.Year(1992), Audience: tc.channel, GapMs: 120_000, PodMax: 4}
		if got := pod.Coverage(catalog, w, pod.Policy{}).Bookends; got != tc.want {
			t.Errorf("%s channel: bookends = %d, want %d", tc.channel, got, tc.want)
		}
	}
	if got := pod.PoolCounts(catalog, pod.Policy{}).Bookends; got != 4 {
		t.Errorf("catalog bookends = %d, want the 4 that carry their facts", got)
	}
}

// A bookend above the channel's ceiling, or one missing a fact, is no candidate, and the fit says why.
func TestFitForABookendNamesWhyItDoesNotAir(t *testing.T) {
	w := pod.Window{ChannelID: "c", Era: pod.Year(1992), Audience: pod.Kids}
	late := pod.Clip{Hash: "late", Path: "late", Role: pod.StationID, Era: 1992, Audience: pod.LateNight, DurationMs: 5_000}
	if got := pod.FitFor(late, w, pod.Policy{}).Reason; got != pod.FitAudience {
		t.Errorf("late-night ident on a kids channel: reason = %q, want audience", got)
	}
	undated := pod.Clip{Hash: "undated", Path: "undated", Role: pod.Bumper, Audience: pod.Kids, DurationMs: 5_000}
	if got := pod.FitFor(undated, w, pod.Policy{}).Reason; got != pod.FitFacts {
		t.Errorf("undated bumper: reason = %q, want facts", got)
	}
	family := pod.Clip{Hash: "family", Path: "family", Role: pod.Bumper, Era: 1992, Audience: pod.Family, DurationMs: 5_000}
	general := pod.Window{ChannelID: "c", Era: pod.Year(1992), Audience: pod.General}
	if got := pod.FitFor(family, general, pod.Policy{}).Reason; got != "" {
		t.Errorf("family bumper on a general channel: reason = %q, want none", got)
	}
}
