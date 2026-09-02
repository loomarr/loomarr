package fillerreview

import (
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/fillerbakeoff"
	"github.com/loomarr/loomarr/internal/fillereval"
	"github.com/loomarr/loomarr/internal/httpx"
)

func validateTemporalStructureOpenRouterConfig(config TemporalStructureOpenRouterConfig) (string, *http.Client, func() time.Time, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		baseURL = fillerbakeoff.OpenRouterBaseURL
	}
	parsed, err := url.Parse(baseURL)
	loopback := err == nil && config.AllowInsecureTestURL && reviewLoopback(parsed.Hostname())
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (!loopback && (parsed.Scheme != "https" || parsed.Hostname() != "openrouter.ai" || parsed.Path != "/api/v1")) {
		return "", nil, nil, fmt.Errorf("OpenRouter structure assessment requires the canonical HTTPS API base")
	}
	if config.APIKey == "" || strings.TrimSpace(config.PublicManifestPath) == "" || strings.TrimSpace(config.CheckpointDir) == "" || strings.TrimSpace(config.Model) == "" || strings.Contains(strings.ToLower(config.Model), "latest") || strings.TrimSpace(config.ModelFamily) == "" || strings.TrimSpace(config.UpstreamProvider) == "" || strings.TrimSpace(config.UpstreamProviderSlug) == "" || strings.TrimSpace(config.AssessorID) == "" || !validTemporalStructureOpenRouterReasoningMode(config.ReasoningMode) || config.ExpectedCases <= 0 || config.MaxRequests != config.ExpectedCases || config.PerCaseTimeout <= 0 || config.MaxSpendNanoUSD <= 0 || config.MaxChargeNanoUSD <= 0 || config.MaxChargeNanoUSD > config.MaxSpendNanoUSD {
		return "", nil, nil, fmt.Errorf("OpenRouter structure assessment requires exact identity and one bounded request per expected case")
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	if err := validateTemporalStructureOpenRouterSnapshot(config, baseURL, now().UTC()); err != nil {
		return "", nil, nil, err
	}
	client := config.Client
	if client == nil {
		client = httpx.NewNamed("filler-temporal-structure-openrouter", httpx.TimeoutLLM)
	}
	copy := *client
	copy.Timeout = 0
	copy.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return baseURL, &copy, now, nil
}

func validateTemporalStructureOpenRouterSnapshot(config TemporalStructureOpenRouterConfig, baseURL string, now time.Time) error {
	if err := fillerbakeoff.ValidateOpenRouterSnapshot(config.Snapshot); err != nil {
		return err
	}
	if config.Snapshot.SourceBaseURL != baseURL {
		return fmt.Errorf("OpenRouter structure snapshot does not bind the request base")
	}
	age := now.Sub(config.Snapshot.RetrievedAt)
	if age < 0 || age > 24*time.Hour {
		return fmt.Errorf("OpenRouter structure assessment is outside the snapshot's 24-hour window")
	}
	model := openRouterTemporalModel(config.Snapshot, config.Model)
	if model.ID == "" || !slices.Contains(model.InputModalities, "text") || !slices.Contains(model.InputModalities, "video") {
		return fmt.Errorf("OpenRouter structure model is absent or lacks text/video input")
	}
	for _, endpoint := range model.Endpoints {
		if endpoint.ProviderName == config.UpstreamProvider && endpoint.ProviderSlug == config.UpstreamProviderSlug && endpoint.ZDR && endpoint.Status == 0 && endpoint.MaxCompletionTokens >= temporalStructureOpenRouterMaxTokens && slices.Contains(endpoint.SupportedParameters, "response_format") && slices.Contains(endpoint.SupportedParameters, "structured_outputs") && slices.Contains(endpoint.SupportedParameters, "reasoning") {
			return nil
		}
	}
	return fmt.Errorf("OpenRouter structure route is absent, non-ZDR, or lacks strict structured output with explicit reasoning control")
}

func validateTemporalStructureOpenRouterResult(result TemporalStructureOpenRouterResult, manifest TemporalStructureChallengeManifest, selected []TemporalStructureChallengePublicCase) error {
	if result.SchemaVersion != TemporalStructureOpenRouterResultSchemaVersion || result.ContractVersion != TemporalStructureOpenRouterResultContract || result.ChallengeID != manifest.ChallengeID || !reviewSHA256(result.PublicManifestSHA256) || !reviewSHA256(result.SelectionSHA256) || !reviewSHA256(result.CapabilitySnapshotSHA256) || result.PromptSHA256 != temporalStructureOpenRouterPromptSHA256() || result.ResolvedModel == "" || result.UpstreamProvider == "" || result.UpstreamProviderSlug == "" || !validTemporalStructureOpenRouterReasoningMode(result.ReasoningMode) || result.Assessor.ID == "" || result.Assessor.Provider != "openrouter" || result.Assessor.Model == "" || strings.Contains(strings.ToLower(result.Assessor.Model), "latest") || result.Assessor.ModelFamily == "" || result.Assessor.ModelDigest != result.CapabilitySnapshotSHA256 || result.Assessor.PromptVersion != TemporalStructureOpenRouterPromptVersion || result.Requests != len(selected) || result.Requests != len(result.Attempts) || len(result.Assessments) != len(selected) || result.MaxRequests != len(selected) || result.MaxSpendNanoUSD <= 0 || result.MaxChargeNanoUSD <= 0 || result.MaxChargeNanoUSD > result.MaxSpendNanoUSD || result.ChargedNanoUSD < 0 || result.ConsumedNanoUSD < result.ChargedNanoUSD || result.ConsumedNanoUSD > result.MaxSpendNanoUSD || result.CompletedAt.Before(manifest.GeneratedAt) || result.ProductionAdmissionAllowed || len(result.SelectionAliases) != len(selected) {
		return fmt.Errorf("OpenRouter structure result identity, counts, accounting, or admission boundary is invalid")
	}
	if result.SelectionSHA256 != temporalTruthJSONSHA(result.SelectionAliases) || result.UnknownChargeReservations < 0 || result.UnknownChargeReservations > result.Requests {
		return fmt.Errorf("OpenRouter structure result selection or unknown reservations are invalid")
	}
	if err := validateTemporalStructureOpenRouterResultAttempts(result, manifest.GeneratedAt); err != nil {
		return err
	}
	for index, item := range selected {
		assessment := result.Assessments[index]
		attempt := result.Attempts[index]
		if result.SelectionAliases[index] != item.Alias || assessment.Alias != item.Alias || attempt.Alias != item.Alias {
			return fmt.Errorf("OpenRouter structure result is not an ordered selection")
		}
		if err := validateTemporalStructureAssessment(assessment, item.Video.DurationMS, manifest.GeneratedAt, result.CompletedAt); err != nil {
			return err
		}
		call := assessment.Inference.Calls[0]
		if call.ResponseSHA256 != attempt.ResponseSHA256 || call.LatencyMS != attempt.LatencyMS || call.PromptTokens != attempt.PromptTokens || call.CompletionTokens != attempt.CompletionTokens || call.OperationalFailure != attempt.OperationalFailure {
			return fmt.Errorf("OpenRouter structure result assessment and attempt drift")
		}
	}
	return nil
}

func validateTemporalStructureOpenRouterResultAttempts(result TemporalStructureOpenRouterResult, generatedAt time.Time) error {
	var charged, consumed int64
	unknown := 0
	for index, attempt := range result.Attempts {
		if attempt.RequestedAt.Before(generatedAt) || attempt.RequestedAt.After(result.CompletedAt) || !reviewSHA256(attempt.RequestSHA256) || attempt.ReservedNanoUSD != result.MaxChargeNanoUSD || attempt.LatencyMS < 0 || attempt.PromptTokens < 0 || attempt.CompletionTokens < 0 || attempt.ChargedNanoUSD < 0 || attempt.ChargedNanoUSD > attempt.ReservedNanoUSD {
			return fmt.Errorf("OpenRouter structure result attempt %d has invalid identity or accounting", index)
		}
		settled := attempt.State == temporalOpenRouterAttemptAccepted || attempt.State == temporalOpenRouterAttemptFailed
		unsettled := attempt.State == temporalOpenRouterAttemptUnsettled
		settledNanoUSD, chargeErr := fillereval.USDToNanoCeil(attempt.ChargedAmountUSD)
		if !settled && !unsettled || settled && (attempt.ChargedAmountUSD == "" || chargeErr != nil || settledNanoUSD != attempt.ChargedNanoUSD || !reviewSHA256(attempt.ResponseSHA256) || attempt.RawResponsePath == "") || unsettled && (attempt.ChargedAmountUSD != "" || attempt.ChargedNanoUSD != 0) || attempt.State == temporalOpenRouterAttemptAccepted && attempt.OperationalFailure != "" || (attempt.State == temporalOpenRouterAttemptFailed || attempt.State == temporalOpenRouterAttemptUnsettled) && !validTemporalOpenRouterFailure(attempt.OperationalFailure) {
			return fmt.Errorf("OpenRouter structure result attempt %d has invalid terminal settlement", index)
		}
		if attempt.ResponseSHA256 == "" {
			if !unsettled || attempt.RawResponsePath != "" {
				return fmt.Errorf("OpenRouter structure result attempt %d has no bound response", index)
			}
		} else if !reviewSHA256(attempt.ResponseSHA256) || attempt.RawResponsePath != filepath.ToSlash(filepath.Join(temporalStructureOpenRouterResponsesDir, attempt.Alias+".json")) {
			return fmt.Errorf("OpenRouter structure result attempt %d has invalid response authority", index)
		}
		if charged > result.MaxSpendNanoUSD-attempt.ChargedNanoUSD {
			return fmt.Errorf("OpenRouter structure result charged spend exceeds the ceiling")
		}
		charged += attempt.ChargedNanoUSD
		cost := attempt.ChargedNanoUSD
		if unsettled {
			unknown++
			cost = attempt.ReservedNanoUSD
		}
		if consumed > result.MaxSpendNanoUSD-cost {
			return fmt.Errorf("OpenRouter structure result attempts exceed the spend ceiling")
		}
		consumed += cost
	}
	if charged != result.ChargedNanoUSD || consumed != result.ConsumedNanoUSD || unknown != result.UnknownChargeReservations {
		return fmt.Errorf("OpenRouter structure result aggregate spend or reservation accounting drift")
	}
	return nil
}

func validTemporalStructureOpenRouterReasoningMode(value string) bool {
	return value == TemporalStructureOpenRouterReasoningDisabled || value == TemporalStructureOpenRouterReasoningRequired
}
