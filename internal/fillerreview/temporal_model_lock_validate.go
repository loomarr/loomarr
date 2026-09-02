package fillerreview

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/fillereval"
)

func loadTemporalHumanLockAuthority(assessmentPath, attestationPath string) (TemporalHumanAssessmentSet, TemporalHumanReviewAttestation, string, string, error) {
	set, err := readStrictJSON[TemporalHumanAssessmentSet](assessmentPath)
	if err != nil {
		return TemporalHumanAssessmentSet{}, TemporalHumanReviewAttestation{}, "", "", fmt.Errorf("read locked human assessment: %w", err)
	}
	attestationRaw, err := os.ReadFile(attestationPath)
	if err != nil {
		return TemporalHumanAssessmentSet{}, TemporalHumanReviewAttestation{}, "", "", fmt.Errorf("read human attestation: %w", err)
	}
	var attestation TemporalHumanReviewAttestation
	if err := decodeStrictReviewJSON(attestationRaw, &attestation); err != nil {
		return TemporalHumanAssessmentSet{}, TemporalHumanReviewAttestation{}, "", "", fmt.Errorf("decode human attestation: %w", err)
	}
	setSHA, err := hashFile(assessmentPath)
	if err != nil {
		return TemporalHumanAssessmentSet{}, TemporalHumanReviewAttestation{}, "", "", err
	}
	attestationFileSHA := hashBytes(attestationRaw)
	if attestation.SchemaVersion != TemporalHumanReviewSchemaVersion || attestation.ContractVersion != TemporalHumanReviewContractVersion || attestation.AssessmentSetSHA256 != setSHA || temporalHumanAttestationSHA256(attestation) != attestation.AttestationSHA256 || attestation.BatchID != set.BatchID || attestation.ReviewerID != set.ReviewerID || attestation.PackageSHA256 != set.PackageSHA256 || attestation.SubmissionSHA256 != set.SubmissionSHA256 {
		return TemporalHumanAssessmentSet{}, TemporalHumanReviewAttestation{}, "", "", fmt.Errorf("human assessment and attestation authority drift")
	}
	return set, attestation, setSHA, attestationFileSHA, nil
}

func validateTemporalModelPostHumanOrder(result OpenRouterTemporalResult, human TemporalHumanReviewAttestation, releasedAt time.Time) error {
	if human.LockedAt.IsZero() || result.CompletedAt.Before(human.LockedAt) || releasedAt.Before(result.CompletedAt) {
		return fmt.Errorf("model inference and release must occur after the immutable human lock")
	}
	for _, attempt := range result.Attempts {
		if attempt.RequestedAt.Before(human.LockedAt) || attempt.RequestedAt.After(result.CompletedAt) {
			return fmt.Errorf("model attempt predates the human lock or follows result completion")
		}
	}
	for _, assessment := range result.AssessmentSet.Assessments {
		if assessment.Inference.AssessedAt.Before(human.LockedAt) || assessment.Inference.AssessedAt.After(result.CompletedAt) {
			return fmt.Errorf("model assessment predates the human lock or follows result completion")
		}
	}
	return nil
}

func validateTemporalHumanReferenceForModel(set TemporalHumanAssessmentSet, attestation TemporalHumanReviewAttestation, pack TemporalModelReviewPackage, aliasMap map[string]string) error {
	if set.SchemaVersion != TemporalHumanReviewSchemaVersion || set.ContractVersion != TemporalHumanReviewContractVersion || strings.TrimSpace(set.ReviewerID) == "" || set.EvidenceManifestSHA256 != pack.EvidenceManifestSHA256 || set.SelectionSHA256 != pack.SelectionSHA256 || len(set.Assessments) != len(pack.Cases) {
		return fmt.Errorf("human reference does not bind the model evidence set")
	}
	durationByEvidence := make(map[string]int64, len(pack.Cases))
	for _, item := range pack.Cases {
		durationByEvidence[aliasMap[item.Alias]] = item.DurationMS
	}
	seen := make(map[string]struct{}, len(set.Assessments))
	for _, assessment := range set.Assessments {
		duration, exists := durationByEvidence[assessment.EvidenceAlias]
		if !exists {
			return fmt.Errorf("human reference names evidence outside the model package")
		}
		if _, duplicate := seen[assessment.EvidenceAlias]; duplicate {
			return fmt.Errorf("human reference repeats evidence alias %q", assessment.EvidenceAlias)
		}
		seen[assessment.EvidenceAlias] = struct{}{}
		if assessment.DecisiveAtMS < 0 || assessment.DecisiveAtMS >= duration || !validHumanUnit(assessment.Unit) {
			return fmt.Errorf("human reference contains an invalid assessment")
		}
		if assessment.Unit == fillereval.UnitStandalone {
			if assessment.Role == nil || !validHumanRole(*assessment.Role) {
				return fmt.Errorf("human standalone reference lacks a closed role")
			}
		} else if assessment.Role != nil {
			return fmt.Errorf("human non-standalone reference carries a role")
		}
	}
	if len(seen) != len(durationByEvidence) || attestation.LockedAt.Before(set.CompletedAt) {
		return fmt.Errorf("human reference is incomplete or its lock predates completion")
	}
	return nil
}
