package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// Sentinels the SystemLLMService returns; the handlers map them to HTTP status.
var (
	// ErrModelNotPulled: Select was asked for a model the host hasn't downloaded.
	ErrModelNotPulled = errors.New("model not pulled")
	// ErrNotLocal: select/pull was called while the provider isn't local Ollama.
	ErrNotLocal = errors.New("provider is not local ollama")
)

// SystemLLMService backs /v1/system/llm* (§8.1 model selection). Implemented in the
// composition root over llm.Prober + llm.Swappable + the settings store. The API
// layer stays decoupled from the llm types (like SearchService/catalog) by speaking
// these plain view structs.
type SystemLLMService interface {
	// Status returns the current active model + provider and, for a local Ollama
	// provider, the machine probe + annotated catalog. For a hosted provider the
	// catalog is empty (nothing local to probe/pull) and Local is false.
	Status(ctx context.Context) (SystemLLMStatus, error)
	// Select sets the active local model (persisted + hot-swapped). Returns
	// ErrModelNotPulled if the host hasn't pulled it (→ 409), or ErrNotLocal if the
	// provider isn't local Ollama (→ 409).
	Select(ctx context.Context, model string) error
	// Pull starts a background Ollama pull of a catalog model, streaming progress
	// over the event bus. Returns a job id. ErrNotLocal if not a local provider.
	Pull(ctx context.Context, model string) (jobID string, err error)
}

// SystemLLMStatus is the API view of the LLM host + selection (§8.1).
type SystemLLMStatus struct {
	Provider    string         `json:"provider" doc:"Active provider: ollama|openai"`
	Model       string         `json:"model" doc:"Currently active model tag/id"`
	Local       bool           `json:"local" doc:"True for local Ollama (probe + catalog apply)"`
	GPUName     string         `json:"gpuName,omitempty"`
	VRAMGiB     float64        `json:"vramGiB,omitempty" doc:"Detected GPU VRAM (0 = unknown/none)"`
	OllamaVer   string         `json:"ollamaVersion,omitempty"`
	Reachable   bool           `json:"reachable" doc:"LLM host answered the probe"`
	Recommended string         `json:"recommended,omitempty" doc:"Best-fit model tag for this machine"`
	Catalog     []LLMModelView `json:"catalog" doc:"Curated local models annotated for this machine"`
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
		Description: "Admin only. Active model + provider; for local Ollama, detected VRAM/version, " +
			"pulled models, and the curated catalog with fit verdicts + a recommended default (§8.1).",
		Tags: []string{"system"},
	}, s.systemLLMStatus)

	huma.Register(api, huma.Operation{
		OperationID: "system-llm-select", Method: http.MethodPost, Path: "/v1/system/llm/select",
		Summary: "Select the active local model",
		Description: "Admin only. Persists the choice and hot-swaps the running suggester (§8.1). " +
			"409 if the model isn't pulled yet, or the provider isn't local Ollama.",
		Tags: []string{"system"},
	}, s.systemLLMSelect)

	huma.Register(api, huma.Operation{
		OperationID: "system-llm-pull", Method: http.MethodPost, Path: "/v1/system/llm/pull",
		Summary: "Download a local model",
		Description: "Admin only. Starts an Ollama pull as a background job; percent-complete " +
			"streams over /v1/events (§8.1). Idempotent.",
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
		Model string `json:"model" doc:"Ollama model tag to activate (must be pulled)" example:"qwen3:8b"`
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
	switch err := s.systemLLM.Select(ctx, in.Body.Model); err {
	case nil:
	case ErrModelNotPulled:
		return nil, huma.Error409Conflict("model not pulled yet — POST /v1/system/llm/pull first")
	case ErrNotLocal:
		return nil, huma.Error409Conflict("model selection applies only to a local Ollama provider")
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
