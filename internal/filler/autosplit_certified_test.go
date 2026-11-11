package filler

import (
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/split"
	"github.com/loomarr/loomarr/internal/filler/structure"
	"github.com/loomarr/loomarr/internal/filler/structure/cut"
	"github.com/loomarr/loomarr/internal/filler/structure/window"
)

func certifiedAutoPolicy() *AutoSplitPolicy {
	return &AutoSplitPolicy{
		Enabled:     func() bool { return true },
		MaxDuration: func() time.Duration { return 2 * time.Minute },
	}
}

func allowCertifiedStructure(t *testing.T) *StructureMaterializationPolicy {
	t.Helper()
	authority := structure.Authority{
		SchemaVersion: structure.AuthoritySchemaVersion, ContractVersion: structure.AuthorityContractVersion,
		CertificateSHA256: strings.Repeat("f", 64), AssessmentMediaProfileSHA256: strings.Repeat("8", 64),
		MinimumSourceDurationMS: 1, MaximumSourceDurationMS: 120_000, MaximumAssessmentMediaBytes: 64 << 20,
		ReducerVersion:                  structure.ReducerContractVersion,
		BoundaryToleranceMS:             2_000,
		AllowedUnits:                    []structure.Unit{structure.UnitCompilation, structure.UnitProgrammeSpots},
		AllowedRoles:                    []structure.Role{structure.RoleCommercial, structure.RoleProgrammeFragment, structure.RolePromo},
		AutomaticMaterializationAllowed: true,
	}
	for _, pair := range [][2]string{{"assessor-a", "family-a"}, {"assessor-b", "family-b"}} {
		authority.Assessors = append(authority.Assessors, structure.AssessorProfile{
			ID: pair[0], ModelFamily: pair[1], Provider: "fixture-provider", Model: "fixture-model",
			ModelDigest: strings.Repeat("b", 64), CapabilitySHA256: strings.Repeat("c", 64),
			PromptVersion: "prompt-v1", EvidenceContract: "assessment-v1",
		})
	}
	authority.SHA256 = structure.AuthoritySHA256(authority)
	return &StructureMaterializationPolicy{Authority: &authority}
}

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

func certifiedStructureProposal(t *testing.T) SplitProposal {
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
		{name: "missing structure decision", mutate: func(p *SplitProposal, _ *StructureMaterializationPolicy) { p.StructureDecision = nil }, want: RejectStructureInvalid},
		{name: "uncertified slice", mutate: func(_ *SplitProposal, c *StructureMaterializationPolicy) {
			c.Authority.AutomaticMaterializationAllowed = false
			c.Authority.SHA256 = structure.AuthoritySHA256(*c.Authority)
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

func TestCertifiedStructureMaterializableRefusesDuplicateAndShortCertifiedCandidates(t *testing.T) {
	proposal := certifiedStructureProposal(t)
	proposal.Segments[0].DupOf = "old/ad.mp4"
	partition := CertifiedStructureMaterializable(proposal, certifiedAutoPolicy(), allowCertifiedStructure(t), 40*time.Second)
	if partition.Reject != RejectDuplicate || len(partition.Confirm) != 0 || len(partition.Hold) != 2 {
		t.Fatalf("partition = %+v, want duplicate and short certified candidates held", partition)
	}
	if got := partition.Hold[0].HoldReason; got != string(RejectDuplicate) {
		t.Fatalf("duplicate hold reason = %q, want %q", got, RejectDuplicate)
	}
	if got := partition.Hold[1].HoldReason; got != string(RejectTooShort) {
		t.Fatalf("short hold reason = %q, want %q", got, RejectTooShort)
	}
}

func TestCertifiedStructureMaterializableUsesDistinctWindowAuthority(t *testing.T) {
	proposal := certifiedStructureProposal(t)
	proposal.StructureDecision = passingWindowStructureDecision(t, *proposal.StructureDecision)
	projected, err := split.ProjectConfirmedDecision(proposal.Source, nil, *proposal.StructureDecision)
	if err != nil {
		t.Fatal(err)
	}
	proposal.Structure = &projected
	withoutWindowAuthority := CertifiedStructureMaterializable(proposal, certifiedAutoPolicy(), allowCertifiedStructure(t), 10*time.Second)
	if withoutWindowAuthority.Reject != RejectStructureUncertified || len(withoutWindowAuthority.Confirm) != 0 {
		t.Fatalf("complete-video authority admitted window decision: %+v", withoutWindowAuthority)
	}
	withWindowAuthority := CertifiedStructureMaterializable(proposal, certifiedAutoPolicy(), allowCertifiedWindowStructure(t, *proposal.StructureDecision), 10*time.Second)
	if withWindowAuthority.Reject != AutoSplitOK || len(withWindowAuthority.Confirm) != 2 || len(withWindowAuthority.Hold) != 0 {
		t.Fatalf("window authority did not admit certified held children: %+v", withWindowAuthority)
	}
}

// The certified gate reads the plan, not the tags: untagged segments of a certified window plan
// still materialize (they are tagged on their own ladder afterwards).
func TestCertifiedWindowPlanMaterializesUntaggedSegments(t *testing.T) {
	proposal := certifiedStructureProposal(t)
	proposal.StructureDecision = passingWindowStructureDecision(t, *proposal.StructureDecision)
	projected, err := split.ProjectConfirmedDecision(proposal.Source, nil, *proposal.StructureDecision)
	if err != nil {
		t.Fatal(err)
	}
	proposal.Structure = &projected
	for index := range proposal.Segments {
		proposal.Segments[index].Category = ""
		proposal.Segments[index].Audience = ""
	}
	applied := CertifiedStructureMaterializable(proposal, certifiedAutoPolicy(),
		allowCertifiedWindowStructure(t, *proposal.StructureDecision), 10*time.Second)
	if applied.Verdict() != AutoSplitOK || len(applied.Confirm) != 2 || len(applied.Hold) != 0 {
		t.Fatalf("certified application decision=%+v", applied)
	}
}

// Without a certifying authority every segment is held, however clean the detection looks.
func TestCertifiedGateHoldsWithoutAuthority(t *testing.T) {
	proposal := certifiedStructureProposal(t)
	application := CertifiedStructureMaterializable(proposal, certifiedAutoPolicy(), nil, 10*time.Second)
	if application.Reject != RejectStructureUncertified || len(application.Confirm) != 0 || len(application.Hold) != 2 {
		t.Fatalf("application partition without authority = %+v", application)
	}
}

// Without a retained decision the assessment cannot be verified, so every segment is held.
func TestCertifiedGateHoldsWithoutDecision(t *testing.T) {
	proposal := certifiedStructureProposal(t)
	proposal.StructureDecision = nil
	application := CertifiedStructureMaterializable(proposal, certifiedAutoPolicy(), allowCertifiedStructure(t), 10*time.Second)
	if application.Reject != RejectStructureInvalid || len(application.Confirm) != 0 || len(application.Hold) != 2 {
		t.Fatalf("application partition without decision = %+v", application)
	}
}

func passingWindowStructureDecision(t *testing.T, complete structure.Artifact) *structure.Artifact {
	t.Helper()
	plan, err := window.NewPlan(complete.Decision.Source, cut.SoftwareProfile())
	if err != nil {
		t.Fatal(err)
	}
	items := make([]structure.AssessmentMedia, len(plan.Windows))
	for index, window := range plan.Windows {
		items[index] = structure.AssessmentMedia{
			SHA256: strings.Repeat(string(rune('4'+index)), 64), Bytes: 1_024,
			DurationMS:    window.MediaEndMS - window.MediaStartMS,
			ProfileSHA256: plan.Profile.AssessmentMediaProfileSHA256,
			LineageSHA256: strings.Repeat(string(rune('7'+index)), 64),
		}
	}
	input, err := structure.NewWindowMediaSetInput(complete.Decision.Source, plan.SHA256, items)
	if err != nil {
		t.Fatal(err)
	}
	candidates := append([]structure.Candidate(nil), complete.Decision.Candidates...)
	for index := range candidates {
		candidates[index].InputSHA256 = input.SHA256
		candidates[index].Assessor.PromptVersion = window.DirectVideoPromptVersion
		candidates[index].Assessor.EvidenceContract = window.CallRecordContractVersion
	}
	artifact, err := structure.NewArtifact(structure.Request{
		Source: complete.Decision.Source, Input: input, BoundaryToleranceMS: complete.BoundaryToleranceMS, Candidates: candidates,
	}, complete.DecidedAt)
	if err != nil {
		t.Fatal(err)
	}
	return &artifact
}

func allowCertifiedWindowStructure(t *testing.T, artifact structure.Artifact) *StructureMaterializationPolicy {
	t.Helper()
	maximumBytes := int64(0)
	for _, item := range artifact.Decision.Input.Items {
		maximumBytes = max(maximumBytes, item.Bytes)
	}
	authority := window.MaterializationAuthority{
		SchemaVersion:             window.MaterializationAuthoritySchemaVersion,
		ContractVersion:           window.MaterializationAuthorityContractVersion,
		WindowCertificationSHA256: strings.Repeat("d", 64), ShortLongShadowSHA256: strings.Repeat("e", 64),
		WindowProfileSHA256:          window.ProfileFor(cut.SoftwareProfile()).SHA256,
		AssessmentMediaProfileSHA256: artifact.Decision.Input.ProfileSHA256,
		MinimumSourceDurationMS:      1, MaximumSourceDurationMS: artifact.Decision.Source.DurationMS,
		MaximumWindowBytes: maximumBytes, MaximumWindows: len(artifact.Decision.Input.Items),
		ReducerVersion: artifact.ReducerVersion, BoundaryToleranceMS: artifact.BoundaryToleranceMS,
		AllowedUnits: []structure.Unit{artifact.Decision.Unit},
		AllowedRoles: []structure.Role{structure.RoleCommercial, structure.RolePromo},
		ReviewerID:   "fixture-reviewer", ReviewedAt: time.Date(2026, 9, 14, 2, 0, 0, 0, time.UTC),
		AutomaticMaterializationAllowed: true,
	}
	for _, candidate := range artifact.Decision.Candidates {
		authority.Assessors = append(authority.Assessors, structure.Profile(candidate.Assessor))
	}
	authority.SHA256 = window.MaterializationAuthoritySHA256(authority)
	return &StructureMaterializationPolicy{WindowAuthority: &authority}
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

func TestReassessProposalStructureReplaysPairedProjection(t *testing.T) {
	proposal := certifiedStructureProposal(t)
	assessment, err := reassessProposalStructure(proposal, proposal.Structure.AssessedAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := split.ValidateDecisionProjection(assessment, *proposal.StructureDecision); err != nil {
		t.Fatalf("reassessed paired projection did not replay: %v", err)
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
	observations := []split.Observation{
		structureObservation("chapter-left", split.ObservationChapterEdge, split.ObservationProposesBoundary, 20_000, 20_000),
		structureObservation("chapter-right", split.ObservationChapterEdge, split.ObservationProposesBoundary, 50_000, 50_000),
		structureRoleObservation(t, 70_000, 0, 20_000, split.RoleProgrammeFragment, "programme-left"),
		structureRoleObservation(t, 70_000, 20_000, 50_000, split.RoleCommercial, "commercial"),
		structureRoleObservation(t, 70_000, 50_000, 70_000, split.RoleProgrammeFragment, "programme-right"),
	}
	assessment := assessStructure(t, 70_000, observations, []split.RoleClaim{
		structureClaim(0, 20_000, split.RoleProgrammeFragment, "programme-left"),
		structureClaim(20_000, 50_000, split.RoleCommercial, "commercial"),
		structureClaim(50_000, 70_000, split.RoleProgrammeFragment, "programme-right"),
	})
	proposal := SplitProposal{Source: assessment.Source, Structure: &assessment, Segments: []split.Segment{
		{StartMs: 20_000, EndMs: 50_000, Category: "toys"},
	}}
	proposal.StructureDecision = passingStructureDecision(t, assessment)
	projected, err := split.ProjectConfirmedDecision(proposal.Source, &assessment, *proposal.StructureDecision)
	if err != nil {
		t.Fatal(err)
	}
	proposal.Structure = &projected
	partition := CertifiedStructureMaterializable(proposal, certifiedAutoPolicy(), allowCertifiedStructure(t), 10*time.Second)
	if partition.Reject != AutoSplitOK || len(partition.Confirm) != 1 || partition.Confirm[0].StartMs != 20_000 || len(partition.Discard) != 2 || len(partition.Hold) != 0 {
		t.Fatalf("programme/filler partition = %+v", partition)
	}
}

// The plan's own refusals: auto-split off holds everything, an empty plan has nothing to cut, and
// a span longer than one advert holds the whole plan back (a certified plan is cut whole or not).
func TestCertifiedPlanRefusals(t *testing.T) {
	segments := []split.Segment{{StartMs: 0, EndMs: 30_000}, {StartMs: 30_000, EndMs: 60_000}}
	off := &AutoSplitPolicy{Enabled: func() bool { return false }}
	if got := certifiedPlanMaterializable(segments, off, 0); got.Reject != RejectDisabled || len(got.Hold) != 2 {
		t.Errorf("auto-split off = %+v, want every segment held as disabled", got)
	}
	if got := certifiedPlanMaterializable(nil, certifiedAutoPolicy(), 0); got.Reject != RejectNoSegments {
		t.Errorf("empty plan = %+v, want %q", got, RejectNoSegments)
	}
	long := append([]split.Segment(nil), segments...)
	long[1].EndMs = 30_000 + (3 * time.Minute).Milliseconds()
	got := certifiedPlanMaterializable(long, certifiedAutoPolicy(), 0)
	if got.Reject != RejectTooLong || len(got.Confirm) != 0 || len(got.Hold) != 2 {
		t.Errorf("an over-long span = %+v, want the plan held as too long", got)
	}
}
