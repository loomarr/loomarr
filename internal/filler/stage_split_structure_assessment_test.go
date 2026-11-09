package filler_test

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/filler/split"
	"github.com/loomarr/loomarr/internal/filler/structure"
	"github.com/loomarr/loomarr/internal/filler/structure/cut"
	"github.com/loomarr/loomarr/internal/filler/structure/window"
)

type capturedStructureDecisioner struct {
	artifact structure.Artifact
	media    []split.AssessmentSource
	err      error
}

func (d *capturedStructureDecisioner) Assess(_ context.Context, media split.AssessmentSource) (structure.Artifact, error) {
	d.media = append(d.media, media)
	return d.artifact, d.err
}

func TestSplitStagePersistsCompleteTimelineDecisionOnceAndProjectsItsCuts(t *testing.T) {
	st := newSplitMemStore()
	hash := seedCompilation(st, "comps/model-timeline.mp4", 60_000)
	clip := st.clips[hash]
	clip.IsComposite = true
	st.clips[hash] = clip
	tools := &fakeTools{chapters: []filler.Chapter{
		{StartMs: 0, EndMs: 28_000, Title: "detector left"},
		{StartMs: 28_000, EndMs: 60_000, Title: "detector right"},
	}}
	drop := t.TempDir()
	splitter := newSplitter(st, tools, nil, drop)
	proposal, err := splitter.Propose(t.Context(), hash)
	if err != nil {
		t.Fatal(err)
	}
	decisioner := &capturedStructureDecisioner{artifact: structureDecisionArtifact(t, proposal.Source, 30_000, false)}
	stage := filler.NewSplitStage(splitter, st).WithFixedStructureRuntime(filler.StructureRuntime{Decisioner: decisioner})

	result, err := stage.Run(t.Context(), clip)
	if err != nil || result.Verdict != pipelinestate.VerdictReview {
		t.Fatalf("result=%+v error=%v", result, err)
	}
	stored, err := st.GetSplitProposal(t.Context(), proposal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(decisioner.media) != 1 || decisioner.media[0].Source != proposal.Source ||
		decisioner.media[0].FullPath != filepath.Join(drop, filepath.FromSlash(clip.Path)) || stored.StructureDecision == nil ||
		stored.StructureDecision.SHA256 != decisioner.artifact.SHA256 || stored.Structure == nil ||
		stored.Structure.Kind != split.KindCompilationBreak || len(stored.Structure.Plan) != 2 ||
		stored.Structure.Plan[0].EndMs != 30_000 || stored.Structure.Plan[1].StartMs != 30_000 {
		t.Fatalf("media=%+v stored=%+v", decisioner.media, stored)
	}
	if _, err := stage.Run(t.Context(), clip); err != nil {
		t.Fatal(err)
	}
	if len(decisioner.media) != 1 {
		t.Fatalf("persisted decision was reassessed: calls=%d", len(decisioner.media))
	}
}

// The long-reel files apply live (#1659), so the runtime can be swapped mid-split. Here it is
// swapped on EVERY read: a stage that read each part at its point of use would assess under one
// runtime and record the shadow under the other.
func TestSplitStageNeverMixesTwoStructureRuntimesInOneSplit(t *testing.T) {
	st := newSplitMemStore()
	hash := seedCompilation(st, "comps/swapped-runtime.mp4", 60_000)
	clip := st.clips[hash]
	clip.IsComposite = true
	st.clips[hash] = clip
	tools := &fakeTools{chapters: []filler.Chapter{
		{StartMs: 0, EndMs: 28_000}, {StartMs: 28_000, EndMs: 60_000},
	}}
	splitter := newSplitter(st, tools, nil, t.TempDir())
	proposal, err := splitter.Propose(t.Context(), hash)
	if err != nil {
		t.Fatal(err)
	}
	artifact := structureDecisionArtifact(t, proposal.Source, 30_000, false)
	type runtime struct {
		decisioner *capturedStructureDecisioner
		shadow     *splitShadowCapture
	}
	runtimes := [2]runtime{
		{&capturedStructureDecisioner{artifact: artifact}, &splitShadowCapture{}},
		{&capturedStructureDecisioner{artifact: artifact}, &splitShadowCapture{}},
	}
	reads := 0
	stage := filler.NewSplitStage(splitter, st).WithStructureRuntime(func() filler.StructureRuntime {
		rt := runtimes[reads%2]
		reads++
		return filler.StructureRuntime{Decisioner: rt.decisioner, Shadow: rt.shadow}
	})

	if _, err := stage.Run(t.Context(), clip); err != nil {
		t.Fatal(err)
	}
	first, second := runtimes[0], runtimes[1]
	if reads != 1 || len(first.decisioner.media) != 1 || first.shadow.calls != 1 ||
		len(second.decisioner.media) != 0 || second.shadow.calls != 0 {
		t.Fatalf("one split read the runtime %d times; assessed by first=%d second=%d, shadowed by first=%d second=%d",
			reads, len(first.decisioner.media), len(second.decisioner.media), first.shadow.calls, second.shadow.calls)
	}

	// The next split takes the swapped runtime whole. The persisted decision is not reassessed.
	if _, err := stage.Run(t.Context(), clip); err != nil {
		t.Fatal(err)
	}
	if reads != 2 || second.shadow.calls != 1 || first.shadow.calls != 1 || len(second.decisioner.media) != 0 {
		t.Fatalf("second split: reads=%d shadowed by first=%d second=%d, reassessed=%d",
			reads, first.shadow.calls, second.shadow.calls, len(second.decisioner.media))
	}
}

func TestHeldCompleteTimelineDecisionDoesNotReplaceDetectorAssessment(t *testing.T) {
	st := newSplitMemStore()
	hash := seedCompilation(st, "comps/held-timeline.mp4", 60_000)
	tools := &fakeTools{chapters: []filler.Chapter{
		{StartMs: 0, EndMs: 28_000}, {StartMs: 28_000, EndMs: 60_000},
	}}
	splitter := newSplitter(st, tools, nil, t.TempDir())
	proposal, err := splitter.Propose(t.Context(), hash)
	if err != nil {
		t.Fatal(err)
	}
	detectorSHA := proposal.Structure.SHA256
	decisioner := &capturedStructureDecisioner{artifact: structureDecisionArtifact(t, proposal.Source, 30_000, true)}
	updated, err := splitter.AssessProposalStructure(t.Context(), *proposal, decisioner)
	if err != nil {
		t.Fatal(err)
	}
	if updated.StructureDecision == nil || updated.StructureDecision.Decision.Status != structure.StatusHeld ||
		updated.Structure == nil || updated.Structure.SHA256 != detectorSHA || updated.Structure.Plan[0].EndMs != 28_000 {
		t.Fatalf("updated proposal=%+v", updated)
	}
}

// A subtype disagreement confirms structure with no role, which cannot project yet (#1069). The paid
// assessment must still be kept, so the next pass does not buy the same answer again.
func TestRolelessConfirmedDecisionIsKeptWithoutReplacingDetectorAssessment(t *testing.T) {
	st := newSplitMemStore()
	hash := seedCompilation(st, "comps/roleless-timeline.mp4", 60_000)
	tools := &fakeTools{chapters: []filler.Chapter{
		{StartMs: 0, EndMs: 28_000}, {StartMs: 28_000, EndMs: 60_000},
	}}
	splitter := newSplitter(st, tools, nil, t.TempDir())
	proposal, err := splitter.Propose(t.Context(), hash)
	if err != nil {
		t.Fatal(err)
	}
	detectorSHA := proposal.Structure.SHA256
	agreed := structureDecisionArtifact(t, proposal.Source, 30_000, false).Decision
	candidates := slices.Clone(agreed.Candidates)
	candidates[1].Segments = slices.Clone(candidates[1].Segments)
	candidates[1].Segments[0].Role = structure.RolePSA
	artifact, err := structure.NewArtifact(structure.Request{
		Source: agreed.Source, Input: agreed.Input, BoundaryToleranceMS: 2_000, Candidates: candidates,
	}, time.Date(2026, time.October, 28, 7, 0, 0, 0, time.UTC))
	if err != nil || artifact.Decision.Status != structure.StatusConfirmed || artifact.Decision.Segments[0].Role != "" {
		t.Fatalf("roleless artifact = %+v err=%v", artifact.Decision, err)
	}
	decisioner := &capturedStructureDecisioner{artifact: artifact}
	updated, err := splitter.AssessProposalStructure(t.Context(), *proposal, decisioner)
	if err != nil {
		t.Fatal(err)
	}
	if updated.StructureDecision == nil || updated.StructureDecision.SHA256 != artifact.SHA256 ||
		updated.Structure == nil || updated.Structure.SHA256 != detectorSHA {
		t.Fatalf("updated proposal=%+v", updated)
	}
	stored, err := st.GetSplitProposal(t.Context(), proposal.ID)
	if err != nil || stored.StructureDecision == nil || stored.StructureDecision.SHA256 != artifact.SHA256 {
		t.Fatalf("stored proposal decision = %+v err=%v", stored.StructureDecision, err)
	}
}

func TestSplitStageAssessmentFailureHoldsProposalForReview(t *testing.T) {
	st := newSplitMemStore()
	hash := seedCompilation(st, "comps/assessment-unavailable.mp4", 60_000)
	clip := st.clips[hash]
	clip.IsComposite = true
	st.clips[hash] = clip
	tools := &fakeTools{chapters: []filler.Chapter{
		{StartMs: 0, EndMs: 30_000}, {StartMs: 30_000, EndMs: 60_000},
	}}
	drop := t.TempDir()
	splitter := newSplitter(st, tools, nil, drop)
	proposal, err := splitter.Propose(t.Context(), hash)
	if err != nil {
		t.Fatal(err)
	}
	decisioner := &capturedStructureDecisioner{err: errors.New("provider unavailable at /private/provider-response.json")}
	stage := filler.NewSplitStage(splitter, st).WithFixedStructureRuntime(filler.StructureRuntime{Decisioner: decisioner})

	result, err := stage.Run(t.Context(), clip)
	if err != nil || result.Verdict != pipelinestate.VerdictReview {
		t.Fatalf("result=%+v error=%v", result, err)
	}
	if strings.Contains(result.Note, "provider") || strings.Contains(result.Note, "/private/") {
		t.Fatalf("review note exposes assessment error: %q", result.Note)
	}
	stored, err := st.GetSplitProposal(t.Context(), proposal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Source != proposal.Source || len(stored.Segments) != len(proposal.Segments) || stored.StructureDecision != nil {
		t.Fatalf("assessment failure changed pending proposal: %+v", stored)
	}
	if len(st.clips) != 1 {
		t.Fatalf("assessment failure created split children: clips=%d", len(st.clips))
	}
}

func TestSplitStageAssessmentCancellationRemainsResumable(t *testing.T) {
	st := newSplitMemStore()
	hash := seedCompilation(st, "comps/assessment-cancelled.mp4", 60_000)
	clip := st.clips[hash]
	clip.IsComposite = true
	st.clips[hash] = clip
	tools := &fakeTools{chapters: []filler.Chapter{
		{StartMs: 0, EndMs: 30_000}, {StartMs: 30_000, EndMs: 60_000},
	}}
	splitter := newSplitter(st, tools, nil, t.TempDir())
	if _, err := splitter.Propose(t.Context(), hash); err != nil {
		t.Fatal(err)
	}
	decisioner := &capturedStructureDecisioner{err: context.Canceled}
	stage := filler.NewSplitStage(splitter, st).WithFixedStructureRuntime(filler.StructureRuntime{Decisioner: decisioner})

	result, err := stage.Run(t.Context(), clip)
	if !errors.Is(err, context.Canceled) || result.Verdict == pipelinestate.VerdictReview {
		t.Fatalf("result=%+v error=%v", result, err)
	}
}

func TestSplitStageProjectsConfirmedWindowSetDecisionIntoCompilationCuts(t *testing.T) {
	st := newSplitMemStore()
	hash := seedCompilation(st, "comps/window-timeline.mp4", 300_000)
	clip := st.clips[hash]
	clip.IsComposite = true
	st.clips[hash] = clip
	tools := &fakeTools{chapters: []filler.Chapter{
		{StartMs: 0, EndMs: 145_000, Title: "detector left"},
		{StartMs: 145_000, EndMs: 300_000, Title: "detector right"},
	}}
	drop := t.TempDir()
	splitter := newSplitter(st, tools, nil, drop)
	proposal, err := splitter.Propose(t.Context(), hash)
	if err != nil {
		t.Fatal(err)
	}
	artifact := structureWindowDecisionArtifact(t, proposal.Source, 150_000)
	stage := filler.NewSplitStage(splitter, st).WithFixedStructureRuntime(filler.StructureRuntime{
		Decisioner: &capturedStructureDecisioner{artifact: artifact},
	})
	result, err := stage.Run(t.Context(), clip)
	if err != nil || result.Verdict != pipelinestate.VerdictReview {
		t.Fatalf("result=%+v error=%v", result, err)
	}
	stored, err := st.GetSplitProposal(t.Context(), proposal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.StructureDecision == nil || stored.StructureDecision.Decision.Input.Kind != structure.AssessmentInputWindowMediaSet ||
		stored.Structure == nil || stored.Structure.Kind != split.KindCompilationBreak || len(stored.Structure.Plan) != 2 ||
		stored.Structure.Plan[0].StartMs != 0 || stored.Structure.Plan[0].EndMs != 150_000 ||
		stored.Structure.Plan[1].StartMs != 150_000 || stored.Structure.Plan[1].EndMs != 300_000 {
		t.Fatalf("stored proposal=%+v", stored)
	}
}

func structureDecisionArtifact(t *testing.T, source split.SourceAsset, joinMS int64, disagree bool) structure.Artifact {
	t.Helper()
	core := structure.Source{SHA256: source.SHA256, Bytes: source.Bytes, DurationMS: source.DurationMs}
	media := structure.AssessmentMedia{
		SHA256: strings.Repeat("9", 64), Bytes: source.Bytes, DurationMS: source.DurationMs,
		ProfileSHA256: strings.Repeat("8", 64), LineageSHA256: strings.Repeat("7", 64),
	}
	input, err := structure.NewCompleteVideoInput(core, media)
	if err != nil {
		t.Fatal(err)
	}
	candidate := func(id, family, digest string, segments []structure.Segment) structure.Candidate {
		return structure.Candidate{
			Source: core, InputSHA256: input.SHA256,
			Assessor: structure.Assessor{
				ID: id, ModelFamily: family, Provider: "captured", Model: "model-" + id,
				ModelDigest: strings.Repeat("a", 64), CapabilitySHA256: strings.Repeat("b", 64),
				PromptVersion: "prompt-v1", EvidenceContract: "assessment-v1",
				AssessmentSHA256: strings.Repeat(digest, 64),
			},
			Unit: structure.UnitCompilation, Segments: segments,
		}
	}
	segments := []structure.Segment{
		{StartMS: 0, EndMS: joinMS, Role: structure.RoleCommercial},
		{StartMS: joinMS, EndMS: source.DurationMs, Role: structure.RolePromo},
	}
	left := candidate("assessor-a", "family-a", "1", segments)
	right := candidate("assessor-b", "family-b", "2", segments)
	if disagree {
		right.Unit = structure.UnitStandalone
		right.Role = structure.RoleCommercial
		right.Segments = []structure.Segment{{StartMS: 0, EndMS: source.DurationMs, Role: structure.RoleCommercial}}
	}
	artifact, err := structure.NewArtifact(structure.Request{
		Source: core, Input: input, BoundaryToleranceMS: 2_000, Candidates: []structure.Candidate{left, right},
	}, time.Date(2026, time.September, 10, 7, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return artifact
}

func structureWindowDecisionArtifact(t *testing.T, source split.SourceAsset, joinMS int64) structure.Artifact {
	t.Helper()
	core := structure.Source{SHA256: source.SHA256, Bytes: source.Bytes, DurationMS: source.DurationMs}
	plan, err := window.NewPlan(core, cut.SoftwareProfile())
	if err != nil {
		t.Fatal(err)
	}
	media := make([]structure.AssessmentMedia, len(plan.Windows))
	for ordinal, window := range plan.Windows {
		media[ordinal] = structure.AssessmentMedia{
			SHA256: strings.Repeat(string(rune('1'+ordinal)), 64), Bytes: 1_024,
			DurationMS: window.MediaEndMS - window.MediaStartMS, ProfileSHA256: plan.Profile.AssessmentMediaProfileSHA256,
			LineageSHA256: strings.Repeat(string(rune('5'+ordinal)), 64),
		}
	}
	set, err := window.NewMediaSet(plan, media)
	if err != nil {
		t.Fatal(err)
	}
	input, err := window.ReducerInput(set)
	if err != nil {
		t.Fatal(err)
	}
	segments := []structure.Segment{
		{StartMS: 0, EndMS: joinMS, Role: structure.RoleCommercial},
		{StartMS: joinMS, EndMS: source.DurationMs, Role: structure.RolePromo},
	}
	candidate := func(id, family, digit string) structure.Candidate {
		value, candidateErr := structure.NewCandidate(core, input.SHA256, structure.Assessor{
			ID: id, ModelFamily: family, Provider: "captured", Model: "model-" + id,
			ModelDigest: strings.Repeat("a", 64), CapabilitySHA256: strings.Repeat("b", 64),
			PromptVersion:    window.DirectVideoPromptVersion,
			EvidenceContract: window.CallRecordContractVersion,
			AssessmentSHA256: strings.Repeat(digit, 64),
		}, "", segments)
		if candidateErr != nil {
			t.Fatal(candidateErr)
		}
		return value
	}
	artifact, err := structure.NewArtifact(structure.Request{
		Source: core, Input: input, BoundaryToleranceMS: 2_000,
		Candidates: []structure.Candidate{candidate("assessor-a", "family-a", "8"), candidate("assessor-b", "family-b", "9")},
	}, time.Date(2026, time.September, 12, 7, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return artifact
}
