package fillerreview

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/loomarr/loomarr/internal/fillereval"
)

const (
	openRouterCheckpointSchemaVersion = 1
	openRouterCheckpointFilename      = "checkpoint.json"
	openRouterAttemptReserved         = "reserved"
	openRouterAttemptAccepted         = "accepted"
	openRouterAttemptFailed           = "failed"
)

type openRouterCheckpointIdentity struct {
	SchemaVersion            int    `json:"schemaVersion"`
	PackageManifestSHA256    string `json:"packageManifestSha256"`
	CapabilitySnapshotSHA256 string `json:"capabilitySnapshotSha256"`
	TranscriptSetSHA256      string `json:"transcriptSetSha256,omitempty"`
	BaseURL                  string `json:"baseUrl"`
	Model                    string `json:"model"`
	ResolvedModel            string `json:"resolvedModel"`
	UpstreamProvider         string `json:"upstreamProvider"`
	UpstreamProviderSlug     string `json:"upstreamProviderSlug"`
	PromptVersion            string `json:"promptVersion"`
	PromptSHA256             string `json:"promptSha256"`
	ReviewerID               string `json:"reviewerId"`
	BatchID                  string `json:"batchId"`
	ExpectedCases            int    `json:"expectedCases"`
	MaxRequests              int    `json:"maxRequests"`
	MaxSpendNanoUSD          int64  `json:"maxSpendNanoUsd"`
	MaxChargeNanoUSD         int64  `json:"maxChargeNanoUsd"`
}

type openRouterCheckpoint struct {
	Identity    openRouterCheckpointIdentity `json:"identity"`
	Attempts    []ReviewAttempt              `json:"attempts"`
	Submissions []fillereval.LabelSubmission `json:"submissions"`
	Calls       []ReviewCall                 `json:"calls"`
}

func loadOpenRouterCheckpoint(dir string, identity openRouterCheckpointIdentity) (openRouterCheckpoint, error) {
	if err := ensureOpenRouterCheckpointDir(dir); err != nil {
		return openRouterCheckpoint{}, err
	}
	path := filepath.Join(dir, openRouterCheckpointFilename)
	fileInfo, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return openRouterCheckpoint{Identity: identity}, nil
	}
	if err != nil || !fileInfo.Mode().IsRegular() || fileInfo.Mode().Perm()&0o077 != 0 {
		return openRouterCheckpoint{}, fmt.Errorf("OpenRouter review checkpoint file must be private and regular")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return openRouterCheckpoint{}, fmt.Errorf("read private OpenRouter review checkpoint: %w", err)
	}
	var checkpoint openRouterCheckpoint
	if err := decodeStrictReviewJSON(raw, &checkpoint); err != nil {
		return openRouterCheckpoint{}, fmt.Errorf("decode private OpenRouter review checkpoint: %w", err)
	}
	if !reflect.DeepEqual(checkpoint.Identity, identity) {
		return openRouterCheckpoint{}, fmt.Errorf("OpenRouter review checkpoint identity drift")
	}
	if err := validateOpenRouterCheckpoint(checkpoint); err != nil {
		return openRouterCheckpoint{}, err
	}
	return checkpoint, nil
}

func ensureOpenRouterCheckpointDir(dir string) error {
	return ensureOpenRouterCheckpointDirBeforeCreate(dir, nil)
}

func ensureOpenRouterCheckpointDirBeforeCreate(dir string, beforeCreate func()) error {
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(dir), 0o750); err != nil {
			return fmt.Errorf("create private OpenRouter review checkpoint parent: %w", err)
		}
		if beforeCreate != nil {
			beforeCreate()
		}
		if err := os.Mkdir(dir, 0o700); err == nil {
			return nil
		} else if !os.IsExist(err) {
			return fmt.Errorf("create private OpenRouter review checkpoint: %w", err)
		}
		info, err = os.Lstat(dir)
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("OpenRouter review checkpoint directory must be private and not a symlink")
	}
	return nil
}

func persistOpenRouterCheckpoint(dir string, checkpoint openRouterCheckpoint) error {
	if err := validateOpenRouterCheckpoint(checkpoint); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	temporary, err := os.CreateTemp(dir, ".checkpoint-*")
	if err != nil {
		return fmt.Errorf("create private OpenRouter review checkpoint: %w", err)
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
	if err := os.Rename(temporaryPath, filepath.Join(dir, openRouterCheckpointFilename)); err != nil {
		return fmt.Errorf("publish private OpenRouter review checkpoint: %w", err)
	}
	directory, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("sync private OpenRouter review checkpoint directory: %w", err)
	}
	defer func() { _ = directory.Close() }()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync private OpenRouter review checkpoint directory: %w", err)
	}
	return nil
}

func validateOpenRouterCheckpoint(checkpoint openRouterCheckpoint) error {
	identity := checkpoint.Identity
	if identity.SchemaVersion != openRouterCheckpointSchemaVersion || !reviewSHA256(identity.PackageManifestSHA256) || !reviewSHA256(identity.CapabilitySnapshotSHA256) || !reviewSHA256(identity.PromptSHA256) || (identity.TranscriptSetSHA256 != "" && !reviewSHA256(identity.TranscriptSetSHA256)) || identity.BaseURL == "" || identity.Model == "" || identity.ResolvedModel == "" || identity.UpstreamProvider == "" || identity.UpstreamProviderSlug == "" || identity.PromptVersion == "" || identity.ReviewerID == "" || identity.BatchID == "" || identity.ExpectedCases <= 0 || identity.MaxRequests < identity.ExpectedCases || identity.MaxRequests > identity.ExpectedCases+1 || identity.MaxSpendNanoUSD <= 0 || identity.MaxChargeNanoUSD <= 0 || identity.MaxChargeNanoUSD > identity.MaxSpendNanoUSD {
		return fmt.Errorf("OpenRouter review checkpoint identity is invalid")
	}
	accepted := make(map[string]fillereval.LabelSubmission, len(checkpoint.Submissions))
	calls := make(map[string]ReviewCall, len(checkpoint.Calls))
	for index, submission := range checkpoint.Submissions {
		if _, duplicate := accepted[submission.Alias]; duplicate || submission.BatchID != identity.BatchID || submission.ReviewerID != identity.ReviewerID || submission.ReviewedAt.IsZero() || index >= len(checkpoint.Calls) || len(fillereval.ValidateLabels(submission.Labels)) != 0 {
			return fmt.Errorf("OpenRouter review checkpoint contains duplicate or unbound accepted results")
		}
		accepted[submission.Alias] = submission
		call := checkpoint.Calls[index]
		if call.Alias != submission.Alias || !call.ReviewedAt.Equal(submission.ReviewedAt) || call.Attempt <= 0 || !reviewSHA256(call.RequestSHA256) {
			return fmt.Errorf("OpenRouter review checkpoint accepted call is unbound")
		}
		calls[call.Alias] = call
	}
	if len(checkpoint.Calls) != len(checkpoint.Submissions) || len(checkpoint.Attempts) > identity.MaxRequests {
		return fmt.Errorf("OpenRouter review checkpoint result counts are invalid")
	}
	perAliasAttempt := map[string]int{}
	acceptedAttempts := map[string]struct{}{}
	var charged int64
	for _, attempt := range checkpoint.Attempts {
		perAliasAttempt[attempt.Alias]++
		settledCharge, chargeErr := fillereval.USDToNanoCeil(attempt.ChargedAmountUSD)
		settled := attempt.State != openRouterAttemptReserved
		if attempt.Alias == "" || attempt.Attempt != perAliasAttempt[attempt.Alias] || attempt.RequestedAt.IsZero() || !reviewSHA256(attempt.RequestSHA256) || (attempt.State != openRouterAttemptReserved && attempt.State != openRouterAttemptAccepted && attempt.State != openRouterAttemptFailed) || attempt.LatencyMS < 0 || attempt.PromptTokens < 0 || attempt.CompletionTokens < 0 || attempt.ChargedNanoUSD < 0 || attempt.ChargedNanoUSD > identity.MaxChargeNanoUSD || (settled && (chargeErr != nil || settledCharge != attempt.ChargedNanoUSD)) || (!settled && (attempt.ChargedAmountUSD != "" || attempt.ChargedNanoUSD != 0)) {
			return fmt.Errorf("OpenRouter review checkpoint attempt ledger is invalid")
		}
		if charged > identity.MaxSpendNanoUSD-attempt.ChargedNanoUSD {
			return fmt.Errorf("OpenRouter review checkpoint exceeds its spend ceiling")
		}
		charged += attempt.ChargedNanoUSD
		if attempt.State == openRouterAttemptAccepted {
			submission, ok := accepted[attempt.Alias]
			call := calls[attempt.Alias]
			_, duplicate := acceptedAttempts[attempt.Alias]
			if !ok || duplicate || attempt.SubmissionSHA256 != submissionSHA256([]fillereval.LabelSubmission{submission}) || call.Attempt != attempt.Attempt || call.RequestSHA256 != attempt.RequestSHA256 || call.GenerationID != attempt.GenerationID || call.LatencyMS != attempt.LatencyMS || call.PromptTokens != attempt.PromptTokens || call.CompletionTokens != attempt.CompletionTokens || call.ChargedAmountUSD != attempt.ChargedAmountUSD || call.ChargedNanoUSD != attempt.ChargedNanoUSD {
				return fmt.Errorf("OpenRouter review checkpoint accepted result hash or call binding is invalid")
			}
			acceptedAttempts[attempt.Alias] = struct{}{}
		} else if attempt.SubmissionSHA256 != "" {
			return fmt.Errorf("OpenRouter review checkpoint non-accepted attempt binds a submission hash")
		}
	}
	if len(acceptedAttempts) != len(accepted) {
		return fmt.Errorf("OpenRouter review checkpoint accepted result has no exact attempt binding")
	}
	return nil
}

func openRouterCheckpointSpend(checkpoint openRouterCheckpoint) (int64, error) {
	var spent int64
	for _, attempt := range checkpoint.Attempts {
		charge := attempt.ChargedNanoUSD
		if attempt.State == openRouterAttemptReserved {
			charge = checkpoint.Identity.MaxChargeNanoUSD
		}
		if spent > checkpoint.Identity.MaxSpendNanoUSD-charge {
			return 0, fmt.Errorf("OpenRouter review checkpoint exhausts its spend ceiling")
		}
		spent += charge
	}
	return spent, nil
}

func acceptedOpenRouterAliases(checkpoint openRouterCheckpoint) map[string]struct{} {
	accepted := make(map[string]struct{}, len(checkpoint.Submissions))
	for _, submission := range checkpoint.Submissions {
		accepted[submission.Alias] = struct{}{}
	}
	return accepted
}

func validateOpenRouterCheckpointOrder(checkpoint openRouterCheckpoint, cases []Case) error {
	attemptIndex := 0
	submissionIndex := 0
	for _, item := range cases {
		if attemptIndex == len(checkpoint.Attempts) {
			break
		}
		if checkpoint.Attempts[attemptIndex].Alias != item.Alias {
			return fmt.Errorf("OpenRouter review checkpoint attempts do not follow package order")
		}
		accepted := 0
		lastState := ""
		for attemptIndex < len(checkpoint.Attempts) && checkpoint.Attempts[attemptIndex].Alias == item.Alias {
			state := checkpoint.Attempts[attemptIndex].State
			if state == openRouterAttemptAccepted {
				accepted++
			}
			lastState = state
			attemptIndex++
		}
		if accepted > 1 || (accepted == 1 && lastState != openRouterAttemptAccepted) {
			return fmt.Errorf("OpenRouter review checkpoint contains duplicate or non-final accepted attempts")
		}
		if accepted == 1 {
			if submissionIndex >= len(checkpoint.Submissions) || checkpoint.Submissions[submissionIndex].Alias != item.Alias {
				return fmt.Errorf("OpenRouter review checkpoint accepted results do not follow package order")
			}
			submissionIndex++
			continue
		}
		if attemptIndex != len(checkpoint.Attempts) {
			return fmt.Errorf("OpenRouter review checkpoint continued beyond a failed alias")
		}
		break
	}
	if attemptIndex != len(checkpoint.Attempts) || submissionIndex != len(checkpoint.Submissions) {
		return fmt.Errorf("OpenRouter review checkpoint contains aliases outside the package sequence")
	}
	return nil
}

func nextOpenRouterAttempt(checkpoint openRouterCheckpoint, alias string) int {
	next := 1
	for _, attempt := range checkpoint.Attempts {
		if attempt.Alias == alias {
			next++
		}
	}
	return next
}

func openRouterCheckpointNow(now func() time.Time) time.Time { return now().UTC() }
