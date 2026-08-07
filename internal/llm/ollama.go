package llm

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/mantonx/loomarr/internal/httpx"
	"github.com/mantonx/loomarr/internal/metrics"
)

// Ollama is the local-Ollama provider (§8 homelab default). Pinned to the real
// /api/chat tool-use contract (testkit/fixtures/llm/*, llama3.1:8b): tool-call
// arguments arrive as a PARSED OBJECT (not a string), format:json forces valid
// JSON output, stream:false returns one complete response.
type Ollama struct {
	baseURL string
	model   string
	// keepAlive is the residency hint sent with every call (§8.2). Ollama accepts a
	// duration string ("30m") or 0 to unload immediately; "" omits the field entirely,
	// leaving Ollama's own 5-minute default.
	keepAlive string
	// guessedModel records that NO model was configured, so `model` below is the
	// built-in fallback tag rather than anybody's choice. A Chat still tries it (a
	// misconfigured install failing loudly on a real request is correct), but Warm
	// does not — see Warm.
	guessedModel bool
	http         *http.Client
}

// NewOllama builds an Ollama provider. baseURL e.g. http://ollama:11434. Residency
// defaults to Ollama's own behavior; WithKeepAlive sets the §8.2 hint.
func NewOllama(baseURL, model string) *Ollama {
	guessed := model == ""
	if guessed {
		model = "llama3.1:8b"
	}
	return &Ollama{
		baseURL:      strings.TrimRight(baseURL, "/"),
		model:        model,
		guessedModel: guessed,
		http:         httpx.NewNamed("llm", httpx.TimeoutLLM),
	}
}

// WithKeepAlive sets how long the model stays resident between calls (§8.2). An
// empty string leaves Ollama's default (5m idle unload); "0" unloads immediately.
// Returns the provider for chaining at construction.
func (o *Ollama) WithKeepAlive(d string) *Ollama {
	o.keepAlive = d
	return o
}

func (o *Ollama) Name() string { return "ollama" }

// --- wire types (pinned to the capture) ---

type ollamaChatReq struct {
	Model    string          `json:"model"`
	Stream   bool            `json:"stream"`
	Format   string          `json:"format,omitempty"` // "json" forces valid JSON output
	Messages []ollamaMessage `json:"messages"`
	Tools    []ollamaTool    `json:"tools,omitempty"`
	// Think disables a thinking/reasoning model's chain-of-thought for this call.
	// Thinking-capable models (Qwen3, etc.) can, with tools present, return EMPTY
	// content or leak reasoning markers that break tool-calls + JSON (open Ollama
	// bugs #10976/#14601). We force it off on tool turns. Pointer so it's only sent
	// when set — a non-thinking model that doesn't know the field is left untouched.
	Think *bool `json:"think,omitempty"`
	// Options carries Ollama's sampling knobs (temperature, top_p, num_predict) plus
	// num_ctx. Ollama defaults num_ctx to a small window; the 6-round grounded tool
	// loop grows the message history, so a too-small context silently truncates the
	// tool results the model must ground against — set it explicitly. Omitted when
	// empty so a zero-config call still works.
	Options map[string]any `json:"options,omitempty"`
	// KeepAlive is how long Ollama holds the model in VRAM after this call (§8.2).
	// A string because Ollama accepts both a duration ("30m") and a bare number of
	// seconds; omitted when empty so we inherit its 5-minute default rather than
	// asserting one. This is the difference between a ~9s load and a ~0.5s answer on
	// the next turn — including BETWEEN the rounds of one tool loop.
	KeepAlive string `json:"keep_alive,omitempty"`
}

type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
	// Images is Ollama's per-message multimodal field (§10 V44 vision tier): base64 frames,
	// NO data-URI prefix (unlike the hosted image_url part). This is the ONE V44 change to the
	// shared Chat wire type, so `omitempty` is load-bearing, not cosmetic: an image-free text
	// request must serialise byte-for-byte as it did before this field existed. A nil slice on
	// every text message keeps the field out of the body entirely — guarded by a test that
	// marshals a text-only message and asserts no "images" key appears.
	Images []string `json:"images,omitempty"`
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
	// Token accounting (§17): Ollama reports prompt vs generated eval counts.
	PromptEvalCount int `json:"prompt_eval_count"`
	EvalCount       int `json:"eval_count"`
}

// Chat implements Provider against Ollama /api/chat.
func (o *Ollama) Chat(ctx context.Context, messages []Message, opts ChatOptions) (Response, error) {
	req := ollamaChatReq{
		Model:     o.model,
		Stream:    false,
		Messages:  toOllamaMessages(messages),
		Tools:     toOllamaTools(opts.Tools),
		Options:   ollamaOptions(opts),
		KeepAlive: o.keepAlive,
	}
	if opts.JSONMode {
		req.Format = "json"
	}
	// Disable thinking whenever tools are in play: a reasoning model's chain-of-
	// thought interferes with tool-calls + JSON on Ollama (empty output / leaked
	// markers). Our grounding loop always passes tools, so this is effectively
	// always off for the suggester — the intended behavior.
	if len(opts.Tools) > 0 {
		off := false
		req.Think = &off
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
	metrics.LLMTokens(out.PromptEvalCount, out.EvalCount) // §17: no-op on 0

	return Response{
		Content:   out.Message.Content,
		ToolCalls: fromOllamaToolCalls(out.Message.ToolCalls),
	}, nil
}

// AskAboutImages sends one question about a clip's keyframes and returns the model's answer
// (§10 V44 vision tier, local path). Ollama's /api/chat takes the frames in the per-message
// `images` array as bare base64 — no data-URI prefix (that is the hosted image_url shape,
// vision.go) — so a fully-local install running a vision model (llava / llama-vision) gets
// visual tagging without egress or per-clip cost. Ollama reports a `vision` capability
// (probed live 2026-08-03, §10 quality gate); this is the wiring that uses it.
//
// It reuses the shared Chat wire type deliberately — the images ride on the same ollamaMessage
// the text path uses, which is why that field carries omitempty (see ollamaMessage.Images):
// no images ⇒ the field is absent ⇒ a text request is unchanged. NO tools, NO json format,
// NO num_ctx block: vision is one stateless question, not the grounded tool loop, so it sends
// the minimal request and returns the answer as a Response for the caller to parse tags from.
func (o *Ollama) AskAboutImages(ctx context.Context, prompt string, jpegs [][]byte) (Response, error) {
	if len(jpegs) == 0 {
		return Response{}, fmt.Errorf("vision request carries no images")
	}
	imgs := make([]string, 0, len(jpegs))
	for _, jpg := range jpegs {
		imgs = append(imgs, base64.StdEncoding.EncodeToString(jpg))
	}

	body, err := json.Marshal(ollamaChatReq{
		Model:     o.model,
		Stream:    false,
		KeepAlive: o.keepAlive,
		Messages: []ollamaMessage{{
			Role:    "user",
			Content: prompt,
			Images:  imgs,
		}},
	})
	if err != nil {
		return Response{}, fmt.Errorf("marshal ollama vision request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/chat", strings.NewReader(string(body)))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := o.http.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("ollama vision: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Response{}, fmt.Errorf("ollama vision: status %d", resp.StatusCode)
	}
	var out ollamaChatResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Response{}, fmt.Errorf("decode ollama vision response: %w", err)
	}
	metrics.LLMTokens(out.PromptEvalCount, out.EvalCount) // §17: no-op on 0
	return Response{Content: strings.TrimSpace(out.Message.Content)}, nil
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

// Warm asks Ollama to load the model into memory without generating anything
// (§8.2). An /api/chat call with NO messages is Ollama's documented preload: it
// resolves the model, loads the weights, and returns — so the next real call skips
// the ~9s cold load that otherwise lands on whoever describes the first channel.
//
// The keep_alive hint rides along, which is what makes the warm-up outlive Ollama's
// 5-minute idle unload; without it a preload at boot would have evaporated long
// before anyone reached the suggest screen.
//
// Errors are returned for logging but are never fatal to a caller: a failed warm-up
// costs latency, not correctness — the next Chat loads the model itself.
//
// A GUESSED model is not warmed. On a fresh install nobody has picked one yet (§8.1
// leaves LLM_MODEL blank so the wizard's ranked picker owns the choice), so warming
// would fire the built-in fallback tag at a host that has almost certainly never
// pulled it: a 404 logged as a warm-up "failure" for a model the operator never
// asked for, on every boot until they finish onboarding. There is nothing to warm
// yet, and saying so by doing nothing is more honest than a request we expect to
// fail. The pick itself warms (Swappable.Set), so this costs no real latency.
func (o *Ollama) Warm(ctx context.Context) error {
	if o.guessedModel {
		return ErrNothingToWarm
	}
	body, err := json.Marshal(ollamaChatReq{
		Model:     o.model,
		Stream:    false,
		KeepAlive: o.keepAlive,
	})
	if err != nil {
		return fmt.Errorf("marshal ollama warm request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/chat", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := o.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("ollama warm: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ollama warm: status %d", resp.StatusCode)
	}
	return nil
}

var (
	_ Provider = (*Ollama)(nil)
	_ Warmer   = (*Ollama)(nil)
)
