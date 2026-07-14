package testkit

import (
	"context"
	"sync"
	"time"

	"github.com/mantonx/loomarr/internal/llm"
)

// LLM is the shared llm.Provider test double (CLAUDE.md: one mock per service).
// It is scriptable: a test enqueues the sequence of responses the model should
// return across the tool-use loop. This lets the grounding gate drive the exact
// paths §19 requires — a normal tool-call→final-JSON flow, AND the critical
// fabricated-titles case (a final JSON referencing ids that were never in a tool
// result), which the suggester's validation must drop.
type LLM struct {
	mu    sync.Mutex
	turns []llm.Response // dequeued front-to-back per Chat call
	// Delay simulates a slow model, to exercise JOB_TIMEOUT (§8). Zero = instant.
	Delay time.Duration
	Calls int
}

// NewLLM builds a scripted provider that returns the given responses in order,
// one per Chat call.
func NewLLM(responses ...llm.Response) *LLM {
	return &LLM{turns: responses}
}

func (m *LLM) Name() string { return "mock" }

func (m *LLM) Chat(ctx context.Context, _ []llm.Message, _ llm.ChatOptions) (llm.Response, error) {
	if m.Delay > 0 {
		select {
		case <-time.After(m.Delay):
		case <-ctx.Done():
			return llm.Response{}, ctx.Err() // honors JOB_TIMEOUT cancellation
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Calls++
	if len(m.turns) == 0 {
		return llm.Response{}, nil // no more scripted turns → empty final
	}
	next := m.turns[0]
	m.turns = m.turns[1:]
	return next, nil
}

// ToolCallResponse builds a Response that requests one tool call (turn 1 shape).
func ToolCallResponse(name string, args map[string]any) llm.Response {
	return llm.Response{ToolCalls: []llm.ToolCall{{ID: "call_1", Name: name, Arguments: args}}}
}

// FinalResponse builds a Response with final JSON content and no tool calls.
func FinalResponse(jsonContent string) llm.Response {
	return llm.Response{Content: jsonContent}
}

var _ llm.Provider = (*LLM)(nil)
