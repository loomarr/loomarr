// Package llm is the LLM provider abstraction (design §8): one provider-neutral
// Chat primitive with tool-use, implemented by exactly TWO wire kinds — Ollama
// (the homelab default) and OpenAI-compatible. `NewProvider` builds one or the
// other and nothing else.
//
// ⚠ **`llm.provider` names a WIRE KIND, not a brand**, and this doc said otherwise
// until 2026-08-10: it claimed an "Anthropic (opt-in)" provider, which does not
// exist — there is no anthropic.go, and no case for it. Claude is reached over the
// OpenAI-compatible wire through OpenRouter, which `llm.hosted_provider` selects as
// a BRAND on top of the same transport (see hosted.go). Conflating the two sends a
// reader looking for an implementation that was never written, and an operator
// looking for a provider value that nothing accepts.
//
// The provider only translates messages/tools to/from its wire format and
// normalizes quirks (Ollama tool-call arguments arrive as a parsed object, the
// OpenAI shape as a JSON string — see testkit/fixtures/llm/FINDINGS.md). The
// *grounding loop* lives in the suggester (internal/suggest), which owns the
// catalog tool — this package is a dumb pipe, so grounding logic isn't smeared
// across providers.
package llm

import (
	"context"
	"errors"
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

// Warmer is an OPTIONAL Provider capability (§8.2): preload the model so the next
// Chat doesn't pay the cold-load cost (~9s vs ~0.5s warm for an 8B local model).
//
// Deliberately separate from Provider rather than a method on it, because it is only
// meaningful for a LOCAL runtime that loads weights on demand. A hosted endpoint has
// no residency to manage — there is nothing for it to implement, and a no-op method
// on every provider would imply otherwise. Callers type-assert; a provider that
// doesn't implement it is simply never warmed.
type Warmer interface {
	// Warm loads the model, returning once it is resident. Best-effort by contract:
	// an error is worth logging but never fatal, since the next Chat loads it anyway.
	// Returns ErrNothingToWarm when there is no model to preload.
	Warm(ctx context.Context) error
}

// ErrNothingToWarm reports that a warm-up was declined because there is no model to
// preload — not that one was attempted and failed. A caller distinguishes it with
// errors.Is so it can stay silent instead of claiming either outcome.
//
// It exists because both alternatives lie. Returning nil makes a skip indistinguishable
// from a successful preload, and a fresh install logged `llm model warmed took=0` having
// warmed nothing; returning a plain error makes an expected, correct state read as a
// failure. This is a third outcome, so it gets its own name.
var ErrNothingToWarm = errors.New("llm: no model configured to warm")

// Evictor is the MIRROR of Warmer (§8.2, §9.1 V47): unload the local model from memory
// NOW, instead of waiting out keep_alive. It exists because the suggester and playout share
// one GPU — a resident 6GB model and several ffmpeg hardware encoders contend for VRAM, and a
// hardware encode that cannot allocate its device context fails silently (no frame → a black
// channel). A live stream is latency-critical and a suggestion can afford a cold reload, so
// when the encoders need VRAM, playout evicts the model and reclaims it.
//
// Optional and local-only for the same reason as Warmer: a hosted endpoint has no residency
// to manage. Callers type-assert; a provider that doesn't implement it is simply never evicted
// (and a hosted LLM never contends for the local GPU, so there is nothing to do).
type Evictor interface {
	// Evict unloads the model, returning once the request is acknowledged. Best-effort by
	// contract: an error is worth logging but never fatal — the worst case is the VRAM stays
	// occupied and the encode falls back to software (§9.1). ErrNothingToEvict when there is
	// no model to unload (nothing was resident to begin with).
	Evict(ctx context.Context) error
}

// ErrNothingToEvict mirrors ErrNothingToWarm: the unload was declined because there is no
// configured model to unload, not that one failed. Same three-outcome honesty.
var ErrNothingToEvict = errors.New("llm: no model configured to evict")

// ParseProvider validates the configured provider (§15 LLM_PROVIDER). `ollama` is
// the local default; `openai` is the OpenAI-compatible client (which also reaches
// hosted OpenAI/Gemini/Groq/OpenRouter/… and Ollama's own /v1 mode).
func ParseProvider(s string) (string, error) {
	switch s {
	case "ollama", "openai":
		return s, nil
	default:
		return "", fmt.Errorf("unknown LLM_PROVIDER %q (want ollama|openai)", s)
	}
}

// NewProvider builds the configured provider (§8). url is the base (Ollama
// baseURL, or the OpenAI-compatible base ending in /v1); key is used only by the
// openai provider. An unknown provider falls back to Ollama (the safe local
// default) after ParseProvider has already validated at startup.
func NewProvider(provider, url, model, key string) Provider {
	if provider == "openai" {
		return NewOpenAI(url, model, key)
	}
	return NewOllama(url, model)
}
