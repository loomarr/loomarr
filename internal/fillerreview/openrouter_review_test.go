package fillerreview

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillerbakeoff"
)

func TestRunOpenRouterReviewPinsZDRRouteAndPaidAccounting(t *testing.T) {
	packageDir, transcript := reviewPackageFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatal("missing API key")
		}
		if r.Header.Get("X-OpenRouter-Metadata") != "enabled" {
			t.Fatal("missing metadata request")
		}
		var request struct {
			Model    string `json:"model"`
			Provider struct {
				Order             []string `json:"order"`
				AllowFallbacks    bool     `json:"allow_fallbacks"`
				RequireParameters bool     `json:"require_parameters"`
				DataCollection    string   `json:"data_collection"`
				ZDR               bool     `json:"zdr"`
			} `json:"provider"`
			Messages []struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"messages"`
			ResponseFormat struct {
				JSONSchema struct {
					Strict bool `json:"strict"`
				} `json:"json_schema"`
			} `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(request)
		if request.Model != "review/vendor-model" || len(request.Provider.Order) != 1 || request.Provider.Order[0] != "provider/route" || request.Provider.AllowFallbacks || !request.Provider.RequireParameters || request.Provider.DataCollection != "deny" || !request.Provider.ZDR || !request.ResponseFormat.JSONSchema.Strict || strings.Contains(string(encoded), "case-secret") {
			t.Fatalf("request = %s", encoded)
		}
		labels := `{"disposition":"eligible","contentRole":"commercial","taxonomy":{"product":["cola"]},"policyFlags":[],"slices":["commercial"],"evidence":[{"id":"frame-01","kind":"frame","claim":"content_role","value":"commercial","provenance":"cases/review-one/frame-01.jpg","atMs":1000}],"reviewQuestion":""}`
		response := map[string]any{
			"id": "generation-1", "model": "review/vendor-model",
			"choices":             []any{map[string]any{"message": map[string]string{"content": labels}}},
			"usage":               map[string]any{"prompt_tokens": 200, "completion_tokens": 50, "cost": 0.001},
			"openrouter_metadata": map[string]any{"attempt": 1, "attempts": []any{map[string]any{"provider": "Provider Route", "model": "review/vendor-model", "status": 200}}, "endpoints": map[string]any{"available": []any{map[string]any{"provider": "Provider Route", "model": "review/vendor-model", "selected": true}}}},
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	now := time.Date(2026, 8, 27, 22, 0, 0, 0, time.UTC)
	snapshot := fillerbakeoff.OpenRouterSnapshot{SchemaVersion: fillerbakeoff.OpenRouterSnapshotSchemaVersion, SourceBaseURL: server.URL, RetrievedAt: now.Add(-time.Hour), Requests: 3, ResponseBytes: 100, Models: []fillerbakeoff.OpenRouterModelSnapshot{{ID: "review/vendor-model", CanonicalSlug: "review/vendor-model", Name: "Reviewer", Created: 1, InputModalities: []string{"image", "text"}, OutputModalities: []string{"text"}, Endpoints: []fillerbakeoff.OpenRouterEndpointSnapshot{{Name: "Route", ModelID: "review/vendor-model", ProviderName: "Provider Route", ProviderSlug: "provider/route", Quantization: "unknown", ContextLength: 32768, MaxCompletionTokens: 4096, SupportedParameters: []string{"response_format", "structured_outputs"}, Pricing: map[string]string{"completion": "0.000001", "prompt": "0.000001"}, Status: 0, ZDR: true}}}}}
	run, submissions, err := RunOpenRouterReview(context.Background(), OpenRouterReviewConfig{PackageDir: packageDir, Transcripts: []fillerbakeoff.TranscriptArtifact{transcript}, BaseURL: server.URL, APIKey: "test-key", Snapshot: snapshot, Model: "review/vendor-model", UpstreamProvider: "Provider Route", UpstreamProviderSlug: "provider/route", ReviewerID: "hosted-reviewer-a", ExpectedCases: 1, PerCaseTimeout: time.Second, MaxRequests: 1, MaxSpendNanoUSD: 2_000_000, MaxChargeNanoUSD: 2_000_000, AllowInsecureTestURL: true, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if len(submissions) != 1 || run.Requests != 1 || run.PromptTokens != 200 || run.CompletionTokens != 50 || run.ChargedNanoUSD != 1_000_000 || run.CapabilitySnapshotSHA256 == "" || run.UpstreamProvider != "Provider Route" {
		t.Fatalf("run=%+v submissions=%+v", run, submissions)
	}
}

func TestValidReviewAttemptLedgerAcceptsOmittedOrExactOptionalDetail(t *testing.T) {
	t.Parallel()
	config := OpenRouterReviewConfig{Model: "review/vendor-model", UpstreamProvider: "Provider Route", Snapshot: fillerbakeoff.OpenRouterSnapshot{Models: []fillerbakeoff.OpenRouterModelSnapshot{{ID: "review/vendor-model", CanonicalSlug: "review/vendor-model-202608"}}}}
	var wire openRouterReviewResponse
	if !validReviewAttemptLedger(wire, config) {
		t.Fatal("omitted optional attempt detail was rejected")
	}
	wire.Metadata.Attempts = append(wire.Metadata.Attempts, struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
		Status   int    `json:"status"`
	}{Provider: config.UpstreamProvider, Model: "review/vendor-model-202608", Status: 200})
	if !validReviewAttemptLedger(wire, config) {
		t.Fatal("exact attempt detail was rejected")
	}
	wire.Metadata.Attempts[0].Provider = "Other"
	if validReviewAttemptLedger(wire, config) {
		t.Fatal("mismatched attempt detail was accepted")
	}
}
