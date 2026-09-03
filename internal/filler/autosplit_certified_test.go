package filler

import (
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillerstructure"
)

func certifiedAutoPolicy() *AutoSplitPolicy {
	return &AutoSplitPolicy{
		Enabled: func() bool { return true }, MinConfidence: func() int { return 70 },
		MaxDuration: func() time.Duration { return 2 * time.Minute },
	}
}

func allowCertifiedStructure(t *testing.T) *StructureMaterializationPolicy {
	t.Helper()
	authority := fillerstructure.Authority{
		SchemaVersion: fillerstructure.AuthoritySchemaVersion, ContractVersion: fillerstructure.AuthorityContractVersion,
		CertificateSHA256: strings.Repeat("f", 64), AssessmentMediaProfileSHA256: strings.Repeat("8", 64),
		MinimumSourceDurationMS: 1, MaximumSourceDurationMS: 120_000, MaximumAssessmentMediaBytes: 64 << 20,
		ReducerVersion:                  fillerstructure.ReducerContractVersion,
		BoundaryToleranceMS:             2_000,
		AllowedUnits:                    []fillerstructure.Unit{fillerstructure.UnitCompilation, fillerstructure.UnitProgrammeSpots},
		AllowedRoles:                    []fillerstructure.Role{fillerstructure.RoleCommercial, fillerstructure.RoleProgrammeFragment, fillerstructure.RolePromo},
		AutomaticMaterializationAllowed: true,
	}
	for _, pair := range [][2]string{{"assessor-a", "family-a"}, {"assessor-b", "family-b"}} {
		authority.Assessors = append(authority.Assessors, fillerstructure.AssessorProfile{
			ID: pair[0], ModelFamily: pair[1], Provider: "fixture-provider", Model: "fixture-model",
			ModelDigest: strings.Repeat("b", 64), CapabilitySHA256: strings.Repeat("c", 64),
			PromptVersion: "prompt-v1", EvidenceContract: "assessment-v1",
		})
	}
	authority.SHA256 = fillerstructure.AuthoritySHA256(authority)
	return &StructureMaterializationPolicy{Authority: &authority}
}

func passingStructureDecision(t *testing.T, assessment SourceStructureAssessment) *fillerstructure.Artifact {
	t.Helper()
	unit := fillerstructure.UnitCompilation
	if assessment.Kind == StructureProgrammeSpots {
		unit = fillerstructure.UnitProgrammeSpots
	}
	source := fillerstructure.Source{SHA256: assessment.Source.SHA256, Bytes: assessment.Source.Bytes, DurationMS: assessment.DurationMs}
	media := fillerstructure.AssessmentMedia{SHA256: strings.Repeat("9", 64), Bytes: assessment.Source.Bytes, DurationMS: assessment.DurationMs, ProfileSHA256: strings.Repeat("8", 64), LineageSHA256: strings.Repeat("7", 64)}
	segments := make([]fillerstructure.Segment, 0, len(assessment.Plan))
	for _, planned := range assessment.Plan {
		segments = append(segments, fillerstructure.Segment{
			StartMS: planned.StartMs, EndMS: planned.EndMs, Role: fillerstructure.Role(planned.Role),
		})
	}
	candidate := func(id, family, digest string) fillerstructure.Candidate {
		return fillerstructure.Candidate{
			Source: source, Media: media,
			Assessor: fillerstructure.Assessor{
				ID: id, ModelFamily: family, Provider: "fixture-provider", Model: "fixture-model",
				ModelDigest: strings.Repeat("b", 64), CapabilitySHA256: strings.Repeat("c", 64),
				PromptVersion: "prompt-v1", EvidenceContract: "assessment-v1",
				AssessmentSHA256: strings.Repeat(digest, 64),
			},
			Unit: unit, Segments: segments,
		}
	}
	artifact, err := fillerstructure.NewArtifact(fillerstructure.Request{
		Source: source, Media: media, BoundaryToleranceMS: 2_000,
		Candidates: []fillerstructure.Candidate{
			candidate("assessor-a", "family-a", "1"), candidate("assessor-b", "family-b", "2"),
		},
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return &artifact
}

func certifiedStructureProposal(t *testing.T) SplitProposal {
	t.Helper()
	observations := []StructureObservation{
		structureObservation("black", ObservationBlackInterval, ObservationProposesBoundary, 29_900, 30_100),
		structureObservation("silence", ObservationSilenceInterval, ObservationProposesBoundary, 29_900, 30_100),
		structureObservation("role-left", ObservationOCRLogoChange, ObservationContextOnly, 0, 30_000),
		structureObservation("role-right", ObservationTranscriptChange, ObservationContextOnly, 30_000, 60_000),
	}
	assessment := assessStructure(t, 60_000, observations, []StructureRoleClaim{
		structureClaim(0, 30_000, SegmentRoleCommercial, "role-left"),
		structureClaim(30_000, 60_000, SegmentRolePromo, "role-right"),
	})
	segments := []SplitSegment{
		{StartMs: 0, EndMs: 30_000, Category: "toys", BoundaryConfidence: 90},
		{StartMs: 30_000, EndMs: 60_000, Category: "television", BoundaryConfidence: 90},
	}
	return SplitProposal{
		Source: assessment.Source, Structure: &assessment,
		StructureDecision: passingStructureDecision(t, assessment), Segments: segments,
	}
}

func TestCertifiedStructureMaterializableRequiresCompleteCertifiedPlan(t *testing.T) {
	proposal := certifiedStructureProposal(t)
	partition := CertifiedStructureMaterializable(proposal, certifiedAutoPolicy(), allowCertifiedStructure(t), 10*time.Second)
	if partition.Reject != AutoSplitOK || len(partition.Confirm) != 2 || len(partition.Hold) != 0 || len(partition.Discard) != 0 {
		t.Fatalf("certified partition = %+v", partition)
	}

	tests := []struct {
		name   string
		mutate func(*SplitProposal, *StructureMaterializationPolicy)
		want   AutoSplitReject
	}{
		{name: "missing assessment", mutate: func(p *SplitProposal, _ *StructureMaterializationPolicy) { p.Structure = nil }, want: RejectStructureMissing},
		{name: "missing structure decision", mutate: func(p *SplitProposal, _ *StructureMaterializationPolicy) { p.StructureDecision = nil }, want: RejectStructureUncertified},
		{name: "uncertified slice", mutate: func(_ *SplitProposal, c *StructureMaterializationPolicy) {
			c.Authority.AutomaticMaterializationAllowed = false
			c.Authority.SHA256 = fillerstructure.AuthoritySHA256(*c.Authority)
		}, want: RejectStructureUncertified},
		{name: "duplicate detector span", mutate: func(p *SplitProposal, _ *StructureMaterializationPolicy) {
			p.Segments = append(p.Segments, p.Segments[0])
		}, want: RejectStructureMismatch},
		{name: "prior partial generation", mutate: func(p *SplitProposal, _ *StructureMaterializationPolicy) { p.Spawned = []string{"child"} }, want: RejectStructureMismatch},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := certifiedStructureProposal(t)
			certification := allowCertifiedStructure(t)
			test.mutate(&candidate, certification)
			got := CertifiedStructureMaterializable(candidate, certifiedAutoPolicy(), certification, 10*time.Second)
			if got.Reject != test.want || len(got.Confirm) != 0 {
				t.Fatalf("partition = %+v, want reject %q", got, test.want)
			}
		})
	}
}

func TestCertifiedStructureMaterializableDoesNotRequirePreChildEnrichment(t *testing.T) {
	proposal := certifiedStructureProposal(t)
	proposal.Segments[1].Category = ""
	proposal.Segments[1].Audience = ""
	proposal.Segments[1].SuggestedEra = 1980
	partition := CertifiedStructureMaterializable(proposal, certifiedAutoPolicy(), allowCertifiedStructure(t), 10*time.Second)
	if partition.Reject != AutoSplitOK || len(partition.Confirm) != 2 || len(partition.Hold) != 0 {
		t.Fatalf("child-only metadata blocked materialization: %+v", partition)
	}
}

func TestCertifiedStructureMaterializableDoesNotInventLegacyBoundaryConfidence(t *testing.T) {
	proposal := certifiedStructureProposal(t)
	for index := range proposal.Segments {
		proposal.Segments[index].BoundaryConfidence = 0
		proposal.Segments[index].Unsplittable = true
	}
	partition := CertifiedStructureMaterializable(proposal, certifiedAutoPolicy(), allowCertifiedStructure(t), 10*time.Second)
	if partition.Reject != AutoSplitOK || len(partition.Confirm) != 2 || len(partition.Hold) != 0 {
		t.Fatalf("certified decision was converted back into detector confidence: %+v", partition)
	}
}

func TestCertifiedStructureMaterializableSeparatesProgrammeDiscardFromFiller(t *testing.T) {
	observations := []StructureObservation{
		structureObservation("chapter-left", ObservationChapterEdge, ObservationProposesBoundary, 20_000, 20_000),
		structureObservation("chapter-right", ObservationChapterEdge, ObservationProposesBoundary, 50_000, 50_000),
		structureObservation("programme-left", ObservationTranscriptChange, ObservationContextOnly, 0, 20_000),
		structureObservation("commercial", ObservationOCRLogoChange, ObservationContextOnly, 20_000, 50_000),
		structureObservation("programme-right", ObservationTranscriptChange, ObservationContextOnly, 50_000, 70_000),
	}
	assessment := assessStructure(t, 70_000, observations, []StructureRoleClaim{
		structureClaim(0, 20_000, SegmentRoleProgrammeFragment, "programme-left"),
		structureClaim(20_000, 50_000, SegmentRoleCommercial, "commercial"),
		structureClaim(50_000, 70_000, SegmentRoleProgrammeFragment, "programme-right"),
	})
	proposal := SplitProposal{Source: assessment.Source, Structure: &assessment, Segments: []SplitSegment{
		{StartMs: 20_000, EndMs: 50_000, Category: "toys"},
	}}
	proposal.StructureDecision = passingStructureDecision(t, assessment)
	partition := CertifiedStructureMaterializable(proposal, certifiedAutoPolicy(), allowCertifiedStructure(t), 10*time.Second)
	if partition.Reject != AutoSplitOK || len(partition.Confirm) != 1 || partition.Confirm[0].StartMs != 20_000 || len(partition.Discard) != 2 || len(partition.Hold) != 0 {
		t.Fatalf("programme/filler partition = %+v", partition)
	}
}
