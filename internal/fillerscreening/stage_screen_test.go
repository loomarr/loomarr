package fillerscreening

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/loomarr/loomarr/internal/filler"
)

func TestSegmentScreeningStageAppliesOnlyToMaterializedChildren(t *testing.T) {
	stage := NewSegmentScreeningStage(nil, nil, t.TempDir())
	if applies, _ := stage.Applies(t.Context(), filler.StoreClip{}); applies {
		t.Fatal("screening stage applied to a top-level clip")
	}
	if applies, note := stage.Applies(t.Context(), filler.StoreClip{Clip: filler.Clip{ParentHash: "parent"}}); !applies || note != "" {
		t.Fatalf("screening stage did not apply to child: applies=%v note=%q", applies, note)
	}
}

func TestSegmentScreeningStageHoldsChildWhenRuntimeOrReleaseIsUnavailable(t *testing.T) {
	clip, _, root := renderedChildStageFixture(t)
	stage := NewSegmentScreeningStage(nil, nil, root)
	out, err := stage.Run(t.Context(), clip)
	if err != nil || out.Verdict != filler.VerdictReview || out.Note == "" {
		t.Fatalf("missing runtime did not hold child: out=%+v err=%v", out, err)
	}

	runtime, _, _ := screeningStageRuntime(t, filler.ScreenPass)
	stage = NewSegmentScreeningStage(runtime, nil, root)
	out, err = stage.Run(t.Context(), clip)
	if err != nil || out.Verdict != filler.VerdictReview || out.Note != "rendered-child screening passed but production release is not authorized" {
		t.Fatalf("missing release did not hold passing child: out=%+v err=%v", out, err)
	}
	tags, state := filler.ReadSidecarTagsState(filepath.Join(root, filepath.FromSlash(clip.Path)))
	if state != filler.SidecarValid || tags.SegmentScreening == nil {
		t.Fatalf("durable screening reference missing before release hold: state=%v tags=%+v", state, tags)
	}
}

func TestSegmentScreeningStageMapsClosedRejectWithoutPublishingRestrictedDetail(t *testing.T) {
	clip, _, root := renderedChildStageFixture(t)
	runtime, _, _ := screeningStageRuntime(t, filler.ScreenReject)
	stage := NewSegmentScreeningStage(runtime, nil, root)
	out, err := stage.Run(t.Context(), clip)
	if err != nil || out.Verdict != filler.VerdictReject || out.Reason != filler.ReasonScreening ||
		out.Detail != "rendered child rejected by spoken_safety" {
		t.Fatalf("closed reject was not preserved: out=%+v err=%v", out, err)
	}
}

func TestSegmentScreeningStageRequiresTerminalCertificationBeforeContinuing(t *testing.T) {
	clip, _, root := renderedChildStageFixture(t)
	runtime, repository, profiles := screeningStageRuntime(t, filler.ScreenPass)
	certification, err := NewSegmentScreeningCertification(
		screeningReleaseFixture(profiles, true), repository,
	)
	if err != nil {
		t.Fatal(err)
	}
	stage := NewSegmentScreeningStage(runtime, certification, root)
	out, err := stage.Run(t.Context(), clip)
	if err != nil || out.Verdict != filler.VerdictContinue {
		t.Fatalf("certified four-axis child did not continue: out=%+v err=%v", out, err)
	}
}

func TestSegmentScreeningStageTurnsOperationalFailureIntoReview(t *testing.T) {
	clip, _, root := renderedChildStageFixture(t)
	runtime, _, _ := screeningStageRuntime(t, filler.ScreenPass)
	evaluator := runtime.evaluators[1].(*capturedSegmentScreeningEvaluator)
	evaluator.err = errors.New("private provider response")
	stage := NewSegmentScreeningStage(runtime, nil, root)
	out, err := stage.Run(context.Background(), clip)
	if err != nil || out.Verdict != filler.VerdictReview || out.Note != "a rendered-child screening authority is unavailable" {
		t.Fatalf("operational failure escaped fail-closed translation: out=%+v err=%v", out, err)
	}
}

func renderedChildStageFixture(t *testing.T) (filler.StoreClip, filler.SidecarTags, string) {
	t.Helper()
	root := t.TempDir()
	tags := screeningChildTagsFixture(t)
	path := filepath.Join(root, filepath.FromSlash(tags.MediaAssets.Playback.Asset.Path))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("rendered child"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := filler.WriteSidecarTags(path, tags, false); err != nil {
		t.Fatal(err)
	}
	clip := filler.StoreClip{Clip: filler.Clip{
		Hash: tags.MediaAssets.Playback.Asset.ClipHash, Path: tags.MediaAssets.Playback.Asset.Path,
		ParentHash: tags.ConditioningLineage.ParentHash,
	}}
	return clip, tags, root
}

func screeningStageRuntime(t *testing.T, spokenOutcome filler.SegmentScreeningOutcome) (*SegmentScreeningRuntime, *FileSegmentScreeningEvidenceRepository, []SegmentScreeningAxisProfile) {
	t.Helper()
	order := []string{}
	evaluators := segmentScreeningEvaluatorFixtures(&order)
	evaluators[filler.ScreenSpokenSafety].outcome = spokenOutcome
	repository, err := NewFileSegmentScreeningEvidenceRepository(filepath.Join(t.TempDir(), "evidence"))
	if err != nil {
		t.Fatal(err)
	}
	runtime := mustSegmentScreeningRuntime(t, evaluators, repository)
	profiles := make([]SegmentScreeningAxisProfile, 0, len(segmentScreeningAxisOrder))
	for _, axis := range segmentScreeningAxisOrder {
		profiles = append(profiles, evaluators[axis].profile)
	}
	return runtime, repository, profiles
}
