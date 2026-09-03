package filler

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"time"

	"github.com/loomarr/loomarr/internal/fillerstructure"
	"github.com/loomarr/loomarr/internal/fillerstructurewindow"
)

// CompleteWindowStructureAssessor sees one exact window and no peer answers. Ordinary provider
// failures return a recorded operational-failure assessment; error means no trustworthy evidence.
type CompleteWindowStructureAssessor interface {
	Profile() fillerstructure.AssessorProfile
	AssessWindow(context.Context, fillerstructurewindow.MediaSet, StructureAssessmentWindowMedia) (fillerstructurewindow.RecordedAssessment, error)
}

// StructureWindowEvidenceRepository commits and reloads each complete call before the next call,
// each family stitch before the next family, and the final decision before return.
type StructureWindowEvidenceRepository interface {
	PutStructureWindowAssessmentEvidence(context.Context, fillerstructurewindow.RecordedAssessment) error
	GetStructureWindowAssessmentEvidence(context.Context, fillerstructurewindow.MediaSet, string) (fillerstructurewindow.RecordedAssessment, error)
	PutStructureWindowStitch(context.Context, fillerstructurewindow.StitchResult) error
	PutStructureDecisionArtifact(context.Context, fillerstructure.Artifact) error
}

// StructureWindowAssessmentRuntime owns complete preparation, family-major serial execution,
// deterministic stitching, and the same provider-neutral reduction used by short sources.
type StructureWindowAssessmentRuntime struct {
	assessors         []CompleteWindowStructureAssessor
	profiles          []fillerstructure.AssessorProfile
	preparer          StructureAssessmentWindowMediaPreparer
	evidence          StructureWindowEvidenceRepository
	boundaryTolerance int64
	now               func() time.Time
}

func NewStructureWindowAssessmentRuntime(assessors []CompleteWindowStructureAssessor, preparer StructureAssessmentWindowMediaPreparer, evidence StructureWindowEvidenceRepository, boundaryTolerance int64, now func() time.Time) (*StructureWindowAssessmentRuntime, error) {
	if len(assessors) < 2 || preparer == nil || evidence == nil || boundaryTolerance < 0 ||
		boundaryTolerance >= fillerstructurewindow.CanonicalProfile().ContextOverlapMS || now == nil {
		return nil, errors.New("structure window runtime requires two assessors, media preparer, evidence repository, tolerance, and clock")
	}
	profiles := make([]fillerstructure.AssessorProfile, 0, len(assessors))
	for _, assessor := range assessors {
		if assessor == nil {
			return nil, errors.New("structure window runtime contains a nil assessor")
		}
		profiles = append(profiles, assessor.Profile())
	}
	if err := fillerstructure.ValidateAssessorProfiles(profiles); err != nil {
		return nil, fmt.Errorf("structure window runtime profiles: %w", err)
	}
	return &StructureWindowAssessmentRuntime{
		assessors: append([]CompleteWindowStructureAssessor(nil), assessors...),
		profiles:  slices.Clone(profiles),
		preparer:  preparer, evidence: evidence, boundaryTolerance: boundaryTolerance, now: now,
	}, nil
}

func (r *StructureWindowAssessmentRuntime) Assess(ctx context.Context, input StructureAssessmentSource) (fillerstructure.Artifact, error) {
	if r == nil || len(r.assessors) < 2 || len(r.profiles) != len(r.assessors) || r.preparer == nil || r.evidence == nil || r.now == nil {
		return fillerstructure.Artifact{}, errors.New("structure window runtime is unavailable")
	}
	if err := input.Source.validate(); err != nil || !filepath.IsAbs(input.FullPath) || filepath.Clean(input.FullPath) != input.FullPath {
		return fillerstructure.Artifact{}, errors.New("structure window runtime source is invalid")
	}
	source := fillerstructure.Source{SHA256: input.Source.SHA256, Bytes: input.Source.Bytes, DurationMS: input.Source.DurationMs}
	plan, err := fillerstructurewindow.NewPlan(source)
	if err != nil {
		return fillerstructure.Artifact{}, fmt.Errorf("plan structure windows: %w", err)
	}
	prepared, err := r.preparer.PrepareWindows(ctx, input, plan)
	if err != nil {
		return fillerstructure.Artifact{}, fmt.Errorf("prepare structure windows: %w", err)
	}
	if prepared.Source != input.Source || prepared.Authority.Plan.SHA256 != plan.SHA256 ||
		!reflect.DeepEqual(prepared.Authority.Plan, plan) || validatePreparedStructureAssessmentWindows(prepared) != nil ||
		structureWindowSetReusesSourcePath(prepared, input.FullPath) {
		return fillerstructure.Artifact{}, errors.New("structure window preparer drifted source, plan, or media authority")
	}
	requestInput, err := fillerstructurewindow.ReducerInput(prepared.Authority)
	if err != nil {
		return fillerstructure.Artifact{}, err
	}
	request := fillerstructure.Request{Source: source, Input: requestInput, BoundaryToleranceMS: r.boundaryTolerance}
	for family, assessor := range r.assessors {
		assessments := make([]fillerstructurewindow.Assessment, 0, len(prepared.Windows))
		for ordinal, window := range prepared.Windows {
			if err := ctx.Err(); err != nil {
				return fillerstructure.Artifact{}, err
			}
			recorded, err := assessor.AssessWindow(ctx, prepared.Authority, window)
			if err != nil {
				return fillerstructure.Artifact{}, fmt.Errorf("structure window assessor %d window %d produced no authority: %w", family, ordinal, err)
			}
			if err := fillerstructurewindow.ValidateRecordedAssessment(recorded); err != nil ||
				recorded.Assessment.WindowOrdinal != ordinal || recorded.Assessment.Media != window.Media.Media ||
				!reflect.DeepEqual(recorded.Assessment.Assessor, r.profiles[family]) ||
				!reflect.DeepEqual(recorded.Record.MediaSet, prepared.Authority) {
				return fillerstructure.Artifact{}, fmt.Errorf("structure window assessor %d window %d drifted authority", family, ordinal)
			}
			if err := r.evidence.PutStructureWindowAssessmentEvidence(ctx, recorded); err != nil {
				return fillerstructure.Artifact{}, fmt.Errorf("persist structure window assessor %d window %d: %w", family, ordinal, err)
			}
			replayed, err := r.evidence.GetStructureWindowAssessmentEvidence(ctx, prepared.Authority, recorded.Record.SHA256)
			if err != nil {
				return fillerstructure.Artifact{}, fmt.Errorf("replay structure window assessor %d window %d evidence: %w", family, ordinal, err)
			}
			if !reflect.DeepEqual(replayed, recorded) {
				return fillerstructure.Artifact{}, fmt.Errorf("replay structure window assessor %d window %d evidence drifted", family, ordinal)
			}
			assessments = append(assessments, replayed.Assessment)
		}
		stitched, err := fillerstructurewindow.Stitch(prepared.Authority, assessments, r.boundaryTolerance)
		if err != nil {
			return fillerstructure.Artifact{}, fmt.Errorf("stitch structure window assessor %d: %w", family, err)
		}
		if err := r.evidence.PutStructureWindowStitch(ctx, stitched); err != nil {
			return fillerstructure.Artifact{}, fmt.Errorf("persist structure window assessor %d stitch: %w", family, err)
		}
		candidateInput, candidate, err := fillerstructurewindow.ReducerCandidate(stitched)
		if err != nil {
			return fillerstructure.Artifact{}, fmt.Errorf("project structure window assessor %d candidate: %w", family, err)
		}
		if !reflect.DeepEqual(candidateInput, requestInput) {
			return fillerstructure.Artifact{}, fmt.Errorf("structure window assessor %d candidate drifted common input", family)
		}
		request.Candidates = append(request.Candidates, candidate)
	}
	artifact, err := fillerstructure.NewArtifact(request, r.now())
	if err != nil {
		return fillerstructure.Artifact{}, err
	}
	if err := r.evidence.PutStructureDecisionArtifact(ctx, artifact); err != nil {
		return fillerstructure.Artifact{}, fmt.Errorf("persist structure window decision: %w", err)
	}
	return artifact, nil
}

func structureWindowSetReusesSourcePath(prepared StructureAssessmentWindowMediaSet, sourcePath string) bool {
	for _, window := range prepared.Windows {
		if window.FullPath == sourcePath {
			return true
		}
	}
	return false
}

var _ CompleteTimelineStructureDecisioner = (*StructureWindowAssessmentRuntime)(nil)
