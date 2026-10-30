package api

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/playout"
)

func TestSubtitleWindow_ClampsToWhatAPlayerCanBeWatching(t *testing.T) {
	now := time.Date(2026, 10, 30, 20, 0, 0, 0, time.UTC)
	earliest := now.Add(-playout.DVRHorizon - time.Minute)
	for _, tc := range []struct {
		name             string
		from, to         time.Time
		wantFrom, wantTo time.Time
		ok               bool
	}{
		{"a window at live", now.Add(-time.Minute), now.Add(time.Minute), now.Add(-time.Minute), now.Add(time.Minute), true},
		{"older than the seekable past", now.Add(-time.Hour), now.Add(-15 * time.Minute), earliest, now.Add(-15 * time.Minute), true},
		{"far ahead of live", now, now.Add(time.Hour), now, now.Add(subtitleAhead), true},
		{"too long", earliest, now, earliest, earliest.Add(maxSubtitleWindow), true},
		{"entirely in the past", now.Add(-2 * time.Hour), now.Add(-time.Hour), time.Time{}, time.Time{}, false},
	} {
		from, to, ok := subtitleWindow(tc.from, tc.to, now)
		if ok != tc.ok || (ok && (!from.Equal(tc.wantFrom) || !to.Equal(tc.wantTo))) {
			t.Errorf("%s: got %v..%v ok=%v, want %v..%v ok=%v", tc.name, from, to, ok, tc.wantFrom, tc.wantTo, tc.ok)
		}
	}
}
