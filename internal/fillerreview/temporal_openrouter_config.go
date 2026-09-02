package fillerreview

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/fillerbakeoff"
	"github.com/loomarr/loomarr/internal/httpx"
)

func validateOpenRouterTemporalConfig(config OpenRouterTemporalConfig, requireSelection bool) (string, *http.Client, func() time.Time, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		baseURL = fillerbakeoff.OpenRouterBaseURL
	}
	parsed, err := url.Parse(baseURL)
	loopback := err == nil && config.AllowInsecureTestURL && reviewLoopback(parsed.Hostname())
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (!loopback && (parsed.Scheme != "https" || parsed.Hostname() != "openrouter.ai" || parsed.Path != "/api/v1")) {
		return "", nil, nil, fmt.Errorf("OpenRouter temporal assessment requires the canonical HTTPS API base")
	}
	if config.APIKey == "" || config.PackagePath == "" || requireSelection && config.SelectionPath == "" || config.CheckpointDir == "" || config.Model == "" || strings.Contains(strings.ToLower(config.Model), "latest") || config.ModelFamily == "" || config.UpstreamProvider == "" || config.UpstreamProviderSlug == "" || config.AssessorID == "" || config.ExpectedPackageCases <= 0 || config.ExpectedCalibrationCases <= 0 || config.PerCaseTimeout <= 0 || config.MaxRequests < config.ExpectedCalibrationCases || config.MaxRequests > config.ExpectedCalibrationCases*2 || config.MaxSpendNanoUSD <= 0 || config.MaxChargeNanoUSD <= 0 || config.MaxChargeNanoUSD > config.MaxSpendNanoUSD {
		return "", nil, nil, fmt.Errorf("OpenRouter temporal assessment requires exact identity and positive package, case, request, charge, spend, and timeout ceilings")
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	if err := validateOpenRouterTemporalSnapshot(config, baseURL, now().UTC()); err != nil {
		return "", nil, nil, err
	}
	client := config.Client
	if client == nil {
		client = httpx.NewNamed("filler-temporal-openrouter", httpx.TimeoutLLM)
	}
	copy := *client
	copy.Timeout = 0
	copy.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return baseURL, &copy, now, nil
}

func validateOpenRouterTemporalSnapshot(config OpenRouterTemporalConfig, baseURL string, now time.Time) error {
	if err := fillerbakeoff.ValidateOpenRouterSnapshot(config.Snapshot); err != nil {
		return err
	}
	if config.Snapshot.SourceBaseURL != baseURL {
		return fmt.Errorf("OpenRouter temporal snapshot does not bind the request base")
	}
	age := now.Sub(config.Snapshot.RetrievedAt)
	if age < 0 || age > 24*time.Hour {
		return fmt.Errorf("OpenRouter temporal assessment is outside the snapshot's 24-hour window")
	}
	model := openRouterTemporalModel(config.Snapshot, config.Model)
	if model.ID == "" || !slices.Contains(model.InputModalities, "text") || !slices.Contains(model.InputModalities, "image") {
		return fmt.Errorf("OpenRouter temporal model is absent or lacks text/image input")
	}
	for _, endpoint := range model.Endpoints {
		if endpoint.ProviderName == config.UpstreamProvider && endpoint.ProviderSlug == config.UpstreamProviderSlug && endpoint.ZDR && endpoint.Status == 0 && endpoint.MaxCompletionTokens >= 1024 && slices.Contains(endpoint.SupportedParameters, "response_format") && slices.Contains(endpoint.SupportedParameters, "structured_outputs") && slices.Contains(endpoint.SupportedParameters, "reasoning") {
			return nil
		}
	}
	return fmt.Errorf("OpenRouter temporal route is absent, non-ZDR, or lacks strict structured output with explicit reasoning control")
}

func openRouterTemporalModel(snapshot fillerbakeoff.OpenRouterSnapshot, modelID string) fillerbakeoff.OpenRouterModelSnapshot {
	for _, model := range snapshot.Models {
		if model.ID == modelID {
			return model
		}
	}
	return fillerbakeoff.OpenRouterModelSnapshot{}
}
