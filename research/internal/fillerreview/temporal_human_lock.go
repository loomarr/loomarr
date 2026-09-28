package fillerreview

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/fillereval"
)

type TemporalHumanReviewLockConfig struct {
	PackagePath        string
	PrivateMapPath     string
	SubmissionPath     string
	ExpectedReviewerID string
	LockedAt           time.Time
	MaximumAge         time.Duration
	OutputDir          string
}

type TemporalHumanReviewLockResult struct {
	AssessmentSetSHA256 string
	AttestationSHA256   string
	Assessments         int
}

// LockTemporalHumanReview is the deep validation and unblinding interface. It
// emits both canonical artifacts atomically or leaves no output.
func LockTemporalHumanReview(config TemporalHumanReviewLockConfig) (TemporalHumanReviewLockResult, error) {
	if strings.TrimSpace(config.PackagePath) == "" || strings.TrimSpace(config.PrivateMapPath) == "" || strings.TrimSpace(config.SubmissionPath) == "" || strings.TrimSpace(config.ExpectedReviewerID) == "" || config.LockedAt.IsZero() || config.MaximumAge <= 0 || strings.TrimSpace(config.OutputDir) == "" {
		return TemporalHumanReviewLockResult{}, fmt.Errorf("temporal human review lock requires package, map, submission, reviewer, lock time, maximum age, and output")
	}
	pack, packageSHA, err := LoadTemporalHumanReviewPackage(config.PackagePath)
	if err != nil {
		return TemporalHumanReviewLockResult{}, err
	}
	mapping, err := readStrictJSON[TemporalHumanReviewMap](config.PrivateMapPath)
	if err != nil {
		return TemporalHumanReviewLockResult{}, fmt.Errorf("read temporal human review map: %w", err)
	}
	mapSHA, err := hashFile(config.PrivateMapPath)
	if err != nil {
		return TemporalHumanReviewLockResult{}, err
	}
	aliasMap, err := validateTemporalHumanReviewMap(filepath.Dir(config.PackagePath), pack, packageSHA, mapping)
	if err != nil {
		return TemporalHumanReviewLockResult{}, err
	}
	submissionRaw, err := os.ReadFile(config.SubmissionPath)
	if err != nil {
		return TemporalHumanReviewLockResult{}, fmt.Errorf("read temporal human review submission: %w", err)
	}
	submission, err := decodeTemporalHumanReviewSubmission(submissionRaw)
	if err != nil {
		return TemporalHumanReviewLockResult{}, err
	}
	submissionSHA := hashBytes(submissionRaw)
	set, err := validateAndUnblindTemporalHumanSubmission(config, pack, packageSHA, aliasMap, submission, submissionSHA)
	if err != nil {
		return TemporalHumanReviewLockResult{}, err
	}

	stage, err := beginTemporalHumanReviewStage(config.OutputDir)
	if err != nil {
		return TemporalHumanReviewLockResult{}, err
	}
	defer stage.Cleanup()
	assessmentRaw, err := json.MarshalIndent(set, "", "  ")
	if err != nil {
		return TemporalHumanReviewLockResult{}, err
	}
	assessmentRaw = append(assessmentRaw, '\n')
	assessmentSHA := hashBytes(assessmentRaw)
	attestation := TemporalHumanReviewAttestation{
		SchemaVersion: TemporalHumanReviewSchemaVersion, ContractVersion: TemporalHumanReviewContractVersion,
		BatchID: pack.BatchID, ReviewerID: submission.ReviewerID, LockedAt: config.LockedAt.UTC(),
		PackageSHA256: packageSHA, MapSHA256: mapSHA, SubmissionSHA256: submissionSHA, AssessmentSetSHA256: assessmentSHA,
	}
	attestation.AttestationSHA256 = temporalHumanAttestationSHA256(attestation)
	attestationRaw, err := json.MarshalIndent(attestation, "", "  ")
	if err != nil {
		return TemporalHumanReviewLockResult{}, err
	}
	attestationRaw = append(attestationRaw, '\n')
	if err := writeTemporalTruthNew(filepath.Join(stage.path, "assessment-set.json"), assessmentRaw, 0o600); err != nil {
		return TemporalHumanReviewLockResult{}, err
	}
	if err := writeTemporalTruthNew(filepath.Join(stage.path, "attestation.json"), attestationRaw, 0o600); err != nil {
		return TemporalHumanReviewLockResult{}, err
	}
	if err := stage.Publish(); err != nil {
		return TemporalHumanReviewLockResult{}, err
	}
	return TemporalHumanReviewLockResult{AssessmentSetSHA256: assessmentSHA, AttestationSHA256: attestation.AttestationSHA256, Assessments: len(set.Assessments)}, nil
}

func LoadTemporalHumanReviewPackage(path string) (TemporalHumanReviewPackage, string, error) {
	pack, err := readStrictJSON[TemporalHumanReviewPackage](path)
	if err != nil {
		return TemporalHumanReviewPackage{}, "", fmt.Errorf("read temporal human review package: %w", err)
	}
	digest, err := hashFile(path)
	if err != nil {
		return TemporalHumanReviewPackage{}, "", err
	}
	if err := validateTemporalHumanReviewPackage(filepath.Dir(path), pack); err != nil {
		return TemporalHumanReviewPackage{}, "", err
	}
	return pack, digest, nil
}

func validateTemporalHumanReviewPackage(root string, pack TemporalHumanReviewPackage) error {
	if pack.SchemaVersion != TemporalHumanReviewSchemaVersion || pack.ContractVersion != TemporalHumanReviewContractVersion || pack.QuestionVersion != TemporalHumanReviewQuestionVersion || strings.TrimSpace(pack.BatchID) == "" || pack.PreparedAt.IsZero() || !reviewSHA256(pack.EvidenceManifestSHA256) || !reviewSHA256(pack.SelectionSHA256) || !reviewSHA256(pack.SeedSHA256) || len(pack.Cases) != fillereval.TemporalTruthSelectionCases {
		return fmt.Errorf("temporal human review package identity or case count is invalid")
	}
	aliases := make(map[string]struct{}, len(pack.Cases))
	for _, item := range pack.Cases {
		if !validAlias(item.Alias) || item.DurationMS <= 0 || len(item.Frames) == 0 || len(item.Frames) > TemporalEvidenceMaxFrames {
			return fmt.Errorf("temporal human review package contains an invalid case")
		}
		if _, duplicate := aliases[item.Alias]; duplicate {
			return fmt.Errorf("temporal human review package repeats alias %q", item.Alias)
		}
		aliases[item.Alias] = struct{}{}
		if item.Video.Path != filepath.ToSlash(filepath.Join("cases", item.Alias, "review.mp4")) || item.Video.DurationMS <= 0 || item.Video.DurationMS > item.DurationMS+1_000 || item.Video.Width <= 0 || item.Video.Height <= 0 {
			return fmt.Errorf("temporal human review alias %q has invalid video metadata", item.Alias)
		}
		if err := verifyTemporalTruthEvidenceFile(root, item.Video, TemporalTruthMaximumVideoBytes); err != nil {
			return fmt.Errorf("temporal human review alias %q video: %w", item.Alias, err)
		}
		previousFrameMS := int64(-1)
		for index, frame := range item.Frames {
			expectedPath := filepath.ToSlash(filepath.Join("cases", item.Alias, fmt.Sprintf("frame-%02d.jpg", index+1)))
			if frame.ID != fmt.Sprintf("frame-%02d", index+1) || frame.Path != expectedPath || frame.AtMS < 0 || frame.AtMS >= item.DurationMS || frame.AtMS < previousFrameMS || frame.Width <= 0 || frame.Height <= 0 {
				return fmt.Errorf("temporal human review alias %q frame %d is invalid", item.Alias, index+1)
			}
			previousFrameMS = frame.AtMS
			if err := verifyTemporalTruthEvidenceFile(root, TemporalTruthEvidenceFile{Path: frame.Path, SHA256: frame.SHA256, Bytes: frame.Bytes}, 16<<20); err != nil {
				return fmt.Errorf("temporal human review alias %q frame %d: %w", item.Alias, index+1, err)
			}
			for _, observation := range frame.OCR {
				if err := validateTemporalTruthOCRObservation(observation); err != nil {
					return fmt.Errorf("temporal human review alias %q frame %d: %w", item.Alias, index+1, err)
				}
			}
		}
		previousEnd := int64(0)
		for index, segment := range item.TranscriptSegments {
			if segment.ID != fmt.Sprintf("transcript-%02d", index+1) || strings.TrimSpace(segment.Text) == "" || segment.StartMS < previousEnd || segment.StartMS < 0 || segment.EndMS <= segment.StartMS || segment.EndMS > item.DurationMS {
				return fmt.Errorf("temporal human review alias %q transcript %d is invalid", item.Alias, index+1)
			}
			previousEnd = segment.EndMS
		}
	}
	return nil
}

func validateTemporalHumanReviewMap(publicRoot string, pack TemporalHumanReviewPackage, packageSHA string, mapping TemporalHumanReviewMap) (map[string]string, error) {
	if mapping.SchemaVersion != TemporalHumanReviewSchemaVersion || mapping.ContractVersion != TemporalHumanReviewContractVersion || mapping.BatchID != pack.BatchID || !mapping.PreparedAt.Equal(pack.PreparedAt) || temporalTruthHash([]byte(mapping.Seed)) != pack.SeedSHA256 || mapping.EvidenceManifestSHA256 != pack.EvidenceManifestSHA256 || mapping.SelectionSHA256 != pack.SelectionSHA256 || mapping.PackageSHA256 != packageSHA || !reviewSHA256(mapping.ViewerSHA256) || len(mapping.Entries) != len(pack.Cases) {
		return nil, fmt.Errorf("temporal human review map does not bind the exact package")
	}
	viewerSHA, err := hashFile(filepath.Join(publicRoot, "index.html"))
	if err != nil || viewerSHA != mapping.ViewerSHA256 {
		return nil, fmt.Errorf("temporal human review viewer does not bind its private map")
	}
	publicAliases := make(map[string]struct{}, len(pack.Cases))
	for _, item := range pack.Cases {
		publicAliases[item.Alias] = struct{}{}
	}
	result := make(map[string]string, len(mapping.Entries))
	evidenceAliases := make(map[string]struct{}, len(mapping.Entries))
	for _, entry := range mapping.Entries {
		if _, exists := publicAliases[entry.Alias]; !exists || !strings.HasPrefix(entry.EvidenceAlias, "evidence-") || strings.TrimSpace(entry.EvidenceAlias) == "" {
			return nil, fmt.Errorf("temporal human review map contains an unknown alias")
		}
		if _, duplicate := result[entry.Alias]; duplicate {
			return nil, fmt.Errorf("temporal human review map repeats alias %q", entry.Alias)
		}
		if _, duplicate := evidenceAliases[entry.EvidenceAlias]; duplicate {
			return nil, fmt.Errorf("temporal human review map repeats evidence alias %q", entry.EvidenceAlias)
		}
		result[entry.Alias] = entry.EvidenceAlias
		evidenceAliases[entry.EvidenceAlias] = struct{}{}
	}
	return result, nil
}

func validateAndUnblindTemporalHumanSubmission(config TemporalHumanReviewLockConfig, pack TemporalHumanReviewPackage, packageSHA string, aliasMap map[string]string, submission TemporalHumanReviewSubmission, submissionSHA string) (TemporalHumanAssessmentSet, error) {
	if submission.SchemaVersion != TemporalHumanReviewSchemaVersion || submission.ContractVersion != TemporalHumanReviewContractVersion || submission.BatchID != pack.BatchID || submission.PackageSHA256 != packageSHA || submission.PreparedAt.IsZero() || !submission.PreparedAt.Equal(pack.PreparedAt) || submission.ReviewerID != strings.TrimSpace(config.ExpectedReviewerID) || submission.ReviewerID != strings.TrimSpace(submission.ReviewerID) {
		return TemporalHumanAssessmentSet{}, fmt.Errorf("temporal human review submission identity does not bind the package, reviewer, and prepared time")
	}
	lockedAt := config.LockedAt.UTC()
	if submission.CompletedAt.IsZero() || submission.CompletedAt.Before(pack.PreparedAt) || submission.CompletedAt.After(lockedAt) || submission.CompletedAt.Sub(pack.PreparedAt) > config.MaximumAge || lockedAt.Sub(pack.PreparedAt) > config.MaximumAge {
		return TemporalHumanAssessmentSet{}, fmt.Errorf("temporal human review submission time is stale or outside the prepared/lock interval")
	}
	if len(submission.Answers) != len(pack.Cases) {
		return TemporalHumanAssessmentSet{}, fmt.Errorf("temporal human review submission has %d answers; want exactly %d", len(submission.Answers), len(pack.Cases))
	}
	caseByAlias := make(map[string]TemporalHumanReviewCase, len(pack.Cases))
	for _, item := range pack.Cases {
		caseByAlias[item.Alias] = item
	}
	set := TemporalHumanAssessmentSet{
		SchemaVersion: TemporalHumanReviewSchemaVersion, ContractVersion: TemporalHumanReviewContractVersion,
		BatchID: pack.BatchID, ReviewerID: submission.ReviewerID, EvidenceManifestSHA256: pack.EvidenceManifestSHA256,
		SelectionSHA256: pack.SelectionSHA256, PackageSHA256: packageSHA, SubmissionSHA256: submissionSHA,
		PreparedAt: pack.PreparedAt, CompletedAt: submission.CompletedAt.UTC(),
	}
	seen := make(map[string]struct{}, len(submission.Answers))
	for index, answer := range submission.Answers {
		item, exists := caseByAlias[answer.Alias]
		if !exists {
			return TemporalHumanAssessmentSet{}, fmt.Errorf("temporal human review answer %d names unknown alias %q", index, answer.Alias)
		}
		if _, duplicate := seen[answer.Alias]; duplicate {
			return TemporalHumanAssessmentSet{}, fmt.Errorf("temporal human review submission repeats alias %q", answer.Alias)
		}
		seen[answer.Alias] = struct{}{}
		if answer.ReviewerID != submission.ReviewerID {
			return TemporalHumanAssessmentSet{}, fmt.Errorf("temporal human review submission mixes reviewer identities")
		}
		if err := validateTemporalHumanReviewAnswer(answer, item.DurationMS); err != nil {
			return TemporalHumanAssessmentSet{}, fmt.Errorf("temporal human review answer %q: %w", answer.Alias, err)
		}
		role := answer.Role
		set.Assessments = append(set.Assessments, TemporalHumanReviewAssessment{
			EvidenceAlias: aliasMap[answer.Alias], Unit: answer.Unit, Role: role, DecisiveAtMS: answer.DecisiveAtMS,
		})
	}
	sort.Slice(set.Assessments, func(i, j int) bool { return set.Assessments[i].EvidenceAlias < set.Assessments[j].EvidenceAlias })
	return set, nil
}
