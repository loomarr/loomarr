package fillerreview

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/fillereval"
)

const (
	TemporalHumanReviewSchemaVersion   = 1
	TemporalHumanReviewContractVersion = "filler-temporal-human-review-v1"
	TemporalHumanReviewQuestionVersion = "filler-temporal-two-question-v1"
)

// TemporalHumanReviewPackage is the complete reviewer-visible contract. It
// deliberately contains fresh aliases and no stable evidence alias, corpus
// identity, source field, previous answer, model identity, or selection reason.
type TemporalHumanReviewPackage struct {
	SchemaVersion          int                       `json:"schemaVersion"`
	ContractVersion        string                    `json:"contractVersion"`
	QuestionVersion        string                    `json:"questionVersion"`
	BatchID                string                    `json:"batchId"`
	PreparedAt             time.Time                 `json:"preparedAt"`
	EvidenceManifestSHA256 string                    `json:"evidenceManifestSha256"`
	SelectionSHA256        string                    `json:"selectionSha256"`
	SeedSHA256             string                    `json:"seedSha256"`
	Cases                  []TemporalHumanReviewCase `json:"cases"`
}

type TemporalHumanReviewCase struct {
	Alias              string                       `json:"alias"`
	DurationMS         int64                        `json:"durationMs"`
	Video              TemporalTruthEvidenceFile    `json:"video"`
	Frames             []TemporalTruthEvidenceFrame `json:"frames"`
	TranscriptSegments []TemporalReviewTranscript   `json:"transcriptSegments,omitempty"`
}

// TemporalHumanReviewMap is coordinator-only. Its public package and viewer
// digests make the browser bytes part of the submission-lock authority.
type TemporalHumanReviewMap struct {
	SchemaVersion          int                           `json:"schemaVersion"`
	ContractVersion        string                        `json:"contractVersion"`
	BatchID                string                        `json:"batchId"`
	PreparedAt             time.Time                     `json:"preparedAt"`
	Seed                   string                        `json:"seed"`
	EvidenceManifestSHA256 string                        `json:"evidenceManifestSha256"`
	SelectionSHA256        string                        `json:"selectionSha256"`
	PackageSHA256          string                        `json:"packageSha256"`
	ViewerSHA256           string                        `json:"viewerSha256"`
	Entries                []TemporalHumanReviewMapEntry `json:"entries"`
}

type TemporalHumanReviewMapEntry struct {
	Alias         string `json:"alias"`
	EvidenceAlias string `json:"evidenceAlias"`
}

// TemporalHumanReviewSubmission is the only browser export accepted by the
// lock. ReviewerID is repeated per answer so concatenated or mixed-reviewer
// work fails closed instead of inheriting a top-level identity silently.
type TemporalHumanReviewSubmission struct {
	SchemaVersion   int                         `json:"schemaVersion"`
	ContractVersion string                      `json:"contractVersion"`
	BatchID         string                      `json:"batchId"`
	PackageSHA256   string                      `json:"packageSha256"`
	ReviewerID      string                      `json:"reviewerId"`
	PreparedAt      time.Time                   `json:"preparedAt"`
	CompletedAt     time.Time                   `json:"completedAt"`
	Answers         []TemporalHumanReviewAnswer `json:"answers"`
}

type TemporalHumanReviewAnswer struct {
	Alias        string                   `json:"alias"`
	ReviewerID   string                   `json:"reviewerId"`
	Unit         fillereval.UnitKind      `json:"unit"`
	Role         *fillereval.TemporalRole `json:"role,omitempty"`
	DecisiveAtMS int64                    `json:"decisiveAtMs"`
}

// TemporalHumanAssessmentSet is the canonical, batch-alias-free human result.
// EvidenceAlias is the stable opaque join shared by independently prepared
// human and model batches; source identity remains outside this artifact.
type TemporalHumanAssessmentSet struct {
	SchemaVersion          int                             `json:"schemaVersion"`
	ContractVersion        string                          `json:"contractVersion"`
	BatchID                string                          `json:"batchId"`
	ReviewerID             string                          `json:"reviewerId"`
	EvidenceManifestSHA256 string                          `json:"evidenceManifestSha256"`
	SelectionSHA256        string                          `json:"selectionSha256"`
	PackageSHA256          string                          `json:"packageSha256"`
	SubmissionSHA256       string                          `json:"submissionSha256"`
	PreparedAt             time.Time                       `json:"preparedAt"`
	CompletedAt            time.Time                       `json:"completedAt"`
	Assessments            []TemporalHumanReviewAssessment `json:"assessments"`
}

type TemporalHumanReviewAssessment struct {
	EvidenceAlias string                   `json:"evidenceAlias"`
	Unit          fillereval.UnitKind      `json:"unit"`
	Role          *fillereval.TemporalRole `json:"role,omitempty"`
	DecisiveAtMS  int64                    `json:"decisiveAtMs"`
}

type TemporalHumanReviewAttestation struct {
	SchemaVersion       int       `json:"schemaVersion"`
	ContractVersion     string    `json:"contractVersion"`
	BatchID             string    `json:"batchId"`
	ReviewerID          string    `json:"reviewerId"`
	LockedAt            time.Time `json:"lockedAt"`
	PackageSHA256       string    `json:"packageSha256"`
	MapSHA256           string    `json:"mapSha256"`
	SubmissionSHA256    string    `json:"submissionSha256"`
	AssessmentSetSHA256 string    `json:"assessmentSetSha256"`
	AttestationSHA256   string    `json:"attestationSha256"`
}

func decodeTemporalHumanReviewSubmission(raw []byte) (TemporalHumanReviewSubmission, error) {
	var submission TemporalHumanReviewSubmission
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&submission); err != nil {
		return TemporalHumanReviewSubmission{}, fmt.Errorf("decode temporal human review submission: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return TemporalHumanReviewSubmission{}, fmt.Errorf("decode temporal human review submission: trailing JSON value")
		}
		return TemporalHumanReviewSubmission{}, fmt.Errorf("decode temporal human review submission trailing value: %w", err)
	}
	return submission, nil
}

func validateTemporalHumanReviewAnswer(answer TemporalHumanReviewAnswer, durationMS int64) error {
	if strings.TrimSpace(answer.Alias) == "" || strings.TrimSpace(answer.ReviewerID) == "" {
		return fmt.Errorf("alias and reviewer identity are required")
	}
	if answer.DecisiveAtMS < 0 || answer.DecisiveAtMS >= durationMS {
		return fmt.Errorf("decisive timestamp is outside the case")
	}
	if !validHumanUnit(answer.Unit) {
		return fmt.Errorf("unit is not a closed value")
	}
	if answer.Unit == fillereval.UnitStandalone {
		if answer.Role == nil || !validHumanRole(*answer.Role) {
			return fmt.Errorf("standalone unit requires one closed role")
		}
	} else if answer.Role != nil {
		return fmt.Errorf("only standalone unit may carry a role")
	}
	return nil
}

func validHumanUnit(unit fillereval.UnitKind) bool {
	return slices.Contains([]fillereval.UnitKind{
		fillereval.UnitStandalone, fillereval.UnitCompilation, fillereval.UnitProgrammeExcerpt,
		fillereval.UnitUnusable, fillereval.UnitUnclear,
	}, unit)
}

func validHumanRole(role fillereval.TemporalRole) bool {
	return slices.Contains([]fillereval.TemporalRole{
		fillereval.TemporalRoleCommercial, fillereval.TemporalRolePromo, fillereval.TemporalRoleBumper,
		fillereval.TemporalRolePSA, fillereval.TemporalRoleStationID, fillereval.TemporalRoleTrailer,
		fillereval.TemporalRoleInterstitial, fillereval.TemporalRoleUnclear,
	}, role)
}

func temporalHumanAssessmentSetSHA256(set TemporalHumanAssessmentSet) string {
	set.Assessments = slices.Clone(set.Assessments)
	sort.Slice(set.Assessments, func(i, j int) bool {
		return set.Assessments[i].EvidenceAlias < set.Assessments[j].EvidenceAlias
	})
	return temporalTruthJSONSHA(set)
}

func temporalHumanAttestationSHA256(attestation TemporalHumanReviewAttestation) string {
	attestation.AttestationSHA256 = ""
	return temporalTruthJSONSHA(attestation)
}
