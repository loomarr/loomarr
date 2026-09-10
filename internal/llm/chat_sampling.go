package llm

// ChatProfile identifies a versioned request policy, not a model selection.
type ChatProfile string

const GroundedSelection ChatProfile = "grounded-selection-v2"

// ChatPolicy describes effective parameters for the selected task and model.
// Completion bounds remain caller-owned; explicit routes remain authoritative.
type ChatPolicy struct {
	Temperature              *float64
	TopP                     *float64
	ReasoningEffort          string
	CompletionLimitParameter string
	DefaultUpstream          string
}

// ResolveChatPolicy shares the text wire policy with diagnostic reporting.
func ResolveChatPolicy(provider, model string, opts ChatOptions) ChatPolicy {
	sampling := ChatPolicy{Temperature: opts.Temperature, TopP: opts.TopP, CompletionLimitParameter: "max_tokens"}
	// Exact OpenRouter capability entry, checked against the model and Vertex
	// endpoint metadata. Do not infer compatibility for a future model version.
	if opts.Profile == GroundedSelection && provider == "openrouter" && model == "google/gemini-3.8-flash" {
		sampling.Temperature = nil
		sampling.ReasoningEffort = "low"
	}
	if opts.Profile == GroundedSelection && provider == "openrouter" && model == "openai/gpt-5.6-sol" {
		sampling.Temperature = nil
		sampling.ReasoningEffort = "none"
		sampling.CompletionLimitParameter = "max_completion_tokens"
		sampling.DefaultUpstream = "azure/us"
	}
	return sampling
}
