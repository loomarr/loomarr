package fillerreview

import (
	"fmt"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/fillereval"
)

type temporalPanelComparisonLoaded struct {
	human                    TemporalHumanAssessmentSet
	humanAttestation         TemporalHumanReviewAttestation
	humanSetSHA              string
	humanAttestationFileSHA  string
	first                    TemporalModelAssessmentSet
	firstAttestation         TemporalModelAssessmentAttestation
	firstSetSHA              string
	firstAttestationFileSHA  string
	second                   TemporalModelAssessmentSet
	secondAttestation        TemporalModelAssessmentAttestation
	secondSetSHA             string
	secondAttestationFileSHA string
}

func loadTemporalPanelComparison(config TemporalPanelComparisonConfig) (temporalPanelComparisonLoaded, error) {
	if strings.TrimSpace(config.HumanAssessmentPath) == "" || strings.TrimSpace(config.HumanAttestationPath) == "" || strings.TrimSpace(config.FirstAssessmentPath) == "" || strings.TrimSpace(config.FirstAttestationPath) == "" || strings.TrimSpace(config.SecondAssessmentPath) == "" || strings.TrimSpace(config.SecondAttestationPath) == "" || config.ExpectedCases <= 0 || config.ComparedAt.IsZero() {
		return temporalPanelComparisonLoaded{}, fmt.Errorf("temporal panel comparison requires human and two model locks, expected cases, and comparison time")
	}
	human, humanAttestation, humanSHA, humanAttestationFileSHA, err := loadTemporalHumanLockAuthority(config.HumanAssessmentPath, config.HumanAttestationPath)
	if err != nil {
		return temporalPanelComparisonLoaded{}, err
	}
	first, firstAttestation, firstSHA, firstAttestationFileSHA, err := loadTemporalModelLockAuthority(config.FirstAssessmentPath, config.FirstAttestationPath)
	if err != nil {
		return temporalPanelComparisonLoaded{}, fmt.Errorf("first model lock: %w", err)
	}
	second, secondAttestation, secondSHA, secondAttestationFileSHA, err := loadTemporalModelLockAuthority(config.SecondAssessmentPath, config.SecondAttestationPath)
	if err != nil {
		return temporalPanelComparisonLoaded{}, fmt.Errorf("second model lock: %w", err)
	}
	loaded := temporalPanelComparisonLoaded{
		human: human, humanAttestation: humanAttestation, humanSetSHA: humanSHA, humanAttestationFileSHA: humanAttestationFileSHA,
		first: first, firstAttestation: firstAttestation, firstSetSHA: firstSHA, firstAttestationFileSHA: firstAttestationFileSHA,
		second: second, secondAttestation: secondAttestation, secondSetSHA: secondSHA, secondAttestationFileSHA: secondAttestationFileSHA,
	}
	if err := validateTemporalPanelComparison(loaded, config.ExpectedCases, config.ComparedAt); err != nil {
		return temporalPanelComparisonLoaded{}, err
	}
	return loaded, nil
}

func loadTemporalModelLockAuthority(assessmentPath, attestationPath string) (TemporalModelAssessmentSet, TemporalModelAssessmentAttestation, string, string, error) {
	set, err := readStrictJSON[TemporalModelAssessmentSet](assessmentPath)
	if err != nil {
		return TemporalModelAssessmentSet{}, TemporalModelAssessmentAttestation{}, "", "", err
	}
	attestation, err := readStrictJSON[TemporalModelAssessmentAttestation](attestationPath)
	if err != nil {
		return TemporalModelAssessmentSet{}, TemporalModelAssessmentAttestation{}, "", "", err
	}
	setSHA, err := hashFile(assessmentPath)
	if err != nil {
		return TemporalModelAssessmentSet{}, TemporalModelAssessmentAttestation{}, "", "", err
	}
	attestationFileSHA, err := hashFile(attestationPath)
	if err != nil {
		return TemporalModelAssessmentSet{}, TemporalModelAssessmentAttestation{}, "", "", err
	}
	if err := validateTemporalModelLockAuthority(set, attestation, setSHA); err != nil {
		return TemporalModelAssessmentSet{}, TemporalModelAssessmentAttestation{}, "", "", err
	}
	return set, attestation, setSHA, attestationFileSHA, nil
}

func validateTemporalModelLockAuthority(set TemporalModelAssessmentSet, attestation TemporalModelAssessmentAttestation, setSHA string) error {
	if set.SchemaVersion != TemporalModelAssessmentSchemaVersion || set.ContractVersion != TemporalModelAssessmentContractVersion || !reviewSHA256(setSHA) || !reviewSHA256(set.EvidenceManifestSHA256) || !reviewSHA256(set.SelectionSHA256) || !reviewSHA256(set.PackageSHA256) || !reviewSHA256(set.MapSHA256) || !reviewSHA256(set.RawResultSHA256) || !reviewSHA256(set.SnapshotFileSHA256) || !reviewSHA256(set.CapabilitySnapshotSHA256) || !reviewSHA256(set.HumanAssessmentSetSHA256) || !reviewSHA256(set.HumanAttestationFileSHA256) || !reviewSHA256(set.HumanAttestationSHA256) || strings.TrimSpace(set.PanelSlot) == "" || strings.TrimSpace(set.BatchID) == "" || set.ReleasedAt.IsZero() || strings.TrimSpace(set.Assessor.ID) == "" || strings.TrimSpace(set.Assessor.ModelFamily) == "" {
		return fmt.Errorf("model assessment set identity is invalid")
	}
	if attestation.SchemaVersion != TemporalModelAssessmentSchemaVersion || attestation.ContractVersion != TemporalModelAssessmentContractVersion || attestation.AssessmentSetSHA256 != setSHA || temporalModelAssessmentAttestationSHA256(attestation) != attestation.AttestationSHA256 || attestation.PanelSlot != set.PanelSlot || attestation.BatchID != set.BatchID || attestation.ReleasedAt != set.ReleasedAt || attestation.PackageSHA256 != set.PackageSHA256 || attestation.MapSHA256 != set.MapSHA256 || attestation.RawResultSHA256 != set.RawResultSHA256 || attestation.SnapshotFileSHA256 != set.SnapshotFileSHA256 || attestation.CapabilitySnapshotSHA256 != set.CapabilitySnapshotSHA256 || attestation.HumanAssessmentSetSHA256 != set.HumanAssessmentSetSHA256 || attestation.HumanAttestationFileSHA256 != set.HumanAttestationFileSHA256 || attestation.HumanAttestationSHA256 != set.HumanAttestationSHA256 {
		return fmt.Errorf("model assessment and attestation authority drift")
	}
	seen := make(map[string]struct{}, len(set.Assessments))
	for _, assessment := range set.Assessments {
		if strings.TrimSpace(assessment.EvidenceAlias) == "" {
			return fmt.Errorf("model assessment has empty evidence alias")
		}
		if _, duplicate := seen[assessment.EvidenceAlias]; duplicate {
			return fmt.Errorf("model assessment repeats evidence alias %q", assessment.EvidenceAlias)
		}
		seen[assessment.EvidenceAlias] = struct{}{}
		if err := validateTemporalLockedModelAssessment(assessment); err != nil {
			return fmt.Errorf("model assessment %q: %w", assessment.EvidenceAlias, err)
		}
	}
	return nil
}

func validateTemporalLockedModelAssessment(assessment TemporalLockedModelAssessment) error {
	if assessment.OperationalFailure != nil {
		if assessment.Unit != nil || assessment.Role != nil || strings.TrimSpace(string(assessment.OperationalFailure.Code)) == "" {
			return fmt.Errorf("operational failure is mixed with semantic labels or lacks a code")
		}
		return nil
	}
	if assessment.Unit == nil || !validHumanUnit(assessment.Unit.Kind) || len(assessment.UnitDecisiveAtMS) == 0 || !validTemporalDecisiveTimes(assessment.UnitDecisiveAtMS) {
		return fmt.Errorf("unit label or decisive evidence is invalid")
	}
	if assessment.Unit.Kind == fillereval.UnitStandalone {
		if assessment.Role == nil || !validHumanRole(assessment.Role.Kind) || len(assessment.RoleDecisiveAtMS) == 0 || !validTemporalDecisiveTimes(assessment.RoleDecisiveAtMS) {
			return fmt.Errorf("standalone role label or decisive evidence is invalid")
		}
	} else if assessment.Role != nil || len(assessment.RoleDecisiveAtMS) != 0 {
		return fmt.Errorf("non-standalone assessment carries role evidence")
	}
	return nil
}

func validTemporalDecisiveTimes(values []int64) bool {
	previous := int64(-1)
	for _, value := range values {
		if value < 0 || value < previous {
			return false
		}
		previous = value
	}
	return true
}

func validateTemporalPanelComparison(loaded temporalPanelComparisonLoaded, expectedCases int, comparedAt time.Time) error {
	if len(loaded.human.Assessments) != expectedCases || len(loaded.first.Assessments) != expectedCases || len(loaded.second.Assessments) != expectedCases {
		return fmt.Errorf("temporal panel comparison requires exactly %d complete assessments from every rater", expectedCases)
	}
	if loaded.human.EvidenceManifestSHA256 != loaded.first.EvidenceManifestSHA256 || loaded.human.EvidenceManifestSHA256 != loaded.second.EvidenceManifestSHA256 || loaded.human.SelectionSHA256 != loaded.first.SelectionSHA256 || loaded.human.SelectionSHA256 != loaded.second.SelectionSHA256 {
		return fmt.Errorf("temporal panel raters do not bind the same evidence selection")
	}
	if loaded.first.HumanAssessmentSetSHA256 != loaded.humanSetSHA || loaded.second.HumanAssessmentSetSHA256 != loaded.humanSetSHA || loaded.first.HumanAttestationFileSHA256 != loaded.humanAttestationFileSHA || loaded.second.HumanAttestationFileSHA256 != loaded.humanAttestationFileSHA || loaded.first.HumanAttestationSHA256 != loaded.humanAttestation.AttestationSHA256 || loaded.second.HumanAttestationSHA256 != loaded.humanAttestation.AttestationSHA256 {
		return fmt.Errorf("temporal model panels do not bind the same human lock")
	}
	if loaded.first.PanelSlot == loaded.second.PanelSlot || loaded.first.Assessor.ID == loaded.second.Assessor.ID || strings.EqualFold(loaded.first.Assessor.ModelFamily, loaded.second.Assessor.ModelFamily) {
		return fmt.Errorf("temporal panel comparison requires distinct slots, assessors, and model families")
	}
	if comparedAt.Before(loaded.firstAttestation.ReleasedAt) || comparedAt.Before(loaded.secondAttestation.ReleasedAt) {
		return fmt.Errorf("temporal panel comparison predates a model release")
	}
	humanAliases := make(map[string]struct{}, expectedCases)
	for _, assessment := range loaded.human.Assessments {
		humanAliases[assessment.EvidenceAlias] = struct{}{}
	}
	for _, assessments := range [][]TemporalLockedModelAssessment{loaded.first.Assessments, loaded.second.Assessments} {
		for _, assessment := range assessments {
			if _, exists := humanAliases[assessment.EvidenceAlias]; !exists {
				return fmt.Errorf("temporal model panel names evidence outside the human reference")
			}
		}
	}
	return nil
}

func (loaded temporalPanelComparisonLoaded) reportIdentity(comparedAt time.Time) TemporalPanelComparisonReport {
	return TemporalPanelComparisonReport{
		SchemaVersion: TemporalPanelComparisonSchemaVersion, ContractVersion: TemporalPanelComparisonContractVersion,
		ComparedAt: comparedAt, EvidenceManifestSHA256: loaded.human.EvidenceManifestSHA256, SelectionSHA256: loaded.human.SelectionSHA256,
		HumanAssessmentSetSHA256: loaded.humanSetSHA, HumanAttestationFileSHA256: loaded.humanAttestationFileSHA, HumanAttestationSHA256: loaded.humanAttestation.AttestationSHA256,
		FirstAssessmentSetSHA256: loaded.firstSetSHA, FirstAttestationFileSHA256: loaded.firstAttestationFileSHA, FirstAttestationSHA256: loaded.firstAttestation.AttestationSHA256,
		SecondAssessmentSetSHA256: loaded.secondSetSHA, SecondAttestationFileSHA256: loaded.secondAttestationFileSHA, SecondAttestationSHA256: loaded.secondAttestation.AttestationSHA256,
		HumanReviewerID: loaded.human.ReviewerID, FirstAssessor: loaded.first.Assessor, SecondAssessor: loaded.second.Assessor, Cases: len(loaded.human.Assessments),
	}
}
