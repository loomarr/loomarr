package llm_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/llm"
)

// sseServer writes each frame, flushing, after its delay. A nil-frame list with
// stall=true then blocks until the client hangs up, which is what a wedged
// single-slot llama.cpp looks like from outside.
type sseFrame struct {
	after time.Duration
	data  string
}

func sseServer(t *testing.T, frames []sseFrame, stall bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		flusher.Flush()
		for _, f := range frames {
			select {
			case <-time.After(f.after):
			case <-r.Context().Done():
				return
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", f.data)
			flusher.Flush()
		}
		if stall {
			<-r.Context().Done()
			return
		}
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	t.Cleanup(srv.Close)
	return srv
}

func steadyFrames(first time.Duration) []sseFrame {
	return []sseFrame{
		{first, `{"id":"g1","model":"m","choices":[{"delta":{"content":"hel"}}]}`},
		{40 * time.Millisecond, `{"choices":[{"delta":{"content":"lo"}}]}`},
		{40 * time.Millisecond, `{"choices":[{"delta":{},"finish_reason":"length"}]}`},
		{40 * time.Millisecond, `{"choices":[],"usage":{"prompt_tokens":100,"completion_tokens":7,"prompt_tokens_details":{"cached_tokens":64}}}`},
	}
}

// The production shape: a queue in front of a single slot delays the first token
// far past any idle budget, then decode streams steadily. Today's fixed
// whole-request timeout fails this; a first-token budget plus an idle budget passes.
func TestOpenAI_SlowFirstTokenThenSteadyStreamSucceeds(t *testing.T) {
	srv := sseServer(t, steadyFrames(400*time.Millisecond), false)
	o := llm.NewOpenAI(srv.URL, "m", "").WithTimeouts(llm.Timeouts{
		FirstToken: 3 * time.Second, Idle: 250 * time.Millisecond, Total: 10 * time.Second,
	})
	ctx := llm.WithCallSite(context.Background(), "test.slow_first")
	resp, err := o.Chat(ctx, []llm.Message{{Role: llm.User, Content: "hi"}}, llm.ChatOptions{})
	if err != nil {
		t.Fatalf("slow-first-token stream must succeed: %v", err)
	}
	if resp.Content != "hello" {
		t.Errorf("content = %q, want hello", resp.Content)
	}
	if resp.FinishReason != "length" {
		t.Errorf("finish_reason = %q, want length", resp.FinishReason)
	}
	tok := resp.Attribution.Tokens
	if tok.Prompt != 100 || tok.Completion != 7 || tok.Cached != 64 {
		t.Errorf("usage = %+v, want prompt 100 / completion 7 / cached 64", tok)
	}
}

func TestOpenAI_StreamAssemblesToolCallDeltas(t *testing.T) {
	srv := sseServer(t, []sseFrame{
		{0, `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"catalog_search","arguments":"{\"query\":"}}]}}]}`},
		{0, `{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"matrix\"}"}}]},"finish_reason":"tool_calls"}]}`},
	}, false)
	o := llm.NewOpenAI(srv.URL, "m", "")
	resp, err := o.Chat(context.Background(), []llm.Message{{Role: llm.User, Content: "hi"}},
		llm.ChatOptions{Tools: []llm.ToolSchema{{Name: "catalog_search"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "catalog_search" || resp.ToolCalls[0].Arguments["query"] != "matrix" {
		t.Fatalf("tool call not reassembled: %+v", resp.ToolCalls)
	}
}

// A stall mid-stream is a wedge, not queueing: it fails fast with a cause that
// says which budget fired and which call site was waiting.
func TestOpenAI_StallMidStreamFailsWithIdleTimeoutNamingCallSite(t *testing.T) {
	srv := sseServer(t, steadyFrames(0)[:2], true)
	o := llm.NewOpenAI(srv.URL, "m", "").WithTimeouts(llm.Timeouts{
		FirstToken: 3 * time.Second, Idle: 150 * time.Millisecond, Total: 10 * time.Second,
	})
	ctx := llm.WithCallSite(context.Background(), "suggest.grounding")
	started := time.Now()
	_, err := o.Chat(ctx, []llm.Message{{Role: llm.User, Content: "hi"}}, llm.ChatOptions{})
	if err == nil {
		t.Fatal("stalled stream must fail")
	}
	if !errors.Is(err, llm.ErrIdleTimeout) {
		t.Fatalf("want ErrIdleTimeout, got %v", err)
	}
	if !strings.Contains(err.Error(), "suggest.grounding") {
		t.Errorf("error must name the call site: %v", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("a timeout must still read as a deadline so provider_timeout is chosen: %v", err)
	}
	if time.Since(started) > 2*time.Second {
		t.Errorf("idle timeout took %s; it should fire near the 150ms budget", time.Since(started))
	}
}

func TestOpenAI_NoFirstTokenFailsWithFirstTokenTimeout(t *testing.T) {
	srv := sseServer(t, nil, true)
	o := llm.NewOpenAI(srv.URL, "m", "").WithTimeouts(llm.Timeouts{
		FirstToken: 150 * time.Millisecond, Idle: time.Second, Total: 10 * time.Second,
	})
	_, err := o.Chat(llm.WithCallSite(context.Background(), "filler.vision"),
		[]llm.Message{{Role: llm.User, Content: "hi"}}, llm.ChatOptions{})
	if !errors.Is(err, llm.ErrFirstTokenTimeout) || !strings.Contains(err.Error(), "filler.vision") {
		t.Fatalf("want first-token timeout naming filler.vision, got %v", err)
	}
}

func TestOpenAI_TotalCapBoundsASteadyStream(t *testing.T) {
	frames := make([]sseFrame, 100)
	for i := range frames {
		frames[i] = sseFrame{30 * time.Millisecond, `{"choices":[{"delta":{"content":"x"}}]}`}
	}
	srv := sseServer(t, frames, false)
	o := llm.NewOpenAI(srv.URL, "m", "").WithTimeouts(llm.Timeouts{
		FirstToken: time.Second, Idle: time.Second, Total: 300 * time.Millisecond,
	})
	_, err := o.Chat(context.Background(), []llm.Message{{Role: llm.User, Content: "hi"}}, llm.ChatOptions{})
	if !errors.Is(err, llm.ErrTotalTimeout) {
		t.Fatalf("want ErrTotalTimeout, got %v", err)
	}
}

// The body's own message must survive to whoever persists the failure.
func TestOpenAI_ServerErrorBodyMessageIsInTheError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"slot 0 is busy: context shift failed"}}`))
	}))
	defer srv.Close()
	o := llm.NewOpenAI(srv.URL, "m", "")
	_, err := o.Chat(context.Background(), []llm.Message{{Role: llm.User, Content: "hi"}}, llm.ChatOptions{})
	if err == nil || !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "slot 0 is busy") {
		t.Fatalf("status and body message must reach the error, got %v", err)
	}
}

// One INFO line per successful call and one WARN per failed call, carrying the
// facts #1410 could not see: call site, status, finish_reason, token usage.
func TestOpenAI_LogsOneLinePerCallWithFinishReasonAndUsage(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	srv := sseServer(t, steadyFrames(0), false)
	o := llm.NewOpenAI(srv.URL, "m", "").WithLogger(log)
	if _, err := o.Chat(llm.WithCallSite(context.Background(), "test.ok"),
		[]llm.Message{{Role: llm.User, Content: "hi"}}, llm.ChatOptions{}); err != nil {
		t.Fatal(err)
	}
	line := buf.String()
	for _, want := range []string{"level=INFO", "call_site=test.ok", "status=200", "finish_reason=length",
		"prompt_tokens=100", "cached_tokens=64", "completion_tokens=7"} {
		if !strings.Contains(line, want) {
			t.Errorf("success log missing %q:\n%s", want, line)
		}
	}
	if strings.Count(line, "\n") != 1 {
		t.Errorf("want exactly one log line per call:\n%s", line)
	}

	buf.Reset()
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":{"message":"upstream gone"}}`))
	}))
	defer bad.Close()
	o = llm.NewOpenAI(bad.URL, "m", "").WithLogger(log)
	_, _ = o.Chat(llm.WithCallSite(context.Background(), "test.bad"),
		[]llm.Message{{Role: llm.User, Content: "hi"}}, llm.ChatOptions{})
	line = buf.String()
	for _, want := range []string{"level=WARN", "call_site=test.bad", "status=502", "upstream gone"} {
		if !strings.Contains(line, want) {
			t.Errorf("failure log missing %q:\n%s", want, line)
		}
	}
}
