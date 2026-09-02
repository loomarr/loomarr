package fillerreview

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillereval"
)

func TestCompareTemporalPanelIsSymmetricDiagnosticEvidence(t *testing.T) {
	config := newTemporalPanelComparisonFixture(t)
	report, err := CompareTemporalPanel(config)
	if err != nil {
		t.Fatal(err)
	}
	if report.Cases != 1 || report.ThreeWayExactUnitAgreement != 1 || report.ThreeWayStandaloneAgreement != 1 || report.ThreeWayRoleComparable != 1 || report.ThreeWayRoleAgreement != 0 {
		t.Fatalf("three-way summary = %+v", report)
	}
	if len(report.PairSummaries) != 3 || report.PairSummaries[0].Pair != "human_first" || report.PairSummaries[0].RoleAgreement != 1 || report.PairSummaries[1].Pair != "human_second" || report.PairSummaries[1].RoleAgreement != 0 || report.PairSummaries[2].Pair != "first_second" || report.PairSummaries[2].RoleAgreement != 0 {
		t.Fatalf("pair summaries = %+v", report.PairSummaries)
	}
	if report.HumanTimestampsInformative != 1 || report.HumanTimestampsAtStart != 0 || report.PairSummaries[0].UnitEvidenceDistance.Comparable != 1 || report.PairSummaries[0].UnitEvidenceDistance.Within2000MS != 1 {
		t.Fatalf("timestamp summary = %+v", report.PairSummaries)
	}
	if len(report.DiagnosticCandidates) != 1 || len(report.DiagnosticCandidates[0].Reasons) != 1 || report.DiagnosticCandidates[0].Reasons[0] != "role_disagreement" {
		t.Fatalf("diagnostic candidates = %+v", report.DiagnosticCandidates)
	}
	if !report.Disposition.TargetedHumanReviewAllowed || report.Disposition.NextAction != "targeted_review_optional" || report.Disposition.ProductionAdmissionAllowed {
		t.Fatalf("disposition = %+v", report.Disposition)
	}
}

func TestCompareTemporalPanelFailsClosedOnAuthorityDrift(t *testing.T) {
	config := newTemporalPanelComparisonFixture(t)
	attestation, err := readStrictJSON[TemporalModelAssessmentAttestation](config.SecondAttestationPath)
	if err != nil {
		t.Fatal(err)
	}
	attestation.AssessmentSetSHA256 = temporalTruthHash([]byte("drift"))
	config.SecondAttestationPath = writeTemporalHumanJSON(t, t.TempDir(), "attestation.json", attestation)
	if _, err := CompareTemporalPanel(config); err == nil {
		t.Fatal("drifted model authority was accepted")
	}
}

func TestPublishTemporalPanelComparisonIsImmutable(t *testing.T) {
	config := newTemporalPanelComparisonFixture(t)
	config.OutputPath = filepath.Join(t.TempDir(), "comparison.json")
	if _, digest, err := PublishTemporalPanelComparison(config); err != nil || !reviewSHA256(digest) {
		t.Fatalf("publish digest=%q err=%v", digest, err)
	}
	info, err := os.Stat(config.OutputPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%v err=%v", info.Mode(), err)
	}
	if _, _, err := PublishTemporalPanelComparison(config); err == nil {
		t.Fatal("immutable comparison was overwritten")
	}
}

func TestTemporalPanelDispositionRefusesLargeHumanQueue(t *testing.T) {
	candidates := make([]TemporalPanelDiagnosticCandidate, TemporalPanelMaximumTargetedCases+1)
	for index := range candidates {
		candidates[index].EvidenceAlias = "evidence"
	}
	disposition := temporalPanelDisposition(candidates)
	if disposition.NextAction != "improve_pipeline" || disposition.TargetedHumanReviewAllowed || len(disposition.TargetedHumanReviewCases) != 0 || disposition.ProductionAdmissionAllowed {
		t.Fatalf("disposition = %+v", disposition)
	}
}

func TestTemporalPanelReasonsExposeSharedUsabilityBlindSpot(t *testing.T) {
	comparison := TemporalPanelCaseComparison{
		Human: TemporalPanelLabel{Unit: fillereval.UnitUnusable},
		First: TemporalPanelLabel{Unit: fillereval.UnitStandalone}, Second: TemporalPanelLabel{Unit: fillereval.UnitStandalone},
		HumanFirst:  TemporalPanelPairComparison{Comparable: true, StandaloneClassAgreement: false},
		HumanSecond: TemporalPanelPairComparison{Comparable: true, StandaloneClassAgreement: false},
		FirstSecond: TemporalPanelPairComparison{Comparable: true, ExactUnitAgreement: true, StandaloneClassAgreement: true},
	}
	reasons := temporalPanelDiagnosticReasons(comparison)
	if len(reasons) != 3 || reasons[0] != "human_unusable_model_miss" || reasons[1] != "standalone_class_disagreement" || reasons[2] != "unit_disagreement" {
		t.Fatalf("reasons = %v", reasons)
	}
}

func newTemporalPanelComparisonFixture(t *testing.T) TemporalPanelComparisonConfig {
	t.Helper()
	fixture := newTemporalModelLockFixture(t)
	firstDir := filepath.Join(t.TempDir(), "first")
	if _, err := LockTemporalModelAssessment(TemporalModelAssessmentLockConfig{
		PackagePath: fixture.packagePath, PrivateMapPath: fixture.mapPath, ResultPath: fixture.resultPath,
		SnapshotPath: fixture.snapshotPath, HumanAssessmentPath: fixture.humanAssessmentPath,
		HumanAttestationPath: fixture.humanAttestationPath, ExpectedCases: 1,
		ReleasedAt: fixture.modelTime.Add(time.Minute), OutputDir: firstDir,
	}); err != nil {
		t.Fatal(err)
	}
	firstSet, err := readStrictJSON[TemporalModelAssessmentSet](filepath.Join(firstDir, "assessment-set.json"))
	if err != nil {
		t.Fatal(err)
	}
	firstAttestation, err := readStrictJSON[TemporalModelAssessmentAttestation](filepath.Join(firstDir, "attestation.json"))
	if err != nil {
		t.Fatal(err)
	}

	secondSet := firstSet
	secondSet.PanelSlot = "panel-b"
	secondSet.BatchID = "model-panel-b-test"
	secondSet.Assessor.ID = "panel-b-model"
	secondSet.Assessor.Model = "review/other-model"
	secondSet.Assessor.ModelFamily = "claude-opus"
	secondSet.Assessments = append([]TemporalLockedModelAssessment(nil), firstSet.Assessments...)
	secondRole := *secondSet.Assessments[0].Role
	secondRole.Kind = fillereval.TemporalRolePromo
	secondSet.Assessments[0].Role = &secondRole
	secondDir := t.TempDir()
	secondSetPath := writeTemporalHumanJSON(t, secondDir, "assessment-set.json", secondSet)
	secondSetSHA, err := hashFile(secondSetPath)
	if err != nil {
		t.Fatal(err)
	}
	secondAttestation := firstAttestation
	secondAttestation.PanelSlot = secondSet.PanelSlot
	secondAttestation.BatchID = secondSet.BatchID
	secondAttestation.AssessmentSetSHA256 = secondSetSHA
	secondAttestation.AttestationSHA256 = temporalModelAssessmentAttestationSHA256(secondAttestation)
	secondAttestationPath := writeTemporalHumanJSON(t, secondDir, "attestation.json", secondAttestation)

	return TemporalPanelComparisonConfig{
		HumanAssessmentPath: fixture.humanAssessmentPath, HumanAttestationPath: fixture.humanAttestationPath,
		FirstAssessmentPath: filepath.Join(firstDir, "assessment-set.json"), FirstAttestationPath: filepath.Join(firstDir, "attestation.json"),
		SecondAssessmentPath: secondSetPath, SecondAttestationPath: secondAttestationPath,
		ExpectedCases: 1, ComparedAt: fixture.modelTime.Add(2 * time.Minute),
	}
}
