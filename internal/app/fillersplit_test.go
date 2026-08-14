package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
func (f splitFakeTools) KeyframesIn(context.Context, string, int64, int64, int) ([][]byte, error) {
	return nil, nil
}

func (f splitFakeTools) Keyframes(context.Context, string, int) ([][]byte, error) {
	return nil, fmt.Errorf("no keyframes in tests")
}

// ⚠ It must actually WRITE, and write something different per span. A segment's identity is the
// hash of its bytes (§10 V38c), so a `Cut` that wrote nothing leaves nothing to hash, and one that
// wrote identical bytes would make every segment of a reel the same clip — which is the correct
// outcome for identical content and therefore indistinguishable from the collapse bug this file
// now guards against. Returning a bare nil was safe only while nobody read the result.
func (f splitFakeTools) Cut(_ context.Context, _ string, start, end int64, out string) error {
	return os.WriteFile(out, []byte(fmt.Sprintf("cut %d-%d", start, end)), 0o644)
}

// compHash is the seeded compilation's IDENTITY, and compPath its LOCATION.
//
// ⚠ **They are deliberately different strings.** This fixture set `Hash` and `Path` to the same
// value, with a comment explaining that Split looks the clip up by id — which made the lookup
// appear to work no matter which of the two the code passed. Production hashes are 64 hex
// characters and never equal a path, so every key confusion in the split path was invisible here:
// `Confirm` looked the compilation up by path against a hash-keyed `GetClip` and could never
// commit a split at all (§10 V51a). Two distinct values is what makes that class visible.
const (
	compHash = "a3f90000000000000000000000000000000000000000000000000000000000c1"
	compPath = "a3/f9/" + compHash + ".mp4"
)

func newSplitAdapter(t *testing.T, bus *events.Bus, withSplitter bool) (fillerServiceAdapter, store.Store) {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/f.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	// A compilation to split.
	if err := st.UpsertClip(context.Background(), store.Clip{Clip: filler.Clip{
		Hash: compHash,
		Path: compPath, Name: "1987.mp4", Kind: filler.Commercial, DurationMs: 61_000,
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
			func() time.Duration { return 10 * time.Second },
			func() string { n++; return fmt.Sprintf("sp_%d", n) }, time.Now, nil)
	}
	return a, st
}

// No drop-folder ⇒ the typed unavailable error the API renders as a 409 naming
// the Settings remedy (§10, V34).
func TestSplit_NoDropFolderIsUnavailable(t *testing.T) {
	a, _ := newSplitAdapter(t, events.NewBus(), false)
	if _, err := a.Split(context.Background(), compHash); !errors.Is(err, api.ErrSplitUnavailable) {
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

	jobID, err := a.Split(context.Background(), compHash)
	if err != nil {
		t.Fatal(err)
	}
	if jobID == "" {
		t.Fatal("no job id returned")
	}

	var statuses []string
	var terminal *api.FillerSplitEvent
	deadline := time.After(5 * time.Second)
	for terminal == nil {
		select {
		case ev := <-ch:
			if ev.Type != "filler_split" {
				continue
			}
			// ⚠ A TYPED payload. huma names an SSE frame after its payload's Go type, so a
			// frame published as anything but api.FillerSplitEvent goes out unnamed and the
			// review UI's listener never fires — see internal/api/events.go.
			p, ok := ev.Payload.(api.FillerSplitEvent)
			if !ok {
				t.Fatalf("frame payload = %T, want api.FillerSplitEvent", ev.Payload)
			}
			if p.JobID != jobID {
				t.Errorf("frame for a different job: %v", p.JobID)
			}
			statuses = append(statuses, p.Status)
			if p.Status == "success" || p.Status == "error" {
				terminal = &p
			}
		case <-deadline:
			t.Fatalf("no terminal filler_split frame within 5s (statuses so far: %v)", statuses)
		}
	}
	if len(statuses) < 2 || statuses[0] != "running" {
		t.Errorf("frames = %v, want running first, then a terminal", statuses)
	}
	if terminal.Status != "success" {
		t.Fatalf("terminal = error %v", terminal.Error)
	}
	propID := terminal.ProposalID
	// ⚠ `terminal.Segments != 2` used to be `terminal["segments"] != 2` against an `any`.
	// That comparison is only correct while the boxed value happens to be an `int` — the
	// same literal against an int64 or a float64 is silently false, and the assertion
	// passes for the wrong reason. A typed field cannot be compared wrongly here.
	if propID == "" || terminal.Segments != 2 {
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

// ⚠ **The confirm round trip against the REAL store — the coverage whose absence let split
// confirm ship broken (§10 V51a).**
//
// Every existing test above stops at Propose. Confirm was exercised only by `internal/filler`'s
// own suite, whose store keys clips by PATH — so it answered a lookup production does not
// perform, and the two defects underneath went unseen for two phases:
//
//  1. the proposal carried `clip.Path` while `Confirm` fed it to a hash-keyed `GetClip`, so every
//     confirm returned "compilation … no longer in the catalog" for a clip that was right there;
//  2. segments were upserted with an empty `Hash`, and `ON CONFLICT(hash)` made each one
//     overwrite the last — a 41-segment reel collapsing into a single row.
//
// This asserts the outcome an operator actually needs: N reviewed segments become N distinct,
// airable catalog rows, each pointing back at a parent that is still there.
func TestConfirmSplit_WritesEverySegmentAsItsOwnRow(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, "sqlite://"+t.TempDir()+"/f.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	// A real file at the sharded location the row claims, so the cut reads something.
	drop := t.TempDir()
	if err := os.MkdirAll(filepath.Join(drop, "a3", "f9"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(drop, compPath), []byte("compilation"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertClip(ctx, store.Clip{Clip: filler.Clip{
		Hash: compHash, Path: compPath, Name: "1987.mp4",
		Kind: filler.Commercial, DurationMs: 61_000, Source: "archive",
	}, UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}

	n := 0
	sp := filler.NewSplitter(fillerSplitStoreAdapter{st}, splitFakeTools{chapters: []filler.Chapter{
		{StartMs: 0, EndMs: 30_000, Title: "McDonald's"},
		{StartMs: 30_000, EndMs: 61_000, Title: "Lego"},
	}}, nil, drop, func() time.Duration { return 10 * time.Second },
		func() string { n++; return fmt.Sprintf("sp_%d", n) }, time.Now, nil)

	prop, err := sp.Propose(ctx, compHash)
	if err != nil {
		t.Fatal(err)
	}
	// The proposal addresses the compilation by IDENTITY, not by location.
	if prop.ClipHash != compHash {
		t.Fatalf("proposal.ClipHash = %q, want the compilation's hash %q", prop.ClipHash, compHash)
	}

	spawned, err := sp.Confirm(ctx, prop.ID, prop.Segments)
	if err != nil {
		t.Fatalf("Confirm: %v — this is the failure that made split review a dead end", err)
	}
	// ⚠ The returned hashes are what the pipeline enrols (§10 V51b). An empty slice here means a
	// confirmed reel produces segments nothing ever prepares — never transcoded, never tagged.
	if len(spawned) != len(prop.Segments) {
		t.Errorf("Confirm returned %d hashes for %d segments — every cut must be enrollable",
			len(spawned), len(prop.Segments))
	}

	clips, err := st.ListClips(ctx, store.ClipFilter{IncludeComposites: true})
	if err != nil {
		t.Fatal(err)
	}
	var segments []store.Clip
	var parent *store.Clip
	for i, c := range clips {
		if c.Hash == compHash {
			parent = &clips[i]
			continue
		}
		segments = append(segments, c)
	}

	if len(segments) != 2 {
		t.Fatalf("catalog holds %d segments, want 2 — a reel collapsing into one row is the "+
			"empty-hash upsert bug (%+v)", len(segments), segments)
	}
	seen := map[string]bool{}
	for _, s := range segments {
		if s.Hash == "" {
			t.Errorf("segment %q has no identity — UpsertClip keys on hash", s.Name)
		}
		if seen[s.Hash] {
			t.Errorf("two segments share the hash %q", s.Hash)
		}
		seen[s.Hash] = true
		if s.ParentHash != compHash {
			t.Errorf("segment %q parentHash = %q, want %q — lineage is the point of V45",
				s.Name, s.ParentHash, compHash)
		}
		// Filed where its identity says it lives, and the bytes are actually there.
		if want := filler.ClipRelPath(s.Hash, ".mp4"); s.Path != want {
			t.Errorf("segment %q path = %q, want the sharded %q", s.Name, s.Path, want)
		}
		if _, err := os.Stat(filepath.Join(drop, s.Path)); err != nil {
			t.Errorf("segment %q has a catalog row but no file: %v", s.Name, err)
		}
	}

	// ⚠ V45: the parent is KEPT and marked a composite, not deleted.
	if parent == nil {
		t.Fatal("the parent was deleted on confirm — V45 keeps it as a composite")
	}
	if !parent.IsComposite {
		t.Error("the parent survived but is not marked composite — it would still be airable")
	}
}

// --- V54a: a re-detect returns the reel to the belt ---------------------------

// parkReel puts the compilation in the state that was unreachable before V54a and returns a
// predicate reporting whether the pipeline can actually SEE it.
//
// ⚠ Claimability is asserted through `ListPipelineWork`, never by reading `Disposition` back. The
// defect was never "the column says review" — it was "nothing claims this row, ever", and a test
// that reads the field it just wrote passes against an un-park that leaves `next_run` in the
// future, which is the same dead end wearing a different value.
func parkReel(t *testing.T, st store.Store) func() bool {
	t.Helper()
	ctx := context.Background()
	if err := st.UpsertClipPipeline(ctx, filler.ClipPipeline{
		ClipHash: compHash, Stage: filler.StageSplit, Status: filler.StatusQueued,
		Disposition: filler.DispositionReview, Attempts: 3,
		NextRun:    time.Now().UTC().Add(24 * time.Hour), // parked rows are not due
		EnrolledAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	return func() bool {
		work, err := st.ListPipelineWork(ctx, time.Now().UTC(), 0)
		if err != nil {
			t.Fatalf("list pipeline work: %v", err)
		}
		for _, r := range work {
			if r.ClipHash == compHash {
				return true
			}
		}
		return false
	}
}

// awaitSplitTerminal drains the bus until this job's terminal frame lands, returning its status.
func awaitSplitTerminal(t *testing.T, ch <-chan events.Event, jobID string) string {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev := <-ch:
			if ev.Type != "filler_split" {
				continue
			}
			p, ok := ev.Payload.(api.FillerSplitEvent)
			if !ok || p.JobID != jobID {
				continue
			}
			if p.Status == "success" || p.Status == "error" {
				return p.Status
			}
		case <-deadline:
			t.Fatal("no terminal filler_split frame within 5s")
		}
	}
}

// The fix: `POST /v1/filler/split` on a parked reel puts it back where the pipeline can see it.
// Without this the re-detect writes a freshly scored proposal that no rung will ever read.
func TestSplit_ReDetectReturnsTheParkedReelToTheBelt(t *testing.T) {
	bus := events.NewBus()
	ch, unsub := bus.Subscribe()
	t.Cleanup(unsub)
	a, st := newSplitAdapter(t, bus, true)
	a.pipeline = filler.NewPipeline(st, fillerPipelineClipAdapter{st}, nil, filler.Budget{}, nil, time.Now, nil)
	onBelt := parkReel(t, st)

	if onBelt() {
		t.Fatal("precondition: a parked reel must not be claimable before the re-detect")
	}
	jobID, err := a.Split(context.Background(), compHash)
	if err != nil {
		t.Fatal(err)
	}
	if got := awaitSplitTerminal(t, ch, jobID); got != "success" {
		t.Fatalf("detection status = %q, want success", got)
	}
	if !onBelt() {
		t.Fatal("the reel is still invisible to ListPipelineWork — the re-detect did not un-park it")
	}
	row, _, err := st.GetClipPipeline(context.Background(), compHash)
	if err != nil {
		t.Fatal(err)
	}
	if row.Stage != filler.StageSplit {
		t.Errorf("stage = %q, want the reel left at the split rung", row.Stage)
	}
	if row.Attempts != 0 {
		t.Errorf("attempts = %d, want 0 — retries spent losing to the old gate are given back", row.Attempts)
	}
}

// ⚠ The ORDERING guarantee, observable through its consequence. The un-park must run only after a
// SUCCESSFUL detection: un-parking first makes the row claimable while detection is still running,
// and the rung would then ground the outgoing segment list or re-Propose the same reel
// concurrently. A failed detection must therefore leave the reel exactly as it found it.
func TestSplit_FailedDetectionLeavesTheReelParked(t *testing.T) {
	bus := events.NewBus()
	ch, unsub := bus.Subscribe()
	t.Cleanup(unsub)
	a, st := newSplitAdapter(t, bus, true)
	a.pipeline = filler.NewPipeline(st, fillerPipelineClipAdapter{st}, nil, filler.Budget{}, nil, time.Now, nil)
	onBelt := parkReel(t, st)

	// Detection refuses a clip with no probed duration — a failure raised INSIDE Propose, after
	// Split's synchronous existence check has already passed and the job is running.
	clip, err := st.GetClip(context.Background(), compHash)
	if err != nil {
		t.Fatal(err)
	}
	clip.DurationMs = 0
	clip.UpdatedAt = time.Now().UTC()
	if err := st.UpsertClip(context.Background(), clip); err != nil {
		t.Fatal(err)
	}

	jobID, err := a.Split(context.Background(), compHash)
	if err != nil {
		t.Fatal(err)
	}
	if got := awaitSplitTerminal(t, ch, jobID); got != "error" {
		t.Fatalf("detection status = %q, want error", got)
	}
	if onBelt() {
		t.Fatal("a FAILED detection un-parked the reel — the un-park is running before Propose, or regardless of it")
	}
	row, _, err := st.GetClipPipeline(context.Background(), compHash)
	if err != nil {
		t.Fatal(err)
	}
	if row.Attempts != 3 {
		t.Errorf("attempts = %d, want 3 — a failed detection must not reset the retry budget", row.Attempts)
	}
}
