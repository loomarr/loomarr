package api

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
)

func transformError(t *testing.T, ctx context.Context, status int) string {
	t.Helper()
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	req := httptest.NewRequest(http.MethodGet, "/v1/guide", nil).WithContext(ctx)
	hctx := humatest.NewContext(&huma.Operation{Method: http.MethodGet, Path: "/v1/guide"}, req, httptest.NewRecorder())
	model := &huma.ErrorModel{Status: status, Title: "Internal Server Error", Errors: []*huma.ErrorDetail{{Message: "context canceled"}}}
	if _, err := errorTransformer(log)(hctx, "", model); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// A client that hung up is not a server fault: no ERROR "request failed" line, and the event
// name must not be api.request_failed (which dashboards and the diagnostics feed read as ours).
func TestErrorTransformer_ClientCancelIsNotLoggedAsServerError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out := transformError(t, ctx, http.StatusInternalServerError)
	if strings.Contains(out, "level=ERROR") || strings.Contains(out, "api.request_failed") {
		t.Fatalf("client cancel logged as a server error:\n%s", out)
	}
}

func TestErrorTransformer_GenuineServerErrorStillLogsError(t *testing.T) {
	out := transformError(t, context.Background(), http.StatusInternalServerError)
	if !strings.Contains(out, "level=ERROR") || !strings.Contains(out, "api.request_failed") {
		t.Fatalf("genuine 500 not logged as an error:\n%s", out)
	}
}
