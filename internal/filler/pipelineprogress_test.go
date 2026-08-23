package filler

import (
	"context"
	"testing"
	"time"
)

// The §10 database progress throttle, driven through the REAL runner.
//
// ⚠ **In-package (`package filler`) on purpose, unlike `pipeline_test.go`.** A stage reports
// progress by calling `reportProgress`, which reads an UNEXPORTED context key — so a stage defined
// in `filler_test` cannot report anything at all, and a test written there could only poke the
// store directly. That is exactly the shape this file exists to replace: `fillerincoming_test.go`
// asserted the `-1` rendering by writing `Progress: -1` into a store row by hand, which stayed
// green for the whole time production could not produce `-1` (`onProgress` dropped it on a blanket
// `percent < 0` guard). A test that writes the state it is checking for cannot notice that nothing
// else writes it.
//
// The cost is a second store fake; `pipeMemStore` lives in the external test package and cannot be
// reached from here. It is kept to the minimum the runner touches.

type progMemStore struct {
	clips map[string]StoreClip
	rows  map[string]ClipPipeline
	// writes counts persisted rows, which is the property under test — the throttle governs how
	// often a row reaches the store, and nothing else observes it.
	writes int
}

func newProgMemStore(hash string) *progMemStore {
	return &progMemStore{
		clips: map[string]StoreClip{hash: {Clip: Clip{Hash: hash, Path: "a/" + hash + ".mp4", Name: hash}}},
		rows: map[string]ClipPipeline{hash: {
			ClipHash: hash, Stage: StageProbe, Status: StatusQueued, Disposition: DispositionRunning,
		}},
	}
}

func (m *progMemStore) ListPipelineWork(_ context.Context, _ time.Time, _ int) ([]ClipPipeline, error) {
	var out []ClipPipeline
	for _, r := range m.rows {
		if !r.Disposition.Terminal() {
			out = append(out, r)
		}
	}
	return out, nil
}

func (m *progMemStore) PipelineOverview(_ context.Context, at time.Time) (PipelineOverview, error) {
	rows := make([]ClipPipeline, 0, len(m.rows))
	for _, row := range m.rows {
		rows = append(rows, row)
	}
	return SummarizePipelines(rows, at), nil
}
func (m *progMemStore) ListClipsWithoutPipeline(context.Context, int) ([]StoreClip, error) {
	return nil, nil
}
func (m *progMemStore) GetClipPipeline(_ context.Context, h string) (ClipPipeline, bool, error) {
	r, ok := m.rows[h]
	return r, ok, nil
}
func (m *progMemStore) ListClipPipelines(context.Context, PipelineFilter) ([]ClipPipeline, error) {
	return nil, nil
}
func (m *progMemStore) UpsertClipPipeline(_ context.Context, p ClipPipeline) error {
	m.rows[p.ClipHash] = p
	m.writes++
	return nil
}
func (m *progMemStore) RetryClipPipeline(_ context.Context, _ ClipPipeline, p ClipPipeline, restore bool) error {
	m.rows[p.ClipHash] = p
	if restore {
		c := m.clips[p.ClipHash]
		c.RemovedAt, c.Held, c.AutoFiled = time.Time{}, true, false
		m.clips[p.ClipHash] = c
	}
	return nil
}
func (m *progMemStore) GetClip(_ context.Context, id string) (StoreClip, bool, error) {
	c, ok := m.clips[id]
	return c, ok, nil
}
func (m *progMemStore) SetClipsRemoved(context.Context, []string, time.Time) (int, error) {
	return 0, nil
}
func (m *progMemStore) SetClipsHeld(context.Context, []string, bool, bool, time.Time) (int, error) {
	return 0, nil
}

// reportingStage is a rung that emits a scripted progress script, the way transcode emits ffmpeg's
// and the way `tag`/`vision` emit NoMeasurement.
type reportingStage struct {
	id      StageID
	samples []int
	// tick advances the shared clock before each sample, modelling real elapsed time between
	// ffmpeg's ~1Hz reports.
	tick  func()
	after func()
}

func (s *reportingStage) ID() StageID     { return s.id }
func (s *reportingStage) Cost() StageCost { return CostCheap }
func (s *reportingStage) Applies(context.Context, StoreClip) (bool, string) {
	return true, ""
}
func (s *reportingStage) Run(ctx context.Context, _ StoreClip) (StageResult, error) {
	for _, pct := range s.samples {
		if s.tick != nil {
			s.tick()
		}
		reportProgress(ctx, s.id, pct)
	}
	if s.after != nil {
		s.after()
	}
	return StageResult{Verdict: VerdictContinue}, nil
}

// passStage is a rung that does nothing, so the ladder can complete around the one under test.
type passStage struct{ id StageID }

func (s passStage) ID() StageID     { return s.id }
func (s passStage) Cost() StageCost { return CostCheap }
func (s passStage) Applies(context.Context, StoreClip) (bool, string) {
	return true, ""
}
func (s passStage) Run(context.Context, StoreClip) (StageResult, error) {
	return StageResult{Verdict: VerdictContinue}, nil
}

// ladderWith returns a full ladder with `st` substituted for its own rung.
func ladderWith(st Stage) []Stage {
	var out []Stage
	for _, id := range StageOrder {
		if id == st.ID() {
			out = append(out, st)
			continue
		}
		out = append(out, passStage{id: id})
	}
	return out
}

// movableClock is a hand-advanced clock — the throttle's time half cannot be tested against the
// frozen clock the other pipeline tests use.
type movableClock struct{ at time.Time }

func (c *movableClock) now() time.Time      { return c.at }
func (c *movableClock) add(d time.Duration) { c.at = c.at.Add(d) }

func newProgClock() *movableClock {
	return &movableClock{at: time.Unix(1_800_000_000, 0).UTC()}
}

// ── the sentinel ────────────────────────────────────────────────────────────────────────────────

// A stage that cannot measure itself must leave `-1` in the STORED row, not the 0 the runner
// initialised it with.
//
// ⚠ This is the whole A6 defect. `tag` and `vision` both call `reportProgress(ctx, id,
// NoMeasurement)`, and `onProgress` discarded it — so the row kept the 0 written when the stage
// went RUNNING, and the UI's `progress >= 0` bar test passed on a value that meant "we were never
// told". Every run of either stage rendered a bar frozen at zero, which §10 names as the
// fabricated-progress failure it forbids.
func TestProgress_NoMeasurementReachesTheStoredRow(t *testing.T) {
	clock := newProgClock()
	st := newProgMemStore("c1")
	// Captured mid-stage: the ladder completes, so the final row has moved on. What matters is the
	// value the row carried WHILE the unmeasurable stage was running.
	var during int
	stage := &reportingStage{id: StageProbe, samples: []int{NoMeasurement}}
	stage.after = func() { during = st.rows["c1"].Progress }

	p := NewPipeline(st, st, ladderWith(stage), DefaultBudget(), nil, clock.now, nil)
	if _, err := p.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	if during != NoMeasurement {
		t.Fatalf("stored progress during an unmeasurable stage = %d, want %d (NoMeasurement)\n"+
			"0 renders as a bar frozen at zero — a claim about work the stage never made",
			during, NoMeasurement)
	}
}

// ── the throttle ────────────────────────────────────────────────────────────────────────────────

// A stage that crawls one point at a time must still reach the database, on the TIME half.
//
// ⚠ Under the old throttle this wrote NOTHING between 0 and 100: the baseline was `row.Progress`,
// which the skip branch also advanced, so `percent` was perpetually `baseline + 1` and the
// `+10` test could never be satisfied. ffmpeg reports about once a second, so this is not a corner
// case — it is what every long transcode did.
func TestProgress_ACrawlingStagePersistsOnTheTimeHalf(t *testing.T) {
	clock := newProgClock()
	st := newProgMemStore("c1")

	// 12 samples of +1, one second apart: never 10 points of movement between writes, so ONLY the
	// time half can save this.
	samples := make([]int, 0, 12)
	for i := 1; i <= 12; i++ {
		samples = append(samples, i)
	}
	var writesDuring int
	stage := &reportingStage{
		id:      StageProbe,
		samples: samples,
		tick:    func() { clock.add(time.Second) },
	}
	stage.after = func() { writesDuring = st.writes }

	p := NewPipeline(st, st, ladderWith(stage), DefaultBudget(), nil, clock.now, nil)
	if _, err := p.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	// One write when the stage went RUNNING, then one roughly every 2s across 12s.
	if writesDuring < 4 {
		t.Fatalf("writes during a 12s crawl = %d, want at least 4 — a reload mid-transcode must not "+
			"snap back to 0%%; the ≥2s half of the §10 throttle is what makes that true", writesDuring)
	}
}

// The points half, with the clock frozen so ONLY movement can trigger a write — and the baseline
// must be the last PERSISTED value, not the last reported one.
func TestProgress_SkippedWritesDoNotMoveTheBaseline(t *testing.T) {
	clock := newProgClock() // never advanced: the time half can never fire
	st := newProgMemStore("c1")

	// 1..9 must not write (under 10 points from the persisted 0); 10 must.
	samples := []int{1, 2, 3, 4, 5, 6, 7, 8, 9}
	var beforeTen, afterTen int
	stage := &reportingStage{id: StageProbe, samples: samples}
	stage.after = func() { beforeTen = st.writes }

	p := NewPipeline(st, st, ladderWith(stage), DefaultBudget(), nil, clock.now, nil)
	if _, err := p.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Re-run with the tenth sample appended, so the two counts differ by exactly the write it earns.
	st2 := newProgMemStore("c2")
	stage2 := &reportingStage{id: StageProbe, samples: append(append([]int{}, samples...), 10)}
	stage2.after = func() { afterTen = st2.writes }
	p2 := NewPipeline(st2, st2, ladderWith(stage2), DefaultBudget(), nil, newProgClock().now, nil)
	if _, err := p2.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	if afterTen != beforeTen+1 {
		t.Fatalf("writes with the 10th point = %d, without = %d, want exactly one more.\n"+
			"If the skipped writes moved the baseline, 10 is only +1 from the last REPORTED 9 and "+
			"never earns its write — which is the bug this asserts against", afterTen, beforeTen)
	}
}

// The SSE side is not throttled by the database side: every sample publishes, because the live UI
// is the thing the percentage is FOR.
func TestProgress_EverySamplePublishesEvenWhenItDoesNotWrite(t *testing.T) {
	clock := newProgClock()
	st := newProgMemStore("c1")

	var published int
	notify := func(ClipPipeline, StoreClip) { published++ }

	samples := []int{1, 2, 3, 4, 5}
	stage := &reportingStage{id: StageProbe, samples: samples}
	var publishedDuring, writesDuring int
	stage.after = func() { publishedDuring, writesDuring = published, st.writes }

	p := NewPipeline(st, st, ladderWith(stage), DefaultBudget(), notify, clock.now, nil)
	if _, err := p.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	// One publish for the RUNNING transition, then one per sample.
	if publishedDuring < len(samples) {
		t.Errorf("publishes = %d, want at least %d — one per sample; the throttle governs the "+
			"DATABASE, not the bus", publishedDuring, len(samples))
	}
	if writesDuring > 1 {
		t.Errorf("writes = %d, want 1 (the RUNNING transition only) — five samples under 10 points "+
			"with a frozen clock must not reach the store", writesDuring)
	}
}
