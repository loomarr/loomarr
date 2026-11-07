package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/filler/research"
)

type fixtureFillerResearch struct {
	status  research.WebStatus
	config  research.WebConfig
	ok      bool
	message string
}

func (f *fixtureFillerResearch) Status(context.Context) (research.WebStatus, error) {
	return f.status, nil
}

func (f *fixtureFillerResearch) Test(_ context.Context, config research.WebConfig) (research.WebStatus, bool, string, error) {
	f.config = config
	return f.status, f.ok, f.message, nil
}

func TestFillerResearchRoutesAreAdminOnlyAndNeverEchoSecrets(t *testing.T) {
	service := &fixtureFillerResearch{ok: true, message: "Web search is ready.", status: research.WebStatus{
		StructuredEnabled: true, Provider: research.WebProviderBrave, Configured: true,
		State: research.WebStateReady, Month: "2026-09", RequestCount: 3, RequestLimit: 100,
		LastSuccessAt: time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC),
	}}
	server := httptest.NewServer(api.Router(slog.New(slog.DiscardHandler), api.Options{
		Auth: testAuthorizer{}, FillerResearch: service,
	}))
	defer server.Close()

	request := func(method, path, token, body string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(method, server.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	for _, token := range []string{"", memberToken} {
		resp := request(http.MethodGet, "/v1/filler/research/status", token, "")
		if token == "" && resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("anonymous status = %d", resp.StatusCode)
		}
		if token == memberToken && resp.StatusCode != http.StatusForbidden {
			t.Fatalf("member status = %d", resp.StatusCode)
		}
		_ = resp.Body.Close()
	}

	resp := request(http.MethodGet, "/v1/filler/research/status", adminToken, "")
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || strings.Contains(string(raw), "apiKey") || !strings.Contains(string(raw), `"requestCount":3`) {
		t.Fatalf("admin status=%d body=%s", resp.StatusCode, raw)
	}

	const secret = "brave-secret-never-return"
	payload, _ := json.Marshal(map[string]string{"provider": "brave", "apiKey": secret})
	resp = request(http.MethodPost, "/v1/filler/research/test", adminToken, string(payload))
	raw, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(raw), `"ok":true`) || strings.Contains(string(raw), secret) {
		t.Fatalf("test status=%d body=%s", resp.StatusCode, raw)
	}
	if service.config.Provider != research.WebProviderBrave || service.config.BraveAPIKey != secret {
		t.Fatalf("tested config = %+v", service.config)
	}
}
