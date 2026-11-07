package assessment

import (
	"context"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/structure/window"
)

// StructureAssessmentWindowMedia pairs one path-free window authority with its machine-local
// retained location. Paths are transport inputs only and never enter decision identity.
type StructureAssessmentWindowMedia struct {
	Window   window.Window
	Media    window.WindowMedia
	FullPath string
}

// StructureAssessmentWindowMediaSet is the complete prepared input for one long-reel plan.
type StructureAssessmentWindowMediaSet struct {
	Source    filler.SplitSourceAsset
	Authority window.MediaSet
	Windows   []StructureAssessmentWindowMedia
}

// StructureAssessmentWindowMediaPreparer returns either every verified window or no usable set.
type StructureAssessmentWindowMediaPreparer interface {
	PrepareWindows(context.Context, filler.StructureAssessmentSource, window.Plan) (StructureAssessmentWindowMediaSet, error)
}

var _ StructureAssessmentWindowMediaPreparer = (*FFmpegStructureAssessmentMediaPreparer)(nil)
