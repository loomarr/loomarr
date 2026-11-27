package filler_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/split"
	"github.com/loomarr/loomarr/internal/filler/structure"
	"github.com/loomarr/loomarr/internal/media/tools"
)

// structureDecisionArtifact is a confirmed two-assessor decision that cuts source at joinMS into a
// commercial and a promo.
func structureDecisionArtifact(t *testing.T, source split.SourceAsset, joinMS int64) structure.Artifact {
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
	segments := []structure.Segment{
		{StartMS: 0, EndMS: joinMS, Role: structure.RoleCommercial},
		{StartMS: joinMS, EndMS: source.DurationMs, Role: structure.RolePromo},
	}
	candidate := func(id, family, digest string) structure.Candidate {
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
	artifact, err := structure.NewArtifact(structure.Request{
		Source: core, Input: input, BoundaryToleranceMS: 2_000,
		Candidates: []structure.Candidate{candidate("assessor-a", "family-a", "1"), candidate("assessor-b", "family-b", "2")},
	}, time.Date(2026, time.September, 10, 7, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return artifact
}

func TestConfirmBindsExactStructureDecisionToPublishedChildren(t *testing.T) {
	store := newSplitMemStore()
	oldHash := seedCompilation(store, "comps/decided.mp4", 60_000)
	drop := t.TempDir()
	full := filepath.Join(drop, "comps", "decided.mp4")
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("complete compilation"), 0o644); err != nil {
		t.Fatal(err)
	}
	hash := bindCompilationIdentity(t, store, oldHash, full)
	stageParentForSplitReview(store, hash)
	splitter := newSplitter(store, &fakeTools{chapters: []tools.Chapter{
		{StartMs: 0, EndMs: 30_000}, {StartMs: 30_000, EndMs: 60_000},
	}}, nil, drop)
	proposal, err := splitter.Propose(t.Context(), hash)
	if err != nil {
		t.Fatal(err)
	}
	artifact := structureDecisionArtifact(t, proposal.Source, 30_000)
	assessment, err := split.ProjectConfirmedDecision(proposal.Source, proposal.Structure, artifact)
	if err != nil {
		t.Fatal(err)
	}
	proposal.Structure, proposal.StructureDecision = &assessment, &artifact
	if err := store.UpdateSplitProposal(t.Context(), *proposal); err != nil {
		t.Fatal(err)
	}
	spawned, err := splitter.Confirm(t.Context(), proposal.ID, proposal.Segments)
	if err != nil {
		t.Fatal(err)
	}
	if len(spawned) != 2 {
		t.Fatalf("spawned=%v", spawned)
	}
	wantKinds := []clips.Role{clips.Commercial, clips.Interstitial}
	wantRoles := []split.Role{split.RoleCommercial, split.RolePromo}
	for index, childHash := range spawned {
		child, found, err := store.GetClip(t.Context(), childHash)
		if err != nil || !found {
			t.Fatalf("child %s found=%v error=%v", childHash, found, err)
		}
		if child.Role != wantKinds[index] {
			t.Fatalf("child %s kind=%q want=%q", childHash, child.Role, wantKinds[index])
		}
		tags, ok := asset.ReadSidecarTags(filepath.Join(drop, filepath.FromSlash(child.Path)))
		lineage := tags.ConditioningLineage
		if !ok || lineage == nil || lineage.StructureDecisionSHA256 != artifact.SHA256 ||
			lineage.StructureRole != wantRoles[index] || tags.Kind != string(wantKinds[index]) {
			t.Fatalf("child %s lineage=%+v ok=%v", childHash, tags.ConditioningLineage, ok)
		}
	}
}
