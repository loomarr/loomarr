package filler

import (
	"context"
	"fmt"
	"strings"

	"github.com/loomarr/loomarr/internal/defects"
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

func (s *DefectsStage) ID() StageID     { return StageDefects }
func (s *DefectsStage) Cost() StageCost { return CostCheap }

// Applies to every clip that may air as an ad of its own, the clips the floor judges.
func (s *DefectsStage) Applies(_ context.Context, c StoreClip) (bool, string) {
	if !defectCandidate(c.Clip) {
		return false, "a recording of a whole break is judged through its segments"
	}
	return true, ""
}

// Run admits a clip that passed, or that had too little picture to judge, and refuses one that
// failed. Without a current verdict the clip waits its turn, spending no attempt.
func (s *DefectsStage) Run(ctx context.Context, c StoreClip) (StageResult, error) {
	rec, ok, err := s.verdicts.GetClipDefects(ctx, c.Hash)
	if err != nil {
		return StageResult{}, err
	}
	if !ok || rec.Readings.Version != defects.MeasurementVersion || rec.Assessment.Version != defects.FloorVersion ||
		rec.Era != c.Era {
		return StageResult{Verdict: VerdictDefer, Note: "waiting for the defect floor to measure the picture and sound"}, nil
	}
	switch rec.Verdict {
	case defects.VerdictPass:
		return StageResult{Verdict: VerdictContinue, Note: "meets the floor for its era"}, nil
	case defects.VerdictUndecided:
		return StageResult{Verdict: VerdictContinue, Note: "too little picture to judge against the floor"}, nil
	case defects.VerdictFail:
		return StageResult{Verdict: VerdictReject, Reason: ReasonDefect, Detail: defectDetail(rec)}, nil
	default:
		return StageResult{Verdict: VerdictDefer, Note: "waiting for the judge to rule on a borderline picture"}, nil
	}
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
