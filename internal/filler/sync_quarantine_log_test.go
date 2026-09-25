package filler

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// A quarantined artifact is revisited by every sync pass. It must warn when it first lands there
// and again only if the reason changes — not once per pass.
func TestSyncerWarnQuarantined_RepeatsOnlyWhenTheReasonChanges(t *testing.T) {
	var buf bytes.Buffer
	s := NewSyncer(nil, nil, Layout{}, time.Now,
		slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	for pass := 0; pass < 10; pass++ {
		s.warnQuarantined("ads/coke.mp4", errors.New("provenance mismatch"))
	}
	s.warnQuarantined("ads/coke.mp4", errors.New("source master missing"))
	s.warnQuarantined("ads/pepsi.mp4", errors.New("provenance mismatch"))

	if got := strings.Count(buf.String(), "level=WARN"); got != 3 {
		t.Fatalf("WARNs = %d, want 3 (first, changed reason, other file)\n%s", got, buf.String())
	}
}

func TestSyncerWarnQuarantined_ThrottleIsWiredByTheConstructor(t *testing.T) {
	if NewSyncer(nil, nil, Layout{}, nil, nil).quarantined == nil {
		t.Fatal("NewSyncer left the quarantine throttle nil, so every pass would warn")
	}
}
