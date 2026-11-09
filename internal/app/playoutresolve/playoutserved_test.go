package playoutresolve

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/filler/pod"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/provision"
)

// Resolving what airs is a read. The admin Info page's default-audio /tracks request, the guide
// and every other read that asks the resolver what is on must leave the household's airing
// history and clip rotation exactly as it found them: only a viewer receiving segments airs
// something (RecordAiringServed).
func TestResolvingTheAiringRecordsNothing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	brk := pod.Pod{Entries: []pod.Entry{
		{Path: clipAt("1994/toys.mp4"), Hash: "hash-toys", Name: "toys", DurationMs: 8000},
	}}
	probed := func(context.Context, string) (playout.MediaTracks, error) { return playout.MediaTracks{}, nil }

	t.Run("filler", func(t *testing.T) {
		t.Parallel()
		r := fillerResolver(t, dir, brk)
		plays, history := &recordingPlays{}, &recordedAirings{}
		r.clipPlays, r.airings, r.probeTracks = plays, history, probed

		if _, _, err := r.AiringNow(context.Background(), "ch1"); err != nil {
			t.Fatal(err)
		}
		if _, err := r.Tracks(context.Background(), "ch1"); err != nil {
			t.Fatal(err)
		}
		if len(plays.calls) != 0 || history.n != 0 {
			t.Fatalf("reads recorded %v clip plays and %d airings, want none", plays.calls, history.n)
		}
	})

	t.Run("programme", func(t *testing.T) {
		t.Parallel()
		slots := []schedule.Slot{{Kind: schedule.SlotProgram, Key: "movie:tmdb:90000001", LibraryItemID: "item-1", DurationMs: 60_000}}
		history := &recordedAirings{}
		now := testPlayoutAnchor().Add(20 * time.Second)
		r := stillResolver(t, slots, now, history)
		r.probeTracks = probed

		if _, _, err := r.AiringNow(context.Background(), "ch1"); err != nil {
			t.Fatal(err)
		}
		if _, err := r.Tracks(context.Background(), "ch1"); err != nil {
			t.Fatal(err)
		}
		if history.n != 0 {
			t.Fatalf("reads recorded %d airings, want none", history.n)
		}
	})
}

// A programme is recorded under its provisioning key, stamped with its start and with when the
// viewer received it, never earlier than that start (#1674).
func TestRecordAiringServedStampsAProgramme(t *testing.T) {
	t.Parallel()
	slots := []schedule.Slot{
		{Kind: schedule.SlotProgram, Key: "movie:tmdb:90000001", LibraryItemID: "item-1", DurationMs: 60_000},
		{Kind: schedule.SlotProgram, Key: "movie:tmdb:90000002", LibraryItemID: "item-2", DurationMs: 60_000},
	}
	history := &recordedAirings{}
	start := testPlayoutAnchor().Add(60 * time.Second)

	// The packager resolves item-2 up to its run-ahead early (here 7 s into it, on a wall clock still
	// 5 s before it): the record is stamped with the start, never observed before it.
	resolvedAt := start.Add(7 * time.Second)
	r := stillResolver(t, slots, start.Add(-5*time.Second), history)
	airing, _, err := r.AiringAt(context.Background(), "ch1", resolvedAt)
	if err != nil {
		t.Fatal(err)
	}
	r.RecordAiringServed(context.Background(), "ch1", airing, resolvedAt)
	if history.n != 1 || !history.lastAiredAt.Equal(start) || !history.lastRecordedAt.Equal(start) {
		t.Fatalf("recorded %d airings aired %v / observed %v, want one at the start %v",
			history.n, history.lastAiredAt, history.lastRecordedAt, start)
	}

	// A tune-in 20 s into item-2: aired is still the programme's start, observed is when it was served.
	later := start.Add(20 * time.Second)
	r = stillResolver(t, slots, later, history)
	airing, _, err = r.AiringAt(context.Background(), "ch1", later)
	if err != nil {
		t.Fatal(err)
	}
	r.RecordAiringServed(context.Background(), "ch1", airing, later)
	if !history.lastAiredAt.Equal(start) || !history.lastRecordedAt.Equal(later) {
		t.Fatalf("tune-in recorded aired %v / observed %v, want %v / %v", history.lastAiredAt, history.lastRecordedAt, start, later)
	}
}

// ctxAwareAirings is a recorder that refuses a write under a done context, as a database does.
type ctxAwareAirings struct {
	recorded int
	err      error // fails every write when set
}

func (c *ctxAwareAirings) RecordAiring(ctx context.Context, _ string, _ provision.Key, _ string, _, _ time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.err != nil {
		return c.err
	}
	c.recorded++
	return nil
}

// A viewer that leaves right after receiving its first segment ends the context the serve ran
// under; the airing it just received is still recorded, because the write does not depend on it.
func TestRecordAiringServedWritesAfterTheRunContextEnds(t *testing.T) {
	t.Parallel()
	slots := []schedule.Slot{{Kind: schedule.SlotProgram, Key: "movie:tmdb:90000001", LibraryItemID: "item-1", DurationMs: 60_000}}
	history := &ctxAwareAirings{}
	now := testPlayoutAnchor().Add(20 * time.Second)
	r := stillResolver(t, slots, now, history)
	airing, _, err := r.AiringAt(context.Background(), "ch1", now)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r.RecordAiringServed(ctx, "ch1", airing, now)
	if history.recorded != 1 {
		t.Fatalf("recorded %d airings under a cancelled run context, want 1", history.recorded)
	}
}

// A failed airing write is a defect worth an operator's attention (recency-aware placement loses
// that airing), so it logs at Warn, not Debug.
func TestRecordAiringServedLogsAFailedWriteAtWarn(t *testing.T) {
	t.Parallel()
	slots := []schedule.Slot{{Kind: schedule.SlotProgram, Key: "movie:tmdb:90000001", LibraryItemID: "item-1", DurationMs: 60_000}}
	now := testPlayoutAnchor().Add(20 * time.Second)
	r := stillResolver(t, slots, now, &ctxAwareAirings{err: errors.New("database is gone")})
	var logs bytes.Buffer
	r.log = slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	airing, _, err := r.AiringAt(context.Background(), "ch1", now)
	if err != nil {
		t.Fatal(err)
	}

	r.RecordAiringServed(context.Background(), "ch1", airing, now)
	if !strings.Contains(logs.String(), "level=WARN") || !strings.Contains(logs.String(), "database is gone") {
		t.Fatalf("a failed airing write logged:\n%s\nwant a Warn naming the error", logs.String())
	}
}

// A filler clip is counted by hash at its scheduled start, so a mid-clip serve is the same
// observation as the first. An entry with no hash (the bumper card) is not counted.
func TestRecordAiringServedCountsAFillerClipByHash(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	brk := pod.Pod{Entries: []pod.Entry{
		{Path: clipAt("1994/toys.mp4"), Hash: "hash-toys", Name: "toys", DurationMs: 8000},
	}}
	r := fillerResolver(t, dir, brk)
	plays := &recordingPlays{}
	r.clipPlays = plays

	base := testPlayoutAnchor()
	now := base.Add(3 * time.Second)
	r.now = func() time.Time { return now }
	airing, _, err := r.AiringAt(context.Background(), "ch1", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(plays.calls) != 0 {
		t.Fatalf("resolving counted %v, want nothing before a viewer is served", plays.calls)
	}
	r.RecordAiringServed(context.Background(), "ch1", airing, now)
	if len(plays.calls) != 1 || plays.calls[0] != "hash-toys" || plays.channels[0] != "ch1" || !plays.at[0].Equal(base) {
		t.Fatalf("recorded %v on %v at %v, want the clip hash on ch1 at its scheduled start %v",
			plays.calls, plays.channels, plays.at, base)
	}

	noHash := fillerResolver(t, dir, pod.Pod{Entries: []pod.Entry{
		{Path: clipAt("1994/toys.mp4"), Name: "no-hash", DurationMs: 8000},
	}})
	noHash.clipPlays = plays
	airing, _, err = noHash.AiringAt(context.Background(), "ch1", base)
	if err != nil {
		t.Fatal(err)
	}
	noHash.RecordAiringServed(context.Background(), "ch1", airing, base)
	if len(plays.calls) != 1 {
		t.Fatalf("recorded %v, want the entry with no hash not counted", plays.calls)
	}
}
