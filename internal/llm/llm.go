// Package llm is the LLM provider abstraction (design §8): one provider-neutral
// Chat primitive with tool-use, implemented by Ollama (homelab default) and
// Anthropic (opt-in). The provider only translates messages/tools to/from its
// wire format and normalizes provider quirks (Ollama tool-call arguments are a
// parsed object; Anthropic's are a JSON string + require tool_use_id echo — see
// testkit/fixtures/llm/FINDINGS.md). The *grounding loop* lives in the suggester
// (internal/suggest), which owns the catalog tool — this package is a dumb pipe,
// so grounding logic isn't smeared across providers.
package llm

import (
	"context"
	"fmt"
)

// Role is a chat message role.
type Role string

const (
	System    Role = "system"
	User      Role = "user"
	Assistant Role = "assistant"
	Tool      Role = "tool"
)

// Message is one provider-neutral chat message. A Tool message carries the JSON
// result of a tool call the assistant requested; an Assistant message may carry
// ToolCalls it wants executed.
type Message struct {
	Role      Role
	Content   string     // text (or the JSON tool result, for Role==Tool)
	ToolCalls []ToolCall // set on an Assistant turn that requests tools
	// ToolCallID correlates a Tool-result message to the call it answers.
	// Required by Anthropic; Ollama ignores it (FINDINGS finding 2). The suggester
	// always sets it so both providers work.
	ToolCallID string
}

// ToolCall is a model's request to invoke a tool. Arguments is always a decoded
// object (the Ollama provider passes it through; the Anthropic provider parses
// its JSON-string form) — so the suggester sees one shape (FINDINGS finding 1).
type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
}

// ToolSchema is a provider-neutral tool definition (JSON-schema parameters). The
// suggester supplies the catalog tool; providers translate it to their format.
type ToolSchema struct {
	Name        string
	Description string
	Parameters  map[string]any // JSON Schema object for the arguments
}

// Response is one assistant turn: either tool calls to execute, or final content.
type Response struct {
	Content   string     // final text (JSON when JSONMode was requested)
	ToolCalls []ToolCall // non-empty ⇒ the model wants tools run before continuing
}

// WantsTools reports whether the model requested tool execution this turn.
func (r Response) WantsTools() bool { return len(r.ToolCalls) > 0 }

// ChatOptions tune a single Chat call.
type ChatOptions struct {
	Tools    []ToolSchema // tools the model may call this turn
	JSONMode bool         // force the final content to be valid JSON (§8 output contract)
	// Sampling controls (provider-neutral). Pointers so "unset" is distinct from a
	// deliberate 0 — temperature 0 is a valid, desirable value for JSON/tool-calling
	// (deterministic, schema-adherent). A nil pointer ⇒ the provider's default.
	Temperature *float64
	TopP        *float64
	MaxTokens   int // 0 ⇒ provider default
}

// Provider is the provider-neutral LLM primitive (§8). One Chat turn: given the
// conversation so far + options, return the model's next turn (tool calls or
// final content). The suggester loops on this to run the grounding cycle.
type Provider interface {
	Chat(ctx context.Context, messages []Message, opts ChatOptions) (Response, error)
	// Name identifies the provider for logs/metrics.
	Name() string
}

// ParseProvider validates the configured provider (§15 LLM_PROVIDER).
func ParseProvider(s string) (string, error) {
	switch s {
	case "ollama", "anthropic":
		return s, nil
	default:
		return "", fmt.Errorf("unknown LLM_PROVIDER %q (want ollama|anthropic)", s)
	}
}
