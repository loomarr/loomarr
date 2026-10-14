package filler

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/llm"
)

type optsCapture struct{ opts []llm.ChatOptions }

func (c *optsCapture) Name() string { return "capture" }
func (c *optsCapture) Chat(_ context.Context, _ []llm.Message, opts llm.ChatOptions) (llm.Response, error) {
	c.opts = append(c.opts, opts)
	return llm.Response{Content: `{"spans":[]}`}, nil
}

// The rescue call is a bounded JSON extraction: it must not run at the server's default
// temperature with no output limit (a runaway answer holds a single-slot server for minutes).
func TestFindAdBreaks_SendsBoundedLowTemperatureJSONRequest(t *testing.T) {
	model := &optsCapture{}
	_, _ = findAdBreaks(context.Background(), model, []TranscriptSegment{{StartMs: 0, EndMs: 25000, Text: "buy soap"}}, 30000, newSegmentFloor(0))
	if len(model.opts) != 1 {
		t.Fatalf("calls = %d", len(model.opts))
	}
	opts := model.opts[0]
	if !opts.JSONMode || opts.MaxTokens <= 0 || opts.MaxTokens > 1024 || opts.Temperature == nil || *opts.Temperature > 0.2 {
		t.Errorf("rescue options = %+v, want JSON mode, max_tokens <= 1024, temperature <= 0.2", opts)
	}
}
