package filler_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

// failingClips makes every clip read fail with *err, so a pass reports a failed clip without
// persisting anything — the state in which the same clip fails identically on every pass.
type failingClips struct {
	*pipeMemStore
	err *error
}

func (f failingClips) GetClip(context.Context, string) (filler.StoreClip, bool, error) {
	return filler.StoreClip{}, false, *f.err
}

func logPipe(st *pipeMemStore, failure *error, buf *bytes.Buffer) *filler.Pipeline {
	at := time.Unix(1_800_000_000, 0).UTC()
	log := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return filler.NewPipeline(st, failingClips{st, failure}, asSlice(allStages()), filler.DefaultBudget(), nil,
		func() time.Time { return at }, log)
}

func clipFailedWarns(buf *bytes.Buffer) int {
	n := 0
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.Contains(line, "level=WARN") && strings.Contains(line, "clip failed") {
			n++
		}
	}
	return n
}

// Production shape: 25 clips × 15 passes logged 375 identical WARNs. One condition, one warning.
func TestPipeline_RepeatedIdenticalClipFailureWarnsOnce(t *testing.T) {
	st := newPipeMemStore()
	for _, h := range []string{"c1", "c2"} {
		seedEnrolled(st, h)
	}
	var buf bytes.Buffer
	failure := errors.New("disk on fire")
	p := logPipe(st, &failure, &buf)

	for pass := 0; pass < 5; pass++ {
		if _, err := p.RunOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if got := clipFailedWarns(&buf); got != 2 {
		t.Fatalf("clip-failed WARNs across 5 passes = %d, want 2 (one per clip)\n%s", got, buf.String())
	}
}

func TestPipeline_ChangedClipFailureWarnsAgain(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "c1")
	var buf bytes.Buffer
	failure := errors.New("disk on fire")
	p := logPipe(st, &failure, &buf)

	for pass := 0; pass < 3; pass++ {
		if _, err := p.RunOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	failure = errors.New("different failure")
	if _, err := p.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := clipFailedWarns(&buf); got != 2 {
		t.Fatalf("clip-failed WARNs = %d, want 2 (first failure + changed error)\n%s", got, buf.String())
	}
}
