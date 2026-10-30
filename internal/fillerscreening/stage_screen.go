package fillerscreening

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/loomarr/loomarr/internal/filler"
)

// SegmentScreeningStage is the rendered-child safety boundary between structure materialization
// and metadata enrichment. It deliberately uses the concrete coordinator and certification
// modules: those deep modules own retry settlement and release replay, while this adapter only
// resolves portable media paths and translates their closed outcomes into pipeline verdicts.
type SegmentScreeningStage struct {
	runtime       *SegmentScreeningRuntime
	certification *SegmentScreeningCertification
	clipDir       string
}

func NewSegmentScreeningStage(runtime *SegmentScreeningRuntime, certification *SegmentScreeningCertification, clipDir string) *SegmentScreeningStage {
	return &SegmentScreeningStage{runtime: runtime, certification: certification, clipDir: clipDir}
}

func (s *SegmentScreeningStage) ID() filler.StageID     { return filler.StageScreen }
func (s *SegmentScreeningStage) Cost() filler.StageCost { return filler.CostVision }

func (s *SegmentScreeningStage) Applies(_ context.Context, clip filler.StoreClip) (bool, string) {
	if !clip.IsSegment() {
		return false, "not a materialized compilation child"
	}
	return true, ""
}

func (s *SegmentScreeningStage) Run(ctx context.Context, clip filler.StoreClip) (filler.StageResult, error) {
	if err := ctx.Err(); err != nil {
		return filler.StageResult{}, err
	}
	if s == nil || s.runtime == nil {
		return segmentScreeningReview(clip, "complete rendered-child screening is not configured"), nil
	}
	if !clip.IsSegment() || !filler.IsContentHash(clip.Hash) || clip.Path == "" ||
		s.clipDir == "" || !filepath.IsAbs(s.clipDir) || filepath.Clean(s.clipDir) != s.clipDir {
		return segmentScreeningReview(clip, "rendered-child identity is incomplete"), nil
	}

	playbackPath := filepath.Join(s.clipDir, filepath.FromSlash(clip.Path))
	tags, state := filler.ReadSidecarTagsState(playbackPath)
	if state != filler.SidecarValid {
		return segmentScreeningReview(clip, "rendered-child sidecar is missing or invalid"), nil
	}
	subject, err := filler.NewSegmentScreeningSubject(clip.Hash, tags)
	if err != nil || subject.Lineage == nil || subject.Lineage.ParentHash != clip.ParentHash {
		return segmentScreeningReview(clip, "rendered-child media lineage is incomplete or drifted"), nil
	}
	media := SegmentScreeningMedia{
		Subject:          subject,
		Manifest:         *tags.MediaAssets,
		SourceMasterPath: filepath.Join(s.clipDir, filepath.FromSlash(tags.MediaAssets.SourceMaster.Path)),
		EvidencePath:     filepath.Join(s.clipDir, filepath.FromSlash(tags.MediaAssets.Evidence.Asset.Path)),
		PlaybackPath:     playbackPath,
	}
	aggregate, err := s.runtime.Screen(ctx, media)
	if err != nil {
		if ctx.Err() != nil {
			return filler.StageResult{}, ctx.Err()
		}
		return segmentScreeningReview(clip, "a rendered-child screening authority is unavailable"), nil
	}
	reference, err := filler.NewSegmentScreeningReference(subject, aggregate)
	if err != nil {
		return segmentScreeningReview(clip, "rendered-child screening evidence is invalid"), nil
	}
	tags.SegmentScreening = &reference
	if err := filler.WriteSidecarTags(playbackPath, tags, false); err != nil {
		return segmentScreeningReview(clip, "rendered-child screening evidence could not be attached"), nil
	}
	filler.ReportProgress(ctx, filler.StageScreen, 100)

	if axes := segmentScreeningAxesWithOutcome(aggregate, filler.ScreenReject); len(axes) != 0 {
		return filler.StageResult{
			Clip: clip, Verdict: filler.VerdictReject, Reason: filler.ReasonScreening,
			Detail: "rendered child rejected by " + strings.Join(axes, ", "),
		}, nil
	}
	if axes := segmentScreeningAxesWithOutcome(aggregate, filler.ScreenHold); len(axes) != 0 {
		return segmentScreeningReview(clip, "rendered-child screening requires review: "+strings.Join(axes, ", ")), nil
	}
	if !aggregate.Passes() {
		return segmentScreeningReview(clip, "rendered-child screening did not produce five passes"), nil
	}
	if s.certification == nil {
		return segmentScreeningReview(clip, "rendered-child screening passed but production release is not authorized"), nil
	}
	if err := s.certification.Verify(ctx, aggregate); err != nil {
		if ctx.Err() != nil {
			return filler.StageResult{}, ctx.Err()
		}
		return segmentScreeningReview(clip, "rendered-child screening passed but terminal release replay failed"), nil
	}
	return filler.StageResult{Clip: clip, Verdict: filler.VerdictContinue}, nil
}

func segmentScreeningReview(clip filler.StoreClip, note string) filler.StageResult {
	return filler.StageResult{Clip: clip, Verdict: filler.VerdictReview, Note: note}
}

func segmentScreeningAxesWithOutcome(evidence filler.SegmentScreeningEvidence, outcome filler.SegmentScreeningOutcome) []string {
	axes := make([]string, 0, len(evidence.Results))
	for _, result := range evidence.Results {
		if result.Outcome == outcome {
			axes = append(axes, string(result.Axis))
		}
	}
	return axes
}

var _ filler.Stage = (*SegmentScreeningStage)(nil)
