package store

import (
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

func TestUnmarshalSplitProposal_AcceptsLegacyBareSegmentArray(t *testing.T) {
	var p filler.SplitProposal
	err := unmarshalSplitProposal(`[{"index":0,"startMs":0,"endMs":30000,"name":"legacy"}]`, &p)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Ready() || len(p.Segments) != 1 || p.Segments[0].Name != "legacy" {
		t.Fatalf("legacy proposal = %+v, want one ready segment", p)
	}
}

func TestMarshalSplitProposalRejectsRoleEvidenceForAnotherSpan(t *testing.T) {
	source := filler.SplitSourceAsset{
		Role: filler.SplitSourceLegacyPlayback, SHA256: strings.Repeat("a", 64), Bytes: 100,
		ClipHash: strings.Repeat("b", 64), Path: "aa/bb/source.mp4", DurationMs: 60_000,
	}
	evidence, err := filler.NewStructureRoleEvidence(filler.StructureRoleEvidenceInput{
		Source: source, StartMs: 0, EndMs: 30_000, Role: filler.SegmentRoleCommercial, Reason: "product offer",
		Frames: [][]byte{[]byte("frame")}, PromptVersion: "prompt-v1", Prompt: "prompt", Response: `{"role":"commercial"}`,
		RequestedProvider: "ollama", ResolvedProvider: "ollama", RequestedModel: "vision", ResolvedModel: "vision",
		Modalities: []string{"image", "text"}, Attempts: 1, AssessedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal := filler.SplitProposal{
		ID: "proposal", ClipHash: source.ClipHash, Source: source,
		Segments: []filler.SplitSegment{{StartMs: 0, EndMs: 30_001, RoleEvidence: &evidence}},
	}
	if _, err := marshalSplitProposal(proposal); err == nil || !strings.Contains(err.Error(), "does not bind") {
		t.Fatalf("span-binding error = %v", err)
	}
}

func TestMarshalSplitProposalRejectsScreeningForAnotherSpan(t *testing.T) {
	source := filler.SplitSourceAsset{
		Role: filler.SplitSourceLegacyPlayback, SHA256: strings.Repeat("a", 64), Bytes: 100,
		ClipHash: strings.Repeat("b", 64), Path: "aa/bb/source.mp4", DurationMs: 60_000,
	}
	screening, err := filler.NewSegmentScreeningEvidence(source, 0, 30_000, []filler.SegmentScreeningResult{
		{Axis: filler.ScreenVisualSafety, Outcome: filler.ScreenPass, AuthoritySHA256: strings.Repeat("1", 64), ReasonCode: "clear"},
		{Axis: filler.ScreenSpokenSafety, Outcome: filler.ScreenPass, AuthoritySHA256: strings.Repeat("2", 64), ReasonCode: "clear"},
		{Axis: filler.ScreenRights, Outcome: filler.ScreenPass, AuthoritySHA256: strings.Repeat("3", 64), ReasonCode: "verified"},
		{Axis: filler.ScreenPlayback, Outcome: filler.ScreenPass, AuthoritySHA256: strings.Repeat("4", 64), ReasonCode: "verified"},
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	proposal := filler.SplitProposal{
		ID: "proposal", ClipHash: source.ClipHash, Source: source,
		Segments: []filler.SplitSegment{{StartMs: 0, EndMs: 30_001, Screening: &screening}},
	}
	if _, err := marshalSplitProposal(proposal); err == nil || !strings.Contains(err.Error(), "does not bind") {
		t.Fatalf("span-binding error = %v", err)
	}
}
