package assessment

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/loomarr/loomarr/internal/filler/structure"
	"github.com/loomarr/loomarr/internal/filler/structure/window"
)

// ErrStructureWindowRetryable marks a window call that produced no answer: the budget held it or
// the request got no response. Nothing is published, so the next pass calls the window again.
var ErrStructureWindowRetryable = errors.New("structure window call produced no answer and is retried on the next pass")

// StructureWindowFamilyRuntime owns one assessor family's complete serial window run. It commits
// and replays every call before advancing, then persists the deterministic whole-source stitch.
// Running families through separate instances prevents either family from observing peer answers.
type StructureWindowFamilyRuntime struct {
	assessor          CompleteWindowStructureAssessor
	profile           structure.AssessorProfile
	evidence          StructureWindowEvidenceRepository
	boundaryTolerance int64
}

func NewStructureWindowFamilyRuntime(assessor CompleteWindowStructureAssessor, evidence StructureWindowEvidenceRepository, boundaryTolerance int64) (*StructureWindowFamilyRuntime, error) {
	if assessor == nil || evidence == nil || boundaryTolerance < 0 ||
		boundaryTolerance >= window.ContextOverlapMS {
		return nil, errors.New("structure window family runtime requires an assessor, evidence repository, and valid tolerance")
	}
	profile := assessor.Profile()
	if err := structure.ValidateAssessorProfile(profile); err != nil {
		return nil, fmt.Errorf("structure window family runtime profile: %w", err)
	}
	return &StructureWindowFamilyRuntime{
		assessor: assessor, profile: profile, evidence: evidence, boundaryTolerance: boundaryTolerance,
	}, nil
}

// Assess runs or resumes every window and returns one replayable source-level family stitch.
func (r *StructureWindowFamilyRuntime) Assess(ctx context.Context, prepared StructureAssessmentWindowMediaSet) (window.StitchResult, error) {
	evidence, err := r.AssessWithEvidence(ctx, prepared)
	if err != nil {
		return window.StitchResult{}, err
	}
	return evidence.Stitch, nil
}

// AssessWithEvidence returns the complete persisted call/accounting chain behind the stitch. It is
// used by certification; production reduction may use Assess when it needs only the stitch.
func (r *StructureWindowFamilyRuntime) AssessWithEvidence(ctx context.Context, prepared StructureAssessmentWindowMediaSet) (StructureWindowFamilyEvidence, error) {
	if r == nil || r.assessor == nil || r.evidence == nil || structure.ValidateAssessorProfile(r.profile) != nil {
		return StructureWindowFamilyEvidence{}, errors.New("structure window family runtime is unavailable")
	}
	wantSource := structure.Source{
		SHA256: prepared.Source.SHA256, Bytes: prepared.Source.Bytes, DurationMS: prepared.Source.DurationMs,
	}
	if prepared.Authority.Plan.Source != wantSource || validatePreparedStructureAssessmentWindows(prepared) != nil {
		return StructureWindowFamilyEvidence{}, errors.New("structure window family runtime media authority is invalid")
	}
	assessments := make([]window.Assessment, 0, len(prepared.Windows))
	recordedAssessments := make([]window.RecordedAssessment, 0, len(prepared.Windows))
	for ordinal, win := range prepared.Windows {
		if err := ctx.Err(); err != nil {
			return StructureWindowFamilyEvidence{}, err
		}
		recorded, found, err := r.evidence.FindStructureWindowAssessmentEvidence(ctx, prepared.Authority, ordinal, r.profile)
		if err != nil {
			return StructureWindowFamilyEvidence{}, fmt.Errorf("find structure window %d evidence: %w", ordinal, err)
		}
		if !found {
			recorded, err = r.assessor.AssessWindow(ctx, prepared.Authority, win)
			if err != nil {
				return StructureWindowFamilyEvidence{}, fmt.Errorf("structure window %d produced no authority: %w", ordinal, err)
			}
		}
		if !validRecordedWindowAuthority(prepared.Authority, win, r.profile, ordinal, recorded) {
			return StructureWindowFamilyEvidence{}, fmt.Errorf("structure window %d drifted authority", ordinal)
		}
		if !found && window.RetryableCallRecord(recorded.Record) {
			// The ledger already holds the settlement. Publishing the operation would replay this
			// empty outcome on every later pass and pin the source until someone intervened.
			return StructureWindowFamilyEvidence{}, fmt.Errorf("structure window %d (%s): %w", ordinal, recorded.Record.State, ErrStructureWindowRetryable)
		}
		if !found {
			if err := r.evidence.PutStructureWindowAssessmentEvidence(ctx, recorded); err != nil {
				return StructureWindowFamilyEvidence{}, fmt.Errorf("persist structure window %d: %w", ordinal, err)
			}
		}
		replayed, err := r.evidence.GetStructureWindowAssessmentEvidence(ctx, prepared.Authority, recorded.Record.SHA256)
		if err != nil {
			return StructureWindowFamilyEvidence{}, fmt.Errorf("replay structure window %d evidence: %w", ordinal, err)
		}
		if !reflect.DeepEqual(replayed, recorded) {
			return StructureWindowFamilyEvidence{}, fmt.Errorf("replay structure window %d evidence drifted", ordinal)
		}
		assessments = append(assessments, replayed.Assessment)
		recordedAssessments = append(recordedAssessments, replayed)
	}
	stitched, err := window.Stitch(prepared.Authority, assessments, r.boundaryTolerance)
	if err != nil {
		return StructureWindowFamilyEvidence{}, fmt.Errorf("stitch structure windows: %w", err)
	}
	if err := r.evidence.PutStructureWindowStitch(ctx, stitched); err != nil {
		return StructureWindowFamilyEvidence{}, fmt.Errorf("persist structure window stitch: %w", err)
	}
	evidence, err := NewStructureWindowFamilyEvidence(recordedAssessments, stitched)
	if err != nil {
		return StructureWindowFamilyEvidence{}, fmt.Errorf("close structure window family evidence: %w", err)
	}
	return evidence, nil
}
