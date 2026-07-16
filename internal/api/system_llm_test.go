package api_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/store"
)

// fakeSystemLLM is a scriptable SystemLLMService for the API-layer tests.
type fakeSystemLLM struct {
	status    api.SystemLLMStatus
	selectErr error
	testErr   error
	selected  api.SelectRequest
	tested    string
	pulled    string
}

func (f *fakeSystemLLM) Status(context.Context) (api.SystemLLMStatus, error) {
	return f.status, nil
}
func (f *fakeSystemLLM) Select(_ context.Context, req api.SelectRequest) error {
	if f.selectErr != nil {
		return f.selectErr
	}
	f.selected = req
	f.status.Model = req.Model
	if req.Provider != "" {
		f.status.Provider = req.Provider
	}
	return nil
}
func (f *fakeSystemLLM) Test(_ context.Context, provider, _, _ string) error {
	f.tested = provider
	return f.testErr
}
func (f *fakeSystemLLM) Pull(_ context.Context, m string) (string, error) {
	f.pulled = m
	return "job-1", nil
}

func serverWithSystemLLM(t *testing.T, svc api.SystemLLMService) *httptest.Server {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/api.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store:     st,
		Auth:      api.NewTokenAuthorizer(adminToken),
		Log:       slog.New(slog.DiscardHandler),
		SystemLLM: svc,
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

// All /v1/system/llm* routes require admin (§8.1: model selection can trigger a
// multi-GB download + change what runs on the box).
func TestSystemLLM_RequiresAdmin(t *testing.T) {
	srv := serverWithSystemLLM(t, &fakeSystemLLM{})
	for _, tc := range []struct {
		method, path, body string
	}{
		{http.MethodGet, "/v1/system/llm", ""},
		{http.MethodPost, "/v1/system/llm/select", `{"model":"qwen3:8b"}`},
		{http.MethodPost, "/v1/system/llm/test", `{"provider":"openrouter"}`},
		{http.MethodPost, "/v1/system/llm/pull", `{"model":"qwen3:8b"}`},
	} {
		resp := do(t, srv, tc.method, tc.path, "", tc.body) // no token
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s %s without admin → %d, want 403", tc.method, tc.path, resp.StatusCode)
		}
	}
}

// When no SystemLLMService is configured (e.g. hosted provider), the routes 501.
func TestSystemLLM_NotConfigured501(t *testing.T) {
	srv := serverWithSystemLLM(t, nil)
	resp := do(t, srv, http.MethodGet, "/v1/system/llm", adminToken, "")
	if resp.StatusCode != http.StatusNotImplemented {
		t.Errorf("status with no service → %d, want 501", resp.StatusCode)
	}
}

// Status returns the probe + catalog + recommendation as admin.
func TestSystemLLM_Status(t *testing.T) {
	svc := &fakeSystemLLM{status: api.SystemLLMStatus{
		Provider: "ollama", Model: "qwen3:8b", Local: true, Reachable: true,
		VRAMGiB: 12, Recommended: "qwen3:8b",
		Catalog: []api.LLMModelView{{Tag: "qwen3:8b", Fit: "fits", Pulled: true, Recommended: true}},
	}}
	srv := serverWithSystemLLM(t, svc)
	resp := do(t, srv, http.MethodGet, "/v1/system/llm", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status → %d", resp.StatusCode)
	}
	var body api.SystemLLMStatus
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body.Recommended != "qwen3:8b" || len(body.Catalog) != 1 || !body.Local {
		t.Errorf("unexpected status body: %+v", body)
	}
}

// Select maps the not-pulled sentinel to 409 (not a 500).
func TestSystemLLM_SelectNotPulled409(t *testing.T) {
	svc := &fakeSystemLLM{selectErr: api.ErrModelNotPulled}
	srv := serverWithSystemLLM(t, svc)
	resp := do(t, srv, http.MethodPost, "/v1/system/llm/select", adminToken, `{"model":"qwen3.5:9b"}`)
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("select un-pulled model → %d, want 409", resp.StatusCode)
	}
}

// Select happy path applies the model and echoes the fresh status.
func TestSystemLLM_SelectOK(t *testing.T) {
	svc := &fakeSystemLLM{status: api.SystemLLMStatus{Provider: "ollama", Local: true}}
	srv := serverWithSystemLLM(t, svc)
	resp := do(t, srv, http.MethodPost, "/v1/system/llm/select", adminToken, `{"model":"llama3.1:8b"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("select → %d, want 200", resp.StatusCode)
	}
	if svc.selected.Model != "llama3.1:8b" {
		t.Errorf("service saw model %q, want llama3.1:8b", svc.selected.Model)
	}
	var body api.SystemLLMStatus
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body.Model != "llama3.1:8b" {
		t.Errorf("select response model = %q, want llama3.1:8b", body.Model)
	}
}

// Hosted select passes provider + key through and maps a bad key to 401.
func TestSystemLLM_SelectHosted(t *testing.T) {
	svc := &fakeSystemLLM{status: api.SystemLLMStatus{Provider: "ollama"}}
	srv := serverWithSystemLLM(t, svc)
	resp := do(t, srv, http.MethodPost, "/v1/system/llm/select", adminToken,
		`{"provider":"openrouter","model":"openai/gpt-4o-mini","apiKey":"sk-or-x"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("hosted select → %d, want 200", resp.StatusCode)
	}
	if svc.selected.Provider != "openrouter" || svc.selected.APIKey != "sk-or-x" {
		t.Errorf("service saw %+v, want provider=openrouter key=sk-or-x", svc.selected)
	}
}

func TestSystemLLM_SelectBadKey401(t *testing.T) {
	svc := &fakeSystemLLM{selectErr: api.ErrKeyInvalid}
	srv := serverWithSystemLLM(t, svc)
	resp := do(t, srv, http.MethodPost, "/v1/system/llm/select", adminToken,
		`{"provider":"openrouter","model":"x","apiKey":"bad"}`)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("bad hosted key → %d, want 401", resp.StatusCode)
	}
}

// A custom endpoint threads its baseUrl through to the service.
func TestSystemLLM_SelectCustomPassesBaseURL(t *testing.T) {
	svc := &fakeSystemLLM{status: api.SystemLLMStatus{Provider: "ollama"}}
	srv := serverWithSystemLLM(t, svc)
	resp := do(t, srv, http.MethodPost, "/v1/system/llm/select", adminToken,
		`{"provider":"custom","model":"my-model","apiKey":"k","baseUrl":"http://localhost:8000/v1"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("custom select → %d, want 200", resp.StatusCode)
	}
	if svc.selected.Provider != "custom" || svc.selected.BaseURL != "http://localhost:8000/v1" {
		t.Errorf("service saw %+v, want provider=custom baseUrl=http://localhost:8000/v1", svc.selected)
	}
}

// A custom select with no baseUrl is rejected at the boundary (422) before the service.
func TestSystemLLM_SelectCustomWithoutBaseURL422(t *testing.T) {
	svc := &fakeSystemLLM{status: api.SystemLLMStatus{Provider: "ollama"}}
	srv := serverWithSystemLLM(t, svc)
	resp := do(t, srv, http.MethodPost, "/v1/system/llm/select", adminToken,
		`{"provider":"custom","model":"my-model","apiKey":"k"}`)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("custom select w/o baseUrl → %d, want 422", resp.StatusCode)
	}
	if svc.selected.Provider != "" {
		t.Errorf("service should NOT have been called, but saw %+v", svc.selected)
	}
}

func TestSystemLLM_SelectUnknownProvider422(t *testing.T) {
	svc := &fakeSystemLLM{selectErr: api.ErrUnknownProvider}
	srv := serverWithSystemLLM(t, svc)
	resp := do(t, srv, http.MethodPost, "/v1/system/llm/select", adminToken,
		`{"provider":"nope","model":"x"}`)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("unknown provider → %d, want 422", resp.StatusCode)
	}
}

// Test endpoint: a bad key is ok=false (200), NOT a 5xx — the UI shows it inline.
func TestSystemLLM_TestReportsInline(t *testing.T) {
	svc := &fakeSystemLLM{testErr: api.ErrKeyInvalid}
	srv := serverWithSystemLLM(t, svc)
	resp := do(t, srv, http.MethodPost, "/v1/system/llm/test", adminToken,
		`{"provider":"openrouter","apiKey":"bad"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("test with bad key → %d, want 200 (inline result)", resp.StatusCode)
	}
	var body struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body.OK || body.Error == "" {
		t.Errorf("bad-key test = %+v, want ok=false with an error", body)
	}
	if svc.tested != "openrouter" {
		t.Errorf("service tested %q, want openrouter", svc.tested)
	}
}

func TestSystemLLM_TestOK(t *testing.T) {
	svc := &fakeSystemLLM{} // testErr nil → success
	srv := serverWithSystemLLM(t, svc)
	resp := do(t, srv, http.MethodPost, "/v1/system/llm/test", adminToken, `{"provider":"openrouter"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("test → %d", resp.StatusCode)
	}
	var body struct {
		OK bool `json:"ok"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if !body.OK {
		t.Error("valid key test should report ok=true")
	}
}

// Pull starts the job and returns its id.
func TestSystemLLM_Pull(t *testing.T) {
	svc := &fakeSystemLLM{}
	srv := serverWithSystemLLM(t, svc)
	resp := do(t, srv, http.MethodPost, "/v1/system/llm/pull", adminToken, `{"model":"qwen3.5:9b"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pull → %d, want 200", resp.StatusCode)
	}
	var body struct {
		JobID string `json:"jobId"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body.JobID == "" || svc.pulled != "qwen3.5:9b" {
		t.Errorf("pull jobID=%q pulled=%q", body.JobID, svc.pulled)
	}
}
