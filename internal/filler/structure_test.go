package filler

import (
	"strings"
	"testing"
	"time"
)

func structureSource(durationMs int64) SplitSourceAsset {
	return SplitSourceAsset{
		Role: SplitSourceLegacyPlayback, SHA256: strings.Repeat("a", 64), Bytes: 1024,
		ClipHash: strings.Repeat("b", 64), Path: "aa/bb/source.mp4", DurationMs: durationMs,
	}
}

func structureObservation(id string, kind StructureObservationKind, effect StructureObservationEffect, startMs, endMs int64) StructureObservation {
	return StructureObservation{
		ID: id, Kind: kind, Effect: effect, StartMs: startMs, EndMs: endMs,
		Producer: "fixture:v1", EvidenceSHA256: strings.Repeat("c", 64),
	}
}

func structureClaim(startMs, endMs int64, role StructureSegmentRole, evidenceID string) StructureRoleClaim {
	return StructureRoleClaim{
		StartMs: startMs, EndMs: endMs, Role: role, EvidenceIDs: []string{evidenceID},
		Reason: "fixture evidence establishes the interval role",
	}
}

func assessStructure(t *testing.T, durationMs int64, observations []StructureObservation, claims []StructureRoleClaim) SourceStructureAssessment {
	t.Helper()
	assessment, err := AssessSourceStructure(SourceStructureInput{
		Source: structureSource(durationMs), Observations: observations, RoleClaims: claims,
		AssessedAt: time.Date(2026, time.September, 3, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return assessment
}

func TestAssessSourceStructure_DurationAloneDoesNotProveCompilation(t *testing.T) {
	assessment := assessStructure(t, int64((3*time.Hour)/time.Millisecond), nil, nil)
	if assessment.Kind != StructureAmbiguous {
		t.Fatalf("three-hour source kind = %q, want ambiguous without semantic evidence", assessment.Kind)
	}
	if len(assessment.Plan) != 1 || assessment.Plan[0].StartMs != 0 || assessment.Plan[0].EndMs != assessment.DurationMs || assessment.Plan[0].Disposition != StructureUnresolved {
		t.Fatalf("duration-only plan = %+v, want one whole-source unresolved interval", assessment.Plan)
	}
}

func TestAssessSourceStructure_BlackAndSilenceCanResolveACompilation(t *testing.T) {
	observations := []StructureObservation{
		structureObservation("black-1", ObservationBlackInterval, ObservationProposesBoundary, 29_800, 30_200),
		structureObservation("silence-1", ObservationSilenceInterval, ObservationProposesBoundary, 29_900, 30_100),
		structureObservation("role-left", ObservationOCRLogoChange, ObservationSupportsBoundary, 29_950, 30_050),
		structureObservation("role-right", ObservationTranscriptChange, ObservationSupportsBoundary, 29_950, 30_050),
	}
	claims := []StructureRoleClaim{
		structureClaim(0, 30_000, SegmentRoleCommercial, "role-left"),
		structureClaim(30_000, 60_000, SegmentRolePromo, "role-right"),
	}
	assessment := assessStructure(t, 60_000, observations, claims)

	if assessment.Kind != StructureCompilationBreak {
		t.Fatalf("kind = %q, want compilation_break", assessment.Kind)
	}
	if len(assessment.Boundaries) != 1 || assessment.Boundaries[0].Status != BoundaryResolved || assessment.Boundaries[0].AtMs != 30_000 {
		t.Fatalf("boundaries = %+v, want one resolved 30s boundary", assessment.Boundaries)
	}
	if len(assessment.Plan) != 2 || assessment.Plan[0].Disposition != StructureKeep || assessment.Plan[1].Disposition != StructureKeep || assessment.Plan[0].EndMs != assessment.Plan[1].StartMs {
		t.Fatalf("plan = %+v, want two adjacent kept intervals", assessment.Plan)
	}
	if len(assessment.Observations) != len(observations) {
		t.Fatalf("observations = %d, want every independent signal retained", len(assessment.Observations))
	}
	if err := ValidateSourceStructureAssessment(assessment); err != nil {
		t.Fatalf("canonical assessment does not verify: %v", err)
	}
}

func TestAssessSourceStructure_SingleDetectorCannotAuthorizeCuts(t *testing.T) {
	observations := []StructureObservation{
		structureObservation("black-1", ObservationBlackInterval, ObservationProposesBoundary, 29_800, 30_200),
		structureObservation("role-left", ObservationOCRLogoChange, ObservationSupportsBoundary, 29_900, 30_100),
		structureObservation("role-right", ObservationTranscriptChange, ObservationSupportsBoundary, 29_900, 30_100),
	}
	claims := []StructureRoleClaim{
		structureClaim(0, 30_000, SegmentRoleCommercial, "role-left"),
		structureClaim(30_000, 60_000, SegmentRoleCommercial, "role-right"),
	}
	assessment := assessStructure(t, 60_000, observations, claims)

	if assessment.Kind != StructureAmbiguous || assessment.Boundaries[0].Status != BoundaryUnresolved {
		t.Fatalf("single-detector assessment = %+v, want ambiguous with unresolved boundary", assessment)
	}
	for _, segment := range assessment.Plan {
		if segment.Disposition != StructureUnresolved {
			t.Fatalf("unresolved boundary produced publishable segment: %+v", segment)
		}
	}
}

func TestAssessSourceStructure_HardContinuityConflictSurvivesFusion(t *testing.T) {
	observations := []StructureObservation{
		structureObservation("black-1", ObservationBlackInterval, ObservationProposesBoundary, 29_800, 30_200),
		structureObservation("silence-1", ObservationSilenceInterval, ObservationProposesBoundary, 29_900, 30_100),
		structureObservation("same-logo", ObservationVisualContinuity, ObservationContradictsBoundary, 29_000, 31_000),
	}
	assessment := assessStructure(t, 60_000, observations, nil)

	if assessment.Kind != StructureAmbiguous || assessment.Boundaries[0].Status != BoundaryConflicted || len(assessment.Boundaries[0].ConflictIDs) != 1 {
		t.Fatalf("conflicted assessment = %+v", assessment)
	}
}

func TestAssessSourceStructure_ProgrammeMaterialRemainsExplicit(t *testing.T) {
	observations := []StructureObservation{
		structureObservation("chapter-1", ObservationChapterEdge, ObservationProposesBoundary, 30_000, 30_000),
		structureObservation("chapter-2", ObservationChapterEdge, ObservationProposesBoundary, 60_000, 60_000),
		structureObservation("programme-evidence", ObservationTranscriptChange, ObservationSupportsBoundary, 30_000, 30_000),
		structureObservation("commercial-evidence", ObservationOCRLogoChange, ObservationSupportsBoundary, 60_000, 60_000),
	}
	claims := []StructureRoleClaim{
		structureClaim(0, 30_000, SegmentRoleProgrammeFragment, "programme-evidence"),
		structureClaim(30_000, 60_000, SegmentRoleCommercial, "commercial-evidence"),
		structureClaim(60_000, 90_000, SegmentRoleProgrammeFragment, "programme-evidence"),
	}
	assessment := assessStructure(t, 90_000, observations, claims)

	if assessment.Kind != StructureProgrammeSpots {
		t.Fatalf("kind = %q, want programme_with_spots", assessment.Kind)
	}
	if assessment.Plan[0].Disposition != StructureDiscard || assessment.Plan[1].Disposition != StructureKeep || assessment.Plan[2].Disposition != StructureDiscard {
		t.Fatalf("programme plan = %+v, want explicit discard/keep/discard", assessment.Plan)
	}
}

func TestAssessSourceStructure_DiscardStillAccountsForItsExactInterval(t *testing.T) {
	observations := []StructureObservation{
		structureObservation("chapter-1", ObservationChapterEdge, ObservationProposesBoundary, 30_000, 30_000),
		structureObservation("chapter-2", ObservationChapterEdge, ObservationProposesBoundary, 33_000, 33_000),
		structureObservation("role-left", ObservationOCRLogoChange, ObservationSupportsBoundary, 30_000, 30_000),
		structureObservation("role-right", ObservationTranscriptChange, ObservationSupportsBoundary, 33_000, 33_000),
		structureObservation("short", ObservationSilenceInterval, ObservationContextOnly, 30_000, 33_000),
	}
	assessment, err := AssessSourceStructure(SourceStructureInput{
		Source: structureSource(63_000), Observations: observations,
		RoleClaims: []StructureRoleClaim{
			structureClaim(0, 30_000, SegmentRoleCommercial, "role-left"),
			structureClaim(33_000, 63_000, SegmentRolePromo, "role-right"),
		},
		DiscardClaims: []StructureDiscardClaim{{
			StartMs: 30_000, EndMs: 33_000, Reason: DiscardBelowClipFloor,
			EvidenceIDs: []string{"short"},
		}},
		AssessedAt: time.Date(2026, time.September, 3, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Kind != StructureCompilationBreak || len(assessment.Plan) != 3 {
		t.Fatalf("assessment = %+v, want a three-interval compilation plan", assessment)
	}
	discard := assessment.Plan[1]
	if discard.StartMs != 30_000 || discard.EndMs != 33_000 || discard.Disposition != StructureDiscard || discard.DiscardReason != DiscardBelowClipFloor {
		t.Fatalf("discard interval = %+v, want exact explained 30s..33s coverage", discard)
	}
}

func TestAssessSourceStructure_SceneAndDurationAreContextOnly(t *testing.T) {
	observations := []StructureObservation{
		structureObservation("scene-1", ObservationSceneChange, ObservationContextOnly, 30_000, 30_000),
		structureObservation("slot-1", ObservationStandardDuration, ObservationContextOnly, 30_000, 30_000),
	}
	assessment := assessStructure(t, 60_000, observations, nil)
	if assessment.Kind != StructureAmbiguous || len(assessment.Boundaries) != 0 || len(assessment.Plan) != 1 {
		t.Fatalf("context-only observations proposed a split: %+v", assessment)
	}
}

func TestValidateSourceStructureAssessment_RejectsMutation(t *testing.T) {
	assessment := assessStructure(t, 60_000, nil, nil)
	assessment.Plan[0].EndMs--
	if err := ValidateSourceStructureAssessment(assessment); err == nil {
		t.Fatal("mutated coverage/digest was accepted")
	}
}

func TestValidateSourceStructureAssessment_RejectsARehashedInventedVerdict(t *testing.T) {
	assessment := assessStructure(t, 60_000, nil, nil)
	assessment.Kind = StructureCompilationBreak
	assessment.SHA256 = SourceStructureAssessmentSHA256(assessment)
	if err := ValidateSourceStructureAssessment(assessment); err == nil {
		t.Fatal("self-consistent digest allowed a verdict that did not reduce from evidence")
	}
}

func TestAssessSourceStructure_RejectsUnknownRoleEvidence(t *testing.T) {
	_, err := AssessSourceStructure(SourceStructureInput{
		Source: structureSource(60_000), RoleClaims: []StructureRoleClaim{
			structureClaim(0, 60_000, SegmentRoleCommercial, "not-present"),
		},
		AssessedAt: time.Date(2026, time.September, 3, 12, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("role evidence outside the source observation set was accepted")
	}
}

func TestAssessSourceStructure_RejectsRoleClaimOutsideFusedPlan(t *testing.T) {
	observation := structureObservation("role", ObservationOCRLogoChange, ObservationContextOnly, 5_000, 5_000)
	_, err := AssessSourceStructure(SourceStructureInput{
		Source: structureSource(60_000), Observations: []StructureObservation{observation},
		RoleClaims: []StructureRoleClaim{structureClaim(0, 30_000, SegmentRoleCommercial, "role")},
		AssessedAt: time.Date(2026, time.September, 3, 12, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("role claim for a span absent from the fused plan was silently ignored")
	}
}
