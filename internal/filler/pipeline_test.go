package filler_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/platform/storagegovernor"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// --- doubles -----------------------------------------------------------------

// pipeMemStore is an in-memory PipelineStore + ClipStore.
//
// ⚠ Keyed by HASH on both halves, exactly as the real store is. The split fixtures keyed clips by
// PATH and hid two shipped bugs for two phases (§10 V51a); a double that indexes differently from
// the thing it stands in for cannot see key confusion by construction.
type pipeMemStore struct {
	clips     map[string]clipstore.Clip
	rows      map[string]pipelinestate.ClipPipeline
	proposals []filler.SplitProposal
	removed   []string
	held      []string
	holdErr   error
	commitErr error
}

func (m *pipeMemStore) HoldClips(_ context.Context, paths []string, _ time.Time) (int, error) {
	if m.holdErr != nil {
		return 0, m.holdErr
	}
	n := 0
	for _, p := range paths {
		for hash, c := range m.clips {
			if c.Path == p {
				c.Held = true
				m.clips[hash] = c
				n++
			}
		}
	}
	m.held = append(m.held, paths...)
	return n, nil
}

func (m *pipeMemStore) SetClipLanguage(context.Context, string, string, time.Time) error {
	return nil
}
func (m *pipeMemStore) SetClipTranscript(context.Context, string, string, time.Time) error {
	return nil
}
func (m *pipeMemStore) ClearClipVisibleText(context.Context, string, time.Time) error {
	return nil
}
func (m *pipeMemStore) ListSplitProposals(context.Context) ([]filler.SplitProposal, error) {
	return append([]filler.SplitProposal(nil), m.proposals...), nil
}
func (m *pipeMemStore) DeleteSplitProposal(context.Context, string) error { return nil }

func newPipeMemStore() *pipeMemStore {
	return &pipeMemStore{clips: map[string]clipstore.Clip{}, rows: map[string]pipelinestate.ClipPipeline{}}
}

func (m *pipeMemStore) put(c clipstore.Clip) { m.clips[c.Hash] = c }

// SetClipsRemoved records the tombstone the RUNNER writes on a reject (§10 V51b).
//
// ⚠ Keyed by PATH while the clip map is keyed by HASH, exactly as the real store is — and the
// fixture deliberately derives one from the other rather than setting both to the same string.
// A fixture that equated them is what hid the V38c identity split for two releases.
func (m *pipeMemStore) SetClipsRemoved(_ context.Context, paths []string, at time.Time) (int, error) {
	n := 0
	for _, p := range paths {
		for hash, c := range m.clips {
			if c.Path == p {
				c.RemovedAt = at
				m.clips[hash] = c
				n++
			}
		}
	}
	m.removed = append(m.removed, paths...)
	return n, nil
}

func (m *pipeMemStore) GetClip(_ context.Context, id string) (clipstore.Clip, bool, error) {
	c, ok := m.clips[id]
	return c, ok, nil
}

// ListPipelineWork orders like the real store, by next_run then clip_hash, before it applies the
// limit: a limit over map order would hand the engine a different clip each pass.
func (m *pipeMemStore) ListPipelineWork(_ context.Context, now time.Time, limit int) ([]pipelinestate.ClipPipeline, error) {
	var out []pipelinestate.ClipPipeline
	for _, r := range m.rows {
		if r.Disposition != pipelinestate.DispositionRunning {
			continue
		}
		if r.NextRun.After(now) {
			continue
		}
		out = append(out, r)
	}
	slices.SortFunc(out, func(a, b pipelinestate.ClipPipeline) int {
		if c := a.NextRun.Compare(b.NextRun); c != 0 {
			return c
		}
		return strings.Compare(a.ClipHash, b.ClipHash)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// The double hands out work in the real store's order, so a pass with a limit works the oldest due
// clips and never the same arbitrary subset: tie-broken by hash, due rows only.
func TestPipeMemStore_ListPipelineWorkOrdersByNextRunThenHash(t *testing.T) {
	st := newPipeMemStore()
	base := time.Unix(1_800_000_000, 0).UTC()
	for i, hash := range []string{"h", "g", "f", "e", "d", "c", "b", "a"} {
		st.rows[hash] = pipelinestate.ClipPipeline{
			ClipHash: hash, Disposition: pipelinestate.DispositionRunning,
			NextRun: base.Add(time.Duration(i/2) * time.Minute), // pairs share a next_run
		}
	}
	st.rows["later"] = pipelinestate.ClipPipeline{
		ClipHash: "later", Disposition: pipelinestate.DispositionRunning, NextRun: base.Add(time.Hour),
	}

	got, err := st.ListPipelineWork(context.Background(), base.Add(10*time.Minute), 5)
	if err != nil {
		t.Fatal(err)
	}
	var hashes []string
	for _, r := range got {
		hashes = append(hashes, r.ClipHash)
	}
	// Pairs by next_run (h,g) (f,e) (d,c) (b,a): within a pair the lower hash first.
	if want := []string{"g", "h", "e", "f", "c"}; !slices.Equal(hashes, want) {
		t.Fatalf("work = %v, want %v (next_run, then clip_hash, limit applied after the sort)", hashes, want)
	}
}

func (m *pipeMemStore) PipelineOverview(_ context.Context, at time.Time) (pipelinestate.PipelineOverview, error) {
	rows := make([]pipelinestate.ClipPipeline, 0, len(m.rows))
	for _, row := range m.rows {
		rows = append(rows, row)
	}
	return filler.SummarizePipelines(rows, at), nil
}

func (m *pipeMemStore) ListClipsWithoutPipeline(_ context.Context, limit int) ([]clipstore.Clip, error) {
	var out []clipstore.Clip
	for h, c := range m.clips {
		if _, ok := m.rows[h]; ok {
			continue
		}
		out = append(out, c)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (m *pipeMemStore) GetClipPipeline(_ context.Context, hash string) (pipelinestate.ClipPipeline, bool, error) {
	r, ok := m.rows[hash]
	return r, ok, nil
}

func (m *pipeMemStore) ListClipPipelines(_ context.Context, f pipelinestate.PipelineFilter) ([]pipelinestate.ClipPipeline, error) {
	var out []pipelinestate.ClipPipeline
	for _, r := range m.rows {
		if len(f.Dispositions) > 0 {
			matched := false
			for _, disposition := range f.Dispositions {
				matched = matched || r.Disposition == disposition
			}
			if !matched {
				continue
			}
		}
		out = append(out, r)
	}
	return out, nil
}

// ⚠ **It HONOURS cancellation, and that is the point** (§10 V51g). A fake that ignored `ctx` is
// why the original bug was invisible: `onFailure` computed the failure record, the backoff and the
// `MaxAttempts` resolution, then wrote them through the very context whose expiry caused the
// failure — and against a real store every one of those writes was silently discarded. With a fake
// that accepts anything, that code path looks perfect. Rejecting a dead context here is what makes
// the detached-write fix provable rather than asserted.
func (m *pipeMemStore) UpsertClipPipeline(ctx context.Context, p pipelinestate.ClipPipeline) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.rows[p.ClipHash] = p
	return nil
}

func (m *pipeMemStore) RetryClipPipeline(ctx context.Context, _ pipelinestate.ClipPipeline, p pipelinestate.ClipPipeline, restore bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.rows[p.ClipHash] = p
	if restore {
		c := m.clips[p.ClipHash]
		c.RemovedAt, c.Held = time.Time{}, true
		m.clips[p.ClipHash] = c
	}
	return nil
}

func (m *pipeMemStore) CommitFillerReady(_ context.Context, commit filler.ReadyCommit) error {
	if m.commitErr != nil {
		return m.commitErr
	}
	c, ok := m.clips[commit.Event.ClipHash]
	if !ok || !c.Held || c.IsComposite {
		return filler.ErrReadyStale
	}
	c.Held = false
	c.Placement = commit.Event.Placement
	m.clips[c.Hash] = c
	m.rows[c.Hash] = commit.Pipeline
	return nil
}

// fakeStage is a scriptable rung.
type fakeStage struct {
	id      pipelinestate.StageID
	cost    filler.StageCost
	applies bool
	note    string
	result  filler.StageResult
	err     error
	panicV  any
	runs    int
	killCtx func()
	// onRun runs as the rung starts, so a test can make the catalog change mid-pass the way a real
	// split or transcode does.
	onRun func()
}

func (f *fakeStage) ID() pipelinestate.StageID { return f.id }
func (f *fakeStage) Cost() filler.StageCost    { return f.cost }
func (f *fakeStage) Applies(context.Context, clipstore.Clip) (bool, string) {
	return f.applies, f.note
}
func (f *fakeStage) Run(ctx context.Context, _ clipstore.Clip) (filler.StageResult, error) {
	f.runs++
	if f.onRun != nil {
		f.onRun()
	}
	if f.panicV != nil {
		panic(f.panicV)
	}
	// killCtx models the rung whose work outlives the pass: the deadline lands mid-exec, the
	// child process dies, and the stage reports the context error — exactly what ffmpeg, whisper
	// and an LLM turn all do when the budget ends underneath them.
	if f.killCtx != nil {
		f.killCtx()
		return filler.StageResult{}, ctx.Err()
	}
	return f.result, f.err
}

// A rung panic must fail ONE clip, not abort the scheduled pass and leave that row permanently
// `running`. The scheduler has its own last-resort recovery, but it is too far out to persist the
// clip's retry/backoff state or let work behind it advance.
func TestPipeline_StagePanicBecomesARecoverableClipFailure(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "c1")
	stages := allStages()
	stages[pipelinestate.StageVision].panicV = "classifier exploded"

	p := newPipe(st, asSlice(stages), filler.DefaultBudget())
	if _, err := p.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	row := st.rows["c1"]
	if row.Stage != pipelinestate.StageVision || row.Status != pipelinestate.StatusFailed || row.Attempts != 1 {
		t.Fatalf("panic state = %q/%q attempts=%d, want vision/failed attempts=1", row.Stage, row.Status, row.Attempts)
	}
	if row.NextRun.IsZero() {
		t.Error("panic did not receive ordinary failure backoff")
	}
	if got := row.Stages[len(row.Stages)-1].Note; got != "stage vision panicked: classifier exploded" {
		t.Fatalf("panic note = %q", got)
	}
}

func TestPipeline_ScreeningPersistenceFailureNeverFallsThroughToReadiness(t *testing.T) {
	for _, blockedStage := range []pipelinestate.StageID{pipelinestate.StageScreen} {
		t.Run(string(blockedStage), func(t *testing.T) {
			st := newPipeMemStore()
			seedEnrolled(st, "c1")
			stages := allStages()
			stages[blockedStage].err = errors.New("authority store unavailable")

			p := newPipe(st, asSlice(stages), filler.DefaultBudget())
			for range filler.MaxAttempts + 1 {
				if _, err := p.RunOnce(context.Background()); err != nil {
					t.Fatal(err)
				}
				st.rows["c1"] = resetSchedule(st.rows["c1"])
			}

			row := st.rows["c1"]
			if row.Stage != blockedStage || row.Status != pipelinestate.StatusFailed ||
				row.Disposition != pipelinestate.DispositionRunning || row.Attempts != filler.MaxAttempts {
				t.Fatalf("authority failure = %q/%q disposition=%q attempts=%d",
					row.Stage, row.Status, row.Disposition, row.Attempts)
			}
			if stages[pipelinestate.StageFinish].runs != 0 {
				t.Fatalf("score ran %d times without stage %q persistence", stages[pipelinestate.StageFinish].runs, blockedStage)
			}
		})
	}
}

func stage(id pipelinestate.StageID) *fakeStage {
	return &fakeStage{id: id, applies: true, result: filler.StageResult{Verdict: pipelinestate.VerdictContinue}}
}

// allStages returns a passing stage for every rung, so a test can replace just the one it cares
// about and still exercise a complete ladder.
func allStages() map[pipelinestate.StageID]*fakeStage {
	out := map[pipelinestate.StageID]*fakeStage{}
	for _, id := range pipelinestate.StageOrder {
		out[id] = stage(id)
	}
	return out
}

func asSlice(m map[pipelinestate.StageID]*fakeStage) []filler.Stage {
	var out []filler.Stage
	for _, id := range pipelinestate.StageOrder {
		out = append(out, m[id])
	}
	return out
}

func seedEnrolled(st *pipeMemStore, hash string) {
	at := time.Unix(1_799_999_000, 0).UTC()
	st.put(clipstore.Clip{Clip: clips.Clip{
		Hash: hash, Path: "a/b/" + hash + ".mp4", Name: hash,
		Role: clips.Unclassified, Source: "test-source", Held: true,
	}})
	st.rows[hash] = pipelinestate.ClipPipeline{
		ClipHash: hash, Stage: pipelinestate.StageProbe, Status: pipelinestate.StatusQueued,
		Disposition: pipelinestate.DispositionRunning, EnrolledAt: at, UpdatedAt: at,
		PreparationAttempt: 1, PreparationStartedAt: at,
		PreparationStartReason: pipelinestate.PreparationStartedByEnrollment, PreparationProgress: 0,
		StageQueuedAt: at,
	}
}

func newPipe(st *pipeMemStore, stages []filler.Stage, b filler.Budget) *filler.Pipeline {
	at := time.Unix(1_800_000_000, 0).UTC()
	return filler.NewPipeline(st, st, stages, b, nil, func() time.Time { return at }, nil)
}

// --- tests -------------------------------------------------------------------

// A clip that clears every rung ends Ready, having visited each stage exactly once.
func TestPipeline_WalksEveryStageAndBecomesReady(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "c1")
	stages := allStages()

	if _, err := newPipe(st, asSlice(stages), filler.DefaultBudget()).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	row := st.rows["c1"]
	if row.Disposition != pipelinestate.DispositionReady {
		t.Fatalf("disposition = %q, want ready", row.Disposition)
	}
	if row.PreparationProgress != 100 {
		t.Fatalf("preparation progress = %d, want exact Ready completion", row.PreparationProgress)
	}
	if len(row.Stages) != len(pipelinestate.StageOrder) {
		t.Errorf("ladder has %d rungs, want %d — every stage must be recorded, including the boring ones",
			len(row.Stages), len(pipelinestate.StageOrder))
	}
	for id, s := range stages {
		if s.runs != 1 {
			t.Errorf("stage %s ran %d times, want 1", id, s.runs)
		}
	}
}

func TestPipeline_RewindResetsOnlyTheRequestedSuffix(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "stuck")
	row := st.rows["stuck"]
	row.Stage = pipelinestate.StageFinish
	row.Status = pipelinestate.StatusDone
	row.Attempts = 3
	row.Disposition = pipelinestate.DispositionReview
	for _, id := range pipelinestate.StageOrder {
		row.Stages = append(row.Stages, pipelinestate.StageRecord{Stage: id, Status: pipelinestate.StatusDone})
	}
	st.rows["stuck"] = row

	p := newPipe(st, nil, filler.DefaultBudget()).WithRewind(st, "")
	if err := p.Rewind(context.Background(), "stuck", pipelinestate.StageVision, false); err != nil {
		t.Fatal(err)
	}
	got := st.rows["stuck"]
	if got.Stage != pipelinestate.StageVision || got.Status != pipelinestate.StatusQueued || got.Attempts != 0 || got.Disposition != pipelinestate.DispositionRunning {
		t.Fatalf("rewound row = %+v, want vision/queued/running with fresh attempts", got)
	}
	if !got.ForceRun {
		t.Fatal("rewind did not persist the explicit rerun instruction")
	}
	if len(got.Stages) != pipelinestate.StageIndex(pipelinestate.StageVision) {
		t.Fatalf("kept %d stages, want only %d stages before vision", len(got.Stages), pipelinestate.StageIndex(pipelinestate.StageVision))
	}
	if got.PreparationAttempt != 2 || got.PreparationStartReason != pipelinestate.PreparationStartedByRestart || got.PreparationProgress != 0 {
		t.Fatalf("rewind attempt = (%d, %q, %d), want generation 2 restart at zero", got.PreparationAttempt, got.PreparationStartReason, got.PreparationProgress)
	}
}

// A restart is also a withdrawal from playback. A Ready clip is unheld by definition; resetting
// only its conveyor leaves it simultaneously playable and "preparing", then terminal readiness
// rejects it forever because every score attempt still sees the unheld catalog row.
func TestPipeline_RewindReadyClipHoldsItUntilTheRestartCompletes(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "ready-restart")
	clip := st.clips["ready-restart"]
	clip.Held = false
	clip.Placement = clips.PlacementBreakBody
	st.clips[clip.Hash] = clip
	row := st.rows[clip.Hash]
	row.Stage = pipelinestate.StageFinish
	row.Status = pipelinestate.StatusDone
	row.Disposition = pipelinestate.DispositionReady
	row.PreparationProgress = 100
	for _, id := range pipelinestate.StageOrder {
		row.Stages = append(row.Stages, pipelinestate.StageRecord{Stage: id, Status: pipelinestate.StatusDone})
	}
	st.rows[clip.Hash] = row

	p := newPipe(st, asSlice(allStages()), filler.DefaultBudget()).WithRewind(st, "")
	if err := p.Rewind(t.Context(), clip.Hash, pipelinestate.StageFinish, false); err != nil {
		t.Fatal(err)
	}
	if !st.clips[clip.Hash].Held {
		t.Fatal("rewound Ready clip remained playable while its pipeline was running")
	}
	if _, err := p.RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	got := st.rows[clip.Hash]
	if got.Disposition != pipelinestate.DispositionReady || got.Status != pipelinestate.StatusDone || got.Attempts != 1 {
		t.Fatalf("restarted clip = %+v, want one score attempt ending Ready", got)
	}
}

func TestPipeline_RewindDoesNotResetTheConveyorWhenHoldingFails(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "ready-hold-failure")
	clip := st.clips["ready-hold-failure"]
	clip.Held = false
	st.clips[clip.Hash] = clip
	row := st.rows[clip.Hash]
	row.Stage = pipelinestate.StageFinish
	row.Status = pipelinestate.StatusDone
	row.Disposition = pipelinestate.DispositionReady
	st.rows[clip.Hash] = row
	st.holdErr = errors.New("catalog write failed")

	p := newPipe(st, nil, filler.DefaultBudget()).WithRewind(st, "")
	if err := p.Rewind(t.Context(), clip.Hash, pipelinestate.StageFinish, false); !errors.Is(err, st.holdErr) {
		t.Fatalf("Rewind error = %v, want hold failure", err)
	}
	if got := st.rows[clip.Hash]; got.Stage != row.Stage || got.Status != row.Status ||
		got.Disposition != row.Disposition || got.PreparationAttempt != row.PreparationAttempt {
		t.Fatalf("hold failure reset conveyor: got %+v, want unchanged %+v", got, row)
	}
}

func TestPipeline_RewindRunsAStageThatWouldNormallySkip(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "already-tagged")
	row := st.rows["already-tagged"]
	row.Stage = pipelinestate.StageFinish
	row.Status = pipelinestate.StatusDone
	row.Disposition = pipelinestate.DispositionReview
	for _, id := range pipelinestate.StageOrder {
		row.Stages = append(row.Stages, pipelinestate.StageRecord{Stage: id, Status: pipelinestate.StatusDone})
	}
	st.rows["already-tagged"] = row

	vision := stage(pipelinestate.StageVision)
	vision.applies = false
	vision.note = "already inspected"
	p := newPipe(st, []filler.Stage{vision, stage(pipelinestate.StageFinish)}, filler.DefaultBudget()).WithRewind(st, "")
	if err := p.Rewind(context.Background(), "already-tagged", pipelinestate.StageVision, false); err != nil {
		t.Fatal(err)
	}
	if _, err := p.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if vision.runs != 1 {
		t.Fatalf("vision runs = %d, want 1 after an explicit rewind", vision.runs)
	}
	if st.rows["already-tagged"].ForceRun {
		t.Fatal("forced rerun marker survived the requested stage")
	}
}

// A reject Loomarr let go (#3855) has no files; a rewind would put it back on the conveyor with
// its reason erased.
func TestPipeline_RewindRefusesAClipWhoseFilesWereDeleted(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "let-go")
	row := st.rows["let-go"]
	row.Disposition, row.RejectReason = pipelinestate.DispositionRejected, pipelinestate.ReasonUnplayable
	row.FilesDeletedAt = time.Unix(1_900_000_000, 0).UTC()
	st.rows["let-go"] = row
	p := newPipe(st, nil, filler.DefaultBudget()).WithRewind(st, "")
	if err := p.Rewind(t.Context(), "let-go", pipelinestate.StageVision, false); !errors.Is(err, filler.ErrFilesDeleted) {
		t.Fatalf("Rewind = %v, want ErrFilesDeleted", err)
	}
	if got := st.rows["let-go"]; got.Disposition != row.Disposition || got.RejectReason != row.RejectReason {
		t.Fatalf("refused rewind changed the row: %+v", got)
	}
}

func TestPipeline_RewindTranscodeRequiresExplicitForce(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "encoded")
	p := newPipe(st, nil, filler.DefaultBudget()).WithRewind(st, "")
	before := st.rows["encoded"]
	if err := p.Rewind(context.Background(), "encoded", pipelinestate.StageTranscode, false); !errors.Is(err, filler.ErrTranscodeNeedsForce) {
		t.Fatalf("rewind transcode = %v, want ErrTranscodeNeedsForce", err)
	}
	if got := st.rows["encoded"]; got.Stage != before.Stage || got.Status != before.Status {
		t.Errorf("refused rewind mutated row: before=%+v after=%+v", before, got)
	}
}

func TestPipeline_RetryFailureRestoresTerminalExecutionFailureHeld(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "broken-encode")
	c := st.clips["broken-encode"]
	c.RemovedAt, c.Held = time.Now().UTC(), false
	st.clips[c.Hash] = c
	row := st.rows[c.Hash]
	row.Stage, row.Status = pipelinestate.StageTranscode, pipelinestate.StatusFailed
	row.Attempts, row.Disposition = filler.MaxAttempts, pipelinestate.DispositionRejected
	row.RejectReason, row.RejectDetail = pipelinestate.ReasonUnplayable, "ffmpeg exited 1"
	row.PreparationProgress = 37
	row.Stages = []pipelinestate.StageRecord{
		{Stage: pipelinestate.StageProbe, Status: pipelinestate.StatusDone},
		{Stage: pipelinestate.StageTranscode, Status: pipelinestate.StatusFailed},
	}
	st.rows[c.Hash] = row

	p := newPipe(st, nil, filler.DefaultBudget()).WithRewind(st, "")
	if err := p.RetryFailure(context.Background(), c.Hash); err != nil {
		t.Fatal(err)
	}
	got := st.rows[c.Hash]
	if got.Stage != pipelinestate.StageTranscode || got.Status != pipelinestate.StatusQueued ||
		got.Disposition != pipelinestate.DispositionRunning || got.Attempts != 0 || !got.ForceRun {
		t.Fatalf("retried row = %+v, want transcode/queued/running with force", got)
	}
	if len(got.Stages) != 1 || got.Stages[0].Stage != pipelinestate.StageProbe {
		t.Fatalf("retry discarded completed upstream work: %+v", got.Stages)
	}
	if got.PreparationAttempt != row.PreparationAttempt || got.PreparationProgress != 37 || got.PreparationStartReason != row.PreparationStartReason {
		t.Fatalf("retry changed whole attempt: before=%+v after=%+v", row, got)
	}
	clip := st.clips[c.Hash]
	if !clip.RemovedAt.IsZero() || !clip.Held {
		t.Fatalf("restored clip = removed:%v held:%v, want present and held", clip.RemovedAt, clip.Held)
	}
}

func TestPipeline_RetryFailureRefusesContentDecision(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "silent")
	row := st.rows["silent"]
	row.Stage, row.Status = pipelinestate.StageTranscode, pipelinestate.StatusDone
	row.Disposition, row.RejectReason = pipelinestate.DispositionRejected, pipelinestate.ReasonSilentContent
	st.rows[row.ClipHash] = row

	err := newPipe(st, nil, filler.DefaultBudget()).WithRewind(st, "").RetryFailure(context.Background(), row.ClipHash)
	if !errors.Is(err, filler.ErrPipelineNotRetryable) {
		t.Fatalf("retry content decision = %v, want ErrPipelineNotRetryable", err)
	}
	if got := st.rows[row.ClipHash]; got.Disposition != pipelinestate.DispositionRejected {
		t.Fatalf("refused retry mutated row: %+v", got)
	}
}

func TestPipeline_DoesNotFileCompositeStillWaitingForDecision(t *testing.T) {
	st := newPipeMemStore()
	const hash = "review-reel"
	st.put(clipstore.Clip{Clip: clips.Clip{
		Hash: hash, Path: "reels/review.mp4", Name: "Review reel",
		IsComposite: true, Held: true,
	}})
	st.rows[hash] = pipelinestate.ClipPipeline{
		ClipHash: hash, Stage: pipelinestate.StageSplit, Status: pipelinestate.StatusDone,
		Disposition: pipelinestate.DispositionReview,
	}

	if _, err := newPipe(st, nil, filler.DefaultBudget()).WithRewind(st, "").RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := st.clips[hash]; !got.Held {
		t.Fatalf("composite still waiting for a decision was filed: %+v", got)
	}
}

func TestPipeline_ReviewHoldsAPreviouslyFiledClipOutOfRotation(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "quality-review")
	row := st.rows["quality-review"]
	row.Stage = pipelinestate.StageTranscode
	st.rows["quality-review"] = row
	quality := stage(pipelinestate.StageTranscode)
	quality.result = filler.StageResult{
		Verdict: pipelinestate.VerdictReview,
		Note:    "The picture is unchanged for 12.0s; this may be intentional.",
	}

	if _, err := newPipe(st, []filler.Stage{quality}, filler.DefaultBudget()).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	clip := st.clips["quality-review"]
	if !clip.Held || len(st.held) != 1 {
		t.Fatalf("reviewed clip remained airable: clip=%+v held writes=%v", clip, st.held)
	}
}

func TestPipeline_ReviewIsRetriedWhenTheAirabilityHoldFails(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "quality-review-retry")
	row := st.rows["quality-review-retry"]
	row.Stage = pipelinestate.StageTranscode
	st.rows["quality-review-retry"] = row
	quality := stage(pipelinestate.StageTranscode)
	quality.result = filler.StageResult{Verdict: pipelinestate.VerdictReview, Note: "picture unchanged"}
	pipe := newPipe(st, []filler.Stage{quality}, filler.DefaultBudget())

	st.holdErr = errors.New("store unavailable")
	res, err := pipe.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed != 1 || st.rows["quality-review-retry"].Disposition != pipelinestate.DispositionRunning {
		t.Fatalf("failed hold became terminal: result=%+v row=%+v", res, st.rows["quality-review-retry"])
	}

	st.holdErr = nil
	res, err = pipe.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Completed != 1 || st.rows["quality-review-retry"].Disposition != pipelinestate.DispositionReview {
		t.Fatalf("review did not recover: result=%+v row=%+v", res, st.rows["quality-review-retry"])
	}
	if !st.clips["quality-review-retry"].Held || quality.runs != 2 {
		t.Fatalf("hold was not retried: clip=%+v runs=%d", st.clips["quality-review-retry"], quality.runs)
	}
}

// A stage that does not apply is recorded as SKIPPED with its reason, and the clip advances.
//
// ⚠ The reason is the point. §10 makes transcription deliberately selective, so a stage that
// silently does not happen reads as broken — the same rule that makes a reached limit reported
// rather than silent.
func TestPipeline_SkipRecordsWhyAndAdvances(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "c1")
	stages := allStages()
	stages[pipelinestate.StageTranscribe].applies = false
	stages[pipelinestate.StageTranscribe].note = "the description already says enough"

	if _, err := newPipe(st, asSlice(stages), filler.DefaultBudget()).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	if stages[pipelinestate.StageTranscribe].runs != 0 {
		t.Error("a stage that does not apply was executed anyway")
	}
	row := st.rows["c1"]
	if row.Disposition != pipelinestate.DispositionReady {
		t.Errorf("a skipped stage stranded the clip: %q", row.Disposition)
	}
	var found bool
	for _, r := range row.Stages {
		if r.Stage == pipelinestate.StageTranscribe {
			found = true
			if r.Status != pipelinestate.StatusSkipped || r.Note != "the description already says enough" {
				t.Errorf("skip recorded as %+v — the reason must survive", r)
			}
		}
	}
	if !found {
		t.Error("the skipped stage is missing from the ladder entirely")
	}
}

// A rejecting stage stops the clip with its CODE and its measured detail, and nothing downstream runs.
func TestPipeline_RejectStopsTheClipWithItsReason(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "c1")
	stages := allStages()
	stages[pipelinestate.StageProbe].result = filler.StageResult{
		Verdict: pipelinestate.VerdictReject, Reason: pipelinestate.ReasonTooShort, Detail: "8.2s; the floor is 10.0s",
	}

	if _, err := newPipe(st, asSlice(stages), filler.DefaultBudget()).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	row := st.rows["c1"]
	if row.Disposition != pipelinestate.DispositionRejected {
		t.Fatalf("disposition = %q, want rejected", row.Disposition)
	}
	if row.RejectReason != pipelinestate.ReasonTooShort || row.RejectDetail != "8.2s; the floor is 10.0s" {
		t.Errorf("reject lost its reason/detail: %q / %q", row.RejectReason, row.RejectDetail)
	}
	if stages[pipelinestate.StageFinish].runs != 0 {
		t.Error("a rejected clip carried on through the rest of the pipeline")
	}
	assertRowAgreesWithItsLadder(t, row)
}

// assertRowAgreesWithItsLadder checks the one invariant that ties `Status` to the ladder: the row's
// Status IS whatever its current rung recorded.
//
// ⚠ **This is asserted rather than the literal value `done`, because the value is not the point.**
// A verdict path that records `failed` and a verdict path that records `done` are both correct; a
// row whose Status says `running` while its own entry for the SAME rung says otherwise is not, and
// that disagreement is what shipped. Checking the relationship catches every future verdict path,
// including ones with a status neither of these tests names.
func assertRowAgreesWithItsLadder(t *testing.T, row pipelinestate.ClipPipeline) {
	t.Helper()
	for _, rung := range row.Stages {
		if rung.Stage != row.Stage {
			continue
		}
		if row.Status != rung.Status {
			t.Errorf("row says %s/%q but its own ladder entry for %s says %q — one row disagreeing with itself",
				row.Stage, row.Status, rung.Stage, rung.Status)
		}
		return
	}
	t.Errorf("the current rung %q has no ladder entry at all", row.Stage)
}

// ⚠ A clip handed to a PERSON must stop looking like work in progress.
//
// `Disposition` is the clip's outcome and `Status` is the current rung's — and the verdict paths
// set the first, recorded the rung, and left the second at the `running` written on entry. Nothing
// functional broke (every store predicate keys on `disposition`), so the suite stayed green; what
// broke was the picture. `ClipPipeline.resolve` on the frontend prefers `row.stage`/`row.status`
// over the visited ladder — correctly, since a rung mid-run has no entry yet — so the pip pulsed
// "in progress" forever and the rung's note was never rendered. Measured live (§10 V51g): eight
// reels, a local station's among them, every one finished within seconds and every one drawn as still working.
func TestPipeline_ReviewStopsLookingLikeWorkInProgress(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "c1")
	stages := allStages()
	stages[pipelinestate.StageProbe].result = filler.StageResult{
		Verdict: pipelinestate.VerdictReview, Note: "a segment could not be classified",
	}

	if _, err := newPipe(st, asSlice(stages), filler.DefaultBudget()).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	row := st.rows["c1"]
	if row.Disposition != pipelinestate.DispositionReview {
		t.Fatalf("disposition = %q, want review", row.Disposition)
	}
	if row.Status == pipelinestate.StatusRunning {
		t.Error("a clip waiting on a person still reports status=running — the belt draws it busy forever")
	}
	assertRowAgreesWithItsLadder(t, row)
}

// ⚠ A backend failure says nothing about the CLIP, so a non-fatal stage that exhausts its retries
// SKIPS and the clip advances. A missing transcript must never strand a commercial.
func TestPipeline_NonFatalFailureSkipsRatherThanStranding(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "c1")
	stages := allStages()
	stages[pipelinestate.StageTranscribe].err = fmt.Errorf("no whisper on this machine")

	p := newPipe(st, asSlice(stages), filler.DefaultBudget())
	// Each pass burns one attempt; the clock is frozen, so drive it explicitly.
	for i := 0; i < filler.MaxAttempts; i++ {
		if _, err := p.RunOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
		st.rows["c1"] = resetSchedule(st.rows["c1"])
	}

	row := st.rows["c1"]
	if row.Disposition == pipelinestate.DispositionRejected {
		t.Fatal("a missing backend REJECTED the clip — that is a fact about the machine, not the file")
	}
	if row.Disposition != pipelinestate.DispositionReady {
		t.Errorf("disposition = %q, want the clip to have carried on", row.Disposition)
	}
}

// ⚠ …but probe and transcode ARE facts about the file, so exhausting their retries rejects.
func TestPipeline_FatalFailureRejects(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "c1")
	stages := allStages()
	stages[pipelinestate.StageProbe].err = fmt.Errorf("moov atom not found")

	p := newPipe(st, asSlice(stages), filler.DefaultBudget())
	for i := 0; i < filler.MaxAttempts; i++ {
		if _, err := p.RunOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
		st.rows["c1"] = resetSchedule(st.rows["c1"])
	}

	row := st.rows["c1"]
	if row.Disposition != pipelinestate.DispositionRejected || row.RejectReason != pipelinestate.ReasonUnprobeable {
		t.Fatalf("unprobeable clip = %q / %q, want rejected/unprobeable", row.Disposition, row.RejectReason)
	}
	// ⚠ The fatal branch had the SAME omission as the verdict paths, one function away: it records
	// the rung `failed` and sets the disposition, and left Status at `running`.
	assertRowAgreesWithItsLadder(t, row)
}

// ⚠ **No room is not a broken file** (#2031). A household install had 33 of 35 `unplayable`
// rejections caused by `reserve transcode storage (library_limit)`: the storage governor refused
// the work MaxAttempts times and the fatal transcode rung rejected a good clip. However many passes
// the governor refuses, the clip must stay on the conveyor, spend no attempt, and say why it waits.
//
// Both doors are covered: the lease refused before the encode starts, and the write monitor pausing
// an encode that outgrew its lease.
func TestPipeline_AStorageHoldNeverRejectsTheClip(t *testing.T) {
	for _, tc := range []struct {
		name   string
		reason storagegovernor.Reason
		op     string
	}{
		{"reserve refused", storagegovernor.ReasonLibraryLimit, "reserve transcode storage"},
		{"write outgrew its lease", storagegovernor.ReasonHostReserve, "storage write paused"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := newPipeMemStore()
			seedEnrolled(st, "c1")
			row := st.rows["c1"]
			row.Stage = pipelinestate.StageTranscode
			st.rows["c1"] = row
			stages := allStages()
			stages[pipelinestate.StageTranscode].err = fmt.Errorf("transcode: %w", storagegovernor.Pause(tc.op,
				storagegovernor.Decision{Snapshot: storagegovernor.Snapshot{Reason: tc.reason}}))

			at := time.Unix(1_800_000_000, 0).UTC()
			p := newPipe(st, asSlice(stages), filler.DefaultBudget())
			for i := 0; i < filler.MaxAttempts+2; i++ {
				if _, err := p.RunOnce(context.Background()); err != nil {
					t.Fatal(err)
				}
				// Space frees on a cleanup, not within a pass: the clip must not ask again at once.
				if wait := st.rows["c1"].NextRun.Sub(at); wait < 10*time.Minute {
					t.Fatalf("pass %d: held clip due again in %v, want it to wait for space", i, wait)
				}
				st.rows["c1"] = resetSchedule(st.rows["c1"])
			}

			got := st.rows["c1"]
			if got.Disposition != pipelinestate.DispositionRunning || got.RejectReason != "" {
				t.Fatalf("storage-held clip = %q / %q, want still running and unrejected", got.Disposition, got.RejectReason)
			}
			if got.Stage != pipelinestate.StageTranscode || got.Status != pipelinestate.StatusQueued || got.Attempts != 0 {
				t.Fatalf("storage-held row = %q/%q attempts=%d, want transcode/queued with no attempt spent",
					got.Stage, got.Status, got.Attempts)
			}
			if !st.clips["c1"].RemovedAt.IsZero() {
				t.Fatal("a storage hold tombstoned the clip")
			}
			last := got.Stages[len(got.Stages)-1]
			if last.Stage != pipelinestate.StageTranscode || !strings.Contains(last.Note, string(tc.reason)) {
				t.Fatalf("ladder note = %+v, want the transcode rung to say it is waiting for storage", last)
			}
		})
	}
}

// A failure short of the retry limit BACKS OFF rather than resolving — it stays on the same stage,
// scheduled into the future.
func TestPipeline_FailureBacksOffBeforeGivingUp(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "c1")
	stages := allStages()
	stages[pipelinestate.StageProbe].err = fmt.Errorf("transient")

	p := newPipe(st, asSlice(stages), filler.DefaultBudget())
	if _, err := p.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	row := st.rows["c1"]
	if row.Stage != pipelinestate.StageProbe || row.Status != pipelinestate.StatusFailed {
		t.Fatalf("after one failure = %q/%q, want it still on probe and failed", row.Stage, row.Status)
	}
	if row.NextRun.IsZero() {
		t.Error("no backoff was scheduled — the clip would be retried immediately, at full cost, forever")
	}
	if row.Disposition != pipelinestate.DispositionRunning {
		t.Errorf("one failure resolved the clip to %q; it has attempts left", row.Disposition)
	}
}

// ⚠ **THE bug this phase exists for, and it had no test at all** (§10 V51g). A rung whose work
// outlives the pass looped forever: `attempts` was incremented and saved BEFORE the work, the
// failure bookkeeping was written through the context that had just expired and was therefore
// discarded, and the row stayed `running` at whatever attempt count it had reached. Measured live:
// one 16m47s reel at **12 attempts against a MaxAttempts of 3**, re-doing the same first third
// every two minutes while the UI showed it advancing.
//
// Out of TIME is not failing. The clip goes back to `queued`, spends no attempt, takes no backoff,
// and resumes next pass.
func TestPipeline_ADeadlineDefersInsteadOfBurningAnAttempt(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "c1")
	stages := allStages()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stages[pipelinestate.StageProbe].killCtx = cancel

	res, err := newPipe(st, asSlice(stages), filler.DefaultBudget()).RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if res.Deferred != 1 || res.Failed != 0 {
		t.Fatalf("deferred=%d failed=%d, want 1 and 0 — a deadline is not a failure", res.Deferred, res.Failed)
	}
	row := st.rows["c1"]
	// ⚠ The write LANDED despite the dead context. That is the whole fix: before it, this row
	// still said `running` at attempt 1 because the save was made through the cancelled context.
	if row.Status != pipelinestate.StatusQueued {
		t.Errorf("status = %q, want queued — the row must resume, not sit in `running` forever", row.Status)
	}
	if row.Attempts != 0 {
		t.Errorf("attempts = %d, want 0 — the clip did nothing wrong; the pass ran out of time", row.Attempts)
	}
	// ⚠ It YIELDS. Found by watching the fix starve the queue: the work list is oldest-first, so a
	// clip that cannot fit a pass took the whole budget again on the next one, and 84 other clips
	// were never reached. Not a penalty — no attempt was spent — but it goes behind work that can
	// finish. A ZERO NextRun here is the starvation bug, which is why this asserts the opposite of
	// what a failure-backoff test would.
	if !row.NextRun.After(row.UpdatedAt) {
		t.Error("a deferred clip is due immediately; it will take the next pass too, and the queue behind it starves")
	}
	if row.Disposition != pipelinestate.DispositionRunning {
		t.Errorf("disposition = %q, want running", row.Disposition)
	}
}

func TestPipeline_AStageCanYieldAPersistedBatchWithoutFailing(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "c1")
	stages := allStages()
	stages[pipelinestate.StageProbe].err = filler.ErrDeferred

	res, err := newPipe(st, asSlice(stages), filler.DefaultBudget()).RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Deferred != 1 || res.Failed != 0 {
		t.Fatalf("deferred=%d failed=%d, want 1 and 0", res.Deferred, res.Failed)
	}
	row := st.rows["c1"]
	if row.Stage != pipelinestate.StageProbe || row.Status != pipelinestate.StatusQueued {
		t.Fatalf("yielded row = %q/%q, want probe/queued", row.Stage, row.Status)
	}
	if row.Attempts != 0 {
		t.Errorf("attempts = %d, want 0 — a completed batch is progress, not a failure", row.Attempts)
	}
	if !row.NextRun.After(row.UpdatedAt) {
		t.Error("a yielded batch did not give the rest of the queue a turn")
	}
}

// …and the run summary says DEFERRED, not failed. A reel too slow for one pass produced an
// identical WARN every two minutes forever, and the count blamed the clip for the budget.
func TestPipeline_ADeferralIsNotCountedAsAFailure(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "c1")
	stages := allStages()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stages[pipelinestate.StageProbe].killCtx = cancel

	res, err := newPipe(st, asSlice(stages), filler.DefaultBudget()).RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if res.Failed != 0 {
		t.Errorf("failed = %d, want 0 — running out of time is not the clip failing", res.Failed)
	}
	if res.Deferred != 1 {
		t.Errorf("deferred = %d, want 1", res.Deferred)
	}
}

// Budget exhaustion DEFERS: the clip stays queued on the stage it could not afford, so the next
// pass resumes exactly there rather than skipping the expensive rung.
func TestPipeline_BudgetExhaustionDefersRatherThanSkips(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "c1")
	stages := allStages()
	stages[pipelinestate.StageTranscribe].cost = filler.CostWhisper

	b := filler.DefaultBudget()
	// ⚠ ZERO, not nil. A nil closure means "use the default"; zero means "none may be spent", which
	// is how an operator turns an expensive stage off on a busy box.
	b.MaxWhisper = func() int { return 0 }
	if _, err := newPipe(st, asSlice(stages), b).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	row := st.rows["c1"]
	if stages[pipelinestate.StageTranscribe].runs != 0 {
		t.Error("an unaffordable stage ran anyway")
	}
	if row.Stage != pipelinestate.StageTranscribe || row.Status != pipelinestate.StatusQueued {
		t.Fatalf("deferred clip = %q/%q, want it queued on transcribe", row.Stage, row.Status)
	}
	if row.Disposition != pipelinestate.DispositionRunning {
		t.Errorf("running out of budget resolved the clip to %q — it is not finished, it is waiting",
			row.Disposition)
	}
}

// Clips a stage CREATES (split segments) are enrolled at the start of the pipeline, so each one
// runs the whole ladder itself rather than inheriting its parent's progress.
func TestPipeline_SpawnedClipsAreEnrolledFromTheStart(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "reel")
	stages := allStages()
	stages[pipelinestate.StageSplit].result = filler.StageResult{
		Verdict: pipelinestate.VerdictContinue, Spawned: []string{"seg1"},
	}
	// The segment exists only once the split has cut it, which is after the pass's own enrolment.
	stages[pipelinestate.StageSplit].onRun = func() {
		st.put(clipstore.Clip{Clip: clips.Clip{Hash: "seg1", Path: "a/b/seg1.mp4"}})
	}

	if _, err := newPipe(st, asSlice(stages), filler.DefaultBudget()).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	seg, ok := st.rows["seg1"]
	if !ok {
		t.Fatal("a spawned segment was never enrolled — it would sit in the catalog untagged forever")
	}
	if seg.Stage != pipelinestate.StageProbe || seg.Disposition != pipelinestate.DispositionRunning {
		t.Errorf("spawned segment = %q/%q, want probe/running", seg.Stage, seg.Disposition)
	}
}

// A byte-changing stage returns the replacement identity after atomically re-keying storage.
// The runner must follow it: persisting the old hash recreates an orphan pipeline row, and the
// next pass rejects it as a clip that vanished.
func TestPipeline_FollowsAStageIdentityReplacement(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "old-hash")
	stages := allStages()
	replacement := st.clips["old-hash"]
	replacement.Hash = "new-hash"
	replacement.Path = "aa/bb/new-hash.mp4"
	// The re-keyed clip appears when the transcode rewrites storage, after the pass's own enrolment.
	stages[pipelinestate.StageTranscode].onRun = func() { st.clips[replacement.Hash] = replacement }
	stages[pipelinestate.StageTranscode].result = filler.StageResult{
		Clip: replacement, Verdict: pipelinestate.VerdictContinue,
	}

	if _, err := newPipe(st, asSlice(stages), filler.DefaultBudget()).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	row, ok := st.rows["new-hash"]
	if !ok {
		t.Fatal("pipeline persisted the transformed clip under its old content identity")
	}
	if row.ClipHash != "new-hash" || row.Disposition != pipelinestate.DispositionReady {
		t.Errorf("replacement pipeline row = %+v", row)
	}
}

// A terminal clip is not picked up again — otherwise every pass would re-run the whole ladder over
// a catalog that is already done.
func TestPipeline_TerminalClipsAreNotReWorked(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "c1")
	stages := allStages()
	p := newPipe(st, asSlice(stages), filler.DefaultBudget())

	if _, err := p.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := stages[pipelinestate.StageFinish].runs
	if _, err := p.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if stages[pipelinestate.StageFinish].runs != before {
		t.Errorf("a finished clip was worked again (%d → %d)", before, stages[pipelinestate.StageFinish].runs)
	}
}

// Enrolment is lazy: a catalogued clip with no row gets one, at the start.
func TestPipeline_EnrolsCataloguedClips(t *testing.T) {
	st := newPipeMemStore()
	st.put(clipstore.Clip{Clip: clips.Clip{Hash: "c1", Path: "a/b/c1.mp4"}})

	res, err := newPipe(st, asSlice(allStages()), filler.DefaultBudget()).RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Enrolled != 1 {
		t.Fatalf("enrolled %d, want 1", res.Enrolled)
	}
	if _, ok := st.rows["c1"]; !ok {
		t.Error("the clip was counted as enrolled but has no row")
	}
}

func TestPipeline_EnrolmentCarriesAcquisitionFromSidecar(t *testing.T) {
	dir := t.TempDir()
	rel := filepath.Join("ab", "clip.mp4")
	media := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(media), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(media, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := asset.WriteSidecarTags(media, asset.SidecarTags{AcquisitionID: "acq-17"}, false); err != nil {
		t.Fatal(err)
	}
	st := newPipeMemStore()
	st.put(clipstore.Clip{Clip: clips.Clip{Hash: "c1", Path: rel}})

	if _, err := newPipe(st, nil, filler.DefaultBudget()).WithRewind(st, dir).EnrolMissing(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := st.rows["c1"].AcquisitionID; got != "acq-17" {
		t.Fatalf("pipeline acquisition = %q, want acq-17", got)
	}
}

// Download completion needs the cheap half of RunOnce: make the clip visible on the durable
// conveyor without unexpectedly spending a transcode/Whisper budget on an unrelated backlog.
func TestPipeline_EnrolMissingDoesNotRunAStage(t *testing.T) {
	st := newPipeMemStore()
	st.put(clipstore.Clip{Clip: clips.Clip{Hash: "c1", Path: "a/b/c1.mp4"}})
	stages := allStages()

	n, err := newPipe(st, asSlice(stages), filler.DefaultBudget()).EnrolMissing(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("enrolled %d, want 1", n)
	}
	if stages[pipelinestate.StageProbe].runs != 0 {
		t.Fatalf("probe ran %d times; enrol-only nudge must not execute stages", stages[pipelinestate.StageProbe].runs)
	}
	if row := st.rows["c1"]; row.Stage != pipelinestate.StageProbe || row.Status != pipelinestate.StatusQueued {
		t.Fatalf("pipeline row = %+v, want probe/queued", row)
	}
}

// resetSchedule clears the backoff so a test can drive consecutive attempts against a frozen clock.
func resetSchedule(r pipelinestate.ClipPipeline) pipelinestate.ClipPipeline {
	r.NextRun = time.Time{}
	return r
}
