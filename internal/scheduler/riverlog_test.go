package scheduler

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func riverLogOutput(level slog.Level, emit func(*slog.Logger)) string {
	var buf bytes.Buffer
	base := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: level}))
	emit(quietRiverLogger(base))
	return buf.String()
}

// River logs every cron tick whose constructor returned nil at INFO — ~65 lines a minute in
// production, 75% of the container log — so the periodic chatter must sit below INFO.
func TestQuietRiverLogger_PeriodicChatterIsNotInfo(t *testing.T) {
	out := riverLogOutput(slog.LevelInfo, func(l *slog.Logger) {
		l.Info("maintenance.PeriodicJobEnqueuer: nil returned from periodic job constructor, skipping", "job", "x")
		l.Info("producer: Producer job counts", "queue", "default")
	})
	if out != "" {
		t.Fatalf("chatter emitted at INFO:\n%s", out)
	}
}

func TestQuietRiverLogger_ChatterStillAvailableAtDebug(t *testing.T) {
	out := riverLogOutput(slog.LevelDebug, func(l *slog.Logger) {
		l.Info("maintenance.PeriodicJobEnqueuer: nil returned from periodic job constructor, skipping")
	})
	if !strings.Contains(out, "level=DEBUG") {
		t.Fatalf("chatter not demoted to DEBUG:\n%s", out)
	}
}

func TestQuietRiverLogger_KeepsOtherInfoAndErrors(t *testing.T) {
	out := riverLogOutput(slog.LevelInfo, func(l *slog.Logger) {
		l.With("a", 1).WithGroup("g").Info("client: Client started")
		l.Error("maintenance.PeriodicJobEnqueuer: nil returned from periodic job constructor, skipping")
		l.Error("some river failure")
	})
	for _, want := range []string{"level=INFO", "Client started", "level=ERROR", "some river failure"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Count(out, "level=ERROR") != 2 {
		t.Errorf("errors must never be demoted:\n%s", out)
	}
}
