package filler

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/creative"
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
	Judgement *defects.Judgement
	// JudgeFailures counts asks that brought back no ruling since the last one; the clip waits
	// behind clips with fewer.
	JudgeFailures int
	Verdict       defects.Verdict
	VerdictReason string
	MeasuredAt    time.Time
}

// ErrDefectRecordsCorrupt marks a ListClipDefects result that omitted unreadable rows; the rest of
// the map is usable. Any other error is a failed read.
var ErrDefectRecordsCorrupt = errors.New("some defect records are corrupt")

// ErrCopyGone is RetireDefectiveCopy finding no row to retire: creative identity merged it away
// since the pass listed it.
var ErrCopyGone = errors.New("the copy is no longer in the catalog")

// DefectStore is what the defect floor reads and writes.
type DefectStore interface {
	// ListDefectClips is every catalog clip, held ones included: admission waits at the defects
	// rung for a held clip's verdict. A held clip carries its conveyor's stage.
	ListDefectClips(ctx context.Context) ([]DefectClip, error)
	// GetClip re-reads one clip, so evidence is released only from a clip still unheld.
	GetClip(ctx context.Context, hash string) (StoreClip, bool, error)
	// ListClipPaths is every clip's path, removed ones included: a removed clip can be restored,
	// and the assets its sidecar names must survive until then.
	ListClipPaths(ctx context.Context) ([]string, error)
	ListClipDefects(ctx context.Context) (map[string]DefectRecord, error)
	UpsertClipDefects(ctx context.Context, r DefectRecord) error
	// RetireDefectiveCopy takes a failing copy off the catalog for good, keeping its row as the
	// ad's "wanted: better copy" entry (#2036).
	RetireDefectiveCopy(ctx context.Context, hash string, at time.Time) error
	ListClipFingerprints(ctx context.Context, algorithm string) (map[string]creative.Fingerprint, error)
	UpsertClipFingerprint(ctx context.Context, clipHash, algorithm string, fp creative.Fingerprint) error
}

// FingerprintFunc fingerprints a whole file (mediatools.MediaTools.Fingerprint from its start).
type FingerprintFunc func(ctx context.Context, file string, durationMs int64) (creative.Fingerprint, error)

// DefectClip is a catalog clip with its admission time, so a backlog is measured oldest first.
type DefectClip struct {
	StoreClip
	CreatedAt time.Time
	// Stage is a held clip's conveyor rung. The floor measures it only once it reaches the
	// defects rung: before then the bytes are not yet the ones that air, and a whole break not yet
	// split would be judged as one ad.
	Stage StageID
}

// awaitingFloor is a clip the floor should measure now: published, or held at or past the rung.
func (c DefectClip) awaitingFloor() bool {
	return !c.Held || StageIndex(c.Stage) >= StageIndex(StageDefects)
}

// MeasureDefectsFunc decodes one file's defect readings (mediatools.MeasureDefects).
type MeasureDefectsFunc func(ctx context.Context, file string, width, height int, hasAudio bool) (defects.Readings, error)

// DefectFloor is the standing job that keeps every clip's defect record current (#2034) and
// enforces it (#2036). The defects rung admits or refuses a held clip on its verdict; a copy on air
// that definitively fails is retired here and its files deleted.
type DefectFloor struct {
	store       DefectStore
	probe       Prober
	measure     MeasureDefectsFunc
	fingerprint FingerprintFunc
	judge       DefectJudge
	clipDir     string
	now         func() time.Time
	log         *slog.Logger
	// released remembers the admitted clips already holding no evidence copy, so a pass reads only
	// new arrivals' sidecars. It is per process: a restart reads each sidecar once more.
	released map[string]bool
}

// DefectFloorResult counts one pass.
type DefectFloorResult struct {
	Measured   int // decoded and judged
	Reassessed int // judged again from stored readings: a new floor version or a corrected era
	Pending    int // left for the next pass when the decode budget ran out
	Failed     int // could not be probed or measured; recorded as undecided with the reason
	Ruled      int // borderline clips the judge ruled on, or recorded as having no judge
	Held       int // borderline clips left waiting: the judge was out of reach, out of budget or out of turns
	Retired    int // copies that definitively failed, taken off the catalog and deleted
	Released   int // admitted clips whose evidence copy was dropped
}

// NewDefectFloor builds the defect floor job. A nil judge is the same as no vision model.
func NewDefectFloor(store DefectStore, probe Prober, measure MeasureDefectsFunc, fingerprint FingerprintFunc,
	judge DefectJudge, clipDir string, now func() time.Time, log *slog.Logger) *DefectFloor {
	return &DefectFloor{store: store, probe: probe, measure: measure, fingerprint: fingerprint, judge: judge,
		clipDir: clipDir, now: now, log: log, released: map[string]bool{}}
}

// Run measures every clip without a current measurement, held clips first since admission waits
// on them, re-judges every record whose floor version or era is stale, and retires every copy
// that definitively fails. Re-judging needs no decode, so a floor recalibration reaches the whole
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
	// Held clips first: each waits at the defects rung for its verdict. Then oldest first.
	heldFirst := func(c DefectClip) int {
		if c.Held {
			return 0
		}
		return 1
	}
	slices.SortFunc(clips, func(a, b DefectClip) int {
		return cmp.Or(cmp.Compare(heldFirst(a), heldFirst(b)), a.CreatedAt.Compare(b.CreatedAt), strings.Compare(a.Hash, b.Hash))
	})
	final := make(map[string]DefectRecord, len(clips))
	deadline := d.now().Add(defectMeasureBudget)
	judge := d.judgeNow(ctx)
	var asks []defectAsk
	for _, c := range clips {
		if !defectCandidate(c.Clip) || !c.awaitingFloor() {
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
		switch {
		case rec.Assessment.Outcome != defects.Borderline:
			if rec.Judgement != nil || rec.JudgeFailures != 0 {
				rec.Judgement, rec.JudgeFailures, changed = nil, 0, true
			}
		case !needsRuling(rec.Judgement, judge):
		case errors.Is(judge.err, ErrNoDefectJudge):
			rec.Judgement, changed = &defects.Judgement{Version: defects.JudgeVersion, Ruling: defects.RulingNoJudge}, true
			res.Ruled++
		default:
			asks = append(asks, defectAsk{clip: c, rec: rec})
		}
		if err := d.save(ctx, &rec, changed); err != nil {
			return res, err
		}
		final[c.Hash] = rec
	}
	// The judge's turn, after every clip is measured: fewest failed asks first, so a clip the judge
	// keeps failing on falls behind the rest instead of taking every turn.
	slices.SortStableFunc(asks, func(a, b defectAsk) int { return cmp.Compare(a.rec.JudgeFailures, b.rec.JudgeFailures) })
	for _, ask := range asks {
		changed, err := d.ask(ctx, &ask, &judge, &res)
		if err != nil {
			return res, err
		}
		if err := d.save(ctx, &ask.rec, changed); err != nil {
			return res, err
		}
		final[ask.clip.Hash] = ask.rec
	}
	if err := d.retireFailures(ctx, clips, final, &res); err != nil {
		return res, err
	}
	d.releaseEvidence(ctx, clips, final, &res)
	if d.log != nil && (res.Measured > 0 || res.Reassessed > 0 || res.Failed > 0 || res.Ruled > 0 || res.Retired > 0 || res.Released > 0) {
		d.log.Info("defects: records current", "measured", res.Measured, "reassessed", res.Reassessed,
			"pending", res.Pending, "failed", res.Failed, "ruled", res.Ruled, "held", res.Held,
			"retired", res.Retired, "released", res.Released)
	}
	return res, ctx.Err()
}

// releaseEvidence drops the evidence copy of every admitted clip (#2036). Admission is the last
// reader: screening and split read it before a clip airs, and every later check (this floor,
// creative identity) reads the playback file. A rewind to a rung that needs it starts at the
// transcode instead, which rebuilds it from the source master. The sidecar stops naming it before
// the file goes, so a failed unlink leaves only bytes.
func (d *DefectFloor) releaseEvidence(ctx context.Context, clips []DefectClip, final map[string]DefectRecord, res *DefectFloorResult) {
	var admitted []DefectClip
	for _, c := range clips {
		rec, ok := final[c.Hash]
		if !ok || c.Held || d.released[c.Hash] ||
			rec.Verdict != defects.VerdictPass && rec.Verdict != defects.VerdictUndecided {
			continue
		}
		admitted = append(admitted, c)
	}
	var referenced map[string]int
	for _, c := range admitted {
		full, err := ClipPath(d.clipDir, c.Path, filepath.Ext(c.Path))
		if err != nil {
			continue
		}
		tags, ok := ReadSidecarTags(full)
		if !ok || tags.MediaAssets == nil || tags.MediaAssets.Evidence == nil {
			d.released[c.Hash] = true
			continue
		}
		// The judge can hold a pass for minutes, and a rewind holds the clip before it reads the
		// sidecar. Re-read here, so a clip re-entering the conveyor keeps its evidence.
		if now, found, err := d.store.GetClip(ctx, c.Hash); err != nil || !found || now.Held {
			continue
		}
		if referenced == nil {
			if referenced, err = d.referenced(ctx); err != nil {
				d.warn("defects: evidence copies were not released; the catalog could not be listed", "err", err)
				return
			}
		}
		evidence := derivativePath(tags.MediaAssets.Evidence)
		tags.MediaAssets.Evidence = nil
		if err := WriteSidecarTags(full, tags, false); err != nil {
			d.warn("defects: an admitted clip's evidence copy could not be released", "clip", c.Hash, "err", err)
			continue
		}
		d.released[c.Hash] = true
		res.Released++
		referenced[evidence]--
		if referenced[evidence] > 0 {
			continue // another clip's sidecar names it too
		}
		if err := os.Remove(filepath.Join(d.clipDir, filepath.FromSlash(evidence))); err != nil && !os.IsNotExist(err) {
			d.warn("defects: a released evidence copy could not be removed", "clip", c.Hash, "file", evidence, "err", err)
		}
	}
}

// retireFailures retires every published copy whose verdict definitively fails; a held one is
// refused at the defects rung instead, which settles its conveyor row. A clip that cannot be
// fingerprinted is left for the next pass: without its fingerprint no cleaner copy of the ad could
// ever be recognised, so deleting it would lose the ad rather than ask for a better one.
func (d *DefectFloor) retireFailures(ctx context.Context, clips []DefectClip, final map[string]DefectRecord, res *DefectFloorResult) error {
	var failing []DefectClip
	for _, c := range clips {
		if rec, ok := final[c.Hash]; ok && !c.Held && definitiveFail(rec) {
			failing = append(failing, c)
		}
	}
	if len(failing) == 0 {
		return nil
	}
	fps, err := d.store.ListClipFingerprints(ctx, creative.Algorithm)
	if err != nil {
		d.warn("defects: some cached fingerprints were unreadable", "err", err)
	}
	referenced, err := d.referenced(ctx)
	if err != nil {
		return err
	}
	for _, c := range failing {
		if ctx.Err() != nil {
			return nil
		}
		retired, err := d.retire(ctx, c, fps, referenced)
		if err != nil {
			return err
		}
		if retired {
			res.Retired++
		}
	}
	return nil
}

// retire records the copy's fingerprint when it has none (identity has not reached it yet), takes it off
// the catalog, then deletes its own files. The durable write commits before any unlink, as in
// CreativeIdentity.merge, so a failed unlink leaves only bytes, never a failing copy on air.
func (d *DefectFloor) retire(ctx context.Context, c DefectClip, fps map[string]creative.Fingerprint, referenced map[string]int) (bool, error) {
	full, err := ClipPath(d.clipDir, c.Path, filepath.Ext(c.Path))
	if err != nil {
		d.warn("defects: a failing copy has no usable path", "clip", c.Hash, "err", err)
		return false, nil
	}
	if _, ok := fps[c.Hash]; !ok {
		durationMs := c.DurationMs
		if durationMs <= 0 && d.probe != nil {
			if p, err := d.probe(ctx, full); err == nil {
				durationMs = p.DurationMs
			}
		}
		fp, err := d.fingerprint(ctx, full, durationMs)
		if err == nil && len(fp.Video) == 0 {
			err = errors.New("no picture to fingerprint")
		}
		if err != nil {
			d.warn("defects: a failing copy could not be fingerprinted; it is retired on a later pass", "clip", c.Hash, "err", err)
			return false, nil
		}
		if err := d.store.UpsertClipFingerprint(ctx, c.Hash, creative.Algorithm, fp); err != nil {
			return false, err
		}
	}
	tags, _ := ReadSidecarTags(full)
	if err := d.store.RetireDefectiveCopy(ctx, c.Hash, d.now().UTC()); errors.Is(err, ErrCopyGone) {
		return false, nil // merged into a better copy, its files with it
	} else if err != nil {
		return false, err
	}
	for f, err := range removeOwnFiles(d.clipDir, full, tags, referenced) {
		d.warn("defects: a retired copy's file could not be removed; it is already off the catalog",
			"clip", c.Hash, "file", f, "err", err)
	}
	return true, nil
}

// referenced counts the sidecars naming each media asset across every clip, removed ones included.
func (d *DefectFloor) referenced(ctx context.Context) (map[string]int, error) {
	paths, err := d.store.ListClipPaths(ctx)
	if err != nil {
		return nil, err
	}
	return referencedAssets(d.clipDir, paths), nil
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

// defectAsk is a borderline clip waiting for the judge this pass.
type defectAsk struct {
	clip DefectClip
	rec  DefectRecord
}

// save decides the record's verdict and writes it when anything changed.
func (d *DefectFloor) save(ctx context.Context, rec *DefectRecord, changed bool) error {
	verdict, reason := defects.Decide(rec.Assessment, rec.Judgement)
	if verdict != rec.Verdict || reason != rec.VerdictReason {
		rec.Verdict, rec.VerdictReason, changed = verdict, reason, true
	}
	if !changed {
		return nil
	}
	return d.store.UpsertClipDefects(ctx, *rec)
}

// ask puts one borderline clip to the judge and reports whether its record changed. A failure that
// belongs to the clip is counted against it and the next clip is asked; a failure of the judge
// itself (an outage, a spent budget) ends the asking for this pass, and every clip still waiting
// stays pending for the next.
func (d *DefectFloor) ask(ctx context.Context, ask *defectAsk, judge *judgeState, res *DefectFloorResult) (bool, error) {
	rec, c := &ask.rec, ask.clip.Clip
	if errors.Is(judge.err, ErrNoDefectJudge) {
		rec.Judgement = &defects.Judgement{Version: defects.JudgeVersion, Ruling: defects.RulingNoJudge}
		res.Ruled++
		return true, nil
	}
	if judge.err != nil || judge.calls >= defectJudgeCalls || ctx.Err() != nil {
		res.Held++
		return false, nil
	}
	judge.calls++
	j, err := d.ruleOn(ctx, rec, c)
	switch {
	case err == nil:
		rec.Judgement, rec.JudgeFailures = &j, 0
		res.Ruled++
		return true, nil
	case errors.Is(err, ErrNoDefectJudge):
		// The settings changed under the pass: record it, and every clip after it, as having none.
		judge.err = ErrNoDefectJudge
		rec.Judgement = &defects.Judgement{Version: defects.JudgeVersion, Ruling: defects.RulingNoJudge}
		res.Ruled++
		return true, nil
	case errors.Is(err, ErrDefectJudgeHeld):
		judge.err = err
		res.Held++
		return false, nil
	case ctx.Err() != nil:
		res.Held++ // the pass was cancelled, not the clip
		return false, nil
	}
	d.warn("defects: judge could not rule", "clip", c.Hash, "err", err)
	rec.JudgeFailures++
	res.Held++
	if !errors.Is(err, ErrDefectJudgeClip) {
		judge.err = err // the judge itself failed: no more asks this pass
	}
	return true, nil
}

// ruleOn asks the judge about one clip, probing the file for its length when the catalog has none.
func (d *DefectFloor) ruleOn(ctx context.Context, rec *DefectRecord, c Clip) (defects.Judgement, error) {
	file, err := ClipPath(d.clipDir, c.Path, filepath.Ext(c.Path))
	if err != nil {
		return defects.Judgement{}, fmt.Errorf("%w: %w", ErrDefectJudgeClip, err)
	}
	durationMs := c.DurationMs
	if durationMs <= 0 && d.probe != nil {
		if p, err := d.probe(ctx, file); err == nil {
			durationMs = p.DurationMs
		}
	}
	if durationMs <= 0 {
		return defects.Judgement{}, fmt.Errorf("%w: the clip's length is unknown", ErrDefectJudgeClip)
	}
	return d.judge.Rule(ctx, DefectJudgeRequest{
		ClipHash: c.Hash, File: file, DurationMs: durationMs, Era: rec.Era, Assessment: rec.Assessment,
	})
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
