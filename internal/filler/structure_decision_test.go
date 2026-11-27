package filler

import (
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/split"
	"github.com/loomarr/loomarr/internal/filler/structure"
)

// passingStructureDecision is a confirmed two-assessor decision over the assessment's plan.
func passingStructureDecision(t *testing.T, assessment split.Assessment) *structure.Artifact {
	t.Helper()
	unit := structure.UnitCompilation
	if assessment.Kind == split.KindProgrammeSpots {
		unit = structure.UnitProgrammeSpots
	}
	source := structure.Source{SHA256: assessment.Source.SHA256, Bytes: assessment.Source.Bytes, DurationMS: assessment.DurationMs}
	media := structure.AssessmentMedia{SHA256: strings.Repeat("9", 64), Bytes: assessment.Source.Bytes, DurationMS: assessment.DurationMs, ProfileSHA256: strings.Repeat("8", 64), LineageSHA256: strings.Repeat("7", 64)}
	input, err := structure.NewCompleteVideoInput(source, media)
	if err != nil {
		t.Fatal(err)
	}
	segments := make([]structure.Segment, 0, len(assessment.Plan))
	for _, planned := range assessment.Plan {
		segments = append(segments, structure.Segment{
			StartMS: planned.StartMs, EndMS: planned.EndMs, Role: structure.Role(planned.Role),
		})
	}
	candidate := func(id, family, digest string) structure.Candidate {
		return structure.Candidate{
			Source: source, InputSHA256: input.SHA256,
			Assessor: structure.Assessor{
				ID: id, ModelFamily: family, Provider: "fixture-provider", Model: "fixture-model",
				ModelDigest: strings.Repeat("b", 64), CapabilitySHA256: strings.Repeat("c", 64),
				PromptVersion: "prompt-v1", EvidenceContract: "assessment-v1",
				AssessmentSHA256: strings.Repeat(digest, 64),
			},
			Unit: unit, Segments: segments,
		}
	}
	artifact, err := structure.NewArtifact(structure.Request{
		Source: source, Input: input, BoundaryToleranceMS: 2_000,
		Candidates: []structure.Candidate{
			candidate("assessor-a", "family-a", "1"), candidate("assessor-b", "family-b", "2"),
		},
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return &artifact
}

// decidedStructureProposal is a two-segment proposal whose structure is the exact projection of a
// confirmed decision.
func decidedStructureProposal(t *testing.T) SplitProposal {
	t.Helper()
	observations := []split.Observation{
		structureObservation("black", split.ObservationBlackInterval, split.ObservationProposesBoundary, 29_900, 30_100),
		structureObservation("silence", split.ObservationSilenceInterval, split.ObservationProposesBoundary, 29_900, 30_100),
		structureRoleObservation(t, 60_000, 0, 30_000, split.RoleCommercial, "role-left"),
		structureRoleObservation(t, 60_000, 30_000, 60_000, split.RolePromo, "role-right"),
	}
	assessment := assessStructure(t, 60_000, observations, []split.RoleClaim{
		structureClaim(0, 30_000, split.RoleCommercial, "role-left"),
		structureClaim(30_000, 60_000, split.RolePromo, "role-right"),
	})
	segments := []split.Segment{
		{StartMs: 0, EndMs: 30_000, Category: "toys", BoundaryConfidence: 90},
		{StartMs: 30_000, EndMs: 60_000, Category: "television", BoundaryConfidence: 90},
	}
	artifact := passingStructureDecision(t, assessment)
	projected, err := split.ProjectConfirmedDecision(assessment.Source, &assessment, *artifact)
	if err != nil {
		t.Fatal(err)
	}
	return SplitProposal{Source: assessment.Source, Structure: &projected, StructureDecision: artifact, Segments: segments}
}

func TestReassessProposalStructureReplaysPairedProjection(t *testing.T) {
	proposal := decidedStructureProposal(t)
	assessment, err := reassessProposalStructure(proposal, proposal.Structure.AssessedAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := split.ValidateDecisionProjection(assessment, *proposal.StructureDecision); err != nil {
		t.Fatalf("reassessed paired projection did not replay: %v", err)
	}
}

func TestValidateStructureDecisionProjectionRequiresExactConfirmedPlan(t *testing.T) {
	proposal := decidedStructureProposal(t)
	if err := split.ValidateDecisionProjection(*proposal.Structure, *proposal.StructureDecision); err != nil {
		t.Fatal(err)
	}

	t.Run("held", func(t *testing.T) {
		request := structureDecisionRequest(*proposal.StructureDecision)
		request.Candidates[1].Segments[0].Role = structure.RolePSA
		artifact, err := structure.NewArtifact(request, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err := split.ValidateDecisionProjection(*proposal.Structure, artifact); err == nil {
			t.Fatal("held decision projected")
		}
	})

	t.Run("different role", func(t *testing.T) {
		request := structureDecisionRequest(*proposal.StructureDecision)
		for index := range request.Candidates {
			request.Candidates[index].Segments[0].Role = structure.RolePSA
		}
		artifact, err := structure.NewArtifact(request, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err := split.ValidateDecisionProjection(*proposal.Structure, artifact); err == nil {
			t.Fatal("different confirmed role projected")
		}
	})

	t.Run("different span", func(t *testing.T) {
		request := structureDecisionRequest(*proposal.StructureDecision)
		for index := range request.Candidates {
			request.Candidates[index].Segments[0].EndMS = 31_000
			request.Candidates[index].Segments[1].StartMS = 31_000
		}
		artifact, err := structure.NewArtifact(request, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err := split.ValidateDecisionProjection(*proposal.Structure, artifact); err == nil {
			t.Fatal("different confirmed span projected")
		}
	})

	t.Run("replay tampered", func(t *testing.T) {
		artifact := *proposal.StructureDecision
		artifact.Decision.Segments = append([]structure.DecisionSegment(nil), artifact.Decision.Segments...)
		artifact.Decision.Segments[0].Role = structure.RolePSA
		artifact.SHA256 = structure.ArtifactSHA256(artifact)
		if err := split.ValidateDecisionProjection(*proposal.Structure, artifact); err == nil {
			t.Fatal("rehashed non-replayable decision projected")
		}
	})
}

func structureDecisionRequest(artifact structure.Artifact) structure.Request {
	candidates := make([]structure.Candidate, len(artifact.Decision.Candidates))
	for index, candidate := range artifact.Decision.Candidates {
		candidate.Segments = append([]structure.Segment(nil), candidate.Segments...)
		candidates[index] = candidate
	}
	return structure.Request{
		Source: artifact.Decision.Source, Input: artifact.Decision.Input, BoundaryToleranceMS: artifact.BoundaryToleranceMS,
		Candidates: candidates,
	}
}
