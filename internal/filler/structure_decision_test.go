package filler

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/structure"
)

func TestValidateStructureDecisionProjectionRequiresExactConfirmedPlan(t *testing.T) {
	proposal := certifiedStructureProposal(t)
	if err := ValidateStructureDecisionProjection(*proposal.Structure, *proposal.StructureDecision); err != nil {
		t.Fatal(err)
	}

	t.Run("held", func(t *testing.T) {
		request := structureDecisionRequest(*proposal.StructureDecision)
		request.Candidates[1].Segments[0].Role = structure.RolePSA
		artifact, err := structure.NewArtifact(request, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidateStructureDecisionProjection(*proposal.Structure, artifact); err == nil {
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
		if err := ValidateStructureDecisionProjection(*proposal.Structure, artifact); err == nil {
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
		if err := ValidateStructureDecisionProjection(*proposal.Structure, artifact); err == nil {
			t.Fatal("different confirmed span projected")
		}
	})

	t.Run("replay tampered", func(t *testing.T) {
		artifact := *proposal.StructureDecision
		artifact.Decision.Segments = append([]structure.DecisionSegment(nil), artifact.Decision.Segments...)
		artifact.Decision.Segments[0].Role = structure.RolePSA
		artifact.SHA256 = structure.ArtifactSHA256(artifact)
		if err := ValidateStructureDecisionProjection(*proposal.Structure, artifact); err == nil {
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
