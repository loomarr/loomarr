package fillerbakeoff

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/filleradmission"
	"github.com/loomarr/loomarr/internal/httpx"
)

const (
	OllamaPromptVersion    = "filler-evidence-ollama-v1"
	maxOllamaResponseBytes = 256 << 10
)

type OllamaConfig struct {
	BaseURL     string
	Model       string
	ModelDigest string
	Client      *http.Client
	Now         func() time.Time
}

type ollamaExtractor struct {
	baseURL, model, modelDigest string
	client                      *http.Client
	now                         func() time.Time
}

type ollamaBakeoffRequest struct {
	Model     string                 `json:"model"`
	Stream    bool                   `json:"stream"`
	Think     bool                   `json:"think"`
	Format    map[string]any         `json:"format"`
	KeepAlive string                 `json:"keep_alive"`
	Options   map[string]any         `json:"options"`
	Messages  []ollamaBakeoffMessage `json:"messages"`
}

type ollamaBakeoffMessage struct {
	Role    string   `json:"role"`
	Content string   `json:"content"`
	Images  []string `json:"images,omitempty"`
}

type ollamaBakeoffResponse struct {
	Model   string `json:"model"`
	Message struct {
		Content  string `json:"content"`
		Thinking string `json:"thinking"`
	} `json:"message"`
	DoneReason         string `json:"done_reason"`
	PromptEvalCount    int64  `json:"prompt_eval_count"`
	EvalCount          int64  `json:"eval_count"`
	PromptEvalDuration int64  `json:"prompt_eval_duration"`
	EvalDuration       int64  `json:"eval_duration"`
}

type ollamaTagsResponse struct {
	Models []struct {
		Name   string `json:"name"`
		Model  string `json:"model"`
		Digest string `json:"digest"`
	} `json:"models"`
}

// NewOllamaExtractor binds one local model tag to the exact registry digest
// currently installed before any inference is attempted.
func NewOllamaExtractor(ctx context.Context, config OllamaConfig) (Extractor, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "" || !loopbackHost(parsed.Hostname()) {
		return nil, fmt.Errorf("ollama bakeoff requires a loopback HTTP API base")
	}
	if strings.TrimSpace(config.Model) == "" || strings.Contains(strings.ToLower(config.Model), "latest") || !validSHA256(config.ModelDigest) {
		return nil, fmt.Errorf("ollama bakeoff requires one concrete model tag and SHA-256 digest")
	}
	client := config.Client
	if client == nil {
		client = httpx.NewNamed("filler-bakeoff-ollama", httpx.TimeoutLLM)
	}
	clientCopy := *client
	clientCopy.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	now := config.Now
	if now == nil {
		now = time.Now
	}
	extractor := &ollamaExtractor{baseURL: baseURL, model: config.Model, modelDigest: config.ModelDigest, client: &clientCopy, now: now}
	if err := extractor.verifyInstalledModel(ctx); err != nil {
		return nil, err
	}
	return extractor, nil
}

func (o *ollamaExtractor) verifyInstalledModel(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.baseURL+"/api/tags", nil)
	if err != nil {
		return err
	}
	response, err := o.client.Do(req)
	if err != nil {
		return fmt.Errorf("ollama model preflight: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxOllamaResponseBytes+1))
	if err != nil || len(raw) > maxOllamaResponseBytes || response.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama model preflight failed with status %d", response.StatusCode)
	}
	var tags ollamaTagsResponse
	if err := decodeProviderJSON(raw, &tags); err != nil {
		return fmt.Errorf("decode Ollama model preflight: %w", err)
	}
	for _, model := range tags.Models {
		if (model.Name == o.model || model.Model == o.model) && model.Digest == o.modelDigest {
			return nil
		}
	}
	return fmt.Errorf("ollama model %q is not installed at digest %s", o.model, o.modelDigest)
}

func (o *ollamaExtractor) Extract(ctx context.Context, request Request) (Extraction, error) {
	if err := o.validateRequest(request); err != nil {
		return Extraction{}, err
	}
	messages, signals, derivative, err := openRouterMessages(request)
	if err != nil {
		return Extraction{}, err
	}
	ollamaMessages, err := toOllamaMessages(messages)
	if err != nil {
		return Extraction{}, err
	}
	body, err := json.Marshal(ollamaBakeoffRequest{
		Model: o.model, Stream: false, Think: false, Format: openRouterEvidenceSchema(), KeepAlive: "10m",
		Options:  map[string]any{"temperature": 0, "num_ctx": 4096, "num_predict": maxOpenRouterOutputTokens},
		Messages: ollamaMessages,
	})
	if err != nil {
		return Extraction{}, fmt.Errorf("marshal Ollama bakeoff request: %w", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return Extraction{}, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	started := o.now()
	response, err := o.client.Do(httpRequest)
	latency := o.now().Sub(started)
	if err != nil {
		return Extraction{Derivative: derivative}, fmt.Errorf("ollama bakeoff request: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxOllamaResponseBytes+1))
	if err != nil || len(raw) > maxOllamaResponseBytes {
		return Extraction{Derivative: derivative}, fmt.Errorf("ollama bakeoff response exceeded its byte ceiling")
	}
	if response.StatusCode != http.StatusOK {
		return Extraction{Derivative: derivative}, fmt.Errorf("ollama bakeoff returned status %d: %s", response.StatusCode, boundedMessage(raw))
	}
	var wire ollamaBakeoffResponse
	if err := decodeProviderJSON(raw, &wire); err != nil {
		return Extraction{Derivative: derivative}, fmt.Errorf("decode Ollama bakeoff response: %w", err)
	}
	idSum := sha256.Sum256(append(body, raw...))
	id := "ollama-" + hex.EncodeToString(idSum[:12])
	latencyMS := max(int64(0), latency.Milliseconds())
	extraction := Extraction{Derivative: derivative, Attribution: filleradmission.Attribution{
		EvaluationID: id, GenerationID: id, Role: request.Route.Role, Rung: request.Route.Rung,
		RequestedProvider: "ollama", RequestedModel: o.model, ResolvedProvider: "ollama",
		ResolvedModel: o.model + "@sha256:" + o.modelDigest, Modalities: slices.Clone(request.Route.Modalities),
		Attempts: 1, LatencyMS: latencyMS, Tokens: filleradmission.TokenUsage{Prompt: wire.PromptEvalCount, Completion: wire.EvalCount},
	}}
	if wire.Model != o.model || wire.DoneReason != "stop" || wire.Message.Thinking != "" || strings.TrimSpace(wire.Message.Content) == "" {
		return extraction, fmt.Errorf("ollama returned an unbound, truncated, thinking, or empty response")
	}
	var output openRouterOutput
	if err := decodeStrictJSON([]byte(wire.Message.Content), &output); err != nil {
		return extraction, fmt.Errorf("decode Ollama structured evidence: %w", err)
	}
	if err := validateOpenRouterOutput(output); err != nil {
		return extraction, err
	}
	if output.AbstentionReason != "" {
		extraction.Attribution.Abstained = true
		extraction.Attribution.AbstentionReason = output.AbstentionReason
		return extraction, nil
	}
	for index, fact := range output.Facts {
		signal, ok := signals[fact.SignalID]
		if !ok {
			return extraction, fmt.Errorf("ollama evidence references unknown signal %q", fact.SignalID)
		}
		extraction.Evidence = append(extraction.Evidence, filleradmission.Evidence{
			ID: fmt.Sprintf("%s-fact-%02d", id, index+1), Claim: fact.Claim, Value: fact.Value,
			Kind: filleradmission.EvidenceKind(signal.Kind), Source: "signal:" + signal.ID,
			Derivative: signal.ID, Location: fact.Location, AtMS: signal.AtMS, EvaluationID: id,
		})
	}
	return extraction, nil
}

func (o *ollamaExtractor) validateRequest(request Request) error {
	route := request.Route
	baseValid := route.Provider == "ollama" && route.Model == o.model && route.StructuredOutput && !route.RequireZDR && !route.AllowFallbacks && route.UpstreamProvider == "" && route.UpstreamProviderSlug == ""
	textRoute := route.Class == RouteText && slices.Equal(route.Modalities, []string{"text"})
	frameRoute := route.Class == RouteFrames && route.Role == "filler_frames" && slices.Contains(route.Modalities, "image") && !slices.Contains(route.Modalities, "audio") && !slices.Contains(route.Modalities, "video")
	if !baseValid || (!textRoute && !frameRoute) {
		return fmt.Errorf("ollama bakeoff requires one digest-pinned local text or frame route")
	}
	if len(request.Packet.Signals) == 0 {
		return fmt.Errorf("ollama bakeoff request carries no signals")
	}
	return nil
}

func toOllamaMessages(messages []openRouterMessage) ([]ollamaBakeoffMessage, error) {
	if len(messages) != 2 {
		return nil, fmt.Errorf("ollama bakeoff requires one system and one user message")
	}
	converted := make([]ollamaBakeoffMessage, 0, len(messages))
	for _, message := range messages {
		var content []string
		var images []string
		for _, part := range message.Content {
			switch part.Type {
			case "text":
				content = append(content, part.Text)
			case "image_url":
				if part.ImageURL == nil {
					return nil, fmt.Errorf("ollama frame part has no image data")
				}
				encoded, ok := strings.CutPrefix(part.ImageURL.URL, "data:image/jpeg;base64,")
				if !ok || encoded == "" {
					return nil, fmt.Errorf("ollama frame part is not inline certified JPEG")
				}
				images = append(images, encoded)
			default:
				return nil, fmt.Errorf("ollama bakeoff does not support %q parts", part.Type)
			}
		}
		converted = append(converted, ollamaBakeoffMessage{Role: message.Role, Content: strings.Join(content, "\n"), Images: images})
	}
	if converted[0].Role != "system" || converted[1].Role != "user" || converted[0].Content == "" || converted[1].Content == "" || len(converted[0].Images) != 0 {
		return nil, fmt.Errorf("ollama bakeoff received an invalid message envelope")
	}
	return converted, nil
}

var _ Extractor = (*ollamaExtractor)(nil)
