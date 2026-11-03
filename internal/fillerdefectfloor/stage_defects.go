package fillerdefectfloor

import (
	"context"
	"fmt"
	"strings"

	"github.com/loomarr/loomarr/internal/defects"
	"github.com/loomarr/loomarr/internal/filler"
)

// The DEFECTS stage (#2036): the defect floor, enforced at admission. It measures nothing itself.
// The defect floor job measures held clips first and records each verdict; this rung waits for
// that verdict and acts on it, so one path decodes and judges, and a recalibration reaches the
// clips still in the conveyor the same way it reaches the catalog.
//
// It runs after the transcode, whose bytes are what airs and what the floor measured, and before
// enrichment, so a copy that fails costs no transcription or hosted look.

// DefectVerdicts reads one clip's defect record.
type DefectVerdicts interface {
	GetClipDefects(ctx context.Context, clipHash string) (DefectRecord, bool, error)
}

// DefectsStage holds a clip until the floor has ruled on it and refuses one that fails.
type DefectsStage struct {
	verdicts DefectVerdicts
}

// NewDefectsStage builds the stage.
func NewDefectsStage(verdicts DefectVerdicts) *DefectsStage {
	return &DefectsStage{verdicts: verdicts}
}

func (s *DefectsStage) ID() filler.StageID     { return filler.StageDefects }
func (s *DefectsStage) Cost() filler.StageCost { return filler.CostCheap }

// Applies to every clip that may air as an ad of its own, the clips the floor judges.
func (s *DefectsStage) Applies(_ context.Context, c filler.StoreClip) (bool, string) {
	switch {
	case c.IsComposite:
		return false, "a recording of a whole break is judged through its segments"
	case !defectCandidate(c.Clip):
		return false, "not a clip that airs"
	}
	return true, ""
}

// Run admits a clip that passed, or that had too little picture to judge, and refuses one that
// failed. Without a current verdict the clip waits its turn, spending no attempt. A held clip that
// fails is refused here, by the pipeline; the floor retires only copies already on air.
func (s *DefectsStage) Run(ctx context.Context, c filler.StoreClip) (filler.StageResult, error) {
	rec, ok, err := s.verdicts.GetClipDefects(ctx, c.Hash)
	if err != nil {
		return filler.StageResult{}, err
	}
	if !ok || rec.Readings.Version != defects.MeasurementVersion || rec.Assessment.Version != defects.FloorVersion ||
		rec.Era != c.Era {
		return filler.StageResult{Verdict: filler.VerdictDefer, Note: "waiting for the defect floor to measure the picture and sound"}, nil
	}
	switch rec.Verdict {
	case defects.VerdictPass:
		return filler.StageResult{Verdict: filler.VerdictContinue, Note: "meets the floor for its era"}, nil
	case defects.VerdictUndecided:
		return filler.StageResult{Verdict: filler.VerdictContinue, Note: "too little picture to judge against the floor"}, nil
	case defects.VerdictFail:
		if definitiveFail(rec) {
			return filler.StageResult{Verdict: filler.VerdictReject, Reason: filler.ReasonDefect, Detail: defectDetail(rec)}, nil
		}
		// No judge could rule: none is configured, or it would not answer. Nothing automatic will
		// settle the clip, so it is the operator's call rather than a wait with no end.
		return filler.StageResult{Verdict: filler.VerdictReview, Note: "borderline picture, and no vision model could rule on it: " + defectDetail(rec)}, nil
	default:
		return filler.StageResult{Verdict: filler.VerdictDefer, Note: "waiting for the judge to rule on a borderline picture"}, nil
	}
}

// definitiveFail is a failing verdict that stands: the detectors' own, or the judge's ruling. A
// borderline clip failed only because no judge could rule (none configured, or it abstained) is
// ruled on again once one can, so it is held off air but keeps its file.
func definitiveFail(rec DefectRecord) bool {
	if rec.Verdict != defects.VerdictFail {
		return false
	}
	return rec.Assessment.Outcome == defects.Fail || rec.Judgement != nil && rec.Judgement.Ruling == defects.RulingFail
}

// defectDetail is the measured fact behind a refusal: each failing check against its bar, or the
// judge's reason when the detectors left it borderline.
func defectDetail(rec DefectRecord) string {
	var parts []string
	for _, f := range rec.Assessment.Findings {
		if f.Outcome == defects.Fail {
			parts = append(parts, fmt.Sprintf("%s %.3g; bar %.3g", f.Check, f.Measured, f.Bar))
		}
	}
	if len(parts) == 0 {
		return rec.VerdictReason
	}
	return strings.Join(parts, ", ")
}
