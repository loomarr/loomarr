package app

import (
	"strings"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
)

var provNow = time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)

// The states a viewer actually sees must each say something DIFFERENT. The point of the
// provenance line is that a pending slot stops being a mystery — "acquiring" and "requested"
// are different situations, and collapsing them would be the `gap bool` mistake in prose.
func TestProvenance_DistinguishesTheStates(t *testing.T) {
	seen := map[string]provision.State{}
	for _, st := range []provision.State{
		provision.Available, provision.Downloading, provision.Requested, provision.Unavailable,
	} {
		got := provenanceOf(provision.Record{State: st}, provNow)
		if got == "" {
			t.Errorf("%s produced no provenance line", st)
			continue
		}
		if prev, dup := seen[got]; dup {
			t.Errorf("%s and %s both render as %q — a viewer cannot tell them apart", st, prev, got)
		}
		seen[got] = st
	}
}

// Download progress is the honest thing to show once a release is moving: "acquiring" alone
// is indistinguishable from stuck.
func TestProvenance_ShowsDownloadProgress(t *testing.T) {
	got := provenanceOf(provision.Record{
		State: provision.Downloading, Progress: 0.62, ETAText: "8m",
	}, provNow)
	if !strings.Contains(got, "62%") {
		t.Errorf("provenance = %q, want the completion percentage", got)
	}
	if !strings.Contains(got, "8m") {
		t.Errorf("provenance = %q, want the arr's own ETA", got)
	}
}

// A stalled download is EXACTLY the case worth surfacing — otherwise it looks identical to
// one that is simply early.
func TestProvenance_SurfacesAStalledQueueWhenThereIsNoProgress(t *testing.T) {
	got := provenanceOf(provision.Record{
		State: provision.Downloading, DownloadStatus: "stalled",
	}, provNow)
	if !strings.Contains(got, "stalled") {
		t.Errorf("provenance = %q, want the queue status when no progress is reported", got)
	}
}

// A deadline is how long before Loomarr gives up. Coarse on purpose: a 48-hour window counted
// to the minute implies precision the process does not have.
func TestProvenance_CountsDownToTheDeadline(t *testing.T) {
	got := provenanceOf(provision.Record{
		State: provision.Requested, Deadline: provNow.Add(41 * time.Hour),
	}, provNow)
	if !strings.Contains(got, "41h") {
		t.Errorf("provenance = %q, want the hours remaining", got)
	}
}

// A passed deadline must say so rather than render a negative duration.
func TestProvenance_PassedDeadlineDoesNotGoNegative(t *testing.T) {
	got := provenanceOf(provision.Record{
		State: provision.Requested, Deadline: provNow.Add(-3 * time.Hour),
	}, provNow)
	if strings.Contains(got, "-") {
		t.Errorf("provenance = %q, want no negative duration", got)
	}
	if !strings.Contains(got, "passed") {
		t.Errorf("provenance = %q, want the passed deadline stated", got)
	}
}

// A record with no deadline set still renders — the deadline is optional, not required.
func TestProvenance_NoDeadlineStillDescribesTheState(t *testing.T) {
	if got := provenanceOf(provision.Record{State: provision.Requested}, provNow); got == "" {
		t.Error("a requested title with no deadline produced no provenance at all")
	}
}

func TestTimeLeft_ScalesTheUnitToTheDistance(t *testing.T) {
	for _, tc := range []struct {
		in   time.Duration
		want string
	}{
		{30 * time.Minute, "m left"},
		{5 * time.Hour, "h left"},
		{5 * 24 * time.Hour, "d left"},
	} {
		got := timeLeft(provNow.Add(tc.in), provNow)
		if !strings.HasSuffix(got, tc.want) {
			t.Errorf("timeLeft(%v) = %q, want a %q unit", tc.in, got, tc.want)
		}
	}
}
