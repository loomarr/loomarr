package fillerreview

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/loomarr/loomarr/internal/fillerbakeoff"
)

const openRouterReviewAttestationSchemaVersion = 1

const (
	OpenRouterReviewerBID            = "hosted-reviewer-b"
	OpenRouterReviewerBExpectedCases = 300
)

type OpenRouterReviewInspectionStatus string

const (
	OpenRouterReviewInspectionAwaitingExplicitMaintainerApproval OpenRouterReviewInspectionStatus = "awaiting_explicit_maintainer_approval"
	OpenRouterReviewInspectionActiveRunLockPresent               OpenRouterReviewInspectionStatus = "active_run_lock_present"
	OpenRouterReviewInspectionCheckpointComplete                 OpenRouterReviewInspectionStatus = "checkpoint_complete"
)

// OpenRouterReviewInspectionConfig identifies one existing hosted-review
// checkpoint and all immutable artifacts and ceilings to which it must remain
// bound. Inspection is read-only and has no provider or credential input.
type OpenRouterReviewInspectionConfig struct {
	ArtifactPaths        OpenRouterReviewInspectionArtifactPaths
	OpenedArtifacts      *OpenRouterReviewInspectionArtifacts
	Model                string
	UpstreamProvider     string
	UpstreamProviderSlug string
	ReviewerID           string
	ExpectedCases        int
	MaxRequests          int
	MaxSpendNanoUSD      int64
	MaxChargeNanoUSD     int64
}

// OpenRouterReviewAttestation is the sanitized, content-addressed inspection
// projection of a private checkpoint. It deliberately excludes aliases,
// labels, prompts, request bodies, generation IDs, timestamps, and paths.
type OpenRouterReviewAttestation struct {
	SchemaVersion                       int                              `json:"schemaVersion"`
	AttestationSHA256                   string                           `json:"attestationSha256,omitempty"`
	Status                              OpenRouterReviewInspectionStatus `json:"status"`
	ProviderExecutionAuthorized         bool                             `json:"providerExecutionAuthorized"`
	PackageManifestSHA256               string                           `json:"packageManifestSha256"`
	CapabilitySnapshotSHA256            string                           `json:"capabilitySnapshotSha256"`
	TranscriptSetSHA256                 string                           `json:"transcriptSetSha256,omitempty"`
	CheckpointSHA256                    string                           `json:"checkpointSha256"`
	CheckpointIdentitySHA256            string                           `json:"checkpointIdentitySha256"`
	ActiveRunLockSHA256                 string                           `json:"activeRunLockSha256,omitempty"`
	PromptSHA256                        string                           `json:"promptSha256"`
	ExpectedCases                       int                              `json:"expectedCases"`
	AcceptedCases                       int                              `json:"acceptedCases"`
	PendingCases                        int                              `json:"pendingCases"`
	HistoricalRequestsUsed              int                              `json:"historicalRequestsUsed"`
	HistoricalRequestsRemaining         int                              `json:"historicalRequestsRemaining"`
	FailedAttempts                      int                              `json:"failedAttempts"`
	HistoricalRequestCeiling            int                              `json:"historicalRequestCeiling"`
	HistoricalSpendCeilingNanoUSD       int64                            `json:"historicalSpendCeilingNanoUsd"`
	HistoricalPerCallCeilingNanoUSD     int64                            `json:"historicalPerCallCeilingNanoUsd"`
	HistoricalRemainingAllowanceNanoUSD int64                            `json:"historicalRemainingAllowanceNanoUsd"`
}

// InspectOpenRouterReviewCheckpoint validates one private checkpoint without
// creating a lock, mutating state, reading credentials, or constructing an
// HTTP client.
func InspectOpenRouterReviewCheckpoint(config OpenRouterReviewInspectionConfig) (OpenRouterReviewAttestation, error) {
	if config.ReviewerID != OpenRouterReviewerBID || config.ExpectedCases != OpenRouterReviewerBExpectedCases {
		return OpenRouterReviewAttestation{}, fmt.Errorf("OpenRouter review inspection requires the exact Reviewer B 300-case contract")
	}
	if config.Model == "" || config.UpstreamProvider == "" || config.UpstreamProviderSlug == "" || config.ReviewerID == "" || config.ExpectedCases <= 0 || config.MaxRequests < config.ExpectedCases || config.MaxRequests > config.ExpectedCases+1 || config.MaxSpendNanoUSD <= 0 || config.MaxChargeNanoUSD <= 0 || config.MaxChargeNanoUSD > config.MaxSpendNanoUSD {
		return OpenRouterReviewAttestation{}, fmt.Errorf("OpenRouter review inspection requires exact artifact identity and positive request, charge, and spend ceilings")
	}
	artifacts := config.OpenedArtifacts
	if artifacts == nil {
		var err error
		artifacts, err = OpenOpenRouterReviewInspectionArtifacts(config.ArtifactPaths)
		if err != nil {
			return OpenRouterReviewAttestation{}, err
		}
		defer func() { _ = artifacts.Close() }()
	}
	baseURL := strings.TrimRight(strings.TrimSpace(artifacts.snapshot.SourceBaseURL), "/")
	if baseURL != fillerbakeoff.OpenRouterBaseURL {
		return OpenRouterReviewAttestation{}, fmt.Errorf("OpenRouter review inspection requires the canonical capability snapshot source")
	}
	reviewConfig := OpenRouterReviewConfig{
		BaseURL: baseURL, Snapshot: artifacts.snapshot, Model: config.Model,
		UpstreamProvider: config.UpstreamProvider, UpstreamProviderSlug: config.UpstreamProviderSlug,
		ReviewerID: config.ReviewerID, ExpectedCases: config.ExpectedCases, MaxRequests: config.MaxRequests,
		MaxSpendNanoUSD: config.MaxSpendNanoUSD, MaxChargeNanoUSD: config.MaxChargeNanoUSD,
	}
	if err := validateOpenRouterReviewSnapshotIdentity(reviewConfig, baseURL); err != nil {
		return OpenRouterReviewAttestation{}, err
	}
	var manifest Package
	if err := decodeStrictReviewJSON(artifacts.manifestRaw, &manifest); err != nil {
		return OpenRouterReviewAttestation{}, fmt.Errorf("read review manifest: %w", err)
	}
	if err := validateInspectionReviewPackage(artifacts.packageRoot, manifest, config.ExpectedCases); err != nil {
		return OpenRouterReviewAttestation{}, err
	}
	_, transcriptSetSHA256, err := indexReviewTranscripts(artifacts.transcripts, manifest)
	if err != nil {
		return OpenRouterReviewAttestation{}, err
	}
	manifestSHA256 := hashBytes(artifacts.manifestRaw)
	expectedIdentity := buildOpenRouterCheckpointIdentity(reviewConfig, baseURL, manifest.BatchID, manifestSHA256, transcriptSetSHA256)

	checkpointRaw := artifacts.checkpointRaw
	var checkpoint openRouterCheckpoint
	if err := decodeStrictReviewJSON(checkpointRaw, &checkpoint); err != nil {
		return OpenRouterReviewAttestation{}, fmt.Errorf("decode private OpenRouter review checkpoint: %w", err)
	}
	if !reflect.DeepEqual(checkpoint.Identity, expectedIdentity) {
		return OpenRouterReviewAttestation{}, fmt.Errorf("OpenRouter review checkpoint identity drift")
	}
	if err := validateOpenRouterCheckpoint(checkpoint); err != nil {
		return OpenRouterReviewAttestation{}, err
	}
	if err := validateOpenRouterCheckpointOrder(checkpoint, manifest.Cases); err != nil {
		return OpenRouterReviewAttestation{}, err
	}

	failedAttempts := 0
	for _, attempt := range checkpoint.Attempts {
		if attempt.State == openRouterAttemptReserved {
			return OpenRouterReviewAttestation{}, fmt.Errorf("OpenRouter review checkpoint has an unsettled reserved request")
		}
		if attempt.State == openRouterAttemptFailed {
			failedAttempts++
		}
	}
	historicalSpendConsumedNanoUSD, err := openRouterCheckpointSpend(checkpoint)
	if err != nil {
		return OpenRouterReviewAttestation{}, err
	}
	identityRaw, err := json.Marshal(expectedIdentity)
	if err != nil {
		return OpenRouterReviewAttestation{}, err
	}
	status := OpenRouterReviewInspectionAwaitingExplicitMaintainerApproval
	activeRunLockSHA256 := ""
	activeRunLockPresent := false
	lockRaw := artifacts.activeLockRaw
	if artifacts.activeLockPresent {
		var lockRecord openRouterActiveRunLockRecord
		if err := decodeStrictReviewJSON(lockRaw, &lockRecord); err != nil || lockRecord.SchemaVersion != openRouterActiveRunLockSchemaVersion || lockRecord.CheckpointIdentitySHA256 != hashBytes(identityRaw) || lockRecord.StartedAt.IsZero() || lockRecord.ProcessID <= 0 {
			return OpenRouterReviewAttestation{}, fmt.Errorf("OpenRouter review active run lock is invalid or identity-drifted")
		}
		status = OpenRouterReviewInspectionActiveRunLockPresent
		activeRunLockSHA256 = hashBytes(lockRaw)
		activeRunLockPresent = true
	}
	if len(checkpoint.Submissions) == config.ExpectedCases && !activeRunLockPresent {
		status = OpenRouterReviewInspectionCheckpointComplete
	}

	attestation := OpenRouterReviewAttestation{
		SchemaVersion: openRouterReviewAttestationSchemaVersion, Status: status,
		PackageManifestSHA256: expectedIdentity.PackageManifestSHA256, CapabilitySnapshotSHA256: expectedIdentity.CapabilitySnapshotSHA256,
		TranscriptSetSHA256: expectedIdentity.TranscriptSetSHA256, CheckpointSHA256: hashBytes(checkpointRaw),
		CheckpointIdentitySHA256: hashBytes(identityRaw), ActiveRunLockSHA256: activeRunLockSHA256,
		PromptSHA256:  expectedIdentity.PromptSHA256,
		ExpectedCases: config.ExpectedCases, AcceptedCases: len(checkpoint.Submissions), PendingCases: config.ExpectedCases - len(checkpoint.Submissions),
		HistoricalRequestsUsed: len(checkpoint.Attempts), HistoricalRequestsRemaining: config.MaxRequests - len(checkpoint.Attempts), FailedAttempts: failedAttempts,
		HistoricalRequestCeiling:            config.MaxRequests,
		HistoricalSpendCeilingNanoUSD:       config.MaxSpendNanoUSD,
		HistoricalPerCallCeilingNanoUSD:     config.MaxChargeNanoUSD,
		HistoricalRemainingAllowanceNanoUSD: config.MaxSpendNanoUSD - historicalSpendConsumedNanoUSD,
	}
	attestationSHA256, err := openRouterReviewAttestationSHA256(attestation)
	if err != nil {
		return OpenRouterReviewAttestation{}, err
	}
	attestation.AttestationSHA256 = attestationSHA256
	return attestation, nil
}

func openRouterReviewAttestationSHA256(attestation OpenRouterReviewAttestation) (string, error) {
	attestation.AttestationSHA256 = ""
	raw, err := json.Marshal(attestation)
	if err != nil {
		return "", err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return "", err
	}
	canonical, err := json.Marshal(fields)
	if err != nil {
		return "", err
	}
	return hashBytes(canonical), nil
}
