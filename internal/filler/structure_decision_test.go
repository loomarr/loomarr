package filler

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillerstructure"
)

func TestValidateStructureDecisionProjectionRequiresExactConfirmedPlan(t *testing.T) {
	proposal := certifiedStructureProposal(t)
	if err := ValidateStructureDecisionProjection(*proposal.Structure, *proposal.StructureDecision); err != nil {
		t.Fatal(err)
	}

	t.Run("held", func(t *testing.T) {
		request := structureDecisionRequest(*proposal.StructureDecision)
		request.Candidates[1].Segments[0].Role = fillerstructure.RolePSA
		artifact, err := fillerstructure.NewArtifact(request, time.Now())
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
			request.Candidates[index].Segments[0].Role = fillerstructure.RolePSA
		}
		artifact, err := fillerstructure.NewArtifact(request, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidateStructureDecisionProjection(*proposal.Structure, artifact); err == nil {
			t.Fatal("different confirmed role projected")
		}
	})
}

func structureDecisionRequest(artifact fillerstructure.Artifact) fillerstructure.Request {
	candidates := make([]fillerstructure.Candidate, len(artifact.Decision.Candidates))
	for index, candidate := range artifact.Decision.Candidates {
		candidate.Segments = append([]fillerstructure.Segment(nil), candidate.Segments...)
		candidates[index] = candidate
	}
	return fillerstructure.Request{
		Source: artifact.Decision.Source, Media: artifact.Decision.Media, BoundaryToleranceMS: artifact.BoundaryToleranceMS,
		Candidates: candidates,
	}
}
