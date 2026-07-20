package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/mantonx/loomarr/internal/httpx"
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
	baseURL string // includes the API root, e.g. https://api.openai.com/v1
	apiKey  string
	model   string
	http    *http.Client
}

// NewOpenAI builds the provider. baseURL is the OpenAI-compatible base
// (".../v1"); a trailing "/v1" is NOT auto-appended — point LLM_URL at the exact
// base the endpoint documents (hosted: https://…/v1; local: http://ollama:11434/v1).
func NewOpenAI(baseURL, model, apiKey string) *OpenAI {
	return &OpenAI{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
		http:    httpx.NewNamed("llm", httpx.TimeoutLLM),
	}
}

func (o *OpenAI) Name() string { return "openai" }

// --- wire types (OpenAI /v1/chat/completions) ---

type openaiChatReq struct {
	Model          string          `json:"model"`
	Messages       []openaiMessage `json:"messages"`
	Tools          []openaiTool    `json:"tools,omitempty"`
	ResponseFormat *openaiRespFmt  `json:"response_format,omitempty"`
	Temperature    *float64        `json:"temperature,omitempty"`
	TopP           *float64        `json:"top_p,omitempty"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
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
	Choices []struct {
		Message openaiMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Chat implements Provider against /v1/chat/completions.
func (o *OpenAI) Chat(ctx context.Context, messages []Message, opts ChatOptions) (Response, error) {
	req := openaiChatReq{
		Model:       o.model,
		Messages:    toOpenAIMessages(messages),
		Tools:       toOpenAITools(opts.Tools),
		Temperature: opts.Temperature,
		TopP:        opts.TopP,
		MaxTokens:   opts.MaxTokens,
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
	if o.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	}

	resp, err := o.http.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("openai chat: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Response{}, fmt.Errorf("openai chat: status %d", resp.StatusCode)
	}
	var out openaiChatResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Response{}, fmt.Errorf("decode openai response: %w", err)
	}
	if out.Error != nil {
		return Response{}, fmt.Errorf("openai chat: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return Response{}, fmt.Errorf("openai chat: empty choices")
	}
	msg := out.Choices[0].Message
	return Response{
		Content:   msg.Content,
		ToolCalls: fromOpenAIToolCalls(msg.ToolCalls),
	}, nil
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

var _ Provider = (*OpenAI)(nil)
