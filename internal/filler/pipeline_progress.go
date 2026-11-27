package filler

import (
	"context"
	"time"

	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// progressThrottle is the database-write throttle for ONE stage run (§10).
//
// ⚠ **`lastWritten` is the last percent actually PERSISTED, and keeping it distinct from
// `row.Progress` is the entire point of this type.** Comparing against `row.Progress` would not
// work: the skipped-write branch also advances it, so the baseline would move with every sample. ffmpeg reports about once a second, making `percent` perpetually `lastWritten + 1`, so
// `percent >= lastWritten + 10` was never satisfied and a long transcode persisted NOTHING between
// 0 and 100. The bug is invisible from the UI, which is fed by the SSE publish that always fires;
// it only shows up as a reload mid-transcode snapping back to 0%.
//
// Scoped to a stage run rather than to the Pipeline because "since the last write" means nothing
// across stages — and because the Pipeline is shared by concurrent clips, where a single shared
// baseline would let one clip's writes throttle another's.
type progressThrottle struct {
	lastWritten int
	lastWriteAt time.Time
}

// due reports whether this sample has earned a database write.
//
// ⚠ **OR, not AND** (§10). A stage that crawls needs the time half to ever reach disk; a stage that
// jumps needs the points half. Requiring both is what "≥2s / ≥10 points" was mis-read as, and it
// means a slow stage writes nothing for minutes.
func (t *progressThrottle) due(percent int, now time.Time) bool {
	return percent >= t.lastWritten+progressWriteStep || !now.Before(t.lastWriteAt.Add(progressWriteInterval))
}

// wrote moves the baseline. ⚠ Called ONLY after a write actually happened — a skipped write that
// moved this would recreate the moving-baseline bug the type exists to prevent.
func (t *progressThrottle) wrote(percent int, now time.Time) {
	t.lastWritten, t.lastWriteAt = percent, now
}

// The two halves of the §10 database throttle.
const (
	progressWriteStep     = 10
	progressWriteInterval = 2 * time.Second
)

// stageProgress builds the ProgressFunc for one stage run, owning that run's throttle state.
func (p *Pipeline) stageProgress(ctx context.Context, row *pipelinestate.ClipPipeline, clip clipstore.Clip) pipelinestate.ProgressFunc {
	// Seeded from the write the runner just made when it set the stage RUNNING at 0.
	t := &progressThrottle{lastWritten: 0, lastWriteAt: p.now()}
	return func(id pipelinestate.StageID, percent int) {
		p.onProgress(ctx, row, clip, t, id, percent)
	}
}

// onProgress persists + publishes intra-stage progress, throttled.
//
// ⚠ Throttled in BOTH directions, and the database side matters more than the SSE side. What has
// to survive a reload is which stage a clip is at and whether it is running; the percentage is
// decoration. Putting a synchronous SQLite write on ffmpeg's progress path — which emits about
// once a second, per clip — to persist decoration is the wrong trade.
func (p *Pipeline) onProgress(ctx context.Context, row *pipelinestate.ClipPipeline, clip clipstore.Clip, t *progressThrottle, id pipelinestate.StageID, percent int) {
	if id != row.Stage {
		return
	}

	// ⚠ **`NoMeasurement` is a STATE, not a small percentage, so it bypasses the throttle
	// entirely.** Dropping it with a blanket `percent < 0` guard would leave the row at the 0 the
	// runner initialised it with — and a persisted 0 renders as a bar frozen at zero, the
	// fabricated-progress claim §10 explicitly forbids. `tag` and `vision` both report it. It fires
	// once per stage, so always writing it costs one row update, not a write per sample.
	if percent == pipelinestate.NoMeasurement {
		row.Progress = pipelinestate.NoMeasurement
		p.writeProgress(ctx, row, clip, t, percent)
		return
	}
	if percent < 0 || percent > 100 {
		return // not a percentage and not the sentinel — nothing meaningful to record
	}

	row.Progress = percent
	// 100 always writes: it is the last sample the stage will send, and losing it leaves the row
	// showing a partial percentage for a stage that finished.
	if percent < 100 && !t.due(percent, p.now()) {
		// Not enough movement to be worth a write; still publish, which is cheap and dropped
		// harmlessly under load. ⚠ Deliberately does NOT touch the throttle baseline.
		p.publish(*row, clip)
		return
	}
	p.writeProgress(ctx, row, clip, t, percent)
}

// writeProgress persists the row and moves the throttle baseline, keeping the two in step.
func (p *Pipeline) writeProgress(ctx context.Context, row *pipelinestate.ClipPipeline, clip clipstore.Clip, t *progressThrottle, percent int) {
	if err := p.save(ctx, *row, clip); err != nil {
		// ⚠ The baseline stays put on a failed write, so the next sample retries rather than
		// waiting another 10 points for a write that never landed.
		if p.log != nil {
			p.log.Debug("filler pipeline: progress write failed", "clip", row.ClipHash, "err", err)
		}
		return
	}
	t.wrote(percent, p.now())
}
