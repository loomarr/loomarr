package fillerreview

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/fillereval"
)

const (
	temporalSuitabilityCheckpointSchemaVersion = 2
	temporalSuitabilityCheckpointFilename      = "suitability-checkpoint.json"
	temporalSuitabilityResponsesDir            = "responses"
)

type SuitabilityOpenRouterAttempt struct {
	EvidenceAlias      string                         `json:"evidenceAlias"`
	RequestedAt        time.Time                      `json:"requestedAt"`
	RequestSHA256      string                         `json:"requestSha256"`
	ResponseSHA256     string                         `json:"responseSha256,omitempty"`
	RawResponsePath    string                         `json:"rawResponsePath,omitempty"`
	GenerationID       string                         `json:"generationId,omitempty"`
	State              string                         `json:"state"`
	LatencyMS          int64                          `json:"latencyMs,omitempty"`
	PromptTokens       int64                          `json:"promptTokens,omitempty"`
	CompletionTokens   int64                          `json:"completionTokens,omitempty"`
	ChargedAmountUSD   string                         `json:"chargedAmountUsd,omitempty"`
	ChargedNanoUSD     int64                          `json:"chargedNanoUsd,omitempty"`
	ReservedNanoUSD    int64                          `json:"reservedNanoUsd"`
	OperationalFailure fillereval.TemporalFailureCode `json:"operationalFailure,omitempty"`
}

type temporalSuitabilityCheckpointIdentity struct {
	SchemaVersion            int    `json:"schemaVersion"`
	EvidenceManifestSHA256   string `json:"evidenceManifestSha256"`
	SelectionSHA256          string `json:"selectionSha256"`
	CapabilitySnapshotSHA256 string `json:"capabilitySnapshotSha256"`
	BaseURL                  string `json:"baseUrl"`
	Model                    string `json:"model"`
	ResolvedModel            string `json:"resolvedModel"`
	ModelFamily              string `json:"modelFamily"`
	UpstreamProvider         string `json:"upstreamProvider"`
	UpstreamProviderSlug     string `json:"upstreamProviderSlug"`
	PromptVersion            string `json:"promptVersion"`
	PromptSHA256             string `json:"promptSha256"`
	ReasoningMode            string `json:"reasoningMode"`
	AssessorID               string `json:"assessorId"`
	ExpectedCases            int    `json:"expectedCases"`
	MaxRequests              int    `json:"maxRequests"`
	MaxSpendNanoUSD          int64  `json:"maxSpendNanoUsd"`
	MaxChargeNanoUSD         int64  `json:"maxChargeNanoUsd"`
}

type temporalSuitabilityCheckpoint struct {
	Identity    temporalSuitabilityCheckpointIdentity `json:"identity"`
	Attempts    []SuitabilityOpenRouterAttempt        `json:"attempts"`
	Assessments []TemporalSuitabilityAssessment       `json:"assessments"`
}

func loadTemporalSuitabilityCheckpoint(dir string, identity temporalSuitabilityCheckpointIdentity, selected []TemporalTruthEvidenceCase) (temporalSuitabilityCheckpoint, error) {
	if err := ensureOpenRouterCheckpointDir(dir); err != nil {
		return temporalSuitabilityCheckpoint{}, err
	}
	path := filepath.Join(dir, temporalSuitabilityCheckpointFilename)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return temporalSuitabilityCheckpoint{Identity: identity}, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return temporalSuitabilityCheckpoint{}, fmt.Errorf("OpenRouter suitability checkpoint must be a private regular file")
	}
	checkpoint, err := readStrictJSON[temporalSuitabilityCheckpoint](path)
	if err != nil {
		return temporalSuitabilityCheckpoint{}, err
	}
	if !reflect.DeepEqual(checkpoint.Identity, identity) {
		return temporalSuitabilityCheckpoint{}, fmt.Errorf("OpenRouter suitability checkpoint identity drift")
	}
	if err := validateTemporalSuitabilityCheckpoint(dir, checkpoint, selected); err != nil {
		return temporalSuitabilityCheckpoint{}, err
	}
	return checkpoint, nil
}

func persistTemporalSuitabilityCheckpoint(dir string, checkpoint temporalSuitabilityCheckpoint, selected []TemporalTruthEvidenceCase) error {
	if err := validateTemporalSuitabilityCheckpoint(dir, checkpoint, selected); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	temporary, err := os.CreateTemp(dir, ".suitability-checkpoint-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(raw); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, filepath.Join(dir, temporalSuitabilityCheckpointFilename)); err != nil {
		return err
	}
	return syncOpenRouterReviewDir(dir)
}

func validateTemporalSuitabilityCheckpoint(dir string, checkpoint temporalSuitabilityCheckpoint, selected []TemporalTruthEvidenceCase) error {
	identity := checkpoint.Identity
	if identity.SchemaVersion != temporalSuitabilityCheckpointSchemaVersion || !reviewSHA256(identity.EvidenceManifestSHA256) || !reviewSHA256(identity.SelectionSHA256) || !reviewSHA256(identity.CapabilitySnapshotSHA256) || !reviewSHA256(identity.PromptSHA256) || identity.BaseURL == "" || identity.Model == "" || identity.ResolvedModel == "" || identity.ModelFamily == "" || identity.UpstreamProvider == "" || identity.UpstreamProviderSlug == "" || identity.PromptVersion != TemporalSuitabilityPromptVersion || !validTemporalSuitabilityReasoningMode(identity.ReasoningMode) || identity.AssessorID == "" || identity.ExpectedCases <= 0 || identity.MaxRequests != identity.ExpectedCases || identity.MaxSpendNanoUSD <= 0 || identity.MaxChargeNanoUSD <= 0 || identity.MaxChargeNanoUSD > identity.MaxSpendNanoUSD {
		return fmt.Errorf("OpenRouter suitability checkpoint identity is invalid")
	}
	if len(selected) != identity.ExpectedCases || len(checkpoint.Attempts) > identity.MaxRequests || len(checkpoint.Assessments) > identity.ExpectedCases {
		return fmt.Errorf("OpenRouter suitability checkpoint counts exceed their identity")
	}
	countsBound := len(checkpoint.Attempts) == len(checkpoint.Assessments) || len(checkpoint.Attempts) == len(checkpoint.Assessments)+1 && checkpoint.Attempts[len(checkpoint.Attempts)-1].State == temporalOpenRouterAttemptReserved
	if !countsBound {
		return fmt.Errorf("OpenRouter suitability checkpoint has an unbound settled attempt")
	}
	var consumed int64
	for index, attempt := range checkpoint.Attempts {
		if index >= len(selected) || attempt.EvidenceAlias != selected[index].Alias || attempt.RequestedAt.IsZero() || !reviewSHA256(attempt.RequestSHA256) || attempt.ReservedNanoUSD != identity.MaxChargeNanoUSD || attempt.LatencyMS < 0 || attempt.PromptTokens < 0 || attempt.CompletionTokens < 0 || attempt.ChargedNanoUSD < 0 || attempt.ChargedNanoUSD > attempt.ReservedNanoUSD {
			return fmt.Errorf("OpenRouter suitability checkpoint attempt %d is invalid", index)
		}
		settled := attempt.State == temporalOpenRouterAttemptAccepted || attempt.State == temporalOpenRouterAttemptFailed
		unsettled := attempt.State == temporalOpenRouterAttemptReserved || attempt.State == temporalOpenRouterAttemptUnsettled
		settledNanoUSD, chargeErr := fillereval.USDToNanoCeil(attempt.ChargedAmountUSD)
		if !settled && !unsettled || settled && (attempt.ChargedAmountUSD == "" || chargeErr != nil || settledNanoUSD != attempt.ChargedNanoUSD || attempt.ResponseSHA256 == "" || attempt.RawResponsePath == "") || unsettled && (attempt.ChargedAmountUSD != "" || attempt.ChargedNanoUSD != 0) || attempt.State == temporalOpenRouterAttemptAccepted && attempt.OperationalFailure != "" || (attempt.State == temporalOpenRouterAttemptFailed || attempt.State == temporalOpenRouterAttemptUnsettled) && !validTemporalOpenRouterFailure(attempt.OperationalFailure) {
			return fmt.Errorf("OpenRouter suitability checkpoint attempt %d has invalid settlement state", index)
		}
		if attempt.ResponseSHA256 != "" {
			if !reviewSHA256(attempt.ResponseSHA256) || attempt.RawResponsePath != filepath.ToSlash(filepath.Join(temporalSuitabilityResponsesDir, attempt.EvidenceAlias+".json")) {
				return fmt.Errorf("OpenRouter suitability response binding is invalid")
			}
			responsePath := filepath.Join(dir, filepath.FromSlash(attempt.RawResponsePath))
			responseInfo, statErr := os.Lstat(responsePath)
			responseSHA, hashErr := hashFile(responsePath)
			if statErr != nil || !responseInfo.Mode().IsRegular() || responseInfo.Mode().Perm() != 0o600 || hashErr != nil || responseSHA != attempt.ResponseSHA256 {
				return fmt.Errorf("OpenRouter suitability raw response binding failed")
			}
		}
		cost := attempt.ChargedNanoUSD
		if unsettled {
			cost = attempt.ReservedNanoUSD
		}
		if consumed > identity.MaxSpendNanoUSD-cost {
			return fmt.Errorf("OpenRouter suitability checkpoint exceeds its spend ceiling")
		}
		consumed += cost
	}
	for index, assessment := range checkpoint.Assessments {
		if assessment.EvidenceAlias != selected[index].Alias {
			return fmt.Errorf("OpenRouter suitability assessments are not an ordered selection prefix")
		}
		if err := validateTemporalSuitabilityAssessment(assessment, selected[index].DurationMS); err != nil {
			return err
		}
		attempt := checkpoint.Attempts[index]
		if assessment.RawResponseSHA256 != attempt.ResponseSHA256 || len(assessment.Inference.Calls) != 1 || assessment.Inference.Calls[0].ResponseSHA256 != attempt.ResponseSHA256 || assessment.Inference.Calls[0].OperationalFailure != attempt.OperationalFailure {
			return fmt.Errorf("OpenRouter suitability assessment and attempt ledger drift")
		}
	}
	return nil
}

func temporalSuitabilityCheckpointSpend(checkpoint temporalSuitabilityCheckpoint) (int64, error) {
	var consumed int64
	for _, attempt := range checkpoint.Attempts {
		cost := attempt.ChargedNanoUSD
		if attempt.State == temporalOpenRouterAttemptReserved || attempt.State == temporalOpenRouterAttemptUnsettled {
			cost = attempt.ReservedNanoUSD
		}
		if consumed > checkpoint.Identity.MaxSpendNanoUSD-cost {
			return 0, fmt.Errorf("OpenRouter suitability checkpoint exhausts its spend ceiling")
		}
		consumed += cost
	}
	return consumed, nil
}

func writeTemporalSuitabilityRawResponse(dir, alias string, raw []byte) (string, error) {
	if len(raw) == 0 || strings.TrimSpace(alias) == "" {
		return "", fmt.Errorf("OpenRouter suitability raw response is empty or unbound")
	}
	relative := filepath.ToSlash(filepath.Join(temporalSuitabilityResponsesDir, alias+".json"))
	path := filepath.Join(dir, filepath.FromSlash(relative))
	if err := writeTemporalTruthNew(path, raw, 0o600); err != nil {
		return "", err
	}
	return relative, nil
}
