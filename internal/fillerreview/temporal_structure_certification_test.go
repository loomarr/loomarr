package fillerreview

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillereval"
)

func TestPublishTemporalStructureCertificationBindsCompleteHoldoutLineage(t *testing.T) {
	fixture := newTemporalStructureHoldoutFixture(t)
	holdoutRoot := filepath.Join(t.TempDir(), "holdout")
	if _, err := BuildTemporalStructureHoldoutPlan(fixture.config(holdoutRoot)); err != nil {
		t.Fatal(err)
	}
	authoringPath := filepath.Join(holdoutRoot, "authoring.json")
	authoring := readStrictTestJSON[TemporalStructureChallengeAuthoring](t, authoringPath)
	media := &fakeTemporalStructureMedia{durationByPath: make(map[string]int64)}
	for _, source := range authoring.Sources {
		media.durationByPath[filepath.Join(fixture.root, filepath.FromSlash(source.Path))] = source.DurationMS
	}
	challengeRoot := filepath.Join(t.TempDir(), "challenge")
	generatedAt := fixture.plannedAt.Add(time.Hour)
	if _, err := BuildTemporalStructureChallenge(context.Background(), TemporalStructureChallengeConfig{
		AuthoringPath: authoringPath, SourceRoot: fixture.root, OutputDir: challengeRoot,
		ChallengeID: "certification-challenge", Seed: "holdout-seed", GeneratedAt: generatedAt, Media: media,
	}); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(challengeRoot, "public", "manifest.json")
	authorityPath := filepath.Join(challengeRoot, "private", "authority.json")
	manifest := readStrictTestJSON[TemporalStructureChallengeManifest](t, manifestPath)
	authority := readStrictTestJSON[TemporalStructureChallengeAuthority](t, authorityPath)
	publicSHA, err := hashFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	authoritySHA, err := hashFile(authorityPath)
	if err != nil {
		t.Fatal(err)
	}
	completedAt := generatedAt.Add(time.Hour)
	first := exactTemporalStructureAssessmentSet(manifest, authority, publicSHA, authoritySHA, completedAt, "assessor-a", "qwen")
	second := exactTemporalStructureAssessmentSet(manifest, authority, publicSHA, authoritySHA, completedAt, "assessor-b", "claude")
	firstPath := writeTemporalHumanJSON(t, t.TempDir(), "first.json", first)
	secondPath := writeTemporalHumanJSON(t, t.TempDir(), "second.json", second)
	comparedAt := completedAt.Add(time.Hour)
	_, comparisonDigest, err := PublishTemporalStructureComparison(TemporalStructureComparisonConfig{
		PublicManifestPath: manifestPath, PrivateAuthorityPath: authorityPath,
		AssessmentPaths: []string{firstPath, secondPath}, ExpectedCases: TemporalStructureHoldoutCases,
		ComparedAt: comparedAt, OutputPath: filepath.Join(t.TempDir(), "comparison.json"),
	})
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "certification.json")
	config := TemporalStructureCertificationConfig{
		HoldoutAuthoringPath: authoringPath, HoldoutReceiptPath: filepath.Join(holdoutRoot, "receipt.json"),
		PublicManifestPath: manifestPath, PrivateAuthorityPath: authorityPath,
		AssessmentPaths: []string{firstPath, secondPath}, ComparedAt: comparedAt,
		CertifiedAt: completedAt.Add(2 * time.Hour), OutputPath: output,
	}
	report, digest, err := PublishTemporalStructureCertification(config)
	if err != nil {
		t.Fatal(err)
	}
	if report.CertificationStatus != TemporalStructureCertificationPassed || len(report.CertifiedSlices) != len(temporalStructureCertificationRequiredSlices) || !reviewSHA256(report.HoldoutAuthoringSHA256) || !reviewSHA256(report.HoldoutReceiptSHA256) || !reviewSHA256(report.PublicManifestSHA256) || !reviewSHA256(report.PrivateAuthoritySHA256) || report.ComparisonSHA256 != comparisonDigest || !reviewSHA256(digest) || report.TrainingAllowed || report.ProductionAdmissionAllowed {
		t.Fatalf("certification report = %+v digest=%q", report, digest)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatal(err)
	}
	if _, _, err := PublishTemporalStructureCertification(config); err == nil {
		t.Fatal("immutable certification output was overwritten")
	}
}

func TestScoreTemporalStructureCertificationRequiresPerfectGlobalAndSliceResults(t *testing.T) {
	comparison := perfectTemporalStructureCertificationComparison()
	report := scoreTemporalStructureCertification(comparison, comparison.ComparedAt.Add(time.Minute))
	if report.CertificationStatus != TemporalStructureCertificationPassed || report.NextAction != "run_locked_shadow_comparison" || report.TrainingAllowed || report.ProductionAdmissionAllowed {
		t.Fatalf("certification disposition = %+v", report)
	}
	if len(report.CertifiedSlices) != len(temporalStructureCertificationRequiredSlices) || len(report.FailureCodes) != 0 {
		t.Fatalf("certified slices or failures = %+v / %v", report.CertifiedSlices, report.FailureCodes)
	}
	for _, slice := range report.Slices {
		if !slice.Passed || slice.Cases != temporalStructureCertificationMinimumSliceCases || slice.Assessors != 2 || len(slice.FailureCodes) != 0 {
			t.Fatalf("slice certification = %+v", slice)
		}
	}
}

func TestScoreTemporalStructureCertificationFailsOnlyAffectedSlice(t *testing.T) {
	comparison := perfectTemporalStructureCertificationComparison()
	for index := range comparison.SliceSummaries {
		if comparison.SliceSummaries[index].Slice == TemporalStructureSliceMixedRoleJoins && comparison.SliceSummaries[index].AssessorID == "assessor-b" {
			comparison.SliceSummaries[index].UnderSplits = 1
			break
		}
	}
	report := scoreTemporalStructureCertification(comparison, comparison.ComparedAt.Add(time.Minute))
	if report.CertificationStatus != TemporalStructureCertificationFailed || report.NextAction != "diagnose_failed_source_and_signal_slices" || len(report.FailureCodes) != 0 {
		t.Fatalf("certification disposition = %+v", report)
	}
	if slices.Contains(report.CertifiedSlices, TemporalStructureSliceMixedRoleJoins) || len(report.CertifiedSlices) != len(temporalStructureCertificationRequiredSlices)-1 {
		t.Fatalf("certified slices = %v", report.CertifiedSlices)
	}
	for _, slice := range report.Slices {
		if slice.Slice == TemporalStructureSliceMixedRoleJoins && (slice.Passed || !slices.Contains(slice.FailureCodes, "under_split")) {
			t.Fatalf("failed slice = %+v", slice)
		}
	}
}

func TestScoreTemporalStructureCertificationGlobalFailureCertifiesNoSlice(t *testing.T) {
	comparison := perfectTemporalStructureCertificationComparison()
	comparison.AssessorSummaries[0].OperationalFailures = 1
	report := scoreTemporalStructureCertification(comparison, comparison.ComparedAt.Add(time.Minute))
	if report.CertificationStatus != TemporalStructureCertificationFailed || !slices.Contains(report.FailureCodes, "operational_failure") || len(report.CertifiedSlices) != 0 {
		t.Fatalf("certification = %+v", report)
	}
}

func TestScoreTemporalStructureCertificationRejectsIncompleteCorpusAndAssessorSummary(t *testing.T) {
	comparison := perfectTemporalStructureCertificationComparison()
	comparison.Cases--
	comparison.AssessorSummaries[0].Cases--
	comparison.AssessorSummaries[0].ExactUnitCorrect--
	comparison.AssessorSummaries[0].CoverageComplete--
	comparison.AssessorSummaries[0].ExactSegmentPlans--
	report := scoreTemporalStructureCertification(comparison, comparison.ComparedAt.Add(time.Minute))
	if !slices.Contains(report.FailureCodes, "insufficient_cases") || len(report.CertifiedSlices) != 0 {
		t.Fatalf("certification = %+v", report)
	}
}

func perfectTemporalStructureCertificationComparison() TemporalStructureComparisonReport {
	comparedAt := time.Date(2026, time.September, 3, 12, 0, 0, 0, time.UTC)
	report := TemporalStructureComparisonReport{
		SchemaVersion: TemporalStructureComparisonSchemaVersion, ContractVersion: TemporalStructureComparisonContractVersion,
		ChallengeID: "challenge", ComparedAt: comparedAt, Cases: TemporalStructureHoldoutCases,
		Assessors: []TemporalStructureAssessorReference{
			{Assessor: fillereval.TemporalAssessorIdentity{ID: "assessor-a"}},
			{Assessor: fillereval.TemporalAssessorIdentity{ID: "assessor-b"}},
		},
	}
	for _, assessorID := range []string{"assessor-a", "assessor-b"} {
		report.AssessorSummaries = append(report.AssessorSummaries, TemporalStructureAssessorSummary{
			AssessorID: assessorID, Cases: TemporalStructureHoldoutCases,
			ExactUnitCorrect: TemporalStructureHoldoutCases, CoverageComplete: TemporalStructureHoldoutCases,
			ExactSegmentPlans: TemporalStructureHoldoutCases, SegmentRoleTargets: 96, SegmentRoleCorrect: 96,
			Boundary: TemporalStructureBoundarySummary{TruthTargets: 72, ComparableTargets: 72, Within2000MS: 72, Within5000MS: 72},
		})
		for _, slice := range temporalStructureCertificationRequiredSlices {
			report.SliceSummaries = append(report.SliceSummaries, TemporalStructureConstructionSummary{
				AssessorID: assessorID, Slice: slice, Cases: temporalStructureCertificationMinimumSliceCases,
				ExactUnitCorrect:   temporalStructureCertificationMinimumSliceCases,
				CoverageComplete:   temporalStructureCertificationMinimumSliceCases,
				ExactSegmentPlans:  temporalStructureCertificationMinimumSliceCases,
				SegmentRoleTargets: 18, SegmentRoleCorrect: 18,
				Boundary: TemporalStructureBoundarySummary{TruthTargets: 12, ComparableTargets: 12, Within2000MS: 12, Within5000MS: 12},
			})
		}
	}
	return report
}

func exactTemporalStructureAssessmentSet(manifest TemporalStructureChallengeManifest, authority TemporalStructureChallengeAuthority, publicSHA, authoritySHA string, completedAt time.Time, assessorID, family string) TemporalStructureAssessmentSet {
	durationByAlias := make(map[string]int64, len(manifest.Cases))
	for _, item := range manifest.Cases {
		durationByAlias[item.Alias] = item.Video.DurationMS
	}
	set := TemporalStructureAssessmentSet{
		SchemaVersion: TemporalStructureAssessmentSchemaVersion, ContractVersion: TemporalStructureAssessmentContractVersion,
		ChallengeID: manifest.ChallengeID, PublicManifestSHA256: publicSHA, PrivateAuthoritySHA256: authoritySHA,
		RawResultSHA256: strings.Repeat("a", 64), SnapshotFileSHA256: strings.Repeat("b", 64),
		CapabilitySnapshotSHA256: strings.Repeat("c", 64), CompletedAt: completedAt, LockedAt: completedAt.Add(time.Minute),
		Assessor: fillereval.TemporalAssessorIdentity{
			ID: assessorID, Provider: "provider", Model: family + "/model", ModelFamily: family,
			ModelDigest: strings.Repeat("d", 64), PromptVersion: "structure-v1",
		},
	}
	for _, truth := range authority.Cases {
		duration := durationByAlias[truth.Alias]
		decisive := []int64{min(int64(1_000), duration/2)}
		if len(truth.JoinTimesMS) > 0 {
			decisive = append([]int64(nil), truth.JoinTimesMS...)
		} else if truth.Unit == fillereval.UnitProgrammeExcerpt {
			decisive = []int64{0, duration}
		}
		assessment := TemporalStructureAssessment{
			Alias: truth.Alias, Unit: &TemporalStructureUnitClaim{Kind: truth.Unit, DecisiveAtMS: decisive, Reason: "closed unit"},
			Inference: temporalStructureTestInference(completedAt.Add(-time.Minute), false),
		}
		for index, part := range truth.Segments {
			endMS := part.OutputEndMS
			if index == len(truth.Segments)-1 {
				endMS = duration
			}
			role := fillereval.TemporalSegmentRole(part.SourceRole)
			if part.Provenance.Kind == TemporalStructureSourceProgrammeParent {
				role = fillereval.TemporalSegmentProgrammeFragment
			}
			atMS := part.OutputStartMS + min(int64(1_000), (endMS-part.OutputStartMS)/2)
			assessment.Segments = append(assessment.Segments, TemporalStructureSegmentClaim{
				StartMS: part.OutputStartMS, EndMS: endMS, Role: role,
				DecisiveAtMS: []int64{atMS}, Reason: "closed segment role",
			})
		}
		if truth.Unit == fillereval.UnitStandalone {
			assessment.Role = &TemporalStructureRoleClaim{Kind: truth.Role, DecisiveAtMS: decisive, Reason: "closed role"}
		}
		set.Assessments = append(set.Assessments, assessment)
	}
	return set
}
