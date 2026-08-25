package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/httpx"
	"github.com/loomarr/loomarr/internal/metrics"
)

// VideoProvider is the narrow direct-video evidence seam (§10 V61). It is separate
// from text chat and frame vision because support must be explicitly certified.
type VideoProvider interface {
	AskAboutVideo(ctx context.Context, prompt string, video VideoInput) (Response, error)
}

const (
	// MaxHostedVideoBytes is the pre-base64 upload ceiling from §10 V61.
	MaxHostedVideoBytes         = 12 << 20
	MaxHostedVideoResponseBytes = 256 << 10
	maxHostedVideoMS            = int64(60_000)
)

// VideoInput is one bounded derivative. URL is represented so incompatible
// transports can be rejected explicitly; Loomarr's OpenRouter adapter accepts
// only inline base64 data for local/private filler.
type VideoInput struct {
	MIMEType   string
	Data       []byte
	URL        string
	DurationMS int64
}

type VideoTransport string

const VideoTransportBase64 VideoTransport = "base64"

// OpenRouterVideoConfig proves the exact model and route selected for a
// certification request. Model capability comes from live hosted metadata.
type OpenRouterVideoConfig struct {
	ProviderKey      string
	Transport        VideoTransport
	BaseURL          string
	APIKey           string
	Model            HostedModel
	UpstreamProvider string
}

type openRouterVideo struct {
	baseURL          string
	apiKey           string
	model            string
	upstreamProvider string
	http             *http.Client
}

type openRouterVideoRequest struct {
	Model     string                   `json:"model"`
	Messages  []openRouterVideoMessage `json:"messages"`
	MaxTokens int                      `json:"max_tokens"`
	Provider  openRouterVideoRoute     `json:"provider"`
}

type openRouterVideoMessage struct {
	Role    string                `json:"role"`
	Content []openRouterVideoPart `json:"content"`
}

type openRouterVideoPart struct {
	Type     string              `json:"type"`
	Text     string              `json:"text,omitempty"`
	VideoURL *openRouterVideoURL `json:"video_url,omitempty"`
}

type openRouterVideoURL struct {
	URL string `json:"url"`
}

type openRouterVideoRoute struct {
	Order             []string `json:"order"`
	AllowFallbacks    bool     `json:"allow_fallbacks"`
	RequireParameters bool     `json:"require_parameters"`
	DataCollection    string   `json:"data_collection"`
	ZDR               bool     `json:"zdr"`
}

// NewOpenRouterVideo constructs a direct-video adapter only for an explicitly
// advertised capability. Thin or image-only metadata fails closed.
func NewOpenRouterVideo(cfg OpenRouterVideoConfig) (VideoProvider, error) {
	if cfg.ProviderKey != "openrouter" {
		return nil, fmt.Errorf("direct hosted video is certified only for OpenRouter")
	}
	if cfg.Transport != VideoTransportBase64 {
		return nil, fmt.Errorf("OpenRouter video transport %q is not certified", cfg.Transport)
	}
	if !cfg.Model.Video {
		return nil, fmt.Errorf("model %q does not advertise video input", cfg.Model.ID)
	}
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("OpenRouter video requires an HTTP API base")
	}
	model := strings.TrimSpace(cfg.Model.ID)
	if before, after, ok := strings.Cut(model, "/"); !ok || before == "" || after == "" {
		return nil, fmt.Errorf("OpenRouter video requires a namespaced model ID")
	}
	if strings.Contains(strings.ToLower(model), "latest") {
		return nil, fmt.Errorf("OpenRouter video refuses moving model alias %q", model)
	}
	upstream := strings.TrimSpace(cfg.UpstreamProvider)
	if upstream == "" {
		return nil, fmt.Errorf("OpenRouter video requires a pinned upstream provider")
	}
	return &openRouterVideo{
		baseURL: baseURL, apiKey: cfg.APIKey, model: model, upstreamProvider: upstream,
		http: httpx.NewNamed("llm-video", httpx.TimeoutLLM),
	}, nil
}

func (o *openRouterVideo) AskAboutVideo(ctx context.Context, prompt string, video VideoInput) (Response, error) {
	if strings.TrimSpace(video.URL) != "" {
		return Response{}, fmt.Errorf("OpenRouter video URL transport is not certified")
	}
	if video.MIMEType != "video/mp4" {
		return Response{}, fmt.Errorf("OpenRouter video requires video/mp4, got %q", video.MIMEType)
	}
	if len(video.Data) == 0 {
		return Response{}, fmt.Errorf("video request carries no data")
	}
	if len(video.Data) > MaxHostedVideoBytes {
		return Response{}, fmt.Errorf("video request is %d bytes, exceeds %d-byte ceiling", len(video.Data), MaxHostedVideoBytes)
	}
	if video.DurationMS <= 0 || video.DurationMS > maxHostedVideoMS {
		return Response{}, fmt.Errorf("video duration %dms is outside the certified range", video.DurationMS)
	}
	if strings.TrimSpace(prompt) == "" {
		return Response{}, fmt.Errorf("video request carries no prompt")
	}
	started := time.Now()

	body, err := json.Marshal(openRouterVideoRequest{
		Model: o.model, MaxTokens: 512,
		Messages: []openRouterVideoMessage{{Role: "user", Content: []openRouterVideoPart{
			{Type: "text", Text: prompt},
			{Type: "video_url", VideoURL: &openRouterVideoURL{
				URL: "data:video/mp4;base64," + base64.StdEncoding.EncodeToString(video.Data),
			}},
		}}},
		Provider: openRouterVideoRoute{
			Order: []string{o.upstreamProvider}, AllowFallbacks: false,
			RequireParameters: true, DataCollection: "deny", ZDR: true,
		},
	})
	if err != nil {
		return Response{}, fmt.Errorf("marshal video request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-OpenRouter-Metadata", "enabled")
	if o.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	}
	resp, err := o.http.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("video chat: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var message bytes.Buffer
		_, _ = io.Copy(&message, io.LimitReader(resp.Body, 512))
		return Response{}, fmt.Errorf("video chat: status %d: %s", resp.StatusCode, strings.TrimSpace(message.String()))
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, MaxHostedVideoResponseBytes+1))
	if err != nil {
		return Response{}, fmt.Errorf("read video response: %w", err)
	}
	if len(raw) > MaxHostedVideoResponseBytes {
		return Response{}, fmt.Errorf("video response exceeded %d-byte ceiling", MaxHostedVideoResponseBytes)
	}
	resp.Body = io.NopCloser(bytes.NewReader(raw))
	var out openaiChatResp
	if err := decodeOpenAIJSON(resp, &out, "video response"); err != nil {
		return Response{}, err
	}
	if out.Error != nil {
		return Response{}, fmt.Errorf("video chat: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return Response{}, fmt.Errorf("video chat: no choices")
	}
	metrics.LLMTokens(out.Usage.PromptTokens, out.Usage.CompletionTokens)
	return Response{
		Content: strings.TrimSpace(out.Choices[0].Message.Content),
		Attribution: attributionFromWire("openrouter", o.model, out.ID, out.Model, out.Usage,
			out.OpenRouterMetadata, o.upstreamProvider, []string{"text", "video", "audio"}, time.Since(started)),
	}, nil
}

var _ VideoProvider = (*openRouterVideo)(nil)
