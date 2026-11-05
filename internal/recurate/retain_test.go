package recurate

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/schedule"
)

func TestRecency(t *testing.T) {
	t.Parallel()
	if got := recency(0); got != 1 {
		t.Errorf("recency(0) = %v, want 1", got)
	}
	if got := recency(-time.Hour); got != 1 {
		t.Errorf("a future timestamp must clamp to 1, got %v", got)
	}
	if got := recency(recencyScale); got != 0.5 {
		t.Errorf("recency(scale) = %v, want 0.5", got)
	}
	prev := 2.0
	for _, d := range []time.Duration{0, time.Hour, 24 * time.Hour, 10 * 24 * time.Hour, 365 * 24 * time.Hour} {
		got := recency(d)
		if got >= prev || got <= 0 {
			t.Errorf("recency(%v) = %v: want strictly decreasing and positive (prev %v)", d, got, prev)
		}
		prev = got
	}
}

func TestIdleFor(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	added := now.Add(-3 * time.Hour)
	aired := now.Add(-time.Hour)
	for _, tc := range []struct {
		name  string
		entry schedule.LineupEntry
		last  time.Time
		want  time.Duration
	}{
		{"last airing wins", schedule.LineupEntry{AddedAt: added}, aired, time.Hour},
		{"never aired counts from AddedAt", schedule.LineupEntry{AddedAt: added}, time.Time{}, 3 * time.Hour},
		{"never aired and unknown AddedAt is the cap", schedule.LineupEntry{}, time.Time{}, staleCap},
	} {
		if got := idleFor(tc.entry, tc.last, now); got != tc.want {
			t.Errorf("%s: idleFor = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestRetainScore(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	fresh := now.Add(-time.Hour)
	stale := now.Add(-30 * 24 * time.Hour)

	good := schedule.LineupEntry{Fit: 0.9}
	poor := schedule.LineupEntry{Fit: 0.2}
	if retainScore(poor, stale, now) >= retainScore(good, fresh, now) {
		t.Error("a poor fit off the air a month must score below a good fit aired an hour ago")
	}
	if retainScore(good, stale, now) >= retainScore(good, fresh, now) {
		t.Error("at equal fit the longer-idle title must score lower")
	}
	if retainScore(poor, fresh, now) >= retainScore(good, fresh, now) {
		t.Error("at equal recency the poorer fit must score lower")
	}
	if got, want := retainScore(schedule.LineupEntry{}, fresh, now), retainScore(schedule.LineupEntry{Fit: schedule.UnscoredFit}, fresh, now); got != want {
		t.Errorf("an unscored entry scores %v, want the neutral fit's %v", got, want)
	}
}
