package defectfloor

import (
	"context"
	"errors"
	"fmt"

	"github.com/loomarr/loomarr/internal/media/defects"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// CheckReplacement measures and judges the staged bytes that would replace an airing clip's file,
// before they take its place (#2121). It is the floor's own rule on the file that would air: the
// same detectors, the bars for the clip's era, and the judge for a borderline picture. It saves
// nothing. A replacement that passes carries this record into the switch; one that does not
// leaves the airing clip and its record as they were.
//
// Unlike a pass over the catalog, a file that cannot be measured is an error rather than an
// undecided verdict, and so is a judge that cannot rule: the clip already airs, so a replacement
// waits for a clear answer instead of being admitted without one.
func (d *DefectFloor) CheckReplacement(ctx context.Context, c clipstore.Clip, staged string) (defects.Record, error) {
	p, err := d.probe(ctx, staged)
	if err != nil {
		return defects.Record{}, fmt.Errorf("probe the replacement: %w", err)
	}
	readings, err := d.measure(ctx, staged, p.Width, p.Height, !p.Silent)
	if err != nil {
		return defects.Record{}, fmt.Errorf("measure the replacement: %w", err)
	}
	rec := defects.Record{ClipHash: c.Hash, Readings: readings, Era: c.Era,
		Assessment: defects.Assess(readings, c.Era), MeasuredAt: d.now()}
	if rec.Assessment.Outcome == defects.Borderline {
		j, err := d.ruleOnReplacement(ctx, rec, staged, p.DurationMs)
		if err != nil {
			return defects.Record{}, err
		}
		rec.Judgement = &j
	}
	rec.Verdict, rec.VerdictReason = defects.Decide(rec.Assessment, rec.Judgement)
	return rec, nil
}

// ruleOnReplacement asks the judge about a borderline replacement. With no judge configured the
// ruling records that, which Decide fails as it would at admission.
func (d *DefectFloor) ruleOnReplacement(ctx context.Context, rec defects.Record, staged string, durationMs int64) (defects.Judgement, error) {
	noJudge := defects.Judgement{Version: defects.JudgeVersion, Ruling: defects.RulingNoJudge}
	if d.judge == nil {
		return noJudge, nil
	}
	if durationMs <= 0 {
		return defects.Judgement{}, errors.New("judge the replacement: its length is unknown")
	}
	j, err := d.judge.Rule(ctx, DefectJudgeRequest{
		ClipHash: rec.ClipHash, File: staged, DurationMs: durationMs, Era: rec.Era, Assessment: rec.Assessment,
	})
	switch {
	case errors.Is(err, ErrNoDefectJudge):
		return noJudge, nil
	case err != nil:
		return defects.Judgement{}, fmt.Errorf("judge the replacement: %w", err)
	}
	return j, nil
}
