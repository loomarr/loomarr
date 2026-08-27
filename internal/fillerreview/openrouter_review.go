package fillerreview

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/fillerbakeoff"
	"github.com/loomarr/loomarr/internal/fillereval"
	"github.com/loomarr/loomarr/internal/httpx"
)

const OpenRouterReviewPromptVersion = "filler-blind-review-openrouter-v7"

type OpenRouterReviewConfig struct {
	PackageDir           string
	Transcripts          []fillerbakeoff.TranscriptArtifact
	BaseURL              string
	APIKey               string
	Snapshot             fillerbakeoff.OpenRouterSnapshot
	Model                string
	UpstreamProvider     string
	UpstreamProviderSlug string
	ReviewerID           string
	ExpectedCases        int
	PerCaseTimeout       time.Duration
	MaxRequests          int
	MaxSpendNanoUSD      int64
	MaxChargeNanoUSD     int64
	AllowInsecureTestURL bool
	Client               *http.Client
	Now                  func() time.Time
}

type openRouterReviewRequest struct {
	Model          string                    `json:"model"`
	Messages       []openRouterReviewMessage `json:"messages"`
	Provider       openRouterReviewRoute     `json:"provider"`
	ResponseFormat openRouterReviewFormat    `json:"response_format"`
	MaxTokens      int                       `json:"max_tokens"`
}

type openRouterReviewMessage struct {
	Role    string                 `json:"role"`
	Content []openRouterReviewPart `json:"content"`
}

type openRouterReviewPart struct {
	Type     string                    `json:"type"`
	Text     string                    `json:"text,omitempty"`
	ImageURL *openRouterReviewMediaURL `json:"image_url,omitempty"`
}

type openRouterReviewMediaURL struct {
	URL string `json:"url"`
}

type openRouterReviewRoute struct {
	Order             []string `json:"order"`
	AllowFallbacks    bool     `json:"allow_fallbacks"`
	RequireParameters bool     `json:"require_parameters"`
	DataCollection    string   `json:"data_collection"`
	ZDR               bool     `json:"zdr"`
}

type openRouterReviewFormat struct {
	Type       string                     `json:"type"`
	JSONSchema openRouterReviewJSONSchema `json:"json_schema"`
}

type openRouterReviewJSONSchema struct {
	Name   string         `json:"name"`
	Strict bool           `json:"strict"`
	Schema map[string]any `json:"schema"`
}

type openRouterReviewResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content   string `json:"content"`
			Reasoning string `json:"reasoning"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int64       `json:"prompt_tokens"`
		CompletionTokens int64       `json:"completion_tokens"`
		Cost             json.Number `json:"cost"`
	} `json:"usage"`
	Metadata struct {
		Attempt   int `json:"attempt"`
		Endpoints struct {
			Available []struct {
				Provider string `json:"provider"`
				Model    string `json:"model"`
				Selected bool   `json:"selected"`
			} `json:"available"`
		} `json:"endpoints"`
		Attempts []struct {
			Provider string `json:"provider"`
			Model    string `json:"model"`
			Status   int    `json:"status"`
		} `json:"attempts"`
	} `json:"openrouter_metadata"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func RunOpenRouterReview(ctx context.Context, config OpenRouterReviewConfig) (ReviewRun, []fillereval.LabelSubmission, error) {
	baseURL, client, now, err := validateOpenRouterReviewConfig(config)
	if err != nil {
		return ReviewRun{}, nil, err
	}
	manifestPath, err := resolveWithin(config.PackageDir, "manifest.json")
	if err != nil {
		return ReviewRun{}, nil, err
	}
	manifest, err := readStrictJSON[Package](manifestPath)
	if err != nil {
		return ReviewRun{}, nil, fmt.Errorf("read review manifest: %w", err)
	}
	if err := validateReviewPackage(config.PackageDir, manifest, config.ExpectedCases); err != nil {
		return ReviewRun{}, nil, err
	}
	transcripts, transcriptSetSHA256, err := indexReviewTranscripts(config.Transcripts, manifest)
	if err != nil {
		return ReviewRun{}, nil, err
	}
	manifestSHA256, err := hashFile(manifestPath)
	if err != nil {
		return ReviewRun{}, nil, err
	}
	run := ReviewRun{
		SchemaVersion: ReviewRunSchemaVersion, BatchID: manifest.BatchID, PackageManifestSHA256: manifestSHA256,
		ReviewerID: config.ReviewerID, Provider: "openrouter", Model: config.Model, ResolvedModel: openRouterReviewModel(config.Snapshot, config.Model).CanonicalSlug,
		UpstreamProvider: config.UpstreamProvider, UpstreamProviderSlug: config.UpstreamProviderSlug,
		PromptVersion:            OpenRouterReviewPromptVersion,
		CapabilitySnapshotSHA256: fillerbakeoff.OpenRouterSnapshotSHA256(config.Snapshot),
		TranscriptSetSHA256:      transcriptSetSHA256, Cases: len(manifest.Cases),
	}
	submissions := make([]fillereval.LabelSubmission, 0, len(manifest.Cases))
	for _, item := range manifest.Cases {
		if run.Requests >= config.MaxRequests || int64(run.Requests+1)*config.MaxChargeNanoUSD > config.MaxSpendNanoUSD {
			return ReviewRun{}, nil, fmt.Errorf("openrouter review request or spend reservation exhausted before alias %q", item.Alias)
		}
		caseCtx, cancel := context.WithTimeout(ctx, config.PerCaseTimeout)
		started := time.Now()
		labels, wire, charged, reviewErr := reviewOneOpenRouter(caseCtx, client, baseURL, config, manifest, item, transcripts)
		latencyMS := max(int64(0), time.Since(started).Milliseconds())
		cancel()
		if reviewErr != nil {
			return ReviewRun{}, nil, fmt.Errorf("review alias %q: %w", item.Alias, reviewErr)
		}
		if run.ChargedNanoUSD > config.MaxSpendNanoUSD-charged {
			return ReviewRun{}, nil, fmt.Errorf("openrouter review exceeded its spend ceiling")
		}
		run.Requests++
		run.PromptTokens += wire.Usage.PromptTokens
		run.CompletionTokens += wire.Usage.CompletionTokens
		run.TotalLatencyMS += latencyMS
		run.ChargedNanoUSD += charged
		reviewedAt := now().UTC()
		run.Calls = append(run.Calls, ReviewCall{
			Alias: item.Alias, ReviewedAt: reviewedAt, GenerationID: wire.ID, LatencyMS: latencyMS,
			PromptTokens: wire.Usage.PromptTokens, CompletionTokens: wire.Usage.CompletionTokens,
			ChargedAmountUSD: wire.Usage.Cost.String(), ChargedNanoUSD: charged,
		})
		submissions = append(submissions, fillereval.LabelSubmission{
			Alias: item.Alias, ReviewerID: config.ReviewerID, BatchID: manifest.BatchID,
			ReviewedAt: reviewedAt, Labels: fillereval.NormalizeLabels(labels),
		})
	}
	run.CompletedAt = now().UTC()
	run.SubmissionSHA256 = submissionSHA256(submissions)
	return run, submissions, nil
}

func openRouterReviewModel(snapshot fillerbakeoff.OpenRouterSnapshot, modelID string) fillerbakeoff.OpenRouterModelSnapshot {
	for _, model := range snapshot.Models {
		if model.ID == modelID {
			return model
		}
	}
	return fillerbakeoff.OpenRouterModelSnapshot{}
}

func validateOpenRouterReviewConfig(config OpenRouterReviewConfig) (string, *http.Client, func() time.Time, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		baseURL = fillerbakeoff.OpenRouterBaseURL
	}
	parsed, err := url.Parse(baseURL)
	loopback := err == nil && config.AllowInsecureTestURL && reviewLoopback(parsed.Hostname())
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (!loopback && (parsed.Scheme != "https" || parsed.Hostname() != "openrouter.ai" || parsed.Path != "/api/v1")) {
		return "", nil, nil, fmt.Errorf("openrouter blind review requires the canonical HTTPS API base")
	}
	if config.APIKey == "" || config.Model == "" || config.UpstreamProvider == "" || config.UpstreamProviderSlug == "" || config.ReviewerID == "" || config.ExpectedCases <= 0 || config.MaxRequests != config.ExpectedCases || config.MaxSpendNanoUSD <= 0 || config.MaxChargeNanoUSD <= 0 || int64(config.MaxRequests) > config.MaxSpendNanoUSD/config.MaxChargeNanoUSD || config.PerCaseTimeout <= 0 {
		return "", nil, nil, fmt.Errorf("openrouter blind review requires exact identity and positive request, charge, spend, and timeout ceilings")
	}
	if err := validateOpenRouterReviewSnapshot(config, baseURL); err != nil {
		return "", nil, nil, err
	}
	client := config.Client
	if client == nil {
		client = httpx.NewNamed("filler-review-openrouter", httpx.TimeoutLLM)
	}
	copy := *client
	copy.Timeout = 0
	copy.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return baseURL, &copy, now, nil
}

func validateOpenRouterReviewSnapshot(config OpenRouterReviewConfig, baseURL string) error {
	if err := fillerbakeoff.ValidateOpenRouterSnapshot(config.Snapshot); err != nil {
		return err
	}
	if config.Snapshot.SourceBaseURL != baseURL {
		return fmt.Errorf("openrouter review snapshot does not bind the request base")
	}
	now := time.Now
	if config.Now != nil {
		now = config.Now
	}
	age := now().UTC().Sub(config.Snapshot.RetrievedAt)
	if age < 0 || age > 24*time.Hour {
		return fmt.Errorf("openrouter review is outside the snapshot's 24-hour window")
	}
	for _, model := range config.Snapshot.Models {
		if model.ID != config.Model {
			continue
		}
		if !slices.Contains(model.InputModalities, "text") || !slices.Contains(model.InputModalities, "image") {
			return fmt.Errorf("openrouter reviewer model lacks text or image input")
		}
		for _, endpoint := range model.Endpoints {
			if endpoint.ProviderName == config.UpstreamProvider && endpoint.ProviderSlug == config.UpstreamProviderSlug && endpoint.ZDR && endpoint.Status == 0 && endpoint.MaxCompletionTokens >= 4096 && slices.Contains(endpoint.SupportedParameters, "response_format") && slices.Contains(endpoint.SupportedParameters, "structured_outputs") {
				return nil
			}
		}
	}
	return fmt.Errorf("openrouter reviewer route is absent, non-ZDR, or lacks strict structured output")
}

func reviewOneOpenRouter(ctx context.Context, client *http.Client, baseURL string, config OpenRouterReviewConfig, manifest Package, item Case, transcripts map[string]fillerbakeoff.TranscriptArtifact) (fillereval.Labels, openRouterReviewResponse, int64, error) {
	content, images, err := reviewerContent(config.PackageDir, manifest, item, transcripts)
	if err != nil {
		return fillereval.Labels{}, openRouterReviewResponse{}, 0, err
	}
	parts := []openRouterReviewPart{{Type: "text", Text: content}}
	for _, image := range images {
		parts = append(parts, openRouterReviewPart{Type: "image_url", ImageURL: &openRouterReviewMediaURL{URL: "data:image/jpeg;base64," + image}})
	}
	payload := openRouterReviewRequest{
		Model: config.Model,
		Messages: []openRouterReviewMessage{
			{Role: "system", Content: []openRouterReviewPart{{Type: "text", Text: reviewerSystemPrompt}}},
			{Role: "user", Content: parts},
		},
		Provider:       openRouterReviewRoute{Order: []string{config.UpstreamProviderSlug}, RequireParameters: true, DataCollection: "deny", ZDR: true},
		ResponseFormat: openRouterReviewFormat{Type: "json_schema", JSONSchema: openRouterReviewJSONSchema{Name: "filler_blind_review", Strict: true, Schema: reviewLabelsSchema(item)}},
		MaxTokens:      4096,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fillereval.Labels{}, openRouterReviewResponse{}, 0, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return fillereval.Labels{}, openRouterReviewResponse{}, 0, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+config.APIKey)
	request.Header.Set("X-OpenRouter-Metadata", "enabled")
	request.Header.Set("HTTP-Referer", "https://github.com/loomarr/loomarr")
	request.Header.Set("X-OpenRouter-Title", "Loomarr filler blind review")
	response, err := client.Do(request)
	if err != nil {
		return fillereval.Labels{}, openRouterReviewResponse{}, 0, err
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxReviewResponseBytes+1))
	if err != nil || len(raw) > maxReviewResponseBytes {
		return fillereval.Labels{}, openRouterReviewResponse{}, 0, fmt.Errorf("openrouter reviewer response exceeded its byte ceiling")
	}
	if response.StatusCode != http.StatusOK {
		return fillereval.Labels{}, openRouterReviewResponse{}, 0, fmt.Errorf("openrouter reviewer returned status %d: %s", response.StatusCode, boundedReviewMessage(raw))
	}
	var wire openRouterReviewResponse
	if err := decodeProviderReviewJSON(raw, &wire); err != nil {
		return fillereval.Labels{}, wire, 0, err
	}
	if wire.Error != nil {
		return fillereval.Labels{}, wire, 0, fmt.Errorf("openrouter reviewer error: %s", strings.TrimSpace(wire.Error.Message))
	}
	charged, err := fillereval.USDToNanoCeil(wire.Usage.Cost.String())
	if err != nil || charged < 0 || charged > config.MaxChargeNanoUSD {
		return fillereval.Labels{}, wire, 0, fmt.Errorf("openrouter reviewer returned missing or out-of-reservation cost")
	}
	if wire.ID == "" || wire.Model != config.Model || len(wire.Choices) != 1 || wire.Metadata.Attempt != 1 || !validReviewAttemptLedger(wire, config) || !selectedReviewEndpoint(wire, config) {
		return fillereval.Labels{}, wire, charged, fmt.Errorf("openrouter reviewer response does not bind the requested one-attempt route (generation=%t model=%q choices=%d attempt=%d attempts=%s selected=%s)", wire.ID != "", wire.Model, len(wire.Choices), wire.Metadata.Attempt, reviewAttemptSummary(wire), reviewEndpointSummary(wire))
	}
	labels, err := decodeReviewLabels([]byte(wire.Choices[0].Message.Content))
	if err != nil {
		return fillereval.Labels{}, wire, charged, fmt.Errorf("decode openrouter review labels: %w (contentBytes=%d reasoningBytes=%d)", err, len(wire.Choices[0].Message.Content), len(wire.Choices[0].Message.Reasoning))
	}
	if failures := fillereval.ValidateLabels(labels); len(failures) > 0 {
		return fillereval.Labels{}, wire, charged, fmt.Errorf("invalid review labels: %s", strings.Join(failures, "; "))
	}
	if err := validateReviewEvidence(item, labels.Evidence, transcripts); err != nil {
		return fillereval.Labels{}, wire, charged, err
	}
	return labels, wire, charged, nil
}

func validReviewAttemptLedger(wire openRouterReviewResponse, config OpenRouterReviewConfig) bool {
	if len(wire.Metadata.Attempts) == 0 {
		return true
	}
	if len(wire.Metadata.Attempts) != 1 {
		return false
	}
	attempt := wire.Metadata.Attempts[0]
	return attempt.Provider == config.UpstreamProvider && attempt.Model == openRouterReviewModel(config.Snapshot, config.Model).CanonicalSlug && attempt.Status >= 200 && attempt.Status < 300
}

func reviewAttemptSummary(wire openRouterReviewResponse) string {
	parts := make([]string, 0, len(wire.Metadata.Attempts))
	for _, attempt := range wire.Metadata.Attempts {
		parts = append(parts, fmt.Sprintf("%q/%q/%d", attempt.Provider, attempt.Model, attempt.Status))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func reviewEndpointSummary(wire openRouterReviewResponse) string {
	parts := make([]string, 0, len(wire.Metadata.Endpoints.Available))
	for _, endpoint := range wire.Metadata.Endpoints.Available {
		if endpoint.Selected {
			parts = append(parts, fmt.Sprintf("%q/%q", endpoint.Provider, endpoint.Model))
		}
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func selectedReviewEndpoint(wire openRouterReviewResponse, config OpenRouterReviewConfig) bool {
	selected := 0
	for _, endpoint := range wire.Metadata.Endpoints.Available {
		if !endpoint.Selected {
			continue
		}
		selected++
		if endpoint.Provider != config.UpstreamProvider || endpoint.Model != openRouterReviewModel(config.Snapshot, config.Model).CanonicalSlug {
			return false
		}
	}
	return selected == 1
}
