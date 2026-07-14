package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/mantonx/loomarr/internal/httpx"
)

// Ollama is the local-Ollama provider (§8 homelab default). Pinned to the real
// /api/chat tool-use contract (testkit/fixtures/llm/*, llama3.1:8b): tool-call
// arguments arrive as a PARSED OBJECT (not a string), format:json forces valid
// JSON output, stream:false returns one complete response.
type Ollama struct {
	baseURL string
	model   string
	http    *http.Client
}

// NewOllama builds an Ollama provider. baseURL e.g. http://ollama:11434.
func NewOllama(baseURL, model string) *Ollama {
	if model == "" {
		model = "llama3.1:8b"
	}
	return &Ollama{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		http:    httpx.New(httpx.TimeoutLLM),
	}
}

func (o *Ollama) Name() string { return "ollama" }

// --- wire types (pinned to the capture) ---

type ollamaChatReq struct {
	Model    string          `json:"model"`
	Stream   bool            `json:"stream"`
	Format   string          `json:"format,omitempty"` // "json" forces valid JSON output
	Messages []ollamaMessage `json:"messages"`
	Tools    []ollamaTool    `json:"tools,omitempty"`
	// Options carries Ollama's sampling knobs (temperature, top_p, num_predict) plus
	// num_ctx. Ollama defaults num_ctx to a small window; the 6-round grounded tool
	// loop grows the message history, so a too-small context silently truncates the
	// tool results the model must ground against — set it explicitly. Omitted when
	// empty so a zero-config call still works.
	Options map[string]any `json:"options,omitempty"`
}

type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
}

type ollamaToolCall struct {
	ID       string `json:"id,omitempty"`
	Function struct {
		Name string `json:"name"`
		// Arguments is a JSON OBJECT on Ollama (FINDINGS finding 1), unmarshaled
		// straight into a map.
		Arguments map[string]any `json:"arguments"`
	} `json:"function"`
}

type ollamaTool struct {
	Type     string `json:"type"` // "function"
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
}

type ollamaChatResp struct {
	Message    ollamaMessage `json:"message"`
	DoneReason string        `json:"done_reason"`
}

// Chat implements Provider against Ollama /api/chat.
func (o *Ollama) Chat(ctx context.Context, messages []Message, opts ChatOptions) (Response, error) {
	req := ollamaChatReq{
		Model:    o.model,
		Stream:   false,
		Messages: toOllamaMessages(messages),
		Tools:    toOllamaTools(opts.Tools),
		Options:  ollamaOptions(opts),
	}
	if opts.JSONMode {
		req.Format = "json"
	}

	body, err := json.Marshal(req)
	if err != nil {
		return Response{}, fmt.Errorf("marshal ollama request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/chat", strings.NewReader(string(body)))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := o.http.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("ollama chat: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Response{}, fmt.Errorf("ollama chat: status %d", resp.StatusCode)
	}
	var out ollamaChatResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Response{}, fmt.Errorf("decode ollama response: %w", err)
	}

	return Response{
		Content:   out.Message.Content,
		ToolCalls: fromOllamaToolCalls(out.Message.ToolCalls),
	}, nil
}

// ollamaChatCtx is the context window we request for the grounded tool loop.
// Ollama's default (often 2–4k) truncates the growing 6-round message history,
// silently dropping the tool results the model must ground against. 8k comfortably
// holds the system prompt + tool schema + several rounds of catalog results.
const ollamaChatCtx = 8192

// ollamaOptions maps the provider-neutral sampling controls to Ollama's `options`
// block, always pinning num_ctx for the tool loop. Returns nil only if there's
// truly nothing to send (never happens here since num_ctx is always set).
func ollamaOptions(opts ChatOptions) map[string]any {
	o := map[string]any{"num_ctx": ollamaChatCtx}
	if opts.Temperature != nil {
		o["temperature"] = *opts.Temperature
	}
	if opts.TopP != nil {
		o["top_p"] = *opts.TopP
	}
	if opts.MaxTokens > 0 {
		o["num_predict"] = opts.MaxTokens
	}
	return o
}

func toOllamaMessages(msgs []Message) []ollamaMessage {
	out := make([]ollamaMessage, 0, len(msgs))
	for _, m := range msgs {
		om := ollamaMessage{Role: string(m.Role), Content: m.Content}
		for _, tc := range m.ToolCalls {
			var otc ollamaToolCall
			otc.ID = tc.ID
			otc.Function.Name = tc.Name
			otc.Function.Arguments = tc.Arguments
			om.ToolCalls = append(om.ToolCalls, otc)
		}
		out = append(out, om)
	}
	return out
}

func toOllamaTools(tools []ToolSchema) []ollamaTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]ollamaTool, 0, len(tools))
	for _, t := range tools {
		var ot ollamaTool
		ot.Type = "function"
		ot.Function.Name = t.Name
		ot.Function.Description = t.Description
		ot.Function.Parameters = t.Parameters
		out = append(out, ot)
	}
	return out
}

func fromOllamaToolCalls(tcs []ollamaToolCall) []ToolCall {
	if len(tcs) == 0 {
		return nil
	}
	out := make([]ToolCall, 0, len(tcs))
	for _, tc := range tcs {
		out = append(out, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}
	return out
}

var _ Provider = (*Ollama)(nil)
