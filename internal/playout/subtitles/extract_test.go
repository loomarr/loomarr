package subtitles

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

const doc = "WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nHello\n"

// fakeReader counts reads per input and answers with doc, or with err while it is set.
type fakeReader struct {
	mu    sync.Mutex
	reads map[string]int
	err   error
}

func (f *fakeReader) read(_ context.Context, src Source) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.reads == nil {
		f.reads = map[string]int{}
	}
	f.reads[src.Input]++
	if f.err != nil {
		return nil, f.err
	}
	return []byte(doc), nil
}

// drain reads every queued track on the calling goroutine.
func drain(e *Extractor) {
	for {
		select {
		case src := <-e.queue:
			e.extract(context.Background(), src)
		default:
			return
		}
	}
}

func TestExtractorReadsOnceThenServes(t *testing.T) {
	reader := &fakeReader{}
	e := newExtractor(reader.read, t.TempDir(), nil, time.Now)
	src := Source{Input: "/media/a.mkv", Stream: 3, Size: 9, ModifiedNs: 100}

	if _, state := e.Cues(src); state != Pending {
		t.Fatalf("first ask = %v, want Pending", state)
	}
	// Asked again before the worker ran: queued once, not twice.
	if _, state := e.Cues(src); state != Pending {
		t.Fatalf("second ask = %v, want Pending", state)
	}
	drain(e)
	cues, state := e.Cues(src)
	if state != Ready || len(cues) != 1 || cues[0].Text != "Hello" {
		t.Fatalf("after the read = %v %+v, want Ready with one cue", state, cues)
	}
	if reader.reads[src.Input] != 1 {
		t.Fatalf("reads = %d, want 1", reader.reads[src.Input])
	}
}

func TestExtractorKeepsTracksAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	reader := &fakeReader{}
	src := Source{Input: "/media/a.mkv", Stream: 3, Size: 9, ModifiedNs: 100}
	first := newExtractor(reader.read, dir, nil, time.Now)
	first.Cues(src)
	drain(first)

	again := newExtractor(reader.read, dir, nil, time.Now)
	if cues, state := again.Cues(src); state != Ready || len(cues) != 1 {
		t.Fatalf("after a restart = %v %+v, want Ready from disk", state, cues)
	}
	// A changed file is a new revision, read again.
	changed := src
	changed.ModifiedNs = 200
	if _, state := again.Cues(changed); state != Pending {
		t.Fatalf("changed file = %v, want Pending", state)
	}
	if reader.reads[src.Input] != 1 {
		t.Fatalf("reads = %d, want 1 before the changed file is read", reader.reads[src.Input])
	}
}

func TestExtractorPrunesUnusedFiles(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 11, 20, 0, 0, 0, time.UTC)
	reader := &fakeReader{}
	e := newExtractor(reader.read, dir, nil, func() time.Time { return now })
	used, unused := Source{Input: "/media/used.mkv", Stream: 2}, Source{Input: "/media/unused.mkv", Stream: 2}
	e.Cues(used)
	e.Cues(unused)
	drain(e)
	stale := now.Add(-unusedFor - time.Hour)
	for _, src := range []Source{used, unused} {
		if err := os.Chtimes(filepath.Join(dir, sourceKey(src)+".vtt"), stale, stale); err != nil {
			t.Fatal(err)
		}
	}
	// Read from disk after a restart, which marks it used.
	restarted := newExtractor(reader.read, dir, nil, func() time.Time { return now })
	if _, state := restarted.Cues(used); state != Ready {
		t.Fatalf("used = %v, want Ready", state)
	}
	restarted.prune()
	if _, err := os.Stat(filepath.Join(dir, sourceKey(used)+".vtt")); err != nil {
		t.Errorf("the used file was removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, sourceKey(unused)+".vtt")); !os.IsNotExist(err) {
		t.Errorf("the unused file was kept: %v", err)
	}
}

func TestExtractorRetriesAFailedTrackLater(t *testing.T) {
	now := time.Date(2026, 11, 11, 20, 0, 0, 0, time.UTC)
	reader := &fakeReader{err: errors.New("unreadable")}
	e := newExtractor(reader.read, "", nil, func() time.Time { return now })
	src := Source{Input: "/media/bad.mkv", Stream: 2}

	e.Cues(src)
	drain(e)
	if _, state := e.Cues(src); state != Failed {
		t.Fatalf("after a failed read = %v, want Failed", state)
	}
	now = now.Add(retryAfter)
	reader.err = nil
	if _, state := e.Cues(src); state != Pending {
		t.Fatalf("after the wait = %v, want Pending (queued again)", state)
	}
	drain(e)
	if _, state := e.Cues(src); state != Ready {
		t.Fatalf("after the retry = %v, want Ready", state)
	}
}

func TestExtractorForgetsTheLeastRecentlyUsed(t *testing.T) {
	now := time.Date(2026, 11, 11, 20, 0, 0, 0, time.UTC)
	e := newExtractor((&fakeReader{}).read, "", nil, func() time.Time { return now })
	for i := range keptTracks + 1 {
		now = now.Add(time.Second)
		e.keep(sourceKey(Source{Stream: i}), &keptTrack{used: now})
	}
	if len(e.kept) != keptTracks {
		t.Fatalf("kept %d tracks, want %d", len(e.kept), keptTracks)
	}
	if _, ok := e.kept[sourceKey(Source{Stream: 0})]; ok {
		t.Fatal("the least recently used track was kept")
	}
}
