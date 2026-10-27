package filler

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/defects"
)

// defectMeasureBudget bounds how long one pass spends decoding. A pass that runs out leaves the rest
// for the next; only a first pass over a large catalog takes more than one.
const defectMeasureBudget = 40 * time.Second

// defectJudgeCalls bounds how many borderline clips one pass puts to the judge. A hosted look is
// a charge and takes seconds; the rest wait for the next pass.
const defectJudgeCalls = 6

// DefectRecord is one clip's latest defect measurement and the verdict it earned (#2034).
type DefectRecord struct {
	ClipHash   string
	Readings   defects.Readings
	Era        int
	Assessment defects.Assessment
	// Judgement is the judge's ruling on a borderline assessment; nil until one is had.
	Judgement     *defects.Judgement
	Verdict       defects.Verdict
	VerdictReason string
	MeasuredAt    time.Time
}

// ErrDefectRecordsCorrupt marks a ListClipDefects result that omitted unreadable rows; the rest of
// the map is usable. Any other error is a failed read.
var ErrDefectRecordsCorrupt = errors.New("some defect records are corrupt")

// DefectStore is what the defect floor reads and writes.
type DefectStore interface {
	// ListDefectClips is every catalog clip, held ones included: the floor is judged before a
	// clip is reviewed, so the record can say what it would retire.
	ListDefectClips(ctx context.Context) ([]DefectClip, error)
	ListClipDefects(ctx context.Context) (map[string]DefectRecord, error)
	UpsertClipDefects(ctx context.Context, r DefectRecord) error
}

// DefectClip is a catalog clip with its admission time, so a backlog is measured oldest first.
type DefectClip struct {
	StoreClip
	CreatedAt time.Time
}

// MeasureDefectsFunc decodes one file's defect readings (mediatools.MeasureDefects).
type MeasureDefectsFunc func(ctx context.Context, file string, width, height int, hasAudio bool) (defects.Readings, error)

// DefectFloor is the standing job that keeps every clip's defect record current (#2034). It only
// records: nothing is held or retired on a verdict until enforcement (#2036).
type DefectFloor struct {
	store   DefectStore
	probe   Prober
	measure MeasureDefectsFunc
	judge   DefectJudge
	clipDir string
	now     func() time.Time
	log     *slog.Logger
}

// DefectFloorResult counts one pass.
type DefectFloorResult struct {
	Measured   int // decoded and judged
	Reassessed int // judged again from stored readings: a new floor version or a corrected era
	Pending    int // left for the next pass when the decode budget ran out
	Failed     int // could not be probed or measured; recorded as undecided with the reason
	Ruled      int // borderline clips the judge ruled on, or recorded as having no judge
	Held       int // borderline clips left waiting: the judge was out of reach, out of budget or out of turns
}

// NewDefectFloor builds the defect floor job. A nil judge is the same as no vision model.
func NewDefectFloor(store DefectStore, probe Prober, measure MeasureDefectsFunc, judge DefectJudge, clipDir string,
	now func() time.Time, log *slog.Logger) *DefectFloor {
	return &DefectFloor{store: store, probe: probe, measure: measure, judge: judge, clipDir: clipDir, now: now, log: log}
}

// Run measures every clip without a current measurement and re-judges every record whose floor
// version or era is stale. Re-judging needs no decode, so a floor recalibration reaches the whole
// catalog in one pass.
func (d *DefectFloor) Run(ctx context.Context) (DefectFloorResult, error) {
	var res DefectFloorResult
	clips, err := d.store.ListDefectClips(ctx)
	if err != nil {
		return res, err
	}
	records, err := d.store.ListClipDefects(ctx)
	switch {
	case errors.Is(err, ErrDefectRecordsCorrupt):
		// Corrupt rows are omitted, so those clips are measured again.
		d.warn("defects: some records were unreadable", "err", err)
	case err != nil:
		// Not a partial read: every clip would look unmeasured and be decoded again.
		return res, err
	}
	slices.SortFunc(clips, func(a, b DefectClip) int {
		return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), strings.Compare(a.Hash, b.Hash))
	})
	deadline := d.now().Add(defectMeasureBudget)
	judge := d.judgeNow(ctx)
	for _, c := range clips {
		if !defectCandidate(c.Clip) {
			continue
		}
		rec, known := records[c.Hash]
		changed := false
		if known && rec.Readings.Version == defects.MeasurementVersion {
			if rec.Assessment.Version != defects.FloorVersion || rec.Era != c.Era {
				a := defects.Assess(rec.Readings, c.Era)
				if !defects.SameQuestion(rec.Assessment, a) {
					rec.Judgement = nil // it ruled on checks this assessment no longer asks about
				}
				rec.Era, rec.Assessment = c.Era, a
				changed = true
				res.Reassessed++
			}
		} else {
			if ctx.Err() != nil || d.now().After(deadline) {
				res.Pending++
				continue
			}
			readings, err := d.measureClip(ctx, c.Clip)
			if err != nil {
				if ctx.Err() != nil {
					res.Pending++ // the pass was cancelled, not the clip
					continue
				}
				// Recorded rather than retried: a clip with no picture, or one the decoder rejects, fails
				// the same way every hour and would keep spending the budget ahead of healthy clips. A
				// new MeasurementVersion measures it again.
				d.warn("defects: clip could not be measured", "clip", c.Hash, "err", err)
				readings = defects.Readings{Version: defects.MeasurementVersion, Unmeasurable: err.Error()}
				res.Failed++
			} else {
				res.Measured++
			}
			rec = DefectRecord{ClipHash: c.Hash, Readings: readings, Era: c.Era,
				Assessment: defects.Assess(readings, c.Era), MeasuredAt: d.now()}
			changed = true
		}
		if d.rule(ctx, &rec, c.Clip, &judge, &res) {
			changed = true
		}
		verdict, reason := defects.Decide(rec.Assessment, rec.Judgement)
		if verdict != rec.Verdict || reason != rec.VerdictReason {
			rec.Verdict, rec.VerdictReason, changed = verdict, reason, true
		}
		if !changed {
			continue
		}
		if err := d.store.UpsertClipDefects(ctx, rec); err != nil {
			return res, err
		}
	}
	if d.log != nil && (res.Measured > 0 || res.Reassessed > 0 || res.Failed > 0 || res.Ruled > 0) {
		d.log.Info("defects: records current", "measured", res.Measured, "reassessed", res.Reassessed,
			"pending", res.Pending, "failed", res.Failed, "ruled", res.Ruled, "held", res.Held)
	}
	return res, ctx.Err()
}

// judgeState is the judge as one pass sees it: asked which model it is once, then counted down.
type judgeState struct {
	model string
	err   error // ErrNoDefectJudge, or why the judge could not be reached
	calls int
}

func (d *DefectFloor) judgeNow(ctx context.Context) judgeState {
	if d.judge == nil {
		return judgeState{err: ErrNoDefectJudge}
	}
	model, err := d.judge.Model(ctx)
	return judgeState{model: model, err: err}
}

// rule asks the judge about a borderline clip that has no current ruling, and reports whether the
// record changed. With no judge configured the clip is recorded as having none; when the judge is
// out of reach, out of budget or out of turns for this pass, the record is left for the next.
func (d *DefectFloor) rule(ctx context.Context, rec *DefectRecord, c Clip, judge *judgeState, res *DefectFloorResult) bool {
	if rec.Assessment.Outcome != defects.Borderline {
		if rec.Judgement == nil {
			return false
		}
		rec.Judgement = nil
		return true
	}
	if !needsRuling(rec.Judgement, *judge) {
		return false
	}
	if errors.Is(judge.err, ErrNoDefectJudge) {
		rec.Judgement = &defects.Judgement{Version: defects.JudgeVersion, Ruling: defects.RulingNoJudge}
		res.Ruled++
		return true
	}
	if judge.err != nil || judge.calls >= defectJudgeCalls || ctx.Err() != nil {
		res.Held++
		return false
	}
	judge.calls++
	file, err := ClipPath(d.clipDir, c.Path, filepath.Ext(c.Path))
	if err != nil {
		d.warn("defects: judge could not find the clip", "clip", c.Hash, "err", err)
		res.Held++
		return false
	}
	j, err := d.judge.Rule(ctx, DefectJudgeRequest{
		ClipHash: c.Hash, File: file, DurationMs: c.DurationMs, Era: rec.Era, Assessment: rec.Assessment,
	})
	switch {
	case errors.Is(err, ErrNoDefectJudge):
		// The settings changed under the pass: record it as the next pass would.
		*judge = judgeState{err: ErrNoDefectJudge}
		rec.Judgement = &defects.Judgement{Version: defects.JudgeVersion, Ruling: defects.RulingNoJudge}
		res.Ruled++
		return true
	case errors.Is(err, ErrDefectJudgeHeld):
		judge.err = err // the ceiling holds for the rest of the pass
		res.Held++
		return false
	case err != nil:
		d.warn("defects: judge could not rule", "clip", c.Hash, "err", err)
		res.Held++
		return false
	}
	rec.Judgement = &j
	res.Ruled++
	return true
}

// needsRuling reports whether a borderline clip should be put to the judge as it stands now. A
// ruling under an older judge contract is asked again; a "no judge" record once a judge exists;
// an abstention only of a different model, since the same one would answer the same.
func needsRuling(j *defects.Judgement, judge judgeState) bool {
	switch {
	case j == nil || j.Version != defects.JudgeVersion:
		return true
	case j.Ruling == defects.RulingNoJudge:
		return judge.err == nil
	case j.Ruling == defects.RulingAbstain:
		return judge.err == nil && j.Model != judge.model
	}
	return false
}

// measureClip probes the playback file for its frame size and sound, then measures it. The file
// judged is the one that airs.
func (d *DefectFloor) measureClip(ctx context.Context, c Clip) (defects.Readings, error) {
	full, err := ClipPath(d.clipDir, c.Path, filepath.Ext(c.Path))
	if err != nil {
		return defects.Readings{}, err
	}
	p, err := d.probe(ctx, full)
	if err != nil {
		return defects.Readings{}, err
	}
	return d.measure(ctx, full, p.Width, p.Height, !p.Silent)
}

// defectCandidate is a clip that may air as an ad of its own once admitted. A composite is a
// recorded break and is judged through its segments; an unplayable clip has no picture to judge.
func defectCandidate(c Clip) bool {
	return !c.IsComposite && c.RemovedAt.IsZero() && c.Placement != PlacementNotPlayable
}

func (d *DefectFloor) warn(msg string, args ...any) {
	if d.log != nil {
		d.log.Warn(msg, args...)
	}
}
