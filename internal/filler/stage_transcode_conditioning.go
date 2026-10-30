package filler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/loomarr/loomarr/internal/mediatools"
	"github.com/loomarr/loomarr/internal/storagegovernor"
)

// conditioningJob is a conditioned split child's transcode once its lineage, parent and
// before-rewrite measurement have checked out: the snapshots the encode reads, and the
// measurement the staged rewrite is compared against.
type conditioningJob struct {
	// req.Path is the source snapshot the encode reads; req.ParentPath the parent's snapshot.
	req     mediatools.ConditioningRequest
	before  mediatools.ConditioningMeasurement
	lineage *ConditioningLineage
	// parentFull is the parent asset as resolved, compared to its snapshot before publication.
	parentFull string
	// stageDir holds the snapshots and the staged encode; the caller removes it.
	stageDir string
}

// prepareConditioning validates a conditioned child's lineage and parent, snapshots both, and
// measures the source before the rewrite. A nil job with a nil error ends the run with the
// returned result: a review, or the re-emitted outcome of a child that is already conditioned.
func (s *TranscodeStage) prepareConditioning(ctx context.Context, c StoreClip, oldFull, sourceFull string, sourceMaster MediaAssetIdentity, storageWrites *storagegovernor.WriteTracker) (job *conditioningJob, done StageResult, err error) {
	review := func(note string) (*conditioningJob, StageResult, error) {
		return nil, conditioningReview(c, note), nil
	}
	tags, hasTags, err := readConditioningSidecar(oldFull)
	if err != nil {
		return nil, StageResult{}, err
	}
	if !hasTags || !validConditioningLineage(tags.ConditioningLineage, c.ParentHash) ||
		(tags.Mezzanine != s.profile.ID() && tags.ConditioningLineage.ChildHash != c.Hash) {
		return review("conditioning lineage is missing or does not match the child")
	}
	parent, found, err := s.store.GetClip(ctx, c.ParentHash)
	if err != nil {
		return nil, StageResult{}, fmt.Errorf("get conditioning parent %s: %w", c.ParentHash, err)
	}
	if !found || parent.Hash != c.ParentHash || parent.Path == "" || !parent.IsComposite {
		return review("conditioning parent is unavailable")
	}
	parentFull := filepath.Join(s.clipDir, filepath.FromSlash(parent.Path))
	parentHash := parent.Hash
	if tags.ConditioningLineage.ParentAssetRole != "" {
		var parentSource SplitSourceAsset
		switch SplitSourceRole(tags.ConditioningLineage.ParentAssetRole) {
		case SplitSourceEvidence:
			parentTags, ok := ReadSidecarTags(parentFull)
			if !ok || parentTags.MediaAssets == nil || parentTags.MediaAssets.Evidence == nil ||
				parentTags.MediaAssets.Evidence.Asset.SHA256 != tags.ConditioningLineage.ParentAssetSHA256 {
				return review("conditioning parent evidence asset is unavailable")
			}
			evidence := parentTags.MediaAssets.Evidence
			parentSource = SplitSourceAsset{
				Role: SplitSourceEvidence, SHA256: evidence.Asset.SHA256, Bytes: evidence.Asset.Bytes,
				ClipHash: evidence.Asset.ClipHash, Path: evidence.Asset.Path, DurationMs: evidence.DurationMs,
			}
		case SplitSourceLegacyPlayback:
			digest, size, err := FileSHA256(parentFull)
			if err != nil || digest != tags.ConditioningLineage.ParentAssetSHA256 {
				return review("conditioning parent playback asset changed")
			}
			parentSource = SplitSourceAsset{
				Role: SplitSourceLegacyPlayback, SHA256: digest, Bytes: size,
				ClipHash: parent.Hash, Path: parent.Path, DurationMs: parent.DurationMs,
			}
		default:
			return review("conditioning parent asset role is invalid")
		}
		resolvedParent, resolvedPath, err := resolveSplitSource(ctx, s.clipDir, parent, parentSource)
		if err != nil {
			return review("conditioning parent asset changed")
		}
		parentFull = resolvedPath
		parentHash = resolvedParent.ClipHash
	}
	stageRoot := filepath.Join(s.clipDir, transcodeStagingDir)
	if err := os.MkdirAll(stageRoot, 0o755); err != nil {
		return nil, StageResult{}, fmt.Errorf("create transcode staging folder: %w", err)
	}
	stageDir, err := os.MkdirTemp(stageRoot, "conditioning-")
	if err != nil {
		return nil, StageResult{}, fmt.Errorf("create conditioning staging folder: %w", err)
	}
	defer func() {
		if job == nil {
			_ = os.RemoveAll(stageDir)
		}
	}()
	// After a committed re-key c.Hash names playback bytes while the retained master keeps the
	// reviewed stream-copy child's sparse identity. Restart validation must bind each role to
	// its own identity rather than asking the master to masquerade as playback.
	snapshotCtx, finishStorage := storageWrites.Monitor(ctx, stageDir)
	snapshots, err := snapshotConditioningArtifacts(snapshotCtx, stageDir, sourceFull, parentFull, sourceMaster.ClipHash, parentHash)
	if err != nil {
		if storageErr := finishStorage(); storageErr != nil {
			return nil, StageResult{}, storageErr
		}
		return review(err.Error())
	}
	if err := finishStorage(); err != nil {
		return nil, StageResult{}, err
	}
	if err := storageWrites.RecordFile(ctx, snapshots.Source); err != nil {
		return nil, StageResult{}, err
	}
	if err := storageWrites.RecordFile(ctx, snapshots.Parent); err != nil {
		return nil, StageResult{}, err
	}
	req := mediatools.ConditioningRequest{
		Path:       snapshots.Source,
		ParentPath: snapshots.Parent,
		IntendedCuts: []mediatools.Interval{{
			StartMs: tags.ConditioningLineage.IntendedStartMs,
			EndMs:   tags.ConditioningLineage.IntendedEndMs,
		}},
	}
	if tags.Mezzanine == s.profile.ID() {
		if tags.MediaQuality == nil || tags.Conditioning == nil ||
			mediatools.ValidateMediaQualityEvidence(*tags.MediaQuality) != nil ||
			validateConditioningPair(*tags.Conditioning, tags.Conditioning.BeforeRewriteHash, tags.Conditioning.AfterRewriteHash) != nil ||
			!conditioningEvidenceMatchesLineage(*tags.Conditioning, *tags.ConditioningLineage) ||
			!reflect.DeepEqual(*tags.MediaQuality, tags.Conditioning.AfterRewrite.Quality) {
			return review("persisted conditioning evidence is incomplete or invalid")
		}
		if tags.ConditioningPublication != nil {
			publication := tags.ConditioningPublication
			if !validConditioningPublication(publication, publication.SourceHash, c.Hash) {
				return review("persisted conditioning publication is invalid")
			}
			if err := s.store.CommitConditioningPublication(ctx, *publication, c); err != nil {
				done, err := conditioningCommitOutcome(c, err)
				return nil, done, err
			}
			if err := clearConditioningPublication(oldFull, publication); err != nil {
				done, err := conditioningCommitOutcome(c, err)
				return nil, done, err
			}
		}
		return nil, mediaQualityResult(c, *tags.MediaQuality), nil
	}
	if s.condition == nil {
		return review("conditioning measurement is unavailable")
	}
	measured, err := s.condition(ctx, req)
	if err != nil {
		return review("conditioning measurement failed")
	}
	if ctx.Err() != nil {
		return review("conditioning measurement was cancelled")
	}
	if err := mediatools.ValidateConditioningEvidence(measured, mediatools.ConditioningBeforeRewrite); err != nil {
		return review("conditioning measurement is incomplete or invalid")
	}
	intended := mediatools.Interval{StartMs: tags.ConditioningLineage.IntendedStartMs, EndMs: tags.ConditioningLineage.IntendedEndMs}
	if err := mediatools.ValidateConditioningCutEvidence(measured, intended); err != nil {
		return review("conditioning cut evidence is incomplete or unavailable")
	}
	return &conditioningJob{
		req: req, before: measured, lineage: tags.ConditioningLineage,
		parentFull: parentFull, stageDir: stageDir,
	}, StageResult{}, nil
}

// measureAfter measures the staged rewrite and pairs it with the before-rewrite measurement. A
// non-empty note is why the child goes to review instead. AfterRewriteHash is left for the caller,
// which knows it only once the staged bytes are hashed.
func (j *conditioningJob) measureAfter(ctx context.Context, condition func(context.Context, mediatools.ConditioningRequest) (mediatools.ConditioningMeasurement, error), beforeHash, stageFull string) (*ConditioningEvidence, string) {
	req := j.req
	req.Path = stageFull
	measured, err := condition(ctx, req)
	if err != nil {
		return nil, "conditioning measurement failed after transcode"
	}
	if ctx.Err() != nil {
		return nil, "conditioning measurement after transcode was cancelled"
	}
	if err := mediatools.ValidateConditioningEvidence(measured, mediatools.ConditioningAfterRewrite); err != nil {
		return nil, "conditioning measurement after transcode is incomplete or invalid"
	}
	parentEdges, err := mediatools.DeriveConditioningParentEdges(j.before, measured)
	if err != nil {
		return nil, "conditioning parent edges after transcode are unavailable"
	}
	return &ConditioningEvidence{
		BeforeRewriteHash:              beforeHash,
		BeforeRewrite:                  j.before,
		AfterRewrite:                   measured,
		DerivedParentEdgesAfterRewrite: parentEdges,
	}, ""
}
