package filler

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillerstructure"
	"github.com/loomarr/loomarr/internal/fillerstructurewindow"
)

// CertifiedShadowFixture exposes the certified two-segment proposal and its policies to the
// external filler_test package, which alone may import the real SQLite store without an import
// cycle.
func CertifiedShadowFixture(t *testing.T) (SplitProposal, *AutoSplitPolicy, *StructureMaterializationPolicy) {
	t.Helper()
	return certifiedStructureProposal(t), certifiedAutoPolicy(), allowCertifiedStructure(t)
}

// The helpers below are test entry points into production code. None of them is reached from a
// binary, so they live here rather than in the package, where deadcode would report them.

// Year is the single-year range — what a channel targeting "1992" means.
func Year(y int) EraRange { return EraRange{From: y, To: y} }

// DefaultBudget is the budget with the historical batch sizes.
func DefaultBudget() Budget {
	return Budget{
		MaxClips:      func() int { return 25 },
		MaxTranscodes: func() int { return 3 },
		MaxWhisper:    func() int { return 10 },
		MaxVision:     func() int { return 5 },
		MaxSplits:     func() int { return 3 },
	}
}

// PlanAcquisition is PlanAcquisitionFor with no coverage gaps.
func PlanAcquisition(intent AcquisitionIntent, candidates []AcquisitionCandidate, existing map[string]ExistingRemoteState) (AcquisitionPlan, error) {
	return PlanAcquisitionFor(intent, candidates, existing, CoverageGaps{})
}

// SummarizePipelines is the in-memory adapter for tests. Production SQL groups equivalent facts
// before calling Add, but both paths cross Lifecycle for the actual decision.
func SummarizePipelines(rows []ClipPipeline, at time.Time) PipelineOverview {
	var out PipelineOverview
	for _, row := range rows {
		out.AddLifecycle(row.Lifecycle(at), 1)
	}
	return out
}

// ScanDir is scanDir with no watch folder excluded. The duration floor is optional because most
// tests only assert that a file is catalogued.
func ScanDir(ctx context.Context, dir string, probe Prober, minDurationMs ...int64) (clips []RawClip, skipped int, err error) {
	if dir == "" {
		return nil, 0, nil
	}
	if probe == nil {
		probe = FFprobe
	}
	var minMs int64
	if len(minDurationMs) > 0 {
		minMs = minDurationMs[0]
	}
	return scanDir(ctx, dir, "", probe, minMs)
}

// GenerateArtwork is generateArtwork with no storage governor, returning the failure count.
func GenerateArtwork(ctx context.Context, dir string, clips []RawClip, render ArtworkRenderer) (failed int) {
	return generateArtwork(ctx, dir, clips, render, nil).Failed
}

// TakeIn is takeInFrom with no source, binding or storage governor.
func TakeIn(watchDir, clipDir string, fetched bool, log func(string, ...any)) (IntakeResult, error) {
	return takeInFrom(context.Background(), watchDir, clipDir, fetched, "", log, nil, nil)
}

func movePath(src, dst string) error {
	return movePathWithStorage(context.Background(), src, dst, nil)
}

func moveIfPresent(src, dst string) error {
	return moveIfPresentWithStorage(context.Background(), src, dst, nil)
}

func preserveSourceMaster(ctx context.Context, clipDir, sourcePath, clipHash string, tags SidecarTags) (MediaAssetIdentity, error) {
	return preserveSourceMasterWithStorage(ctx, clipDir, sourcePath, clipHash, tags, nil)
}

// SidecarFetchedMarkFor is the fetched mark for a source with no acquisition run.
func SidecarFetchedMarkFor(sourceID string) map[string]any {
	return SidecarFetchedMarkForAcquisition(sourceID, "")
}

func structureDecisionSHA256ForInterval(proposal SplitProposal, segment SplitSegment) string {
	authority, ok := structureDecisionAuthorityForInterval(proposal, segment)
	if !ok {
		return ""
	}
	return authority.sha256
}

// ground is groundFromSource for the clip's own file rather than a proposal's evidence derivative.
func (s *SplitStage) ground(ctx context.Context, c StoreClip, segs []SplitSegment) groundPass {
	file := ""
	if s.vision != nil {
		file = filepath.Join(s.vision.ClipDir, filepath.FromSlash(c.Path))
	}
	return s.groundAt(ctx, c, file, SplitSourceAsset{}, segs)
}

// Assess plans and prepares the windows itself, then runs AssessPrepared. Production's hosted
// adapter prepares first and calls AssessPrepared directly.
func (r *StructureWindowAssessmentRuntime) Assess(ctx context.Context, input StructureAssessmentSource) (fillerstructure.Artifact, error) {
	if r == nil || len(r.families) < 2 || len(r.profiles) != len(r.families) || r.preparer == nil || r.evidence == nil || r.now == nil {
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
	return r.AssessPrepared(ctx, input, prepared)
}

var _ CompleteTimelineStructureDecisioner = (*StructureWindowAssessmentRuntime)(nil)
