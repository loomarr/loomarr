package logchange_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/logchange"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newThrottle(c *clock) *logchange.Throttle {
	return logchange.New(time.Hour, logchange.WithClock(c.now))
}

func warnCount(buf *bytes.Buffer) int { return strings.Count(buf.String(), "level=WARN") }

// The production shape: the same 25 clips failed with the same error on 15 passes. Each distinct
// (clip, error) must warn once, not 375 times.
func TestWarn_RepeatedIdenticalConditionLogsOnce(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	th := newThrottle(&clock{t: time.Unix(0, 0)})

	for pass := 0; pass < 15; pass++ {
		for clip := 0; clip < 25; clip++ {
			th.Warn(log, "clip-"+string(rune('a'+clip)), "commit refused: no audio", "clip failed")
		}
	}
	if got := warnCount(&buf); got != 25 {
		t.Fatalf("WARN lines = %d, want 25 (one per clip)", got)
	}
	if !strings.Contains(buf.String(), "level=DEBUG") {
		t.Fatalf("suppressed repeats must remain visible at DEBUG:\n%s", buf.String())
	}
}

func TestWarn_ChangedConditionLogsAgain(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	th := newThrottle(&clock{t: time.Unix(0, 0)})

	th.Warn(log, "clip", "no audio", "clip failed")
	th.Warn(log, "clip", "no audio", "clip failed")
	th.Warn(log, "clip", "too short", "clip failed")
	th.Warn(log, "clip", "too short", "clip failed")
	if got := warnCount(&buf); got != 2 {
		t.Fatalf("WARN lines = %d, want 2", got)
	}
}

func TestWarn_RepeatsAfterTheLongInterval(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	c := &clock{t: time.Unix(0, 0)}
	th := newThrottle(c)

	th.Warn(log, "clip", "no audio", "clip failed")
	c.t = c.t.Add(59 * time.Minute)
	th.Warn(log, "clip", "no audio", "clip failed")
	c.t = c.t.Add(2 * time.Minute)
	th.Warn(log, "clip", "no audio", "clip failed")
	if got := warnCount(&buf); got != 2 {
		t.Fatalf("WARN lines = %d, want 2 (first + after the hour)", got)
	}
}

// A condition that cleared and came back is news again.
func TestForget_ReArmsAKeyThatRecovered(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	th := newThrottle(&clock{t: time.Unix(0, 0)})

	th.Warn(log, "clip", "no audio", "clip failed")
	th.Forget("clip")
	th.Warn(log, "clip", "no audio", "clip failed")
	if got := warnCount(&buf); got != 2 {
		t.Fatalf("WARN lines = %d, want 2", got)
	}
}

func TestThrottle_BoundsItsMemory(t *testing.T) {
	th := logchange.New(time.Hour, logchange.WithMaxKeys(10))
	for i := 0; i < 1000; i++ {
		th.Changed(string(rune(i+'0')), "x")
	}
	if n := th.Len(); n > 10 {
		t.Fatalf("tracked keys = %d, want <= 10", n)
	}
}

func TestWarn_NilLoggerAndNilThrottleAreSafe(t *testing.T) {
	var th *logchange.Throttle
	th.Warn(nil, "k", "s", "m")
	logchange.New(time.Hour).Warn(nil, "k", "s", "m")
}
