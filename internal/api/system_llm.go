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
	Test(ctx context.Context, provider, baseURL, apiKey string) error
	// Pull starts a background Ollama pull of a catalog model, streaming progress
	// over the event bus. Returns a job id. ErrNotLocal if not a local provider.
	Pull(ctx context.Context, model string) (jobID string, err error)
	// Discover returns downloadable local models that are COMPATIBLE with this machine,
	// ranked best-first — the most-popular GGUF repos, each sized against detected VRAM
	// with its best-fitting quant chosen, repos where nothing fits dropped. Best-effort:
	// a source failure returns an empty list + error so the UI degrades to a link.
	Discover(ctx context.Context) ([]DiscoverModelView, error)
}

// DiscoverModelView is one downloadable local-model candidate, already sized + fit-ranked
// for this machine (§8.1). VRAM fit IS known (from the chosen quant's real file size);
// tool-capability is confirmed only after it is pulled and probed. PullRef is the exact
// argument to hand back to Pull.
type DiscoverModelView struct {
	ID          string  `json:"id" doc:"Source repo id, e.g. unsloth/Qwen3.5-4B-GGUF"`
	Label       string  `json:"label" doc:"Human-friendly name, e.g. Qwen3.5 4B"`
	Quant       string  `json:"quant" doc:"The build we sized against — Ollama's latest resolves to it (e.g. Q4_K_M). Informational; the pull is by bare repo ref."`
	PullRef     string  `json:"pullRef" doc:"Model ref to POST to /pull — the bare repo (e.g. hf.co/unsloth/Qwen3.5-4B-GGUF); Ollama pulls its latest tag"`
	SizeGiB     float64 `json:"sizeGiB" doc:"Sized build's on-disk size (VRAM proxy) — what latest downloads"`
	Fit         string  `json:"fit" doc:"fits|tight|wont_fit against detected VRAM"`
	Downloads   int64   `json:"downloads" doc:"Source popularity — a ranking tiebreak only; not shown to the user (§8.1)"`
	Role        string  `json:"role" doc:"Plain-English job in the user's choice: balanced|faster|higher_quality (§8.1)"`
	Recommended bool    `json:"recommended" doc:"True for the single safe-default pick for this machine — the hero card (§8.1). At most one per list."`
	Note        string  `json:"note,omitempty" doc:"Plain-English why-pick-this hint derived from role + fit (§8.1). Presentation only; may be empty."`
}

// SelectRequest is a provider/model/key selection (§8.1). Provider "ollama" (or
// empty, back-compat) selects a local model; "openrouter" the blessed aggregator;
// "custom" a user-supplied OpenAI-compatible endpoint (BaseURL then required).
// APIKey is used only for hosted providers and is stored, never echoed.
type SelectRequest struct {
	Provider string
	Model    string
	APIKey   string
	BaseURL  string // required for the "custom" provider; ignored otherwise
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
	Key           string            `json:"key" doc:"Loomarr provider id (openrouter|custom)"`
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
	Tools       bool    `json:"tools" doc:"Ollama reports tool-calling — required to ground suggestions; a false model is shown but not selectable"`
	Recommended bool    `json:"recommended"`
	Why         string  `json:"why"`
}

// registerSystemLLM mounts /v1/system/llm* (§8.1). Admin-only — model selection
// changes what runs on the box and can trigger a multi-GB download.
func (s *Server) registerSystemLLM(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "system-llm-status", Method: http.MethodGet, Path: "/v1/system/llm",
		Summary: "LLM host probe + model catalog",
		Description: "Admin only. Active provider + model; the local-model catalog (for Ollama: " +
			"detected VRAM/version, fit verdicts, recommended default, pulled flags); and the hosted-" +
			"provider catalog (base URLs, live model lists, keyConfigured). API keys are never returned (§8.1).",
		Tags: []string{"system"},
	}, RoleAdmin), s.systemLLMStatus)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "system-llm-select", Method: http.MethodPost, Path: "/v1/system/llm/select",
		Summary: "Select the active provider + model",
		Description: "Admin only. Persists the choice and hot-swaps the running suggester with no restart (§8.1). " +
			"Local model must be pulled (409 else). Hosted: an optional apiKey is validated live before committing " +
			"(401 on a bad key) and stored as a secret, never echoed.",
		Tags: []string{"system"},
	}, RoleAdmin), s.systemLLMSelect)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "system-llm-test", Method: http.MethodPost, Path: "/v1/system/llm/test",
		Summary: "Test a hosted provider + key",
		Description: "Admin only. Validates a hosted provider + API key WITHOUT swapping (§8.1). " +
			"Returns ok + an error hint; a bad key is ok=false (not a 5xx).",
		Tags: []string{"system"},
	}, RoleAdmin), s.systemLLMTest)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "system-llm-pull", Method: http.MethodPost, Path: "/v1/system/llm/pull",
		Summary: "Download a local model",
		Description: "Admin only. Local Ollama only (409 on a hosted provider). Starts a pull as a " +
			"background job; percent-complete streams over /v1/events (§8.1). Idempotent.",
		Tags: []string{"system"},
	}, RoleAdmin), s.systemLLMPull)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "system-llm-discover", Method: http.MethodGet, Path: "/v1/system/llm/discover",
		Summary: "Compatible downloadable local models",
		Description: "Admin only. Returns the most-popular downloadable GGUF models (Hugging Face) that " +
			"are COMPATIBLE with this machine, ranked best-first (§8.1): each repo's best-fitting quant is " +
			"chosen against detected VRAM and repos where nothing fits are dropped. Each result carries a " +
			"pullRef to hand to POST /pull. Tool-capability is confirmed only AFTER the model is pulled and " +
			"probed. Best-effort: if the source is unreachable the list is empty (browse on huggingface.co).",
		Tags: []string{"system"},
	}, RoleAdmin), s.systemLLMDiscover)
}

func (s *Server) systemLLMDiscover(ctx context.Context, _ *struct{}) (*struct {
	Body struct {
		Models   []DiscoverModelView `json:"models"`
		SourceOK bool                `json:"sourceOk" doc:"False if the model catalog (Hugging Face) was unreachable — the UI shows a browse link, not an empty state"`
	}
}, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.systemLLM == nil {
		return nil, errNotImplemented("Model management unavailable", "Local model management isn't set up on this server.")
	}
	out := &struct {
		Body struct {
			Models   []DiscoverModelView `json:"models"`
			SourceOK bool                `json:"sourceOk" doc:"False if the model catalog (Hugging Face) was unreachable — the UI shows a browse link, not an empty state"`
		}
	}{}
	models, err := s.systemLLM.Discover(ctx)
	if err != nil {
		// A source outage is a degraded, reportable state — not a 5xx, and NOT the same
		// as "zero compatible models". sourceOk=false lets the UI say "couldn't reach the
		// catalog, browse on huggingface.co" instead of the misleading "none found".
		out.Body.SourceOK = false
		out.Body.Models = nil
		return out, nil
	}
	out.Body.SourceOK = true
	out.Body.Models = models
	return out, nil
}

func (s *Server) systemLLMStatus(ctx context.Context, _ *struct{}) (*struct {
	Body SystemLLMStatus
}, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.systemLLM == nil {
		return nil, errNotImplemented("Model management unavailable", "Local model management isn't set up on this server.")
	}
	st, err := s.systemLLM.Status(ctx)
	if err != nil {
		return nil, apiErrWithCause(http.StatusBadGateway, "AI provider unreachable", "Loomarr couldn't reach the AI provider to check its status. Verify the connection and try again.", err)
	}
	return &struct{ Body SystemLLMStatus }{Body: st}, nil
}

type systemLLMSelectInput struct {
	Body struct {
		Provider string `json:"provider,omitempty" doc:"Provider: ollama (local), openrouter, or custom. Empty = ollama." example:"openrouter"`
		Model    string `json:"model" doc:"Model tag/id to activate. Local: must be pulled. Hosted: any id the provider serves." example:"openai/gpt-4o-mini"`
		APIKey   string `json:"apiKey,omitempty" doc:"Hosted API key (validated live, stored as a secret, never echoed). Omit to reuse a stored key." example:"sk-or-v1-…"`
		BaseURL  string `json:"baseUrl,omitempty" doc:"Required for provider=custom: an OpenAI-compatible base URL (…/v1). Ignored otherwise." example:"http://localhost:8000/v1"`
	}
}

func (s *Server) systemLLMSelect(ctx context.Context, in *systemLLMSelectInput) (*struct {
	Body SystemLLMStatus
}, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.systemLLM == nil {
		return nil, errNotImplemented("Model management unavailable", "Local model management isn't set up on this server.")
	}
	if in.Body.Model == "" {
		return nil, errUnprocessable("Model required", "Choose a model to activate.")
	}
	if in.Body.Provider == "custom" && in.Body.BaseURL == "" {
		return nil, errUnprocessable("Base URL required", "A custom endpoint needs its base URL. Enter the OpenAI-compatible base URL for your provider.")
	}
	req := SelectRequest{Provider: in.Body.Provider, Model: in.Body.Model, APIKey: in.Body.APIKey, BaseURL: in.Body.BaseURL}
	switch err := s.systemLLM.Select(ctx, req); err {
	case nil:
	case ErrModelNotPulled:
		return nil, errConflict("Model not ready", "Download this model before selecting it.")
	case ErrUnknownProvider:
		return nil, errUnprocessable("Unknown provider", "That AI provider isn't recognized. Pick one of the listed providers.")
	case ErrKeyInvalid:
		return nil, errUnauthorized("API key rejected", "That API key was rejected. Check the key and try again, or test it first.")
	default:
		return nil, apiErrWithCause(http.StatusBadGateway, "Couldn't select the model", "Loomarr couldn't activate that model. Check the AI provider connection and try again.", err)
	}
	// Return the fresh status so the UI reflects the new active model immediately.
	st, err := s.systemLLM.Status(ctx)
	if err != nil {
		return nil, apiErrWithCause(http.StatusBadGateway, "Model selected, status unavailable", "The model was activated, but Loomarr couldn't re-read the provider status. Refresh to see the current state.", err)
	}
	return &struct{ Body SystemLLMStatus }{Body: st}, nil
}

type systemLLMTestInput struct {
	Body struct {
		Provider string `json:"provider" doc:"Hosted provider key to test: openrouter or custom" example:"openrouter"`
		APIKey   string `json:"apiKey,omitempty" doc:"Key to test; omit to test a stored key" example:"sk-or-v1-…"`
		BaseURL  string `json:"baseUrl,omitempty" doc:"Required for provider=custom: an OpenAI-compatible base URL (…/v1)." example:"http://localhost:8000/v1"`
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
		return nil, errNotImplemented("Model management unavailable", "Local model management isn't set up on this server.")
	}
	if in.Body.Provider == "" {
		return nil, errUnprocessable("Provider required", "Choose which AI provider to test.")
	}
	if in.Body.Provider == "custom" && in.Body.BaseURL == "" {
		return nil, errUnprocessable("Base URL required", "A custom endpoint needs its base URL. Enter the OpenAI-compatible base URL for your provider.")
	}
	out := &struct {
		Body struct {
			OK    bool   `json:"ok"`
			Error string `json:"error,omitempty"`
		}
	}{}
	switch err := s.systemLLM.Test(ctx, in.Body.Provider, in.Body.BaseURL, in.Body.APIKey); err {
	case nil:
		out.Body.OK = true
	case ErrUnknownProvider:
		return nil, errUnprocessable("Unknown provider", "That AI provider isn't recognized. Pick one of the listed providers.")
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
		return nil, errNotImplemented("Model management unavailable", "Local model management isn't set up on this server.")
	}
	if in.Body.Model == "" {
		return nil, errUnprocessable("Model required", "Enter the model to download.")
	}
	jobID, err := s.systemLLM.Pull(ctx, in.Body.Model)
	if err == ErrNotLocal {
		return nil, errConflict("Download not supported", "Downloading models only applies to a local Ollama provider.")
	}
	if err != nil {
		return nil, apiErrWithCause(http.StatusBadGateway, "Couldn't start the download", "Loomarr couldn't start downloading that model. Check the Ollama connection and try again.", err)
	}
	out := &struct {
		Body struct {
			JobID string `json:"jobId" doc:"Poll progress on /v1/events (type=llm_pull)"`
		}
	}{}
	out.Body.JobID = jobID
	return out, nil
}
