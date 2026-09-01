package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/httpx"
	"github.com/loomarr/loomarr/internal/metrics"
)

// OpenAI is the OpenAI-COMPATIBLE provider (§8/§14): one hand-written client
// against POST {base}/v1/chat/completions with tools. It drives every endpoint
// that speaks that shape — OpenAI, Google Gemini (compat endpoint), Groq,
// Together, OpenRouter, AND local Ollama's own /v1 mode — so the model is a
// config choice (LLM_URL base + LLM_MODEL + LLM_API_KEY), not a per-vendor fork.
//
// Deliberately LENIENT for maximum compatibility: response_format is sent only as
// a best-effort JSON hint (some OpenAI-ish gateways reject an unknown
// response_format, so we never hard-require it) — the grounded prompt already
// says "reply with ONLY JSON" and the suggester's repair loop backstops any
// malformed output. Tool-calling IS required (grounding depends on it): a model
// that doesn't emit tool_calls fails grounding, which the §13 wizard check flags.
type OpenAI struct {
	baseURL  string // includes the API root, e.g. https://api.openai.com/v1
	apiKey   string
	model    string
	provider string // branded route identity (openrouter, custom, openai)
	http     *http.Client
	route    *openRouterChatRoute
	metrics  *metrics.Recorder
}

// WithMetrics binds this provider to one application generation's observations.
func (o *OpenAI) WithMetrics(recorder *metrics.Recorder) *OpenAI {
	o.metrics = recorder
	if recorder != nil {
		o.http = httpx.NewNamedObserved("llm", httpx.TimeoutLLM, recorder)
	}
	return o
}

// OpenRouterChatConfig pins one certification request to a concrete private
// OpenRouter route while retaining the shared OpenAI-compatible wire.
type OpenRouterChatConfig struct {
	BaseURL          string
	Model            string
	APIKey           string
	UpstreamProvider string
}

// decodeOpenAIJSON preserves ordinary JSON syntax errors while turning the common "wrong API
// base returned an HTML home page" failure into a setting-level diagnosis. Check after decoding:
// several compatible endpoints and test doubles omit or mislabel Content-Type despite returning
// valid JSON, and compatibility matters more than policing headers that the payload disproves.
func decodeOpenAIJSON(resp *http.Response, out any, operation string) error {
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
		mediaType, _, parseErr := mime.ParseMediaType(contentType)
		if parseErr != nil || mediaType == "" {
			mediaType = "an unknown content type"
		}
		if mediaType != "application/json" && !strings.HasSuffix(mediaType, "+json") {
			return fmt.Errorf("%s: expected JSON, got %s; check llm.url points at the provider API base", operation, mediaType)
		}
		return fmt.Errorf("decode %s: %w", operation, err)
	}
	return nil
}

// NewOpenAI builds the provider. baseURL is the OpenAI-compatible base
// (".../v1"); a trailing "/v1" is NOT auto-appended — point LLM_URL at the exact
// base the endpoint documents (hosted: https://…/v1; local: http://ollama:11434/v1).
func NewOpenAI(baseURL, model, apiKey string) *OpenAI {
	return NewOpenAIForProvider("openai", baseURL, model, apiKey)
}

// NewOpenAIForProvider retains the branded provider identity while using the
// shared OpenAI-compatible wire adapter. OpenRouter needs that identity to opt in
// to routing metadata and interpret usage.cost as a USD billing fact.
func NewOpenAIForProvider(provider, baseURL, model, apiKey string) *OpenAI {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		provider = "openai"
	}
	return &OpenAI{
		baseURL:  strings.TrimRight(baseURL, "/"),
		apiKey:   apiKey,
		model:    model,
		provider: provider,
		http:     httpx.NewNamed("llm", httpx.TimeoutLLM),
	}
}

// NewOpenRouterChat constructs the strict certification lane. Ordinary
// OpenAI-compatible clients intentionally do not inherit these routing controls.
func NewOpenRouterChat(cfg OpenRouterChatConfig) (*OpenAI, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, fmt.Errorf("OpenRouter chat requires an API base")
	}
	if err := ValidateOpenRouterCertificationRoute(cfg.Model, cfg.UpstreamProvider); err != nil {
		return nil, err
	}
	provider := NewOpenAIForProvider("openrouter", cfg.BaseURL, cfg.Model, cfg.APIKey)
	provider.route = &openRouterChatRoute{
		Order: []string{cfg.UpstreamProvider}, AllowFallbacks: false, RequireParameters: true, DataCollection: "deny", ZDR: true,
	}
	return provider, nil
}

// ValidateOpenRouterCertificationRoute is the single fail-closed authority for
// the immutable model and singleton upstream route used by certification lanes.
func ValidateOpenRouterCertificationRoute(model, upstreamProvider string) error {
	rawModel := model
	model = strings.TrimSpace(model)
	provider := strings.TrimSpace(upstreamProvider)
	if model == "" || model != rawModel || len([]rune(model)) > 256 ||
		strings.Count(model, "/") != 1 || strings.ContainsAny(model, " ,*:") {
		return fmt.Errorf("OpenRouter certification requires an exact immutable namespaced model")
	}
	lowerModel := strings.ToLower(model)
	if strings.Contains(lowerModel, "latest") || strings.HasSuffix(lowerModel, "/auto") || strings.HasPrefix(lowerModel, "openrouter/") {
		return fmt.Errorf("OpenRouter certification model %q is a mutable router or alias", model)
	}
	if provider == "" || provider != upstreamProvider || len([]rune(provider)) > 128 || strings.Contains(provider, ",") ||
		provider == "*" || strings.EqualFold(provider, "auto") {
		return fmt.Errorf("OpenRouter certification requires exactly one concrete upstream provider")
	}
	return nil
}

func (o *OpenAI) Name() string { return o.provider }

// --- wire types (OpenAI /v1/chat/completions) ---

type openaiChatReq struct {
	Model          string               `json:"model"`
	Messages       []openaiMessage      `json:"messages"`
	Tools          []openaiTool         `json:"tools,omitempty"`
	ResponseFormat *openaiRespFmt       `json:"response_format,omitempty"`
	Temperature    *float64             `json:"temperature,omitempty"`
	TopP           *float64             `json:"top_p,omitempty"`
	MaxTokens      int                  `json:"max_tokens,omitempty"`
	Provider       *openRouterChatRoute `json:"provider,omitempty"`
}

type openRouterChatRoute struct {
	Order             []string `json:"order"`
	AllowFallbacks    bool     `json:"allow_fallbacks"`
	RequireParameters bool     `json:"require_parameters"`
	DataCollection    string   `json:"data_collection"`
	ZDR               bool     `json:"zdr"`
}

type openaiRespFmt struct {
	Type string `json:"type"` // "json_object"
}

type openaiMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []openaiToolCall `json:"tool_calls,omitempty"`
	// ToolCallID correlates a tool-result message to the call it answers (required
	// by OpenAI-compatible servers for role=="tool").
	ToolCallID string `json:"tool_call_id,omitempty"`
}

type openaiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"` // "function"
	Function struct {
		Name string `json:"name"`
		// Arguments is a JSON STRING on OpenAI (unlike Ollama's object) — the one
		// normalization this provider must do.
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openaiTool struct {
	Type     string `json:"type"` // "function"
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
}

type openaiChatResp struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message openaiMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
	// Token accounting (§17): the OpenAI-compatible usage block. Absent on a
	// provider that omits it ⇒ zero ⇒ LLMTokens skips it.
	Usage              openAIUsage        `json:"usage"`
	OpenRouterMetadata openRouterMetadata `json:"openrouter_metadata"`
}

type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	PromptDetails    struct {
		CachedTokens     int `json:"cached_tokens"`
		CacheWriteTokens int `json:"cache_write_tokens"`
		AudioTokens      int `json:"audio_tokens"`
		ImageTokens      int `json:"image_tokens"`
		VideoTokens      int `json:"video_tokens"`
	} `json:"prompt_tokens_details"`
	CompletionDetails struct {
		ReasoningTokens int `json:"reasoning_tokens"`
		AudioTokens     int `json:"audio_tokens"`
		ImageTokens     int `json:"image_tokens"`
		VideoTokens     int `json:"video_tokens"`
	} `json:"completion_tokens_details"`
	Cost json.Number `json:"cost"`
}

type openRouterMetadata struct {
	Attempt   int `json:"attempt"`
	Endpoints struct {
		Available []struct {
			Model    string `json:"model"`
			Provider string `json:"provider"`
			Selected bool   `json:"selected"`
		} `json:"available"`
	} `json:"endpoints"`
}

// Chat implements Provider against /v1/chat/completions.
func (o *OpenAI) Chat(ctx context.Context, messages []Message, opts ChatOptions) (Response, error) {
	started := time.Now()
	req := openaiChatReq{
		Model:       o.model,
		Messages:    toOpenAIMessages(messages),
		Tools:       toOpenAITools(opts.Tools),
		Temperature: opts.Temperature,
		TopP:        opts.TopP,
		MaxTokens:   opts.MaxTokens,
		Provider:    o.route,
	}
	if opts.JSONMode {
		// Best-effort JSON hint (lenient): a provider that ignores it is fine — the
		// prompt + repair loop enforce JSON regardless.
		req.ResponseFormat = &openaiRespFmt{Type: "json_object"}
	}

	body, err := json.Marshal(req)
	if err != nil {
		return Response{}, fmt.Errorf("marshal openai request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/chat/completions", strings.NewReader(string(body)))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	o.addMetadataHeader(httpReq)
	if o.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	}

	resp, err := o.http.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("openai chat: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// ⚠ The body carries WHY, and a bare status code hides it. On this text path the common
		// 404 is "model not found" — an OpenAI-compatible gateway (OpenRouter, etc.) rejecting the
		// configured `llm.model`, e.g. a slug with a `:variant` suffix the chat endpoint doesn't
		// serve. Without the body the operator sees "status 404" and cannot tell a wrong model
		// from a wrong URL from a dead key. The vision and audio paths already read it; this one
		// was left bare, so a misconfigured model surfaced as an opaque failure (a channel-create
		// "Generation failed" with nothing to act on).
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(io.LimitReader(resp.Body, 512))
		return Response{}, fmt.Errorf("openai chat: status %d: %s", resp.StatusCode, strings.TrimSpace(buf.String()))
	}
	var out openaiChatResp
	if err := decodeOpenAIJSON(resp, &out, "openai response"); err != nil {
		return Response{}, err
	}
	if out.Error != nil {
		return Response{}, fmt.Errorf("openai chat: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return Response{}, fmt.Errorf("openai chat: empty choices")
	}
	if o.metrics != nil {
		o.metrics.LLMTokens(out.Usage.PromptTokens, out.Usage.CompletionTokens)
	}
	msg := out.Choices[0].Message
	return Response{
		Content:   msg.Content,
		ToolCalls: fromOpenAIToolCalls(msg.ToolCalls),
		Attribution: attributionFromWire(o.provider, o.model, out.ID, out.Model, out.Usage,
			out.OpenRouterMetadata, []string{"text"}, time.Since(started)),
	}, nil
}

func (o *OpenAI) addMetadataHeader(req *http.Request) {
	if o.provider == "openrouter" {
		req.Header.Set("X-OpenRouter-Metadata", "enabled")
	}
}

func attributionFromWire(requestedProvider, requestedModel, generationID, reportedModel string,
	usage openAIUsage, metadata openRouterMetadata, modalities []string,
	latency time.Duration,
) Attribution {
	resolvedModel := strings.TrimSpace(reportedModel)
	resolvedProvider := ""
	for _, endpoint := range metadata.Endpoints.Available {
		if endpoint.Selected {
			resolvedProvider = endpoint.Provider
			break
		}
	}
	attempts := metadata.Attempt
	a := Attribution{
		RequestedModel: requestedModel, ResolvedModel: resolvedModel,
		RequestedProvider: requestedProvider, ResolvedProvider: resolvedProvider,
		Modalities: append([]string(nil), modalities...),
		Tokens: TokenUsage{
			Prompt: usage.PromptTokens, Completion: usage.CompletionTokens,
			Reasoning: usage.CompletionDetails.ReasoningTokens,
			Cached:    usage.PromptDetails.CachedTokens, CacheWrite: usage.PromptDetails.CacheWriteTokens,
			Image: usage.PromptDetails.ImageTokens + usage.CompletionDetails.ImageTokens,
			Audio: usage.PromptDetails.AudioTokens + usage.CompletionDetails.AudioTokens,
			Video: usage.PromptDetails.VideoTokens + usage.CompletionDetails.VideoTokens,
		},
		Latency: latency, Attempts: attempts, GenerationID: generationID,
	}
	if requestedProvider == "openrouter" && usage.Cost.String() != "" {
		a.Charge = &Money{Amount: usage.Cost.String(), Currency: "USD"}
	}
	return a
}

func toOpenAIMessages(msgs []Message) []openaiMessage {
	out := make([]openaiMessage, 0, len(msgs))
	for _, m := range msgs {
		om := openaiMessage{Role: string(m.Role), Content: m.Content, ToolCallID: m.ToolCallID}
		for _, tc := range m.ToolCalls {
			var otc openaiToolCall
			otc.ID = tc.ID
			otc.Type = "function"
			otc.Function.Name = tc.Name
			// Arguments go back out as a JSON string (OpenAI's shape).
			if b, err := json.Marshal(tc.Arguments); err == nil {
				otc.Function.Arguments = string(b)
			}
			om.ToolCalls = append(om.ToolCalls, otc)
		}
		out = append(out, om)
	}
	return out
}

func toOpenAITools(tools []ToolSchema) []openaiTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]openaiTool, 0, len(tools))
	for _, t := range tools {
		var ot openaiTool
		ot.Type = "function"
		ot.Function.Name = t.Name
		ot.Function.Description = t.Description
		ot.Function.Parameters = t.Parameters
		out = append(out, ot)
	}
	return out
}

// fromOpenAIToolCalls normalizes OpenAI's JSON-STRING arguments into the decoded
// map[string]any the suggester expects (the one shape difference from Ollama). A
// tool call with unparseable arguments yields an empty map rather than failing —
// the suggester's tool dispatch treats a missing arg as "not provided".
func fromOpenAIToolCalls(tcs []openaiToolCall) []ToolCall {
	if len(tcs) == 0 {
		return nil
	}
	out := make([]ToolCall, 0, len(tcs))
	for _, tc := range tcs {
		args := map[string]any{}
		if tc.Function.Arguments != "" {
			_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
		}
		out = append(out, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: args,
		})
	}
	return out
}

// AskAboutImages sends one question about a clip's keyframes and returns the model's answer
// (§10 V44 vision tier). It follows AskAboutAudio's precedent exactly: a multimodal chat
// completion whose user message carries `image_url` content parts alongside the text prompt,
// deliberately NOT a widening of `Message.Content` (that string is on the hot path of every
// text request, §8). The frames ride as full `data:image/jpeg;base64,…` URIs — the image
// part's shape, unlike the audio part's bare base64 (see vision.go / audio.go).
//
// Like AskAboutAudio it does NOT use the Provider interface: that models a tool-calling chat
// loop, and this is a single stateless question whose answer is a small tagging JSON.
func (o *OpenAI) AskAboutImages(ctx context.Context, prompt string, jpegs [][]byte) (Response, error) {
	if len(jpegs) == 0 {
		return Response{}, fmt.Errorf("vision request carries no images")
	}
	started := time.Now()

	// text prompt first, then one image_url part per keyframe — the order the spec shows.
	parts := make([]visionPart, 0, len(jpegs)+1)
	parts = append(parts, visionPart{Type: "text", Text: prompt})
	for _, jpg := range jpegs {
		parts = append(parts, visionPart{Type: "image_url", ImageURL: &visionPartImage{
			URL: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(jpg),
		}})
	}

	body, err := json.Marshal(visionChatReq{
		Model:    o.model,
		Messages: []visionMessage{{Role: "user", Content: parts}},
	})
	if err != nil {
		return Response{}, fmt.Errorf("marshal vision request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		o.baseURL+"/chat/completions", strings.NewReader(string(body)))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	o.addMetadataHeader(httpReq)
	if o.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	}

	resp, err := o.http.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("vision chat: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// ⚠ The body carries WHY, and it matters more here than on the text path: the common
		// failures are "this model has no image input" and "your key has no credit", which are
		// an operator's to fix and are indistinguishable from a bare status code (audio.go's
		// same reasoning).
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(io.LimitReader(resp.Body, 512))
		return Response{}, fmt.Errorf("vision chat: status %d: %s", resp.StatusCode, strings.TrimSpace(buf.String()))
	}

	var out openaiChatResp
	if err := decodeOpenAIJSON(resp, &out, "vision response"); err != nil {
		return Response{}, err
	}
	if out.Error != nil {
		return Response{}, fmt.Errorf("vision chat: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return Response{}, fmt.Errorf("vision chat: no choices")
	}
	if o.metrics != nil {
		o.metrics.LLMTokens(out.Usage.PromptTokens, out.Usage.CompletionTokens)
	}
	return Response{
		Content: strings.TrimSpace(out.Choices[0].Message.Content),
		Attribution: attributionFromWire(o.provider, o.model, out.ID, out.Model, out.Usage,
			out.OpenRouterMetadata, []string{"text", "image"}, time.Since(started)),
	}, nil
}

var _ Provider = (*OpenAI)(nil)
