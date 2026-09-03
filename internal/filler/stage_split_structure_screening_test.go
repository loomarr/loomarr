package filler_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

type capturedExactSpanScreener struct {
	requests []filler.StructureScreeningMedia
}

func (s *capturedExactSpanScreener) Screen(_ context.Context, media filler.StructureScreeningMedia) ([]filler.SegmentScreeningEvidence, error) {
	s.requests = append(s.requests, media)
	result := make([]filler.SegmentScreeningEvidence, 0, len(media.Intervals))
	for _, interval := range media.Intervals {
		evidence, err := filler.NewSegmentScreeningEvidence(media.Source, interval.StartMs, interval.EndMs, []filler.SegmentScreeningResult{
			{Axis: filler.ScreenVisualSafety, Outcome: filler.ScreenPass, AuthoritySHA256: strings.Repeat("1", 64), ReasonCode: "policy_clear"},
			{Axis: filler.ScreenSpokenSafety, Outcome: filler.ScreenPass, AuthoritySHA256: strings.Repeat("2", 64), ReasonCode: "policy_clear"},
			{Axis: filler.ScreenRights, Outcome: filler.ScreenPass, AuthoritySHA256: strings.Repeat("3", 64), ReasonCode: "rights_verified"},
			{Axis: filler.ScreenPlayback, Outcome: filler.ScreenPass, AuthoritySHA256: strings.Repeat("4", 64), ReasonCode: "playback_verified"},
		}, time.Date(2026, time.September, 10, 9, 0, 0, 0, time.UTC))
		if err != nil {
			return nil, err
		}
		result = append(result, evidence)
	}
	return result, nil
}

func TestSplitStageScreensDecidedKeepSpansWithoutRewritingDetectorSegments(t *testing.T) {
	store := newSplitMemStore()
	hash := seedCompilation(store, "comps/screen-spans.mp4", 60_000)
	clip := store.clips[hash]
	clip.IsComposite = true
	store.clips[hash] = clip
	splitter := newSplitter(store, &fakeTools{chapters: []filler.Chapter{
		{StartMs: 0, EndMs: 28_000}, {StartMs: 28_000, EndMs: 60_000},
	}}, nil, t.TempDir())
	proposal, err := splitter.Propose(t.Context(), hash)
	if err != nil {
		t.Fatal(err)
	}
	decisioner := &capturedStructureDecisioner{artifact: structureDecisionArtifact(t, proposal.Source, 30_000, false)}
	updated, err := splitter.AssessProposalStructure(t.Context(), *proposal, decisioner)
	if err != nil {
		t.Fatal(err)
	}
	proposal = &updated
	screener := &capturedExactSpanScreener{}
	stage := filler.NewSplitStage(splitter, store).WithExactSpanStructureScreening(screener)
	if _, err := stage.Run(t.Context(), clip); err != nil {
		t.Fatal(err)
	}
	stored, err := store.GetSplitProposal(t.Context(), proposal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(screener.requests) != 1 || len(screener.requests[0].Intervals) != 2 ||
		screener.requests[0].Intervals[0].EndMs != 30_000 || len(stored.StructureScreenings) != 2 ||
		stored.Segments[0].EndMs != 28_000 || stored.Segments[1].StartMs != 28_000 {
		t.Fatalf("requests=%+v stored=%+v", screener.requests, stored)
	}
	if _, err := stage.Run(t.Context(), clip); err != nil {
		t.Fatal(err)
	}
	if len(screener.requests) != 1 {
		t.Fatalf("closed screens were repeated: calls=%d", len(screener.requests))
	}
}
