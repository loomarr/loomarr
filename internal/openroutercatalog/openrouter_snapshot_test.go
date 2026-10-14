package openroutercatalog

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/testkit/httpfixture"
)

func TestFetchOpenRouterSnapshotLocksEndpointIdentityPriceCapabilityAndZDR(t *testing.T) {
	t.Parallel()
	transport := httpfixture.NewScriptedTransport(
		httpfixture.Step{Response: snapshotResponse(http.StatusOK, `{"data":[{"id":"vendor/model-1","canonical_slug":"vendor/model-1-20260826","name":"Model One","created":1}]}`)},
		httpfixture.Step{Response: snapshotResponse(http.StatusOK, `{"data":[`+snapshotEndpointFixture("vendor/model-1", "Pinned Provider", "pinned-provider/variant")+`]}`)},
		httpfixture.Step{Response: snapshotResponse(http.StatusOK, `{"data":{"id":"vendor/model-1","name":"Model One","created":1,"architecture":{"input_modalities":["text","image"],"output_modalities":["text"]},"endpoints":[`+snapshotEndpointFixture("vendor/model-1", "Pinned Provider", "pinned-provider/variant")+`]}}`)},
	)
	retrievedAt := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	snapshot, err := FetchOpenRouterSnapshot(context.Background(), OpenRouterSnapshotConfig{
		BaseURL: OpenRouterBaseURL, APIKey: "secret", Models: []string{"vendor/model-1"}, RetrievedAt: retrievedAt,
		Client: &http.Client{Transport: transport},
	})
	if err != nil {
		t.Fatal(err)
	}
	if transport.Calls() != 3 || snapshot.Requests != 3 || snapshot.ResponseBytes <= 0 || len(snapshot.Models) != 1 || snapshot.Models[0].CanonicalSlug != "vendor/model-1-20260826" || len(snapshot.Models[0].Endpoints) != 1 {
		t.Fatalf("snapshot envelope = %#v requests=%d", snapshot, transport.Calls())
	}
	for index, request := range transport.Requests() {
		if request.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("request %d authorization = %q", index, request.Header.Get("Authorization"))
		}
		if request.Header.Get("X-OpenRouter-Title") != "Loomarr filler certification" || request.Header.Get("HTTP-Referer") != "https://github.com/loomarr/loomarr" {
			t.Errorf("request %d client identity headers = %+v", index, request.Header)
		}
	}
	if got := []string{transport.Requests()[0].URL, transport.Requests()[1].URL, transport.Requests()[2].URL}; !slices.Equal(got, []string{OpenRouterBaseURL + "/models", OpenRouterBaseURL + "/endpoints/zdr", OpenRouterBaseURL + "/models/vendor/model-1/endpoints"}) {
		t.Fatalf("request URLs = %v", got)
	}
	endpoint := snapshot.Models[0].Endpoints[0]
	if endpoint.ProviderSlug != "pinned-provider/variant" || endpoint.ProviderName != "Pinned Provider" || !endpoint.ZDR || endpoint.Pricing["prompt"] != "0.000001" || endpoint.Pricing["discount"] != "0.25" || !slices.Equal(endpoint.SupportedParameters, []string{"response_format", "structured_outputs"}) {
		t.Fatalf("endpoint = %#v", endpoint)
	}
	if len(OpenRouterSnapshotSHA256(snapshot)) != 64 {
		t.Fatal("snapshot has no digest")
	}
}

func TestFetchOpenRouterSnapshotPreservesTieredEndpointPricing(t *testing.T) {
	t.Parallel()
	tiered := strings.Replace(
		snapshotEndpointFixture("vendor/model-1", "Pinned Provider", "pinned-provider/variant"),
		`"discount":0.25}`,
		`"discount":0.25,"overrides":[{"min_prompt_tokens":200000,"prompt":"0.000002","completion":"0.000009"}]}`,
		1,
	)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/models":
			_, _ = io.WriteString(writer, `{"data":[{"id":"vendor/model-1","canonical_slug":"vendor/model-1-20260826","name":"Model One","created":1}]}`)
		case "/endpoints/zdr":
			_, _ = io.WriteString(writer, `{"data":[`+tiered+`]}`)
		case "/models/vendor/model-1/endpoints":
			_, _ = io.WriteString(writer, `{"data":{"id":"vendor/model-1","name":"Model One","created":1,"architecture":{"input_modalities":["text","video"],"output_modalities":["text"]},"endpoints":[`+tiered+`]}}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	snapshot, err := FetchOpenRouterSnapshot(context.Background(), OpenRouterSnapshotConfig{
		BaseURL: server.URL, APIKey: "secret", Models: []string{"vendor/model-1"},
		RetrievedAt: time.Date(2026, 9, 5, 2, 30, 0, 0, time.UTC), Client: server.Client(), AllowInsecureTestURL: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	overrides := snapshot.Models[0].Endpoints[0].PricingOverrides
	if len(overrides) != 1 || overrides[0].MinimumPromptTokens != 200_000 ||
		overrides[0].Pricing["prompt"] != "0.000002" || overrides[0].Pricing["completion"] != "0.000009" {
		t.Fatalf("pricing overrides = %+v", overrides)
	}
}

func TestValidateOpenRouterSnapshotReplaysLegacyTierlessSchema(t *testing.T) {
	t.Parallel()
	snapshot := validOpenRouterSnapshot()
	snapshot.SchemaVersion = legacyOpenRouterSnapshotSchema
	if err := ValidateOpenRouterSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot.Models[0].Endpoints[0].PricingOverrides = []OpenRouterPricingOverride{{
		MinimumPromptTokens: 200_000,
		Pricing:             map[string]string{"prompt": "0.000002"},
	}}
	if err := ValidateOpenRouterSnapshot(snapshot); err == nil || !strings.Contains(err.Error(), "cannot contain pricing overrides") {
		t.Fatalf("legacy tier validation error=%v", err)
	}
}

func TestFetchOpenRouterSnapshotFiltersTheTranscriptionCatalog(t *testing.T) {
	t.Parallel()
	transport := httpfixture.NewScriptedTransport(
		httpfixture.Step{Response: snapshotResponse(http.StatusOK, `{"data":[{"id":"openai/whisper-large-v3","canonical_slug":"openai/whisper-large-v3","name":"Whisper","created":1}]}`)},
		httpfixture.Step{Response: snapshotResponse(http.StatusOK, `{"data":[`+strings.Replace(snapshotEndpointFixture("openai/whisper-large-v3", "Pinned Provider", "pinned-provider"), `"context_length":8192`, `"context_length":0`, 1)+`]}`)},
		httpfixture.Step{Response: snapshotResponse(http.StatusOK, `{"data":{"id":"openai/whisper-large-v3","name":"Whisper","created":1,"architecture":{"input_modalities":["audio"],"output_modalities":["transcription"]},"endpoints":[`+strings.Replace(snapshotEndpointFixture("openai/whisper-large-v3", "Pinned Provider", "pinned-provider"), `"context_length":8192`, `"context_length":0`, 1)+`]}}`)},
	)
	snapshot, err := FetchOpenRouterSnapshot(context.Background(), OpenRouterSnapshotConfig{
		BaseURL: OpenRouterBaseURL, APIKey: "secret", Models: []string{"openai/whisper-large-v3"}, OutputModality: "transcription", RetrievedAt: time.Now().UTC(), Client: &http.Client{Transport: transport},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(snapshot.Models[0].InputModalities, []string{"audio"}) || !slices.Equal(snapshot.Models[0].OutputModalities, []string{"transcription"}) {
		t.Fatalf("transcription modalities = %+v", snapshot.Models[0])
	}
	if requests := transport.Requests(); len(requests) != 3 || requests[0].URL != OpenRouterBaseURL+"/models?output_modalities=transcription" || requests[1].URL != OpenRouterBaseURL+"/endpoints/zdr" || requests[2].URL != OpenRouterBaseURL+"/models/openai/whisper-large-v3/endpoints" {
		t.Fatalf("requests = %+v", requests)
	}
}

func TestValidateOpenRouterSnapshotPreservesInactiveSiblingRoutes(t *testing.T) {
	t.Parallel()
	snapshot := validOpenRouterSnapshot()
	inactive := snapshot.Models[0].Endpoints[0]
	inactive.Name = "Inactive Provider | model"
	inactive.ProviderName = "Inactive Provider"
	inactive.ProviderSlug = "inactive-provider/variant"
	inactive.Status = -2
	snapshot.Models[0].Endpoints = append([]OpenRouterEndpointSnapshot{inactive}, snapshot.Models[0].Endpoints...)
	if err := ValidateOpenRouterSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
}

func TestFetchOpenRouterSnapshotRejectsMissingZDRCredentialAndRedirect(t *testing.T) {
	t.Parallel()
	if _, err := FetchOpenRouterSnapshot(context.Background(), OpenRouterSnapshotConfig{Models: []string{"vendor/model"}, RetrievedAt: time.Now().UTC()}); err == nil || !strings.Contains(err.Error(), "API key") {
		t.Fatalf("missing key error = %v", err)
	}
	transport := httpfixture.NewScriptedTransport(httpfixture.Step{Response: &http.Response{StatusCode: http.StatusTemporaryRedirect, Header: http.Header{"Location": []string{"/again"}}, Body: http.NoBody}})
	_, err := FetchOpenRouterSnapshot(context.Background(), OpenRouterSnapshotConfig{
		BaseURL: OpenRouterBaseURL, APIKey: "secret", Models: []string{"vendor/model"}, RetrievedAt: time.Now().UTC(), Client: &http.Client{Transport: transport},
	})
	if err == nil || !strings.Contains(err.Error(), "status 307") {
		t.Fatalf("redirect error = %v", err)
	}
}

func snapshotResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}

func validOpenRouterSnapshot() OpenRouterSnapshot {
	return OpenRouterSnapshot{
		SchemaVersion: OpenRouterSnapshotSchemaVersion, SourceBaseURL: OpenRouterBaseURL, RetrievedAt: time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC), Requests: 3, ResponseBytes: 100,
		Models: []OpenRouterModelSnapshot{{
			ID: "vendor/model-1", CanonicalSlug: "vendor/model-1-20260826", Name: "Model One", Created: 1, InputModalities: []string{"image", "text"}, OutputModalities: []string{"text"},
			Endpoints: []OpenRouterEndpointSnapshot{{
				Name: "Pinned Provider | model", ModelID: "vendor/model-1", ProviderName: "Pinned Provider", ProviderSlug: "pinned-provider/variant",
				Quantization: "fp16", ContextLength: 8192, MaxCompletionTokens: 1024,
				SupportedParameters: []string{"response_format", "structured_outputs"}, Pricing: map[string]string{"completion": "0.000002", "prompt": "0.000001"}, ZDR: true,
			}},
		}},
	}
}

func snapshotEndpointFixture(model, provider, slug string) string {
	return `{"name":"` + provider + ` | model","model_id":"` + model + `","provider_name":"` + provider + `","tag":"` + slug + `","quantization":"fp16","context_length":8192,"max_completion_tokens":1024,"max_prompt_tokens":4096,"supported_parameters":["structured_outputs","response_format"],"pricing":{"prompt":"0.000001","completion":"0.000002","discount":0.25},"status":0,"supports_implicit_caching":false}`
}
