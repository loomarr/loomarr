package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// Sentinels the SystemLLMService returns; the handlers map them to HTTP status.
var (
	// ErrModelNotPulled: Select was asked for a local model the host hasn't downloaded.
	ErrModelNotPulled = errors.New("model not pulled")
	// ErrNotLocal: pull was called while the provider isn't local Ollama.
	ErrNotLocal = errors.New("provider is not local ollama")
	// ErrKeyInvalid: a hosted select/test key failed live validation.
	ErrKeyInvalid = errors.New("api key invalid or provider unreachable")
	// ErrUnknownProvider: a select/test named a provider not in the curated catalog.
	ErrUnknownProvider = errors.New("unknown provider")
)

// SystemLLMService backs /v1/system/llm* (§8.1 model selection). Implemented in the
// composition root over llm.Prober + llm.Swappable + the settings store. The API
// layer stays decoupled from the llm types (like SearchService/catalog) by speaking
// these plain view structs.
type SystemLLMService interface {
	// Status returns the active provider + model, the local-model catalog (probed
	// against the machine when the active provider is Ollama), and the curated
	// hosted-provider catalog with keyConfigured flags. API keys are never included.
	Status(ctx context.Context) (SystemLLMStatus, error)
	// Select sets the active provider + model (persisted + hot-swapped). For a local
	// model, ErrModelNotPulled if the host hasn't pulled it (→ 409). For a hosted
	// provider, the key (if given) is validated live first — a bad key returns
	// ErrKeyInvalid (→ 401). ErrUnknownProvider (→ 422) for an unrecognized provider.
	Select(ctx context.Context, sel SelectRequest) error
	// Test validates a hosted provider + key WITHOUT swapping (the "test my key"
	// button). Returns nil if reachable + authorized, else a descriptive error.
	Test(ctx context.Context, provider, apiKey string) error
	// Pull starts a background Ollama pull of a catalog model, streaming progress
	// over the event bus. Returns a job id. ErrNotLocal if not a local provider.
	Pull(ctx context.Context, model string) (jobID string, err error)
}

// SelectRequest is a provider/model/key selection (§8.1). Provider "ollama" (or
// empty, back-compat) selects a local model; any other value names a curated hosted
// provider. APIKey is used only for hosted providers and is stored, never echoed.
type SelectRequest struct {
	Provider string
	Model    string
	APIKey   string
}

// SystemLLMStatus is the API view of the LLM host + selection (§8.1).
type SystemLLMStatus struct {
	Provider    string         `json:"provider" doc:"Active provider: ollama|openai"`
	Model       string         `json:"model" doc:"Currently active model tag/id"`
	Local       bool           `json:"local" doc:"True for local Ollama (probe + local catalog apply)"`
	GPUName     string         `json:"gpuName,omitempty"`
	VRAMGiB     float64        `json:"vramGiB,omitempty" doc:"Detected GPU VRAM (0 = unknown/none)"`
	OllamaVer   string         `json:"ollamaVersion,omitempty"`
	Reachable   bool           `json:"reachable" doc:"LLM host answered the probe"`
	Recommended string         `json:"recommended,omitempty" doc:"Best-fit local model tag for this machine"`
	Catalog     []LLMModelView `json:"catalog" doc:"Curated local models annotated for this machine"`
	// Hosted is the curated hosted-provider catalog (§8.1). Always present so the UI
	// can offer a switch to a hosted provider. API keys are NEVER included — only a
	// per-provider keyConfigured flag.
	Hosted []HostedProviderView `json:"hosted" doc:"Curated hosted providers + recommended models"`
}

// HostedProviderView is the API view of one curated hosted provider (§8.1). No key.
type HostedProviderView struct {
	Key           string            `json:"key" doc:"Loomarr provider id (openrouter|openai|anthropic|groq|gemini)"`
	Label         string            `json:"label"`
	BaseURL       string            `json:"baseUrl"`
	KeysURL       string            `json:"keysUrl" doc:"Where to obtain an API key"`
	Note          string            `json:"note,omitempty"`
	KeyConfigured bool              `json:"keyConfigured" doc:"A key is stored for this provider (value never returned)"`
	Active        bool              `json:"active" doc:"This is the currently-selected provider"`
	ModelsLive    bool              `json:"modelsLive" doc:"Models were fetched live from the provider (false = curated fallback)"`
	Models        []HostedModelView `json:"models"`
}

// HostedModelView is one hosted model, ranked from live metadata (§8.1).
type HostedModelView struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Why         string `json:"why,omitempty" doc:"Rule-derived rationale (from live pricing/context/tools metadata)"`
	Recommended bool   `json:"recommended,omitempty" doc:"Rule-selected top pick (cheap + tool-capable)"`
	Tools       bool   `json:"tools,omitempty" doc:"Provider advertises tool-calling for this model"`
}

// LLMModelView is one catalog model annotated for the machine (§8.1).
type LLMModelView struct {
	Tag         string  `json:"tag"`
	Label       string  `json:"label"`
	VRAMGiB     float64 `json:"approxVramGiB"`
	Fit         string  `json:"fit" doc:"fits|tight|wont_fit against detected VRAM"`
	Pulled      bool    `json:"pulled" doc:"Already present in the local Ollama"`
	RuntimeOK   bool    `json:"runtimeOk" doc:"Detected Ollama version supports this model"`
	Recommended bool    `json:"recommended"`
	Why         string  `json:"why"`
}

// registerSystemLLM mounts /v1/system/llm* (§8.1). Admin-only — model selection
// changes what runs on the box and can trigger a multi-GB download.
func (s *Server) registerSystemLLM(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "system-llm-status", Method: http.MethodGet, Path: "/v1/system/llm",
		Summary: "LLM host probe + model catalog",
		Description: "Admin only. Active provider + model; the local-model catalog (for Ollama: " +
			"detected VRAM/version, fit verdicts, recommended default, pulled flags); and the hosted-" +
			"provider catalog (base URLs, live model lists, keyConfigured). API keys are never returned (§8.1).",
		Tags: []string{"system"},
	}, s.systemLLMStatus)

	huma.Register(api, huma.Operation{
		OperationID: "system-llm-select", Method: http.MethodPost, Path: "/v1/system/llm/select",
		Summary: "Select the active provider + model",
		Description: "Admin only. Persists the choice and hot-swaps the running suggester with no restart (§8.1). " +
			"Local model must be pulled (409 else). Hosted: an optional apiKey is validated live before committing " +
			"(401 on a bad key) and stored as a secret, never echoed.",
		Tags: []string{"system"},
	}, s.systemLLMSelect)

	huma.Register(api, huma.Operation{
		OperationID: "system-llm-test", Method: http.MethodPost, Path: "/v1/system/llm/test",
		Summary: "Test a hosted provider + key",
		Description: "Admin only. Validates a hosted provider + API key WITHOUT swapping (§8.1). " +
			"Returns ok + an error hint; a bad key is ok=false (not a 5xx).",
		Tags: []string{"system"},
	}, s.systemLLMTest)

	huma.Register(api, huma.Operation{
		OperationID: "system-llm-pull", Method: http.MethodPost, Path: "/v1/system/llm/pull",
		Summary: "Download a local model",
		Description: "Admin only. Local Ollama only (409 on a hosted provider). Starts a pull as a " +
			"background job; percent-complete streams over /v1/events (§8.1). Idempotent.",
		Tags: []string{"system"},
	}, s.systemLLMPull)
}

func (s *Server) systemLLMStatus(ctx context.Context, _ *struct{}) (*struct {
	Body SystemLLMStatus
}, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.systemLLM == nil {
		return nil, huma.Error501NotImplemented("LLM model management not configured")
	}
	st, err := s.systemLLM.Status(ctx)
	if err != nil {
		return nil, huma.Error502BadGateway("LLM probe failed", err)
	}
	return &struct{ Body SystemLLMStatus }{Body: st}, nil
}

type systemLLMSelectInput struct {
	Body struct {
		Provider string `json:"provider,omitempty" doc:"Provider: ollama (local) or a hosted key (openrouter|openai|anthropic|groq|gemini). Empty = ollama." example:"openrouter"`
		Model    string `json:"model" doc:"Model tag/id to activate. Local: must be pulled. Hosted: any id the provider serves." example:"openai/gpt-4o-mini"`
		APIKey   string `json:"apiKey,omitempty" doc:"Hosted API key (validated live, stored as a secret, never echoed). Omit to reuse a stored key." example:"sk-or-v1-…"`
	}
}

func (s *Server) systemLLMSelect(ctx context.Context, in *systemLLMSelectInput) (*struct {
	Body SystemLLMStatus
}, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.systemLLM == nil {
		return nil, huma.Error501NotImplemented("LLM model management not configured")
	}
	if in.Body.Model == "" {
		return nil, huma.Error422UnprocessableEntity("model is required")
	}
	req := SelectRequest{Provider: in.Body.Provider, Model: in.Body.Model, APIKey: in.Body.APIKey}
	switch err := s.systemLLM.Select(ctx, req); err {
	case nil:
	case ErrModelNotPulled:
		return nil, huma.Error409Conflict("model not pulled yet — POST /v1/system/llm/pull first")
	case ErrUnknownProvider:
		return nil, huma.Error422UnprocessableEntity("unknown provider — choose one from GET /v1/system/llm")
	case ErrKeyInvalid:
		return nil, huma.Error401Unauthorized("API key rejected — check the key (or POST /v1/system/llm/test)")
	default:
		return nil, huma.Error502BadGateway("select failed", err)
	}
	// Return the fresh status so the UI reflects the new active model immediately.
	st, err := s.systemLLM.Status(ctx)
	if err != nil {
		return nil, huma.Error502BadGateway("select applied but status re-read failed", err)
	}
	return &struct{ Body SystemLLMStatus }{Body: st}, nil
}

type systemLLMTestInput struct {
	Body struct {
		Provider string `json:"provider" doc:"Hosted provider key to test (openrouter|openai|anthropic|groq|gemini)" example:"openrouter"`
		APIKey   string `json:"apiKey,omitempty" doc:"Key to test; omit to test a stored key" example:"sk-or-v1-…"`
	}
}

// systemLLMTest validates a hosted provider + key without swapping (§8.1).
func (s *Server) systemLLMTest(ctx context.Context, in *systemLLMTestInput) (*struct {
	Body struct {
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
}, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.systemLLM == nil {
		return nil, huma.Error501NotImplemented("LLM model management not configured")
	}
	if in.Body.Provider == "" {
		return nil, huma.Error422UnprocessableEntity("provider is required")
	}
	out := &struct {
		Body struct {
			OK    bool   `json:"ok"`
			Error string `json:"error,omitempty"`
		}
	}{}
	switch err := s.systemLLM.Test(ctx, in.Body.Provider, in.Body.APIKey); err {
	case nil:
		out.Body.OK = true
	case ErrUnknownProvider:
		return nil, huma.Error422UnprocessableEntity("unknown provider — choose one from GET /v1/system/llm")
	default:
		// A bad key / unreachable provider is a normal, reportable outcome (not a
		// server error): 200 with ok=false + the reason, so the UI shows it inline.
		out.Body.OK = false
		out.Body.Error = err.Error()
	}
	return out, nil
}

type systemLLMPullInput struct {
	Body struct {
		Model string `json:"model" doc:"Ollama model tag to download" example:"qwen3.5:9b"`
	}
}

func (s *Server) systemLLMPull(ctx context.Context, in *systemLLMPullInput) (*struct {
	Body struct {
		JobID string `json:"jobId" doc:"Poll progress on /v1/events (type=llm_pull)"`
	}
}, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.systemLLM == nil {
		return nil, huma.Error501NotImplemented("LLM model management not configured")
	}
	if in.Body.Model == "" {
		return nil, huma.Error422UnprocessableEntity("model is required")
	}
	jobID, err := s.systemLLM.Pull(ctx, in.Body.Model)
	if err == ErrNotLocal {
		return nil, huma.Error409Conflict("pull applies only to a local Ollama provider")
	}
	if err != nil {
		return nil, huma.Error502BadGateway("pull failed to start", err)
	}
	out := &struct {
		Body struct {
			JobID string `json:"jobId" doc:"Poll progress on /v1/events (type=llm_pull)"`
		}
	}{}
	out.Body.JobID = jobID
	return out, nil
}
