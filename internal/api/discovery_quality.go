package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/quality"
)

func (s *Server) registerDiscoveryQuality(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "export-discovery-quality", Method: http.MethodGet, Path: "/v1/system/discovery-quality/export",
		Summary:     "Export local discovery-quality measurements",
		Description: "Admin only. Downloads privacy-safe daily aggregates and their referenced run snapshots; no raw observations or receipt keys are included.",
		Tags:        []string{"system"},
	}, RoleAdmin), s.discoveryQualityExport)
}

type discoveryQualityExportOutput struct {
	ContentDisposition string `header:"Content-Disposition"`
	Body               quality.Export
}

func (s *Server) discoveryQualityExport(ctx context.Context, _ *struct{}) (*discoveryQualityExportOutput, error) {
	if s.store == nil {
		return nil, huma.Error501NotImplemented("discovery-quality export is unavailable")
	}
	export, err := s.store.ExportQualityLedger(ctx, time.Now())
	if err != nil {
		return nil, huma.Error500InternalServerError("export discovery-quality measurements", err)
	}
	return &discoveryQualityExportOutput{
		ContentDisposition: `attachment; filename="loomarr-discovery-quality.json"`,
		Body:               export,
	}, nil
}
