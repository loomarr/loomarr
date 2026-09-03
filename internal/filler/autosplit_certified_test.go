package filler

import (
	"testing"
	"time"
)

func certifiedAutoPolicy() *AutoSplitPolicy {
	return &AutoSplitPolicy{
		Enabled: func() bool { return true }, MinConfidence: func() int { return 70 },
		MaxDuration: func() time.Duration { return 2 * time.Minute },
	}
}

func allowCertifiedStructure() *StructureCertificationPolicy {
	return &StructureCertificationPolicy{
		AssessmentCertified: func(SourceStructureAssessment) bool { return true },
		ScreeningCertified:  func(SegmentScreeningEvidence) bool { return true },
	}
}

func passingSegmentScreening(t *testing.T, source SplitSourceAsset, startMs, endMs int64) *SegmentScreeningEvidence {
	t.Helper()
	results := []SegmentScreeningResult{
		{Axis: ScreenVisualSafety, Outcome: ScreenPass, AuthoritySHA256: structureBytesSHA256([]byte("visual")), ReasonCode: "policy_clear"},
		{Axis: ScreenSpokenSafety, Outcome: ScreenPass, AuthoritySHA256: structureBytesSHA256([]byte("spoken")), ReasonCode: "policy_clear"},
		{Axis: ScreenRights, Outcome: ScreenPass, AuthoritySHA256: structureBytesSHA256([]byte("rights")), ReasonCode: "rights_verified"},
		{Axis: ScreenPlayback, Outcome: ScreenPass, AuthoritySHA256: structureBytesSHA256([]byte("playback")), ReasonCode: "playback_verified"},
	}
	evidence, err := NewSegmentScreeningEvidence(source, startMs, endMs, results, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return &evidence
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
	for index := range segments {
		segments[index].Screening = passingSegmentScreening(t, assessment.Source, segments[index].StartMs, segments[index].EndMs)
	}
	return SplitProposal{Source: assessment.Source, Structure: &assessment, Segments: segments}
}

func TestCertifiedAutoConfirmableRequiresCompleteCertifiedScreenedPlan(t *testing.T) {
	proposal := certifiedStructureProposal(t)
	partition := CertifiedAutoConfirmable(proposal, certifiedAutoPolicy(), allowCertifiedStructure(), 10*time.Second)
	if partition.Reject != AutoSplitOK || len(partition.Confirm) != 2 || len(partition.Hold) != 0 || len(partition.Discard) != 0 {
		t.Fatalf("certified partition = %+v", partition)
	}

	tests := []struct {
		name   string
		mutate func(*SplitProposal, *StructureCertificationPolicy)
		want   AutoSplitReject
	}{
		{name: "missing assessment", mutate: func(p *SplitProposal, _ *StructureCertificationPolicy) { p.Structure = nil }, want: RejectStructureMissing},
		{name: "uncertified slice", mutate: func(_ *SplitProposal, c *StructureCertificationPolicy) {
			c.AssessmentCertified = func(SourceStructureAssessment) bool { return false }
		}, want: RejectStructureUncertified},
		{name: "unscreened segment", mutate: func(p *SplitProposal, _ *StructureCertificationPolicy) { p.Segments[0].Screening = nil }, want: RejectSegmentUnscreened},
		{name: "visual safety rejection", mutate: func(p *SplitProposal, _ *StructureCertificationPolicy) {
			for index := range p.Segments[0].Screening.Results {
				if p.Segments[0].Screening.Results[index].Axis == ScreenVisualSafety {
					p.Segments[0].Screening.Results[index].Outcome = ScreenReject
					p.Segments[0].Screening.Results[index].ReasonCode = "explicit_visual_content"
				}
			}
			p.Segments[0].Screening.SHA256 = SegmentScreeningEvidenceSHA256(*p.Segments[0].Screening)
		}, want: RejectSegmentUnscreened},
		{name: "unverified screening authority", mutate: func(_ *SplitProposal, c *StructureCertificationPolicy) {
			c.ScreeningCertified = func(SegmentScreeningEvidence) bool { return false }
		}, want: RejectSegmentUnscreened},
		{name: "span mismatch", mutate: func(p *SplitProposal, _ *StructureCertificationPolicy) { p.Segments[0].EndMs-- }, want: RejectStructureMismatch},
		{name: "prior partial generation", mutate: func(p *SplitProposal, _ *StructureCertificationPolicy) { p.Spawned = []string{"child"} }, want: RejectStructureMismatch},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := certifiedStructureProposal(t)
			certification := allowCertifiedStructure()
			test.mutate(&candidate, certification)
			got := CertifiedAutoConfirmable(candidate, certifiedAutoPolicy(), certification, 10*time.Second)
			if got.Reject != test.want || len(got.Confirm) != 0 {
				t.Fatalf("partition = %+v, want reject %q", got, test.want)
			}
		})
	}
}

func TestCertifiedAutoConfirmableDoesNotPartiallyPublishLegacyHolds(t *testing.T) {
	proposal := certifiedStructureProposal(t)
	proposal.Segments[1].Category = ""
	partition := CertifiedAutoConfirmable(proposal, certifiedAutoPolicy(), allowCertifiedStructure(), 10*time.Second)
	if partition.Reject != RejectUntagged || len(partition.Confirm) != 0 || len(partition.Hold) != 2 {
		t.Fatalf("partial legacy result escaped complete-plan gate: %+v", partition)
	}
}

func TestCertifiedAutoConfirmableSeparatesProgrammeDiscardFromFiller(t *testing.T) {
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
		{StartMs: 0, EndMs: 20_000, BoundaryConfidence: 90},
		{StartMs: 20_000, EndMs: 50_000, Category: "toys", BoundaryConfidence: 90},
		{StartMs: 50_000, EndMs: 70_000, BoundaryConfidence: 90},
	}}
	proposal.Segments[1].Screening = passingSegmentScreening(t, assessment.Source, 20_000, 50_000)
	partition := CertifiedAutoConfirmable(proposal, certifiedAutoPolicy(), allowCertifiedStructure(), 10*time.Second)
	if partition.Reject != AutoSplitOK || len(partition.Confirm) != 1 || partition.Confirm[0].StartMs != 20_000 || len(partition.Discard) != 2 || len(partition.Hold) != 0 {
		t.Fatalf("programme/filler partition = %+v", partition)
	}
}
