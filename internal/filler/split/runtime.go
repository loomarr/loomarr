package split

import (
	"context"

	"github.com/loomarr/loomarr/internal/filler/structure"
)

// AssessmentSource is the exact retained source a decisioner assesses and the file it reads it
// from.
type AssessmentSource struct {
	Source   SourceAsset
	FullPath string
}

// CompleteTimelineDecisioner is the split stage's deep assessment interface. The
// implementation owns independent execution, evidence persistence, and deterministic reduction.
type CompleteTimelineDecisioner interface {
	Assess(context.Context, AssessmentSource) (structure.Artifact, error)
}
