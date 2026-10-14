package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// Timeouts bounds one OpenAI-compatible call without a fixed whole-request deadline.
//
// The production target is a llama.cpp server with ONE slot shared by other heavy clients
// (~238 tok/s prefill, ~24 tok/s decode). A call there spends most of its life waiting, and
// a waiting call is healthy. So the budgets follow what "healthy" looks like:
//
//   - FirstToken covers queueing behind other clients plus our own uncached prefill. One
//     21k-token neighbour is ~90 s, and several can be ahead, so 10 minutes.
//   - Idle applies once tokens flow. Decode emits a chunk every ~40 ms; only a wedge (or a
//     mid-stream slot swap re-prefilling) leaves a longer gap, so 90 s is generous and still
//     fails a dead server in about a minute and a half.
//   - Total caps the whole call (the llm.request_timeout setting). A 2048-token completion
//     is ~85 s at 24 tok/s, so 30 minutes only trips on a runaway.
type Timeouts struct {
	FirstToken time.Duration
	Idle       time.Duration
	Total      time.Duration
}

const (
	DefaultFirstTokenTimeout = 10 * time.Minute
	DefaultIdleTimeout       = 90 * time.Second
	DefaultTotalTimeout      = 30 * time.Minute
)

// DefaultTimeouts is the budget set every OpenAI-compatible client starts with.
func DefaultTimeouts() Timeouts {
	return Timeouts{FirstToken: DefaultFirstTokenTimeout, Idle: DefaultIdleTimeout, Total: DefaultTotalTimeout}
}

func (t Timeouts) orDefaults() Timeouts {
	d := DefaultTimeouts()
	if t.FirstToken <= 0 {
		t.FirstToken = d.FirstToken
	}
	if t.Idle <= 0 {
		t.Idle = d.Idle
	}
	if t.Total <= 0 {
		t.Total = d.Total
	}
	return t
}

// budgetError is a fired call budget. It reads as context.DeadlineExceeded so the suggester
// still selects provider_timeout, while errors.Is against the sentinels says WHICH budget.
type budgetError struct{ outcome string }

func (e *budgetError) Error() string        { return strings.ReplaceAll(e.outcome, "_", " ") }
func (e *budgetError) Is(target error) bool { return target == context.DeadlineExceeded }

var (
	ErrFirstTokenTimeout = &budgetError{"first_token_timeout"}
	ErrIdleTimeout       = &budgetError{"idle_timeout"}
	ErrTotalTimeout      = &budgetError{"total_timeout"}
)

// StatusError is a non-2xx provider reply. Body is the provider's own explanation, capped.
type StatusError struct {
	Op   string
	Code int
	Body string
}

func (e *StatusError) Error() string { return fmt.Sprintf("%s: status %d: %s", e.Op, e.Code, e.Body) }

// callSiteKey carries the caller's name for a call down to the provider.
type callSiteKey struct{}

// WithCallSite names the caller of the LLM calls made under ctx (e.g. "suggest.grounding"),
// so timeouts, logs and metrics say WHICH feature was waiting rather than "llm".
func WithCallSite(ctx context.Context, site string) context.Context {
	return context.WithValue(ctx, callSiteKey{}, site)
}

// contentDeltaKey carries ChatOptions.OnContentDelta down to exchange without widening its
// signature.
type contentDeltaKey struct{}

func withContentDelta(ctx context.Context, fn func(string)) context.Context {
	if fn == nil {
		return ctx
	}
	return context.WithValue(ctx, contentDeltaKey{}, fn)
}

// CallSite returns the name set by WithCallSite, or "unknown".
func CallSite(ctx context.Context) string {
	if site, _ := ctx.Value(callSiteKey{}).(string); site != "" {
		return site
	}
	return "unknown"
}

// completion is one reassembled chat completion, streamed or not.
type completion struct {
	ID, Model    string
	Message      openaiMessage
	FinishReason string
	Usage        openAIUsage
	Meta         openRouterMetadata
	FirstToken   time.Duration
}

type streamChunk struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
	Usage              *openAIUsage       `json:"usage"`
	OpenRouterMetadata openRouterMetadata `json:"openrouter_metadata"`
}

// streamOptions asks for the final usage chunk; without it a streamed reply carries no
// token counts at all.
type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// exchange POSTs body to /chat/completions and returns the reassembled completion. The
// request asks for a stream; a server that ignores that and answers with one JSON document
// is still accepted. Budgets: FirstToken until the first data frame, Idle between frames
// after that, Total over everything. A fired budget cancels the request and is returned as
// its sentinel, naming op and call site.
func (o *OpenAI) exchange(ctx context.Context, op string, body []byte) (completion, int, error) {
	site := CallSite(ctx)
	budgets := o.timeouts.orDefaults()
	started := time.Now()

	callCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	total := time.AfterFunc(budgets.Total, func() { cancel(ErrTotalTimeout) })
	defer total.Stop()
	var progressed atomic.Bool
	watchdog := time.AfterFunc(budgets.FirstToken, func() {
		if progressed.Load() {
			cancel(ErrIdleTimeout)
		} else {
			cancel(ErrFirstTokenTimeout)
		}
	})
	defer watchdog.Stop()

	// fail folds a fired budget into the error; a caller's own cancel or deadline wins.
	fail := func(err error) error {
		var fired *budgetError
		if ctx.Err() == nil && errors.As(context.Cause(callCtx), &fired) {
			return fmt.Errorf("%s [%s]: %w after %s (first token budget %s, idle budget %s, total cap %s)",
				op, site, fired, time.Since(started).Round(time.Millisecond),
				budgets.FirstToken, budgets.Idle, budgets.Total)
		}
		return err
	}

	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodPost, o.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return completion{}, 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	o.addMetadataHeader(httpReq)
	if o.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	}

	resp, err := o.http.Do(httpReq)
	if err != nil {
		return completion{}, 0, fail(fmt.Errorf("%s [%s]: %w", op, site, err))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// ⚠ The body carries WHY, and a bare status code hides it: "model not found" (a wrong
		// llm.model), "no image input", "no credit", or a llama.cpp slot error. It is capped
		// at 512 bytes and never contains our key.
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(io.LimitReader(resp.Body, 512))
		return completion{}, resp.StatusCode, &StatusError{Op: op, Code: resp.StatusCode, Body: strings.TrimSpace(buf.String())}
	}

	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		var out openaiChatResp
		if err := decodeOpenAIJSON(resp, &out, op+" response"); err != nil {
			return completion{}, resp.StatusCode, fail(err)
		}
		if out.Error != nil {
			return completion{}, resp.StatusCode, fmt.Errorf("%s: %s", op, out.Error.Message)
		}
		if len(out.Choices) == 0 {
			return completion{}, resp.StatusCode, fmt.Errorf("%s: empty choices", op)
		}
		c := completion{ID: out.ID, Model: out.Model, Message: out.Choices[0].Message, Usage: out.Usage, Meta: out.OpenRouterMetadata}
		c.FinishReason = out.Choices[0].FinishReason
		return c, resp.StatusCode, nil
	}

	var (
		c         completion
		content   strings.Builder
		calls     []*openaiToolCall
		sawFrame  bool
		sawFinish bool
		sawDone   bool
	)
	onDelta, _ := ctx.Value(contentDeltaKey{}).(func(string))
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		payload, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue // comments / keep-alives prove nothing about token progress
		}
		payload = strings.TrimSpace(payload)
		if !sawFrame {
			c.FirstToken = time.Since(started)
		}
		sawFrame = true
		progressed.Store(true)
		watchdog.Reset(budgets.Idle)
		if payload == "[DONE]" {
			sawDone = true
			break
		}
		var chunk streamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return completion{}, resp.StatusCode, fmt.Errorf("%s [%s]: decode stream frame: %w", op, site, err)
		}
		if chunk.Error != nil {
			return completion{}, resp.StatusCode, fmt.Errorf("%s: %s", op, chunk.Error.Message)
		}
		if c.ID == "" {
			c.ID = chunk.ID
		}
		if c.Model == "" {
			c.Model = chunk.Model
		}
		if chunk.Usage != nil {
			c.Usage = *chunk.Usage
		}
		if chunk.OpenRouterMetadata.Attempt != 0 || len(chunk.OpenRouterMetadata.Endpoints.Available) > 0 {
			c.Meta = chunk.OpenRouterMetadata
		}
		for _, choice := range chunk.Choices {
			content.WriteString(choice.Delta.Content)
			// Reached only after a 2xx event-stream frame has been read, so a failed or
			// replayed request never reports a fragment (see ChatOptions.OnContentDelta).
			if onDelta != nil && choice.Delta.Content != "" {
				onDelta(choice.Delta.Content)
			}
			for _, tc := range choice.Delta.ToolCalls {
				for len(calls) <= tc.Index {
					calls = append(calls, &openaiToolCall{Type: "function"})
				}
				call := calls[tc.Index]
				if tc.ID != "" {
					call.ID = tc.ID
				}
				call.Function.Name += tc.Function.Name
				call.Function.Arguments += tc.Function.Arguments
			}
			if choice.FinishReason != nil && *choice.FinishReason != "" {
				c.FinishReason = *choice.FinishReason
				sawFinish = true
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return completion{}, resp.StatusCode, fail(fmt.Errorf("%s [%s]: read stream: %w", op, site, err))
	}
	// A cancellation that closed the body cleanly still has to surface as its budget.
	if err := fail(nil); err != nil {
		return completion{}, resp.StatusCode, err
	}
	if !sawFrame {
		return completion{}, resp.StatusCode, fmt.Errorf("%s: empty choices", op)
	}
	if !sawDone && !sawFinish {
		return completion{}, resp.StatusCode, fmt.Errorf("%s [%s]: stream ended before a finish_reason or [DONE] (connection dropped mid-reply)", op, site)
	}
	c.Message = openaiMessage{Role: "assistant", Content: content.String()}
	for _, call := range calls {
		c.Message.ToolCalls = append(c.Message.ToolCalls, *call)
	}
	return c, resp.StatusCode, nil
}

// finishCall is the single place a call is accounted for: one log line and one metric
// observation per call, success or failure, carrying the facts a failure analysis needs.
func (o *OpenAI) finishCall(ctx context.Context, op string, started time.Time, c completion, status int, err error) {
	site := CallSite(ctx)
	elapsed := time.Since(started)
	outcome := "ok"
	if err != nil {
		outcome = "error"
		var fired *budgetError
		var statusErr *StatusError
		switch {
		case errors.As(err, &fired):
			outcome = fired.outcome
		case errors.As(err, &statusErr):
			outcome = "http_error"
		case errors.Is(err, context.Canceled):
			outcome = "canceled"
		}
	}
	if o.metrics != nil {
		o.metrics.LLMCall(site, outcome, elapsed)
		if err == nil {
			o.metrics.LLMTokens(c.Usage.PromptTokens, c.Usage.CompletionTokens)
			o.metrics.LLMCachedTokens(c.Usage.PromptDetails.CachedTokens)
		}
	}
	log := o.log
	if log == nil {
		log = slog.Default()
	}
	attrs := []any{
		"call_site", site, "op", op, "provider", o.provider, "model", o.model, "status", status,
		"outcome", outcome, "duration", elapsed.Round(time.Millisecond),
		"first_token", c.FirstToken.Round(time.Millisecond), "finish_reason", c.FinishReason,
		"prompt_tokens", c.Usage.PromptTokens, "cached_tokens", c.Usage.PromptDetails.CachedTokens,
		"completion_tokens", c.Usage.CompletionTokens,
	}
	if err != nil {
		log.Warn("llm call failed", append(attrs, "err", err)...)
		return
	}
	log.Info("llm call", attrs...)
}
