package app

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/schedule"
)

// Resolving what airs is a read. The admin Info page's default-audio /tracks request, the guide
// and every other read that asks the resolver what is on must leave the household's airing
// history and clip rotation exactly as it found them: only a viewer receiving segments airs
// something (RecordAiringServed).
func TestResolvingTheAiringRecordsNothing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pod := filler.Pod{Entries: []filler.PodEntry{
		{Path: clipAt("1994/toys.mp4"), Hash: "hash-toys", Name: "toys", DurationMs: 8000},
	}}
	probed := func(context.Context, string) (playout.MediaTracks, error) { return playout.MediaTracks{}, nil }

	t.Run("filler", func(t *testing.T) {
		t.Parallel()
		r := fillerResolver(t, dir, pod)
		plays, history := &recordingPlays{}, &recordedAirings{}
		r.clipPlays, r.airings, r.probeTracks = plays, history, probed

		if _, _, err := r.AiringNow(context.Background(), "ch1"); err != nil {
			t.Fatal(err)
		}
		if _, _, err := r.Tracks(context.Background(), "ch1", time.Time{}); err != nil {
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
		r := stillResolver(slots, now, history)
		r.probeTracks = probed

		if _, _, err := r.AiringNow(context.Background(), "ch1"); err != nil {
			t.Fatal(err)
		}
		if _, _, err := r.Tracks(context.Background(), "ch1", time.Time{}); err != nil {
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
	r := stillResolver(slots, start.Add(-5*time.Second), history)
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
	r = stillResolver(slots, later, history)
	airing, _, err = r.AiringAt(context.Background(), "ch1", later)
	if err != nil {
		t.Fatal(err)
	}
	r.RecordAiringServed(context.Background(), "ch1", airing, later)
	if !history.lastAiredAt.Equal(start) || !history.lastRecordedAt.Equal(later) {
		t.Fatalf("tune-in recorded aired %v / observed %v, want %v / %v", history.lastAiredAt, history.lastRecordedAt, start, later)
	}
}

// A filler clip is counted by hash at its scheduled start, so a mid-clip serve is the same
// observation as the first. An entry with no hash (the bumper card) is not counted.
func TestRecordAiringServedCountsAFillerClipByHash(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pod := filler.Pod{Entries: []filler.PodEntry{
		{Path: clipAt("1994/toys.mp4"), Hash: "hash-toys", Name: "toys", DurationMs: 8000},
	}}
	r := fillerResolver(t, dir, pod)
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

	noHash := fillerResolver(t, dir, filler.Pod{Entries: []filler.PodEntry{
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
