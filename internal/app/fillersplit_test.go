package app

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/events"
	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/store"
)

// splitFakeTools implements filler.MediaTools with chapter-only behaviour — the
// app-adapter tests exercise the WIRING (job id, SSE frames, store round-trip),
// not detection, which internal/filler's own suite covers.
type splitFakeTools struct {
	chapters []filler.Chapter
}

func (f splitFakeTools) Chapters(context.Context, string) ([]filler.Chapter, error) {
	return f.chapters, nil
}
func (f splitFakeTools) BlackSilence(context.Context, string) ([]filler.Interval, []filler.Interval, error) {
	return nil, nil, nil
}
func (f splitFakeTools) Transcribe(context.Context, string, int64, int64) ([]filler.TranscriptSegment, error) {
	return nil, fmt.Errorf("no whisper in tests")
}
func (f splitFakeTools) GrayFrames(context.Context, string, int64, int64) ([][]byte, error) {
	return nil, fmt.Errorf("no frames in tests")
}
func (f splitFakeTools) Cut(context.Context, string, int64, int64, string) error { return nil }

func newSplitAdapter(t *testing.T, bus *events.Bus, withSplitter bool) (fillerServiceAdapter, store.Store) {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/f.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	// A compilation to split.
	if err := st.UpsertClip(context.Background(), store.Clip{Clip: filler.Clip{
		Path: "comps/1987.mp4", Name: "1987.mp4", Kind: filler.Commercial, DurationMs: 61_000,
	}, UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	a := fillerServiceAdapter{
		bus: bus, newID: func() string { return "job-1" }, timeout: time.Minute,
		splitClips: fillerSplitStoreAdapter{st},
	}
	if withSplitter {
		tools := splitFakeTools{chapters: []filler.Chapter{
			{StartMs: 0, EndMs: 30_000, Title: "McDonald's"},
			{StartMs: 30_000, EndMs: 61_000, Title: "Lego"},
		}}
		n := 0
		a.splitter = filler.NewSplitter(fillerSplitStoreAdapter{st}, tools, nil, t.TempDir(),
			func() string { n++; return fmt.Sprintf("sp_%d", n) }, time.Now, nil)
	}
	return a, st
}

// No drop-folder ⇒ the typed unavailable error the API renders as a 409 naming
// the Settings remedy (§10, V34).
func TestSplit_NoDropFolderIsUnavailable(t *testing.T) {
	a, _ := newSplitAdapter(t, events.NewBus(), false)
	if _, err := a.Split(context.Background(), "comps/1987.mp4"); !errors.Is(err, api.ErrSplitUnavailable) {
		t.Errorf("Split without a splitter = %v, want ErrSplitUnavailable", err)
	}
	if err := a.ConfirmSplit(context.Background(), "sp_1", nil); !errors.Is(err, api.ErrSplitUnavailable) {
		t.Errorf("ConfirmSplit without a splitter = %v, want ErrSplitUnavailable", err)
	}
}

// A missing clip is a SYNCHRONOUS not-found: the caller gets its 404 in the
// response, not as an SSE error frame seconds later.
func TestSplit_MissingClipFailsSynchronously(t *testing.T) {
	a, _ := newSplitAdapter(t, events.NewBus(), true)
	if _, err := a.Split(context.Background(), "gone.mp4"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Split of a missing clip = %v, want store.ErrNotFound", err)
	}
}

// The job contract end to end: a job id immediately, then a running frame and a
// terminal frame carrying the proposal id — and the proposal itself persisted
// for the review to read back after a reconnect (§7, V34).
func TestSplit_ReportsOverTheBusAndPersistsTheProposal(t *testing.T) {
	bus := events.NewBus()
	ch, unsub := bus.Subscribe()
	t.Cleanup(unsub)
	a, st := newSplitAdapter(t, bus, true)

	jobID, err := a.Split(context.Background(), "comps/1987.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if jobID == "" {
		t.Fatal("no job id returned")
	}

	var statuses []string
	var terminal map[string]any
	deadline := time.After(5 * time.Second)
	for terminal == nil {
		select {
		case ev := <-ch:
			if ev.Type != "filler_split" {
				continue
			}
			p, ok := ev.Payload.(map[string]any)
			if !ok {
				t.Fatalf("frame payload = %T, want map", ev.Payload)
			}
			if p["jobId"] != jobID {
				t.Errorf("frame for a different job: %v", p["jobId"])
			}
			status, _ := p["status"].(string)
			statuses = append(statuses, status)
			if status == "success" || status == "error" {
				terminal = p
			}
		case <-deadline:
			t.Fatalf("no terminal filler_split frame within 5s (statuses so far: %v)", statuses)
		}
	}
	if len(statuses) < 2 || statuses[0] != "running" {
		t.Errorf("frames = %v, want running first, then a terminal", statuses)
	}
	if terminal["status"] != "success" {
		t.Fatalf("terminal = error %v", terminal["error"])
	}
	propID, _ := terminal["proposalId"].(string)
	if propID == "" || terminal["segments"] != 2 {
		t.Errorf("terminal frame missing the proposal: %+v", terminal)
	}
	// The proposal is readable afterwards — the review's reconnect truth.
	p, err := st.GetSplitProposal(context.Background(), propID)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Segments) != 2 || p.Segments[0].Name != "McDonald's" {
		t.Errorf("persisted proposal = %+v", p.Segments)
	}
}
