package mediameasure

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/inventory"
)

type fakeSink struct {
	mu           sync.Mutex
	analyses     map[inventory.SourceID]inventory.Analysis
	measurements []inventory.Measurement
	recorded     chan struct{}
}

func newFakeSink() *fakeSink {
	return &fakeSink{analyses: map[inventory.SourceID]inventory.Analysis{}, recorded: make(chan struct{}, 16)}
}

func (f *fakeSink) RecordMeasurement(_ context.Context, m inventory.Measurement) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.measurements = append(f.measurements, m)
	return nil
}

func (f *fakeSink) RecordInventoryAnalysis(_ context.Context, a inventory.Analysis) error {
	f.mu.Lock()
	f.analyses[a.SourceID] = a
	f.mu.Unlock()
	f.recorded <- struct{}{}
	return nil
}

func (f *fakeSink) InventoryAnalysis(_ context.Context, id inventory.SourceID) (inventory.Analysis, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.analyses[id]
	return a, ok, nil
}

func statRevision(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	return info.ModTime().String() + "/" + string(rune(info.Size())), nil
}

func newTestMeasurer(t *testing.T, sink *fakeSink) *Measurer {
	t.Helper()
	m := New(Deps{
		Sink: sink, Tools: DefaultTools("", ""), Revision: statRevision,
		Facts: func(context.Context, string) (inventory.SourceFacts, error) {
			return inventory.SourceFacts{DurationMillis: 7000, Streams: []inventory.Stream{
				{Index: 0, Kind: inventory.StreamVideo}, {Index: 1, Kind: inventory.StreamAudio}}}, nil
		},
		Now: func() time.Time { return time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC) },
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go m.Run(ctx)
	return m
}

func wait(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for the measurement job")
	}
}

func TestJob_MeasuresKeyframesLoudnessAndBreaksOncePerRevision(t *testing.T) {
	path := fadeFixture(t)
	rev, _ := statRevision(path)
	sink := newFakeSink()
	m := newTestMeasurer(t, sink)
	ref := SourceRef{ID: "src-1", Revision: rev, Path: path}

	m.Submit(ref)
	wait(t, sink.recorded)
	a := sink.analyses["src-1"]
	if a.Revision != rev || len(a.Keyframes) != 7 || a.IntegratedLUFS == nil || a.TruePeakDBTP == nil {
		t.Fatalf("analysis = %+v", a)
	}
	if len(a.Breaks) != 1 || a.Breaks[0].AtMs < 3300 || a.Breaks[0].AtMs > 3700 || a.Breaks[0].KeyframeMs != 4000 {
		t.Fatalf("breaks = %+v, want the fade at ~3.5 s snapped to the 4 s keyframe", a.Breaks)
	}

	// The same revision again is a no-op: nothing is re-measured.
	before := len(sink.recorded)
	m.Submit(ref)
	m.Drain()
	if len(sink.recorded) != before {
		t.Fatal("an already-measured revision was measured again")
	}
}

func TestJob_DiscardsAMeasurementOfAFileThatChangedMidRun(t *testing.T) {
	path := fadeFixture(t)
	rev, _ := statRevision(path)
	sink := newFakeSink()
	tools := DefaultTools("", "")
	inner := tools.Run
	tools.Run = func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		out, errOut, err := inner(ctx, name, args...)
		// The file is replaced just after the decode pass finished reading it.
		if name == tools.FFmpeg {
			_ = os.WriteFile(path+".x", []byte("longer replacement"), 0o644)
			_ = os.Rename(path+".x", path)
		}
		return out, errOut, err
	}
	m := New(Deps{Sink: sink, Tools: tools, Revision: statRevision,
		Facts: func(context.Context, string) (inventory.SourceFacts, error) {
			return inventory.SourceFacts{DurationMillis: 7000, Streams: []inventory.Stream{{Kind: inventory.StreamVideo}}}, nil
		}, Now: time.Now})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(ctx)
	m.Submit(SourceRef{ID: "src-1", Revision: rev, Path: path})
	m.Drain()
	if _, ok := sink.analyses["src-1"]; ok {
		t.Fatal("stored an analysis measured from a file that changed while it was being read")
	}
}

func TestJob_RunsOneSourceAtATime(t *testing.T) {
	dir := t.TempDir()
	var running, peak atomic.Int32
	sink := newFakeSink()
	m := New(Deps{Sink: sink, Revision: statRevision, Now: time.Now,
		Tools: Tools{FFmpeg: "ffmpeg", FFprobe: "ffprobe", Run: func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
			n := running.Add(1)
			defer running.Add(-1)
			for {
				p := peak.Load()
				if n <= p || peak.CompareAndSwap(p, n) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			if name == "ffprobe" {
				return []byte("0.000000,48,K_\n"), nil, nil
			}
			return nil, []byte("no audio"), nil
		}},
		Facts: func(context.Context, string) (inventory.SourceFacts, error) {
			return inventory.SourceFacts{DurationMillis: 5000, Streams: []inventory.Stream{{Kind: inventory.StreamVideo}}}, nil
		}})
	for _, id := range []inventory.SourceID{"a", "b", "c"} {
		path := filepath.Join(dir, string(id))
		if err := os.WriteFile(path, []byte(id), 0o644); err != nil {
			t.Fatal(err)
		}
		rev, _ := statRevision(path)
		m.Submit(SourceRef{ID: id, Revision: rev, Path: path})
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(ctx)
	m.Drain()
	if len(sink.analyses) != 3 {
		t.Fatalf("analyses = %d, want 3", len(sink.analyses))
	}
	if peak.Load() != 1 {
		t.Fatalf("peak concurrent tools = %d, want 1: measurement must be one source, one tool at a time", peak.Load())
	}
}

func TestMeasureFacts_RecordsStreamFactsForFirstPlay(t *testing.T) {
	sink := newFakeSink()
	want := inventory.SourceFacts{Container: "matroska,webm", DurationMillis: 1000, Streams: []inventory.Stream{
		{Index: 0, Kind: inventory.StreamVideo, Codec: "h264"}, {Index: 1, Kind: inventory.StreamAudio, Codec: "aac"}}}
	m := New(Deps{Sink: sink, Now: func() time.Time { return time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC) },
		Facts: func(context.Context, string) (inventory.SourceFacts, error) { return want, nil }})
	got, err := m.MeasureFacts(context.Background(), SourceRef{ID: "s", Revision: "r", Path: "/x"})
	if err != nil || got.Container != want.Container {
		t.Fatalf("facts = %+v err %v", got, err)
	}
	if len(sink.measurements) != 1 || sink.measurements[0].Revision != "r" ||
		sink.measurements[0].Observation.Coverage["streams"] != inventory.CoveragePresent ||
		sink.measurements[0].Observation.Coverage["audioStreams"] != inventory.CoveragePresent {
		t.Fatalf("measurements = %+v", sink.measurements)
	}

	failing := New(Deps{Sink: sink, Facts: func(context.Context, string) (inventory.SourceFacts, error) {
		return inventory.SourceFacts{}, errors.New("unreadable")
	}})
	if _, err := failing.MeasureFacts(context.Background(), SourceRef{ID: "s", Revision: "r", Path: "/x"}); err == nil {
		t.Fatal("a failed probe reported success")
	}
}
