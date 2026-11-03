package filler

import (
	"context"

	"github.com/loomarr/loomarr/internal/fillerstructure"
)

type StructureAssessmentSource struct {
	Source   SplitSourceAsset
	FullPath string
}

// CompleteTimelineStructureDecisioner is the split stage's deep assessment interface. The
// implementation owns independent execution, evidence persistence, and deterministic reduction.
type CompleteTimelineStructureDecisioner interface {
	Assess(context.Context, StructureAssessmentSource) (fillerstructure.Artifact, error)
}
