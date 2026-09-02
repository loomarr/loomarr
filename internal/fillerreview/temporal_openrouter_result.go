package fillerreview

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/loomarr/loomarr/internal/fillereval"
)

func DecodeOpenRouterTemporalResult(data []byte) (OpenRouterTemporalResult, error) {
	var result OpenRouterTemporalResult
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return OpenRouterTemporalResult{}, fmt.Errorf("decode OpenRouter temporal result: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return OpenRouterTemporalResult{}, fmt.Errorf("decode OpenRouter temporal result: trailing JSON value")
		}
		return OpenRouterTemporalResult{}, fmt.Errorf("decode OpenRouter temporal result trailing value: %w", err)
	}
	return result, nil
}

// ValidateOpenRouterTemporalResult verifies the immutable result against the
// already verified package/selection and recomputes all monetary accounting.
func ValidateOpenRouterTemporalResult(result OpenRouterTemporalResult, loaded TemporalCalibrationPackage) error {
	return validateOpenRouterTemporalInferenceResult(result, loaded.inferencePackage())
}

// ValidateOpenRouterTemporalModelResult re-verifies both the complete model
// package and its result through the same inference seam used during the run.
func ValidateOpenRouterTemporalModelResult(result OpenRouterTemporalResult, packagePath string, expectedCases int) error {
	loaded, err := loadTemporalModelInferencePackage(packagePath, expectedCases)
	if err != nil {
		return err
	}
	return validateOpenRouterTemporalInferenceResult(result, loaded)
}

func validateOpenRouterTemporalInferenceResult(result OpenRouterTemporalResult, loaded temporalInferencePackage) error {
	if result.SchemaVersion != OpenRouterTemporalResultSchemaVersion || result.ContractVersion != OpenRouterTemporalResultContract || result.SelectionSHA256 != loaded.SelectionSHA256 || !reviewSHA256(result.CapabilitySnapshotSHA256) || result.ResolvedModel == "" || result.UpstreamProvider == "" || result.UpstreamProviderSlug == "" || !reviewSHA256(result.PromptSHA256) || result.MaxRequests < len(loaded.Cases) || result.MaxRequests > len(loaded.Cases)*2 || result.MaxSpendNanoUSD <= 0 || result.MaxChargeNanoUSD <= 0 || result.MaxChargeNanoUSD > result.MaxSpendNanoUSD || result.CompletedAt.IsZero() {
		return fmt.Errorf("OpenRouter temporal result identity or ceilings are invalid")
	}
	if err := fillereval.ValidateTemporalAssessmentSet(result.AssessmentSet, loaded.BatchID, loaded.PackageSHA256, loaded.Signals); err != nil {
		return fmt.Errorf("OpenRouter temporal result assessment set: %w", err)
	}
	if result.AssessmentSet.Assessor.Provider != "openrouter" || result.AssessmentSet.Assessor.ModelDigest != result.CapabilitySnapshotSHA256 || result.AssessmentSet.Assessor.PromptVersion != OpenRouterTemporalPromptVersion {
		return fmt.Errorf("OpenRouter temporal result assessor identity is invalid")
	}
	if result.Requests != len(result.Attempts) || result.Requests > result.MaxRequests {
		return fmt.Errorf("OpenRouter temporal result request accounting is invalid")
	}
	checkpoint := temporalOpenRouterCheckpoint{
		Identity: temporalOpenRouterCheckpointIdentity{
			SchemaVersion: temporalOpenRouterCheckpointSchemaVersion, PackageSHA256: loaded.PackageSHA256,
			SelectionSHA256: loaded.SelectionSHA256, CapabilitySnapshotSHA256: result.CapabilitySnapshotSHA256,
			BaseURL: "validated-result", Model: result.AssessmentSet.Assessor.Model, ResolvedModel: result.ResolvedModel,
			ModelFamily: result.AssessmentSet.Assessor.ModelFamily, UpstreamProvider: result.UpstreamProvider,
			UpstreamProviderSlug: result.UpstreamProviderSlug, PromptVersion: OpenRouterTemporalPromptVersion,
			PromptSHA256: result.PromptSHA256, AssessorID: result.AssessmentSet.Assessor.ID, BatchID: loaded.BatchID,
			ExpectedPackageCases: loaded.PackageCaseCount, ExpectedCalibrationCases: len(loaded.Cases),
			MaxRequests: result.MaxRequests, MaxSpendNanoUSD: result.MaxSpendNanoUSD, MaxChargeNanoUSD: result.MaxChargeNanoUSD,
		},
		Attempts: result.Attempts, Assessments: result.AssessmentSet.Assessments,
	}
	if err := validateTemporalOpenRouterCheckpoint(checkpoint); err != nil {
		return fmt.Errorf("OpenRouter temporal result ledger: %w", err)
	}
	if err := validateTemporalOpenRouterCheckpointAgainstSelection(checkpoint, loaded); err != nil {
		return fmt.Errorf("OpenRouter temporal result binding: %w", err)
	}
	consumed, err := temporalOpenRouterCheckpointSpend(checkpoint)
	if err != nil {
		return err
	}
	var charged int64
	unknown := 0
	for _, attempt := range result.Attempts {
		charged += attempt.ChargedNanoUSD
		if attempt.State == temporalOpenRouterAttemptReserved || attempt.State == temporalOpenRouterAttemptUnsettled {
			unknown++
		}
	}
	if result.ChargedNanoUSD != charged || result.ConsumedNanoUSD != consumed || result.UnknownChargeReservations != unknown {
		return fmt.Errorf("OpenRouter temporal result spend accounting is invalid")
	}
	return nil
}
