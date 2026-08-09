package filler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// The ingest pipeline (§10 V51b): one ordered, per-clip run replacing seven independent cron
// sweeps.
//
// ⚠ **Sequential, one clip at a time, and that is a design choice rather than a simplification.**
// Whisper is ~341s per clip under QEMU (measured, recorded in transcribejob.go) and ffmpeg
// competes with playout for the GPU (§8.1's eviction note, §9.1's cost-aware admission), so a
// worker pool would turn a catalog import into a live-channel outage. It is also what makes the
// SSE story work: at most one clip is running, so "forty clips × eight stages" never arrives at
// the event bus at once, and the 32-deep subscriber buffer is never the thing under pressure.
//
// The scheduler's `ClaimDueScheduledJobs` lease already guarantees one runner per job name across
// replicas, so there is nothing to coordinate beyond it.

// StageCost is what one execution of a stage spends from the per-run budget. A stage declares its
// cost so the budget can be about REAL scarcity (GPU seconds, paid API calls) rather than about
// counting stages.
type StageCost int

const (
	// CostCheap — arithmetic or a metadata read. Bounded only by MaxClips.
	CostCheap StageCost = iota
	CostTranscode
	CostWhisper
	CostVision
	CostSplit
)

// StageResult is what a stage reports back.
type StageResult struct {
	// Clip is the (possibly updated) clip — the transcode stage changes Path/Quality/DurationMs.
	Clip StoreClip
	// Verdict decides whether the clip continues, stops for a human, or is refused.
	Verdict Verdict
	// Reason + Detail are set when Verdict is VerdictReject: a stable code and the measured fact.
	Reason RejectReason
	Detail string
	// Spawned carries hashes this stage CREATED (split segments), enrolled at StageProbe so they
	// run the whole ladder themselves.
	Spawned []string
	// Note is the operator-facing sentence persisted on the ladder for a skip or an oddity.
	Note string
}

// Stage is one rung.
type Stage interface {
	ID() StageID
	// Applies answers "is there work here, for THIS clip, in THIS install?" — WITHOUT executing
	// anything. Returning (false, note) records `skipped` with the note.
	//
	// ⚠ Re-evaluated on every pass, which is what makes a setting change retroactive: flipping
	// `filler.vision.enabled` on picks up clips that already went past that rung, with no
	// migration and no re-sweep.
	Applies(ctx context.Context, c StoreClip) (bool, string)
	// Run does the work for one clip. Progress goes through the context's ProgressFunc.
	Run(ctx context.Context, c StoreClip) (StageResult, error)
	Cost() StageCost
}

// Budget bounds one pass. Every field is a closure so a settings change hot-applies mid-run, the
// same contract `AutoFilePolicy` uses.
//
// ⚠ The defaults carry forward the existing per-job batch constants (LanguageBatch 25,
// TranscribeBatch 10, VisionBatch 5, defaultSplitsPerRun 3) rather than inventing new numbers, so
// the "a backlog drains over cycles instead of in one thundering pass" property those constants
// were chosen to defend is preserved exactly.
type Budget struct {
	MaxClips      func() int
	MaxTranscodes func() int
	MaxWhisper    func() int
	MaxVision     func() int
	MaxSplits     func() int
}

// DefaultBudget is the budget with the historical batch sizes.
func DefaultBudget() Budget {
	return Budget{
		MaxClips:      func() int { return 25 },
		MaxTranscodes: func() int { return 3 },
		MaxWhisper:    func() int { return 10 },
		MaxVision:     func() int { return 5 },
		MaxSplits:     func() int { return 3 },
	}
}

// spend tracks what a single pass has used.
type spend struct {
	clips, transcodes, whisper, vision, splits int
}

// exhausted reports whether the budget for a given cost is spent.
//
// ⚠ **A NIL closure means "use the default"; a closure RETURNING ZERO means "none".** They are
// different states and collapsing them would remove the only way to say "do not transcode anything
// on this box" — the operator control `filler.transcode.backfill` is built on. This is the same
// three-state encoding `FillerSource.FetchEverySeconds` uses (nil = inherit, 0 = never, N = every
// N), and it is written down there for the same reason: a config that cannot express "off"
// silently ignores an operator who asked for it.
func (b Budget) exhausted(s spend, c StageCost) bool {
	limit := func(f func() int, fallback int) int {
		if f == nil {
			return fallback
		}
		return f()
	}
	switch c {
	case CostTranscode:
		return s.transcodes >= limit(b.MaxTranscodes, 3)
	case CostWhisper:
		return s.whisper >= limit(b.MaxWhisper, 10)
	case CostVision:
		return s.vision >= limit(b.MaxVision, 5)
	case CostSplit:
		return s.splits >= limit(b.MaxSplits, 3)
	default:
		return false
	}
}

func (s *spend) charge(c StageCost) {
	switch c {
	case CostTranscode:
		s.transcodes++
	case CostWhisper:
		s.whisper++
	case CostVision:
		s.vision++
	case CostSplit:
		s.splits++
	}
}

// backoff is the delay before a failed stage is retried. Three attempts, then the stage resolves
// (fatal stages reject; the rest skip and let the clip advance).
//
// ⚠ Genuinely new behaviour. The cron jobs had NO retry state — `Work` always returned nil, so a
// failure just waited for the next tick and a permanently-broken clip was retried at full cost
// every hour, forever. That is minutes of Whisper per hour spent on a file that will never decode.
func backoff(attempts int) time.Duration {
	switch {
	case attempts <= 1:
		return 5 * time.Minute
	case attempts == 2:
		return 30 * time.Minute
	default:
		return 2 * time.Hour
	}
}

// MaxAttempts is how many times a stage is retried before it resolves.
const MaxAttempts = 3

// fatalStages are the ones whose exhausted failure REJECTS the clip rather than skipping it.
//
// ⚠ Only probe and transcode. A file we cannot measure is not a clip, and one we cannot re-encode
// cannot be played — those are facts about the file. Everything else is a fact about a BACKEND
// (whisper missing, the LLM down, a vision call refused), which says nothing about the clip, so
// those stages skip and the clip advances. A missing transcript must never strand a commercial.
var fatalStages = map[StageID]RejectReason{
	StageProbe:     ReasonUnprobeable,
	StageTranscode: ReasonUnplayable,
}

// ClipStore is the slice of the store the pipeline needs beyond PipelineStore.
type ClipStore interface {
	GetClip(ctx context.Context, id string) (StoreClip, bool, error)
	// SetClipsRemoved tombstones a refused clip. ⚠ A TOMBSTONE, not a delete: `clips` is a synced
	// cache, so a hard delete would be undone by the next scan finding the file still on disk and
	// the clip would air again.
	SetClipsRemoved(ctx context.Context, paths []string, at time.Time) (int, error)
}

// Notifier receives every state change so it can be published (SSE) — best-effort, never
// load-bearing. The persisted row is the truth; this is the latency optimisation (§8).
type Notifier func(p ClipPipeline, c StoreClip)

// Pipeline runs clips through the stages.
type Pipeline struct {
	store PipelineStore
	clips ClipStore
	// rewind + clipDir back `Rewind` (pipelinerewind.go). Optional: an install that never re-runs
	// a stage does not need them, and `Rewind` refuses rather than half-working without them.
	rewind  RewindStore
	clipDir string
	stages  map[StageID]Stage
	budget  Budget
	notify  Notifier
	now     func() time.Time
	log     *slog.Logger
}

// NewPipeline builds a runner over the given stages. Stages absent from the list are treated as
// permanently skipped, which is how an install without an LLM or without ffmpeg degrades: the
// ladder still renders every rung and says why each one did not run.
func NewPipeline(store PipelineStore, clips ClipStore, stages []Stage, budget Budget, notify Notifier, now func() time.Time, log *slog.Logger) *Pipeline {
	if now == nil {
		now = time.Now
	}
	byID := make(map[StageID]Stage, len(stages))
	for _, s := range stages {
		byID[s.ID()] = s
	}
	return &Pipeline{store: store, clips: clips, stages: byID, budget: budget, notify: notify, now: now, log: log}
}

// PipelineResult summarises one pass.
type PipelineResult struct {
	Enrolled  int
	Advanced  int
	Completed int
	Rejected  int
	Failed    int
	// Deferred counts clips whose pass ended before the rung did — the budget ran out, not the
	// clip. ⚠ Counted SEPARATELY from Failed on purpose: these were logged as failures, so a reel
	// too slow for one pass produced a WARN every two minutes forever and the run summary blamed
	// the clip. A number that says "we ran out of time" is the one an operator can act on (raise
	// the schedule, raise the budget); "failed" sends them looking at the file.
	Deferred int
}

// ErrDeferred marks a pass that ended before the stage did. It is NOT a failure: no attempt is
// spent, no backoff is taken, and the clip resumes exactly where it stopped on the next pass.
var ErrDeferred = errors.New("filler pipeline: the pass ended before this stage finished")

// RunOnce is the cron driver: enrol anything new, then advance as much due work as the budget
// allows.
func (p *Pipeline) RunOnce(ctx context.Context) (PipelineResult, error) {
	var res PipelineResult
	if p.store == nil {
		return res, nil
	}

	n, err := p.enrolMissing(ctx)
	res.Enrolled = n
	if err != nil {
		// ⚠ Not fatal. Enrolment is a convenience that self-heals next pass; failing the whole run
		// because it stumbled would also stop the clips already enrolled from advancing.
		if p.log != nil {
			p.log.Warn("filler pipeline: enrolment failed", "err", err)
		}
	}

	maxClips := 25
	if p.budget.MaxClips != nil {
		if v := p.budget.MaxClips(); v > 0 {
			maxClips = v
		}
	}
	work, err := p.store.ListPipelineWork(ctx, p.now(), maxClips)
	if err != nil {
		return res, err
	}

	var s spend
	for _, row := range work {
		// ⚠ The pass ending is not the JOB failing. This returned the bare context error, so the
		// scheduler logged "scheduled job failed" every two minutes for a run that had worked
		// through as many clips as it could — which is the pipeline doing exactly its job. The
		// summary below still logs, carrying `deferred`, so the run is visible as partial rather
		// than as broken.
		if ctx.Err() != nil {
			res.Deferred += len(work) - res.Advanced - res.Failed - res.Deferred
			break
		}
		if s.clips >= maxClips {
			break
		}
		s.clips++
		outcome, err := p.advance(ctx, row, &s)
		if err != nil {
			// ⚠ A deferral is not a failure and must not be logged as one. Before this, a reel too
			// slow for one pass produced an identical WARN every two minutes indefinitely — twelve
			// of them for the clip that exposed this — while the row showed no sign of trouble.
			if errors.Is(err, ErrDeferred) {
				res.Deferred++
				continue
			}
			res.Failed++
			if p.log != nil {
				p.log.Warn("filler pipeline: clip failed", "clip", row.ClipHash, "err", err)
			}
			continue
		}
		res.Advanced++
		switch outcome {
		case DispositionRejected:
			res.Rejected++
		case DispositionFiled, DispositionReview:
			res.Completed++
		}
	}
	if p.log != nil {
		p.log.Info("filler pipeline run", "enrolled", res.Enrolled, "advanced", res.Advanced,
			"completed", res.Completed, "rejected", res.Rejected, "failed", res.Failed)
	}
	return res, nil
}

// enrolMissing gives every catalogued clip a pipeline row.
//
// ⚠ **Enrolled at `probe/queued`, with NO derived backfill**, and that is the deliberate
// simplification. The obvious alternative — infer each stage's state from what the clip already
// carries (`Language != ""` ⇒ language done, `VisionTagged` ⇒ vision done, …) — is guesswork about
// work done by code that no longer exists, cannot be tested against a fresh install, and has one
// silent failure mode per row.
//
// It costs nothing to skip, because each stage's `Applies` reads the clip's own state: a clip that
// already has a transcript skips `transcribe` on its own say-so. So re-enrolment is cheap where
// the work was done and correct where it was not, without a second copy of that judgement living
// in a migration.
func (p *Pipeline) enrolMissing(ctx context.Context) (int, error) {
	const enrolBatch = 500
	clips, err := p.store.ListClipsWithoutPipeline(ctx, enrolBatch)
	if err != nil {
		return 0, err
	}
	now := p.now().UTC()
	n := 0
	for _, c := range clips {
		if c.Hash == "" {
			continue // not addressable; the scan will re-file it
		}
		row := ClipPipeline{
			ClipHash: c.Hash, Stage: StageProbe, Status: StatusQueued,
			Disposition: DispositionRunning, EnrolledAt: now, UpdatedAt: now,
		}
		if err := p.store.UpsertClipPipeline(ctx, row); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// Advance moves ONE clip as far as it can go this pass. Exported for the on-demand paths (an
// operator pressing a button should not wait for the next cron tick).
func (p *Pipeline) Advance(ctx context.Context, hash string) error {
	row, found, err := p.store.GetClipPipeline(ctx, hash)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("clip %s is not enrolled in the pipeline", hash)
	}
	var s spend
	_, err = p.advance(ctx, row, &s)
	return err
}

// advance walks the stages for one clip until it blocks, defers, or reaches a terminal state.
func (p *Pipeline) advance(ctx context.Context, row ClipPipeline, s *spend) (Disposition, error) {
	clip, found, err := p.clips.GetClip(ctx, row.ClipHash)
	if err != nil {
		return row.Disposition, err
	}
	if !found {
		// The clip is gone (pruned, or its file removed). Retire the row rather than retrying a
		// lookup that will never succeed.
		row.Disposition = DispositionRejected
		row.RejectReason = ReasonUnprobeable
		row.RejectDetail = "the clip is no longer in the catalog"
		return row.Disposition, p.persist(ctx, row, clip)
	}

	for {
		// ⚠ Persist before handing the deadline back. Rungs resolved EARLIER in this same pass —
		// a skip recorded, a `step` onto the next stage — live only in the local `row` until
		// something saves it, so returning here bare threw that work away and the next pass
		// re-derived it. On a clip with several skips in a row that is the difference between
		// converging and looping.
		if ctx.Err() != nil {
			if err := p.persist(ctx, row, clip); err != nil {
				return row.Disposition, err
			}
			return row.Disposition, ErrDeferred
		}
		idx := StageIndex(row.Stage)
		if idx < 0 {
			// An unknown stage (a rename, a downgrade). Restart from the beginning rather than
			// guessing a position — the alternative is silently skipping rungs.
			row.Stage, row.Status, row.Attempts = StageProbe, StatusQueued, 0
			continue
		}

		stage, known := p.stages[row.Stage]
		if !known {
			// Not wired in this build/install: skip and move on, recorded so the ladder says so.
			row.Record(row.Stage, StatusSkipped, "not available on this install", row.Attempts, p.now().UTC())
			if done := p.step(&row); done {
				break
			}
			continue
		}

		if p.budget.exhausted(*s, stage.Cost()) {
			// Out of budget for THIS kind of work. Leave the clip queued — it resumes next pass
			// exactly here, which is what makes a large import drain over cycles.
			row.Status = StatusQueued
			return row.Disposition, p.persist(ctx, row, clip)
		}

		// ⚠ **A composite skips everything except measuring and cutting, and the rule lives HERE
		// rather than in six `Applies` methods.** A compilation is a CONTAINER of adverts, not an
		// advert: transcoding it burns an encode on a file about to be cut up, and tagging or
		// scoring it would describe "twenty unrelated adverts" as though it were one clip — the
		// score rung would then find nothing coherent and could tombstone it as unidentified,
		// destroying the reel its segments are cut from.
		//
		// Six copies of that check is six chances for a new rung to forget it, and forgetting it
		// is silent. One rule, stated once, and a new rung inherits it by existing.
		if clip.IsComposite && row.Stage != StageProbe && row.Stage != StageSplit {
			row.Record(row.Stage, StatusSkipped, "a compilation is cut up rather than filed", row.Attempts, p.now().UTC())
			if done := p.step(&row); done {
				break
			}
			continue
		}

		if applies, note := stage.Applies(ctx, clip); !applies {
			row.Record(row.Stage, StatusSkipped, note, row.Attempts, p.now().UTC())
			if done := p.step(&row); done {
				break
			}
			continue
		}

		row.Status, row.Attempts = StatusRunning, row.Attempts+1
		row.Progress = 0
		if err := p.save(ctx, row, clip); err != nil {
			return row.Disposition, err
		}

		stageCtx := WithProgress(ctx, func(id StageID, percent int) {
			p.onProgress(ctx, &row, clip, id, percent)
		})
		out, runErr := stage.Run(stageCtx, clip)
		s.charge(stage.Cost())

		if runErr != nil {
			// ⚠ **Running out of TIME is not failing, and conflating the two is what made a slow
			// rung permanent.** A cancelled context means the pass's budget ended mid-work — the
			// clip did nothing wrong, so it must not spend an attempt or take a backoff it has not
			// earned. It goes back to `queued` and resumes next pass, which is exactly how the
			// budget-exhausted branch above already behaves; the deadline path simply never got
			// the same treatment.
			//
			// ⚠ The attempt is ROLLED BACK, because it was counted before the work began. That
			// pre-increment is deliberate and protective — a process killed mid-run still burns an
			// attempt, so a crash loop stays bounded — but a deadline is our own doing, and
			// charging the clip for it is what let `attempts` climb without limit.
			if ctx.Err() != nil {
				row.Status = StatusQueued
				if row.Attempts > 0 {
					row.Attempts--
				}
				if err := p.persist(ctx, row, clip); err != nil {
					return row.Disposition, err
				}
				return row.Disposition, ErrDeferred
			}
			if resolved := p.onFailure(&row, runErr); !resolved {
				return row.Disposition, p.persist(ctx, row, clip)
			}
			if done := p.step(&row); done {
				break
			}
			continue
		}

		if out.Clip.Hash != "" {
			clip = out.Clip
		}
		for _, spawned := range out.Spawned {
			// ⚠ A failed enrolment is logged, not fatal. The segment EXISTS in the catalog by this
			// point — the cut succeeded — so failing the parent here would leave the reel looking
			// unsplit while its adverts sit in the catalog. `enrolMissing` picks the segment up on
			// the next pass, which is exactly what that self-healing sweep is for.
			if err := p.Enrol(ctx, spawned); err != nil && p.log != nil {
				p.log.Warn("filler pipeline: could not enrol a spawned clip", "clip", spawned, "err", err)
			}
		}

		switch out.Verdict {
		case VerdictReject:
			row.Record(row.Stage, StatusDone, out.Note, row.Attempts, p.now().UTC())
			row.Disposition = DispositionRejected
			row.RejectReason, row.RejectDetail = out.Reason, out.Detail
			return row.Disposition, p.persist(ctx, row, clip)
		case VerdictReview:
			row.Record(row.Stage, StatusDone, out.Note, row.Attempts, p.now().UTC())
			row.Disposition = DispositionReview
			return row.Disposition, p.persist(ctx, row, clip)
		}

		row.Record(row.Stage, StatusDone, out.Note, row.Attempts, p.now().UTC())
		if done := p.step(&row); done {
			break
		}
	}

	if row.Disposition == DispositionRunning {
		row.Disposition = DispositionFiled
	}
	// ⚠ The terminal write — `filed`, or the last rung done. Detached like the rest: a clip that
	// finished its whole ladder inside a pass that then expired would otherwise be re-run from
	// wherever it was last durably recorded, spending the expensive rungs again.
	return row.Disposition, p.persist(ctx, row, clip)
}

// step advances to the next stage, returning true when the ladder is finished.
func (p *Pipeline) step(row *ClipPipeline) bool {
	idx := StageIndex(row.Stage)
	if idx < 0 || idx+1 >= len(StageOrder) {
		row.Status = StatusDone
		return true
	}
	row.Stage = StageOrder[idx+1]
	row.Status = StatusQueued
	row.Attempts = 0
	row.Progress = 0
	return false
}

// persist writes the row through a context DETACHED from the caller's.
//
// ⚠ **This exists because the bookkeeping for a timeout was being written through the context whose
// expiry caused it.** `onFailure` computes the failure record, the backoff and the `MaxAttempts`
// resolution — and every one of them was discarded, because `p.save(ctx, …)` fails on a cancelled
// context. The only write that survived was the one made BEFORE the work started
// (`status=running`, `attempts++`), which is why a clip could reach 12 attempts against a
// `MaxAttempts` of 3 and never leave `running`.
//
// ⚠ Measured live (§10 V51g): one 16m47s reel looped every two minutes for twelve passes, the row
// animating as though it were progressing. **Any rung that ever times out loops forever** — split
// was simply the first to be slow enough to prove it.
//
// The timeout is short and fixed: this is one row write, and if the store cannot take it in five
// seconds the next pass re-derives the same state anyway.
func (p *Pipeline) persist(ctx context.Context, row ClipPipeline, clip StoreClip) error {
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return p.save(saveCtx, row, clip)
}

// onFailure records a stage failure and decides whether the clip can move on. Returns true when
// the stage has RESOLVED (exhausted its retries and been skipped) so the caller advances.
func (p *Pipeline) onFailure(row *ClipPipeline, err error) bool {
	now := p.now().UTC()
	if row.Attempts < MaxAttempts {
		row.Status = StatusFailed
		row.NextRun = now.Add(backoff(row.Attempts))
		row.Record(row.Stage, StatusFailed, err.Error(), row.Attempts, now)
		return false
	}
	if reason, fatal := fatalStages[row.Stage]; fatal {
		row.Record(row.Stage, StatusFailed, err.Error(), row.Attempts, now)
		row.Disposition = DispositionRejected
		row.RejectReason = reason
		row.RejectDetail = err.Error()
		return false
	}
	// Non-fatal: the backend failed, which says nothing about the clip. Skip and carry on.
	row.Record(row.Stage, StatusSkipped, "gave up after "+fmt.Sprint(row.Attempts)+" attempts: "+err.Error(), row.Attempts, now)
	return true
}

// onProgress persists + publishes intra-stage progress, throttled.
//
// ⚠ Throttled in BOTH directions, and the database side matters more than the SSE side. What has
// to survive a reload is which stage a clip is at and whether it is running; the percentage is
// decoration. Putting a synchronous SQLite write on ffmpeg's progress path — which emits about
// once a second, per clip — to persist decoration is the wrong trade.
func (p *Pipeline) onProgress(ctx context.Context, row *ClipPipeline, clip StoreClip, id StageID, percent int) {
	if id != row.Stage || percent < 0 {
		return
	}
	if percent < row.Progress+10 && percent < 100 {
		// Not enough movement to be worth a write; still publish, which is cheap and dropped
		// harmlessly under load.
		row.Progress = percent
		p.publish(*row, clip)
		return
	}
	row.Progress = percent
	if err := p.save(ctx, *row, clip); err != nil && p.log != nil {
		p.log.Debug("filler pipeline: progress write failed", "clip", row.ClipHash, "err", err)
	}
}

// Enrol puts a clip at the start of the pipeline.
//
// Exported because the MANUAL split-confirm path needs it too: a segment an operator cut by hand
// must run the same ladder as one the split rung cut unattended, or the two paths produce
// different clips from the same reel.
func (p *Pipeline) Enrol(ctx context.Context, hash string) error {
	if hash == "" {
		return nil
	}
	now := p.now().UTC()
	return p.store.UpsertClipPipeline(ctx, ClipPipeline{
		ClipHash: hash, Stage: StageProbe, Status: StatusQueued,
		Disposition: DispositionRunning, EnrolledAt: now, UpdatedAt: now,
	})
}

// save writes the row, tombstones a refused clip, and publishes the result.
func (p *Pipeline) save(ctx context.Context, row ClipPipeline, clip StoreClip) error {
	row.UpdatedAt = p.now().UTC()
	if row.Disposition.Terminal() {
		// A terminal row is not due again; zero the schedule so the work list cannot re-pick it
		// on a clock skew.
		row.NextRun = time.Time{}
	}
	if err := p.store.UpsertClipPipeline(ctx, row); err != nil {
		return err
	}
	if row.Disposition == DispositionRejected {
		p.tombstone(ctx, clip)
	}
	p.publish(row, clip)
	return nil
}

// tombstone takes a refused clip out of rotation.
//
// ⚠ **Two places, one truth, and the split is deliberate.** `clips.removed_at` is WHETHER a clip
// may air — pod assembly's `WHERE removed_at = 0` is the only reader that matters and it is
// unchanged by this phase — while the pipeline row is WHY. Folding the reason into `clips` would
// put it back in the synced cache that gets recreated, and folding airability into the pipeline
// row would mean every pod query joined a table it has no business knowing about.
//
// ⚠ Written HERE rather than in each rejecting stage, so a new rule cannot ship a reject that
// records a reason and leaves the clip playable. That failure would be invisible: the operator
// sees "rejected" in Incoming while the clip keeps airing.
//
// Best-effort by design: the pipeline row is already committed, so a failure here leaves a clip
// marked rejected and still airable until the next pass re-saves it — noisy but recoverable, and
// strictly better than failing the run and losing the reason too.
func (p *Pipeline) tombstone(ctx context.Context, clip StoreClip) {
	if clip.Path == "" {
		return
	}
	if _, err := p.clips.SetClipsRemoved(ctx, []string{clip.Path}, p.now().UTC()); err != nil && p.log != nil {
		p.log.Warn("filler pipeline: could not tombstone a refused clip", "clip", clip.Path, "err", err)
	}
}

func (p *Pipeline) publish(row ClipPipeline, clip StoreClip) {
	if p.notify != nil {
		p.notify(row, clip)
	}
}

// ErrUnknownStage is returned by Rewind for a stage id that is not in StageOrder.
var ErrUnknownStage = errors.New("unknown pipeline stage")
