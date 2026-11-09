package split

import (
	"strings"
	"testing"
	"time"
)

func structureSource(durationMs int64) SourceAsset {
	return SourceAsset{
		Role: SourceLegacyPlayback, SHA256: strings.Repeat("a", 64), Bytes: 1024,
		ClipHash: strings.Repeat("b", 64), Path: "aa/bb/source.mp4", DurationMs: durationMs,
	}
}

func structureObservation(id string, kind ObservationKind, effect ObservationEffect, startMs, endMs int64) Observation {
	return Observation{
		ID: id, Kind: kind, Effect: effect, StartMs: startMs, EndMs: endMs,
		Producer: "fixture:v1", EvidenceSHA256: strings.Repeat("c", 64),
	}
}

func structureClaim(startMs, endMs int64, role Role, evidenceID string) RoleClaim {
	return RoleClaim{
		StartMs: startMs, EndMs: endMs, Role: role, EvidenceIDs: []string{evidenceID},
		Reason: "fixture evidence establishes the interval role",
	}
}

func structureRoleObservation(t *testing.T, durationMs, startMs, endMs int64, role Role, id string) Observation {
	t.Helper()
	evidence, err := NewRoleEvidence(RoleEvidenceInput{
		Source: structureSource(durationMs), StartMs: startMs, EndMs: endMs, Role: role,
		Reason: "fixture evidence establishes the interval role", Frames: [][]byte{[]byte(id)},
		PromptVersion: "fixture-role-v1", Prompt: "classify this exact segment", Response: `{"role":"fixture"}`,
		RequestedProvider: "fixture", ResolvedProvider: "fixture", RequestedModel: "fixture", ResolvedModel: "fixture",
		Modalities: []string{"image", "text"}, Attempts: 1,
		AssessedAt: time.Date(2026, time.September, 4, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	observation, err := NewRoleObservation(id, evidence)
	if err != nil {
		t.Fatal(err)
	}
	return observation
}

func assessStructure(t *testing.T, durationMs int64, observations []Observation, claims []RoleClaim) Assessment {
	t.Helper()
	assessment, err := Assess(Input{
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
	if assessment.Kind != KindAmbiguous {
		t.Fatalf("three-hour source kind = %q, want ambiguous without semantic evidence", assessment.Kind)
	}
	if len(assessment.Plan) != 1 || assessment.Plan[0].StartMs != 0 || assessment.Plan[0].EndMs != assessment.DurationMs || assessment.Plan[0].Disposition != DispositionUnresolved {
		t.Fatalf("duration-only plan = %+v, want one whole-source unresolved interval", assessment.Plan)
	}
}

func TestAssessSourceStructure_BlackAndSilenceCanResolveACompilation(t *testing.T) {
	observations := []Observation{
		structureObservation("black-1", ObservationBlackInterval, ObservationProposesBoundary, 29_800, 30_200),
		structureObservation("silence-1", ObservationSilenceInterval, ObservationProposesBoundary, 29_900, 30_100),
		structureRoleObservation(t, 60_000, 0, 30_000, RoleCommercial, "role-left"),
		structureRoleObservation(t, 60_000, 30_000, 60_000, RolePromo, "role-right"),
	}
	claims := []RoleClaim{
		structureClaim(0, 30_000, RoleCommercial, "role-left"),
		structureClaim(30_000, 60_000, RolePromo, "role-right"),
	}
	assessment := assessStructure(t, 60_000, observations, claims)

	if assessment.Kind != KindCompilationBreak {
		t.Fatalf("kind = %q, want compilation_break", assessment.Kind)
	}
	if len(assessment.Boundaries) != 1 || assessment.Boundaries[0].Status != BoundaryResolved || assessment.Boundaries[0].AtMs != 30_000 {
		t.Fatalf("boundaries = %+v, want one resolved 30s boundary", assessment.Boundaries)
	}
	if len(assessment.Plan) != 2 || assessment.Plan[0].Disposition != DispositionKeep || assessment.Plan[1].Disposition != DispositionKeep || assessment.Plan[0].EndMs != assessment.Plan[1].StartMs {
		t.Fatalf("plan = %+v, want two adjacent kept intervals", assessment.Plan)
	}
	if len(assessment.Observations) != len(observations) {
		t.Fatalf("observations = %d, want every independent signal retained", len(assessment.Observations))
	}
	if err := ValidateAssessment(assessment); err != nil {
		t.Fatalf("canonical assessment does not verify: %v", err)
	}
}

func TestAssessSourceStructure_HoldsKeepClaimWithoutBoundSegmentRoleEvidence(t *testing.T) {
	observations := []Observation{
		structureObservation("chapter", ObservationChapterEdge, ObservationProposesBoundary, 30_000, 30_000),
		structureObservation("raw-role", ObservationOCRLogoChange, ObservationContextOnly, 0, 30_000),
		structureRoleObservation(t, 60_000, 30_000, 60_000, RolePromo, "typed-role"),
	}
	assessment := assessStructure(t, 60_000, observations, []RoleClaim{
		structureClaim(0, 30_000, RoleCommercial, "raw-role"),
		structureClaim(30_000, 60_000, RolePromo, "typed-role"),
	})
	if assessment.Plan[0].Disposition != DispositionUnresolved {
		t.Fatalf("generic observation promoted a keep claim: %+v", assessment.Plan[0])
	}
	if assessment.Plan[1].Disposition != DispositionKeep {
		t.Fatalf("matching typed role evidence did not authorize its keep claim: %+v", assessment.Plan[1])
	}
	if err := ValidateAssessment(assessment); err != nil {
		t.Fatalf("replayed assessment did not retain the bound-role decision: %v", err)
	}
}

func TestAssessSourceStructure_HoldsFabricatedCompleteDecisionRoleMarker(t *testing.T) {
	observations := []Observation{
		structureObservation("complete-decision-boundary-0001", ObservationCompleteTimelineDecision, ObservationProposesBoundary, 30_000, 30_000),
		structureObservation("complete-decision-interval-0001", ObservationCompleteTimelineDecision, ObservationContextOnly, 0, 30_000),
		structureObservation("complete-decision-interval-0002", ObservationCompleteTimelineDecision, ObservationContextOnly, 30_000, 60_000),
	}
	assessment := assessStructure(t, 60_000, observations, []RoleClaim{
		structureClaim(0, 30_000, RoleCommercial, "complete-decision-interval-0001"),
		structureClaim(30_000, 60_000, RolePromo, "complete-decision-interval-0002"),
	})
	if assessment.Kind != KindAmbiguous || assessment.Plan[0].Disposition != DispositionUnresolved || assessment.Plan[1].Disposition != DispositionUnresolved {
		t.Fatalf("fabricated complete-decision markers authorized a plan: %+v", assessment)
	}
}

func TestAssessSourceStructure_HoldsKeepClaimForMismatchedSegmentRoleEvidence(t *testing.T) {
	for _, test := range []struct {
		name         string
		startMs      int64
		endMs        int64
		evidenceRole Role
	}{
		{name: "span", startMs: 1, endMs: 30_000, evidenceRole: RoleCommercial},
		{name: "role", startMs: 0, endMs: 30_000, evidenceRole: RolePromo},
	} {
		t.Run(test.name, func(t *testing.T) {
			observations := []Observation{
				structureObservation("chapter", ObservationChapterEdge, ObservationProposesBoundary, 30_000, 30_000),
				structureRoleObservation(t, 60_000, test.startMs, test.endMs, test.evidenceRole, "mismatched-role"),
				structureRoleObservation(t, 60_000, 30_000, 60_000, RolePromo, "right-role"),
			}
			assessment := assessStructure(t, 60_000, observations, []RoleClaim{
				structureClaim(0, 30_000, RoleCommercial, "mismatched-role"),
				structureClaim(30_000, 60_000, RolePromo, "right-role"),
			})
			if assessment.Plan[0].Disposition != DispositionUnresolved || assessment.Plan[1].Disposition != DispositionKeep {
				t.Fatalf("mismatched role evidence changed plan authority: %+v", assessment.Plan)
			}
		})
	}
}

func TestAssessSourceStructure_SingleDetectorCannotAuthorizeCuts(t *testing.T) {
	observations := []Observation{
		structureObservation("black-1", ObservationBlackInterval, ObservationProposesBoundary, 29_800, 30_200),
		structureObservation("role-left", ObservationOCRLogoChange, ObservationSupportsBoundary, 29_900, 30_100),
		structureObservation("role-right", ObservationTranscriptChange, ObservationSupportsBoundary, 29_900, 30_100),
	}
	claims := []RoleClaim{
		structureClaim(0, 30_000, RoleCommercial, "role-left"),
		structureClaim(30_000, 60_000, RoleCommercial, "role-right"),
	}
	assessment := assessStructure(t, 60_000, observations, claims)

	if assessment.Kind != KindAmbiguous || assessment.Boundaries[0].Status != BoundaryUnresolved {
		t.Fatalf("single-detector assessment = %+v, want ambiguous with unresolved boundary", assessment)
	}
	for _, segment := range assessment.Plan {
		if segment.Disposition != DispositionUnresolved {
			t.Fatalf("unresolved boundary produced publishable segment: %+v", segment)
		}
	}
}

func TestAssessSourceStructure_HardContinuityConflictSurvivesFusion(t *testing.T) {
	observations := []Observation{
		structureObservation("black-1", ObservationBlackInterval, ObservationProposesBoundary, 29_800, 30_200),
		structureObservation("silence-1", ObservationSilenceInterval, ObservationProposesBoundary, 29_900, 30_100),
		structureObservation("same-logo", ObservationVisualContinuity, ObservationContradictsBoundary, 29_000, 31_000),
	}
	assessment := assessStructure(t, 60_000, observations, nil)

	if assessment.Kind != KindAmbiguous || assessment.Boundaries[0].Status != BoundaryConflicted || len(assessment.Boundaries[0].ConflictIDs) != 1 {
		t.Fatalf("conflicted assessment = %+v", assessment)
	}
}

func TestAssessSourceStructure_ProgrammeMaterialRemainsExplicit(t *testing.T) {
	observations := []Observation{
		structureObservation("chapter-1", ObservationChapterEdge, ObservationProposesBoundary, 30_000, 30_000),
		structureObservation("chapter-2", ObservationChapterEdge, ObservationProposesBoundary, 60_000, 60_000),
		structureRoleObservation(t, 90_000, 0, 30_000, RoleProgrammeFragment, "programme-left"),
		structureRoleObservation(t, 90_000, 30_000, 60_000, RoleCommercial, "commercial-evidence"),
		structureRoleObservation(t, 90_000, 60_000, 90_000, RoleProgrammeFragment, "programme-right"),
	}
	claims := []RoleClaim{
		structureClaim(0, 30_000, RoleProgrammeFragment, "programme-left"),
		structureClaim(30_000, 60_000, RoleCommercial, "commercial-evidence"),
		structureClaim(60_000, 90_000, RoleProgrammeFragment, "programme-right"),
	}
	assessment := assessStructure(t, 90_000, observations, claims)

	if assessment.Kind != KindProgrammeSpots {
		t.Fatalf("kind = %q, want programme_with_spots", assessment.Kind)
	}
	if assessment.Plan[0].Disposition != DispositionDiscard || assessment.Plan[1].Disposition != DispositionKeep || assessment.Plan[2].Disposition != DispositionDiscard {
		t.Fatalf("programme plan = %+v, want explicit discard/keep/discard", assessment.Plan)
	}
}

func TestAssessSourceStructure_DiscardStillAccountsForItsExactInterval(t *testing.T) {
	observations := []Observation{
		structureObservation("chapter-1", ObservationChapterEdge, ObservationProposesBoundary, 30_000, 30_000),
		structureObservation("chapter-2", ObservationChapterEdge, ObservationProposesBoundary, 33_000, 33_000),
		structureRoleObservation(t, 63_000, 0, 30_000, RoleCommercial, "role-left"),
		structureRoleObservation(t, 63_000, 33_000, 63_000, RolePromo, "role-right"),
		structureObservation("short", ObservationSilenceInterval, ObservationContextOnly, 30_000, 33_000),
	}
	assessment, err := Assess(Input{
		Source: structureSource(63_000), Observations: observations,
		RoleClaims: []RoleClaim{
			structureClaim(0, 30_000, RoleCommercial, "role-left"),
			structureClaim(33_000, 63_000, RolePromo, "role-right"),
		},
		DiscardClaims: []DiscardClaim{{
			StartMs: 30_000, EndMs: 33_000, Reason: DiscardBelowClipFloor,
			EvidenceIDs: []string{"short"},
		}},
		AssessedAt: time.Date(2026, time.September, 3, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Kind != KindCompilationBreak || len(assessment.Plan) != 3 {
		t.Fatalf("assessment = %+v, want a three-interval compilation plan", assessment)
	}
	discard := assessment.Plan[1]
	if discard.StartMs != 30_000 || discard.EndMs != 33_000 || discard.Disposition != DispositionDiscard || discard.DiscardReason != DiscardBelowClipFloor {
		t.Fatalf("discard interval = %+v, want exact explained 30s..33s coverage", discard)
	}
}

func TestAssessSourceStructure_SceneAndDurationAreContextOnly(t *testing.T) {
	observations := []Observation{
		structureObservation("scene-1", ObservationSceneChange, ObservationContextOnly, 30_000, 30_000),
		structureObservation("slot-1", ObservationStandardDuration, ObservationContextOnly, 30_000, 30_000),
	}
	assessment := assessStructure(t, 60_000, observations, nil)
	if assessment.Kind != KindAmbiguous || len(assessment.Boundaries) != 0 || len(assessment.Plan) != 1 {
		t.Fatalf("context-only observations proposed a split: %+v", assessment)
	}
}

func TestValidateSourceStructureAssessment_RejectsMutation(t *testing.T) {
	assessment := assessStructure(t, 60_000, nil, nil)
	assessment.Plan[0].EndMs--
	if err := ValidateAssessment(assessment); err == nil {
		t.Fatal("mutated coverage/digest was accepted")
	}
}

func TestValidateSourceStructureAssessment_RejectsARehashedInventedVerdict(t *testing.T) {
	assessment := assessStructure(t, 60_000, nil, nil)
	assessment.Kind = KindCompilationBreak
	assessment.SHA256 = AssessmentSHA256(assessment)
	if err := ValidateAssessment(assessment); err == nil {
		t.Fatal("self-consistent digest allowed a verdict that did not reduce from evidence")
	}
}

func TestAssessSourceStructure_RejectsUnknownRoleEvidence(t *testing.T) {
	_, err := Assess(Input{
		Source: structureSource(60_000), RoleClaims: []RoleClaim{
			structureClaim(0, 60_000, RoleCommercial, "not-present"),
		},
		AssessedAt: time.Date(2026, time.September, 3, 12, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("role evidence outside the source observation set was accepted")
	}
}

func TestAssessSourceStructure_RejectsRoleClaimOutsideFusedPlan(t *testing.T) {
	observation := structureObservation("role", ObservationOCRLogoChange, ObservationContextOnly, 5_000, 5_000)
	_, err := Assess(Input{
		Source: structureSource(60_000), Observations: []Observation{observation},
		RoleClaims: []RoleClaim{structureClaim(0, 30_000, RoleCommercial, "role")},
		AssessedAt: time.Date(2026, time.September, 3, 12, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("role claim for a span absent from the fused plan was silently ignored")
	}
}
