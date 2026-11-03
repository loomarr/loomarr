package fillerstructureassessment

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/fillerstructure"
	"github.com/loomarr/loomarr/internal/fillerstructurewindow"
)

// Test-only fixtures. They mirror helpers of the same name in internal/filler's tests and
// unexported helpers of internal/filler, which an external package cannot import; production code
// exports nothing for them.

// structureSource is the retained source asset every evidence test hangs a decision on.
func structureSource(durationMs int64) filler.SplitSourceAsset {
	return filler.SplitSourceAsset{
		Role: filler.SplitSourceLegacyPlayback, SHA256: strings.Repeat("a", 64), Bytes: 1024,
		ClipHash: strings.Repeat("b", 64), Path: "aa/bb/source.mp4", DurationMs: durationMs,
	}
}

// pathContains reports whether child is parent itself or lies beneath it (filler's unexported
// helper of the same name).
func pathContains(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil || filepath.IsAbs(rel) {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Assess plans and prepares the windows itself, then runs AssessPrepared. Production's hosted
// adapter prepares first and calls AssessPrepared directly, so nothing in a binary reaches this.
func (r *StructureWindowAssessmentRuntime) Assess(ctx context.Context, input filler.StructureAssessmentSource) (fillerstructure.Artifact, error) {
	if r == nil || len(r.families) < 2 || len(r.profiles) != len(r.families) || r.preparer == nil || r.evidence == nil || r.now == nil {
		return fillerstructure.Artifact{}, errors.New("structure window runtime is unavailable")
	}
	if err := input.Source.Validate(); err != nil || !filepath.IsAbs(input.FullPath) || filepath.Clean(input.FullPath) != input.FullPath {
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
	return r.AssessPrepared(ctx, input, prepared)
}

var _ filler.CompleteTimelineStructureDecisioner = (*StructureWindowAssessmentRuntime)(nil)
