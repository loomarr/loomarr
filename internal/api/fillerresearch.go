package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/loomarr/loomarr/internal/fillerresearch"
)

type FillerResearchStatusDTO struct {
	StructuredEnabled bool   `json:"structuredEnabled" doc:"Whether Loomarr checks credential-free trusted public sources for missing clip details"`
	Provider          string `json:"provider" enum:"none,brave,searxng"`
	Configured        bool   `json:"configured"`
	State             string `json:"state" enum:"off,unconfigured,ready,degraded,limit_reached"`
	Month             string `json:"month" doc:"UTC usage month in YYYY-MM form"`
	RequestCount      int    `json:"requestCount" minimum:"0"`
	RequestLimit      int    `json:"requestLimit" minimum:"1"`
	LastSuccessAt     string `json:"lastSuccessAt,omitempty" doc:"RFC3339"`
	LastFailureAt     string `json:"lastFailureAt,omitempty" doc:"RFC3339"`
}

type fillerResearchStatusOutput struct {
	Body FillerResearchStatusDTO
}

type fillerResearchTestInput struct {
	Body struct {
		Provider string `json:"provider" enum:"brave,searxng"`
		APIKey   string `json:"apiKey,omitempty" doc:"Brave Search API key used for this validation only; never returned"`
		Endpoint string `json:"endpoint,omitempty" doc:"Credential-free SearXNG HTTPS endpoint used for this validation only"`
	}
}

type fillerResearchTestOutput struct {
	Body struct {
		OK      bool                    `json:"ok"`
		Message string                  `json:"message"`
		Status  FillerResearchStatusDTO `json:"status"`
	}
}

func (s *Server) registerFillerResearch(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "filler-research-status", Method: http.MethodGet, Path: "/v1/filler/research/status",
		Summary:     "Clip-detail research status",
		Description: "Admin only. Reports trusted-source and optional web-search availability and bounded monthly usage. Provider secrets are never returned.",
		Tags:        []string{"filler"},
	}, RoleAdmin), s.fillerResearchStatus)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "filler-research-test", Method: http.MethodPost, Path: "/v1/filler/research/test",
		Summary:     "Test a clip-detail web-search provider",
		Description: "Admin only. Validates proposed Brave Search or SearXNG details before they are saved. The check consumes one bounded monthly request and never returns the credential.",
		Tags:        []string{"filler"},
	}, RoleAdmin), s.fillerResearchTest)
}

func (s *Server) fillerResearchStatus(ctx context.Context, _ *struct{}) (*fillerResearchStatusOutput, error) {
	if s.fillerResearch == nil {
		return nil, errNotImplemented("Clip-detail research unavailable", "Clip-detail research isn't available on this server.")
	}
	status, err := s.fillerResearch.Status(ctx)
	if err != nil {
		return nil, apiErrWithCause(http.StatusInternalServerError, "Could not check clip-detail research", "Loomarr couldn't read the current research status. Try again.", err)
	}
	return &fillerResearchStatusOutput{Body: fillerResearchStatusDTO(status)}, nil
}

func (s *Server) fillerResearchTest(ctx context.Context, in *fillerResearchTestInput) (*fillerResearchTestOutput, error) {
	if s.fillerResearch == nil {
		return nil, errNotImplemented("Clip-detail research unavailable", "Clip-detail research isn't available on this server.")
	}
	config := fillerresearch.WebConfig{
		Provider:    fillerresearch.WebProvider(in.Body.Provider),
		BraveAPIKey: in.Body.APIKey,
		SearXNGURL:  in.Body.Endpoint,
	}
	status, ok, message, err := s.fillerResearch.Test(ctx, config)
	if err != nil {
		return nil, apiErrWithCause(http.StatusBadGateway, "Could not test web search", "Loomarr couldn't complete the provider check. Try again.", err)
	}
	out := &fillerResearchTestOutput{}
	out.Body.OK = ok
	out.Body.Message = message
	out.Body.Status = fillerResearchStatusDTO(status)
	return out, nil
}

func fillerResearchStatusDTO(status fillerresearch.WebStatus) FillerResearchStatusDTO {
	out := FillerResearchStatusDTO{
		StructuredEnabled: status.StructuredEnabled,
		Provider:          string(status.Provider), Configured: status.Configured, State: string(status.State),
		Month: status.Month, RequestCount: status.RequestCount, RequestLimit: status.RequestLimit,
	}
	if !status.LastSuccessAt.IsZero() {
		out.LastSuccessAt = status.LastSuccessAt.UTC().Format(time.RFC3339)
	}
	if !status.LastFailureAt.IsZero() {
		out.LastFailureAt = status.LastFailureAt.UTC().Format(time.RFC3339)
	}
	return out
}
