package llm

// ChatProfile identifies a versioned sampling policy, not a model selection.
type ChatProfile string

const GroundedSelection ChatProfile = "grounded-selection-v1"

// ChatSampling is the effective optional sampling sent by the text adapter.
// Mandatory execution controls remain in ChatOptions and the route configuration.
type ChatSampling struct {
	Temperature     *float64
	TopP            *float64
	ReasoningEffort string
}

// ResolveChatSampling shares the text wire policy with diagnostic reporting.
func ResolveChatSampling(provider, model string, opts ChatOptions) ChatSampling {
	sampling := ChatSampling{Temperature: opts.Temperature, TopP: opts.TopP}
	// Exact OpenRouter capability entry, checked against the model and Vertex
	// endpoint metadata. Do not infer compatibility for a future model version.
	if opts.Profile == GroundedSelection && provider == "openrouter" && model == "google/gemini-3.8-flash" {
		sampling.Temperature = nil
		sampling.ReasoningEffort = "low"
	}
	return sampling
}
