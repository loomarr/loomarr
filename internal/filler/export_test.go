package filler

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/filler/pod"
	"github.com/loomarr/loomarr/internal/filler/split"
	"github.com/loomarr/loomarr/internal/media/defects"
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
func Year(y int) pod.EraRange { return pod.EraRange{From: y, To: y} }

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

// SummarizePipelines is the in-memory adapter for tests. Production SQL groups equivalent facts
// before calling Add, but both paths cross Lifecycle for the actual decision.
func SummarizePipelines(rows []pipelinestate.ClipPipeline, at time.Time) pipelinestate.PipelineOverview {
	var out pipelinestate.PipelineOverview
	for _, row := range rows {
		out.AddLifecycle(row.Lifecycle(at), 1)
	}
	return out
}

func preserveSourceMaster(ctx context.Context, clipDir, sourcePath, clipHash string, tags asset.SidecarTags) (asset.Identity, error) {
	return asset.PreserveSourceMasterWithStorage(ctx, clipDir, sourcePath, clipHash, tags, nil)
}

// SidecarFetchedMarkFor is the fetched mark for a source with no acquisition run.
func SidecarFetchedMarkFor(sourceID string) map[string]any {
	return asset.SidecarFetchedMarkForAcquisition(sourceID, "")
}

func structureDecisionSHA256ForInterval(proposal SplitProposal, segment split.Segment) string {
	authority, ok := structureDecisionAuthorityForInterval(proposal, segment)
	if !ok {
		return ""
	}
	return authority.sha256
}

// ground is groundFromSource for the clip's own file rather than a proposal's evidence derivative.
func (s *SplitStage) ground(ctx context.Context, c cliplibrary.StoreClip, segs []split.Segment) groundPass {
	file := ""
	if s.vision != nil {
		file = filepath.Join(s.vision.ClipDir, filepath.FromSlash(c.Path))
	}
	return s.groundAt(ctx, c, file, split.SourceAsset{}, segs)
}

// AiringReencodeFixture is a clip on air under the software profile and a transcode stage on an
// NVENC host that re-encodes it, for the library re-encode's pipeline tests. Each replacement's
// defect check calls check; switches counts the commits.
func AiringReencodeFixture(t *testing.T, check func(ctx context.Context) error) (*TranscodeStage, cliplibrary.StoreClip, func() int) {
	t.Helper()
	_, stage, store, airing := airingFixture(t, func(ctx context.Context, c cliplibrary.StoreClip, staged string) (defects.Record, error) {
		if err := check(ctx); err != nil {
			return defects.Record{}, err
		}
		return passingCheck(nil)(ctx, c, staged)
	})
	return stage, airing, func() int { return store.calls }
}
