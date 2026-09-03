package filler

import (
	"strings"
	"testing"
	"time"
)

func structureRoleEvidenceFixture(t *testing.T) StructureRoleEvidence {
	t.Helper()
	evidence, err := NewStructureRoleEvidence(StructureRoleEvidenceInput{
		Source: structureSource(60_000), StartMs: 0, EndMs: 30_000,
		Role: SegmentRoleCommercial, Reason: "a separately framed product offer",
		Frames:        [][]byte{[]byte("opening frame"), []byte("closing card")},
		PromptVersion: "segment-role-v1", Prompt: "classify this exact segment", Response: `{"role":"commercial"}`,
		RequestedProvider: "ollama", ResolvedProvider: "ollama", RequestedModel: "vision:latest", ResolvedModel: "vision@sha256:abc",
		Modalities: []string{"text", "image", "image"}, Tokens: StructureRoleTokenUsage{Prompt: 20, Completion: 5, Image: 2},
		LatencyMs: 125, Attempts: 1, AssessedAt: time.Date(2026, time.September, 4, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return evidence
}

func TestNewStructureRoleEvidenceBindsExactRequestAndResponse(t *testing.T) {
	evidence := structureRoleEvidenceFixture(t)
	if len(evidence.FrameSHA256) != 2 || evidence.FrameSHA256[0] == evidence.FrameSHA256[1] || !strings.Contains(evidence.RequestedModel, "latest") || evidence.RequestSHA256 == evidence.ResponseSHA256 || evidence.SHA256 == "" {
		t.Fatalf("role evidence = %+v", evidence)
	}
	observation, err := NewStructureRoleObservation("role-1", evidence)
	if err != nil {
		t.Fatal(err)
	}
	assessment := assessStructure(t, 60_000,
		[]StructureObservation{
			structureObservation("chapter", ObservationChapterEdge, ObservationProposesBoundary, 30_000, 30_000),
			observation,
			structureObservation("right-role", ObservationTranscriptChange, ObservationContextOnly, 30_000, 60_000),
		},
		[]StructureRoleClaim{
			structureClaim(0, 30_000, SegmentRoleCommercial, observation.ID),
			structureClaim(30_000, 60_000, SegmentRolePromo, "right-role"),
		})
	if assessment.Observations[0].RoleEvidence == nil && assessment.Observations[1].RoleEvidence == nil {
		t.Fatal("role evidence disappeared from canonical assessment")
	}
}

func TestValidateStructureRoleEvidenceRejectsMutationEvenWhenRehashed(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*StructureRoleEvidence)
	}{
		{name: "request", mutate: func(e *StructureRoleEvidence) { e.StartMs++ }},
		{name: "source", mutate: func(e *StructureRoleEvidence) { e.Source.SHA256 = strings.Repeat("f", 64) }},
		{name: "provider", mutate: func(e *StructureRoleEvidence) { e.ResolvedProvider = "" }},
		{name: "modalities", mutate: func(e *StructureRoleEvidence) { e.Modalities = []string{"text"} }},
		{name: "frame", mutate: func(e *StructureRoleEvidence) { e.FrameSHA256[0] = "bad" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			evidence := structureRoleEvidenceFixture(t)
			test.mutate(&evidence)
			evidence.SHA256 = StructureRoleEvidenceSHA256(evidence)
			if err := ValidateStructureRoleEvidence(evidence); err == nil {
				t.Fatal("mutated role evidence was accepted")
			}
		})
	}
}

func TestAssessSourceStructureRejectsRoleEvidenceForAnotherSource(t *testing.T) {
	evidence := structureRoleEvidenceFixture(t)
	observation, err := NewStructureRoleObservation("role-1", evidence)
	if err != nil {
		t.Fatal(err)
	}
	source := structureSource(60_000)
	source.SHA256 = strings.Repeat("d", 64)
	_, err = AssessSourceStructure(SourceStructureInput{
		Source: source, Observations: []StructureObservation{observation},
		AssessedAt: time.Date(2026, time.September, 4, 13, 0, 0, 0, time.UTC),
	})
	if err == nil || !strings.Contains(err.Error(), "different source") {
		t.Fatalf("source-binding error = %v", err)
	}
}
