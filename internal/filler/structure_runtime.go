package filler

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"time"

	"github.com/loomarr/loomarr/internal/fillerstructure"
)

type StructureAssessmentMedia struct {
	Source   SplitSourceAsset
	FullPath string
}

// CompleteTimelineStructureAssessor receives no peer answers. Expected provider failures must be
// returned as source-bound Candidate.Failure evidence; error is reserved for missing authority.
type CompleteTimelineStructureAssessor interface {
	Profile() fillerstructure.AssessorProfile
	AssessCompleteTimeline(context.Context, StructureAssessmentMedia) (fillerstructure.Candidate, error)
}

// StructureAssessmentRuntime owns serial independent execution and shared reduction, not provider
// configuration, reservations, or response persistence; those remain inside each assessor adapter.
type StructureAssessmentRuntime struct {
	assessors         []CompleteTimelineStructureAssessor
	boundaryTolerance int64
	now               func() time.Time
}

func NewStructureAssessmentRuntime(assessors []CompleteTimelineStructureAssessor, boundaryTolerance int64, now func() time.Time) (*StructureAssessmentRuntime, error) {
	if len(assessors) < 2 || boundaryTolerance < 0 || now == nil {
		return nil, fmt.Errorf("structure assessment runtime requires two assessors, tolerance, and clock")
	}
	profiles := make([]fillerstructure.AssessorProfile, 0, len(assessors))
	for _, assessor := range assessors {
		if assessor == nil {
			return nil, fmt.Errorf("structure assessment runtime contains a nil assessor")
		}
		profiles = append(profiles, assessor.Profile())
	}
	if err := fillerstructure.ValidateAssessorProfiles(profiles); err != nil {
		return nil, fmt.Errorf("structure assessment runtime profiles: %w", err)
	}
	return &StructureAssessmentRuntime{
		assessors:         append([]CompleteTimelineStructureAssessor(nil), assessors...),
		boundaryTolerance: boundaryTolerance, now: now,
	}, nil
}

func (r *StructureAssessmentRuntime) Assess(ctx context.Context, media StructureAssessmentMedia) (fillerstructure.Artifact, error) {
	if r == nil || len(r.assessors) < 2 || r.now == nil {
		return fillerstructure.Artifact{}, fmt.Errorf("structure assessment runtime is unavailable")
	}
	if err := media.Source.validate(); err != nil || !filepath.IsAbs(media.FullPath) || filepath.Clean(media.FullPath) != media.FullPath {
		return fillerstructure.Artifact{}, fmt.Errorf("structure assessment runtime media is invalid")
	}
	source := fillerstructure.Source{SHA256: media.Source.SHA256, DurationMS: media.Source.DurationMs}
	request := fillerstructure.Request{Source: source, BoundaryToleranceMS: r.boundaryTolerance}
	for index, assessor := range r.assessors {
		if err := ctx.Err(); err != nil {
			return fillerstructure.Artifact{}, err
		}
		candidate, err := assessor.AssessCompleteTimeline(ctx, media)
		if err != nil {
			return fillerstructure.Artifact{}, fmt.Errorf("complete-timeline assessor %d produced no authority: %w", index, err)
		}
		if candidate.Source != source || !reflect.DeepEqual(fillerstructure.Profile(candidate.Assessor), assessor.Profile()) {
			return fillerstructure.Artifact{}, fmt.Errorf("complete-timeline assessor %d drifted source or profile", index)
		}
		request.Candidates = append(request.Candidates, candidate)
	}
	return fillerstructure.NewArtifact(request, r.now())
}
