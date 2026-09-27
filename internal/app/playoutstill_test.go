package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/inventory"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/testkit"
)

type recordedAirings struct{ n int }

func (r *recordedAirings) RecordAiring(context.Context, string, provision.Key, string, time.Time) error {
	r.n++
	return nil
}

func stillResolver(slots []schedule.Slot, now time.Time, airings airingRecorder) *playoutResolver {
	accepted := &stubChannels{}
	accepted.ch.Desired = slots
	return &playoutResolver{
		channels: accepted,
		now:      func() time.Time { return now },
		lib:      library.New(library.Emby, "http://media.test", "token", "test-device"),
		airings:  airings,
	}
}

func TestStillAiring_IsTheProgrammeOnNowAndRecordsNothing(t *testing.T) {
	t.Parallel()
	slots := []schedule.Slot{
		{Kind: schedule.SlotProgram, Title: "first", LibraryItemID: "item-1", DurationMs: 60_000},
		{Kind: schedule.SlotProgram, Title: "second", LibraryItemID: "item-2", DurationMs: 60_000},
	}
	history := &recordedAirings{}
	r := stillResolver(slots, testPlayoutAnchor().Add(75*time.Second), history)

	airing, ok, err := r.StillAiring(context.Background(), "ch1")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v, want the airing on now", ok, err)
	}
	if airing.Offset != 15*time.Second || !airing.At.Equal(testPlayoutAnchor().Add(60*time.Second)) || airing.Key == "" {
		t.Fatalf("airing = %+v, want the second programme, 15 s in, keyed", airing)
	}
	source, ok, err := airing.Source(context.Background())
	if err != nil || !ok || !strings.Contains(source.Input, "item-2") {
		t.Fatalf("source = %+v ok=%v err=%v, want the second programme's stream", source, ok, err)
	}
	if history.n != 0 {
		t.Fatalf("recorded %d airings: showing a still is not airing the programme", history.n)
	}

	later, _, _ := stillResolver(slots, testPlayoutAnchor().Add(90*time.Second), history).StillAiring(context.Background(), "ch1")
	if later.Key != airing.Key {
		t.Fatal("the key moved within one airing, so the frame would be decoded again")
	}
}

func TestIndexedKeyframe_SeeksWithTheMeasuredIndexOfTheFilesCurrentBytes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := testkit.MigratedSQLiteStore(t)
	file := filepath.Join(t.TempDir(), "episode.mkv")
	if err := os.WriteFile(file, []byte("media"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := &playoutResolver{inventory: inventory.New(st), analyses: st, now: time.Now}
	if _, ok := r.ensureLocalInventorySource(ctx, file); !ok {
		t.Fatal("registering the local source failed")
	}
	source, ok := r.knownLocalSource(ctx, file)
	if !ok {
		t.Fatal("the registered source is not found")
	}
	if err := st.RecordInventoryAnalysis(ctx, inventory.Analysis{
		SourceID: source.ID, Revision: source.Revision, AnalyzedAt: time.Now(),
		Keyframes: []inventory.Keyframe{{PTSMs: 0}, {PTSMs: 2000, Offset: 10}, {PTSMs: 4000, Offset: 20}},
	}); err != nil {
		t.Fatal(err)
	}

	if at, ok := r.indexedKeyframe(ctx, file, 3500*time.Millisecond); !ok || at != 2*time.Second {
		t.Fatalf("keyframe = %v ok=%v, want 2s (the last one at or before 3.5 s)", at, ok)
	}
	if _, ok := r.indexedKeyframe(ctx, "http://media.test/stream", time.Second); ok {
		t.Fatal("a stream URL has no measured index")
	}

	// New bytes: the old index would seek to the wrong place, so it must not be used.
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(file, future, future); err != nil {
		t.Fatal(err)
	}
	// Playout registers the new bytes before the measurement job has analysed them.
	if _, ok := r.ensureLocalInventorySource(ctx, file); !ok {
		t.Fatal("registering the new revision failed")
	}
	if _, ok := r.indexedKeyframe(ctx, file, 3500*time.Millisecond); ok {
		t.Fatal("used an index measured for the file's previous bytes")
	}
}

func TestStillAiring_ABreakHasNoStill(t *testing.T) {
	t.Parallel()
	slots := []schedule.Slot{
		{Kind: schedule.SlotFiller, DurationMs: 30_000},
		{Kind: schedule.SlotProgram, LibraryItemID: "item-1", DurationMs: 60_000},
	}
	if _, ok, err := stillResolver(slots, testPlayoutAnchor(), nil).StillAiring(context.Background(), "ch1"); ok || err != nil {
		t.Fatalf("ok=%v err=%v, want a clean miss during a break", ok, err)
	}
}
