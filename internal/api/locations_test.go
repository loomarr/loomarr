package api_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/api"
)

type locationBody struct {
	Locations  []locationResult `json:"locations"`
	Location   locationResult   `json:"location"`
	Suggestion *locationResult  `json:"suggestion"`
}

type locationResult struct {
	Label       string `json:"label"`
	Country     string `json:"country"`
	Market      string `json:"market"`
	Region      string `json:"region"`
	Source      string `json:"source"`
	Approximate bool   `json:"approximate"`
	Attribution string `json:"attribution"`
}

func locationServer(t *testing.T, trustProxy bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(api.Router(slog.New(slog.NewTextHandler(io.Discard, nil)), api.Options{
		Auth: testAuthorizer{}, TrustProxy: trustProxy,
	}))
}

func locationRequest(t *testing.T, client *http.Client, method, rawURL, body string, headers map[string]string) (*http.Response, locationBody) {
	t.Helper()
	req, err := http.NewRequest(method, rawURL, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	var decoded locationBody
	if response.StatusCode == http.StatusOK {
		if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
			t.Fatal(err)
		}
	}
	return response, decoded
}

func TestLocationsSearchAndResolveUseTheEmbeddedIndex(t *testing.T) {
	server := locationServer(t, false)
	defer server.Close()

	response, body := locationRequest(t, server.Client(), http.MethodGet,
		server.URL+"/v1/locations?q="+url.QueryEscape("New York City")+"&limit=3", "", nil)
	if response.StatusCode != http.StatusOK || len(body.Locations) == 0 || body.Locations[0].Country != "US" {
		t.Fatalf("search = %d %#v", response.StatusCode, body)
	}

	response, body = locationRequest(t, server.Client(), http.MethodPost, server.URL+"/v1/locations/resolve",
		`{"latitude":40.7128,"longitude":-74.006}`, nil)
	if response.StatusCode != http.StatusOK || body.Location.Country != "US" || body.Location.Market != "New York City" || body.Location.Source != "device" || body.Location.Approximate {
		t.Fatalf("resolve = %d %#v", response.StatusCode, body)
	}
}

func TestLocationsSearchIncludesPopulatedMunicipalityContext(t *testing.T) {
	server := locationServer(t, false)
	defer server.Close()

	response, body := locationRequest(t, server.Client(), http.MethodGet,
		server.URL+"/v1/locations?q="+url.QueryEscape("North Greenbush")+"&limit=3", "", nil)
	if response.StatusCode != http.StatusOK || len(body.Locations) == 0 ||
		body.Locations[0].Label != "North Greenbush, United States" || body.Locations[0].Region != "New York" {
		t.Fatalf("search municipality = %d %#v", response.StatusCode, body)
	}
}

func TestLocationSuggestionAcceptsKnownAndNeutralHeadersOnlyBehindTrustProxy(t *testing.T) {
	trusted := locationServer(t, true)
	defer trusted.Close()

	for _, test := range []struct {
		name    string
		headers map[string]string
		country string
		source  string
	}{
		{"cloudflare", map[string]string{"CF-IPCountry": "CA"}, "CA", "cloudflare"},
		{"cloudfront", map[string]string{"CloudFront-Viewer-Country": "US", "CloudFront-Viewer-City": "New%20York%20City"}, "US", "cloudfront"},
		{"vercel", map[string]string{"X-Vercel-IP-Country": "GB", "X-Vercel-IP-City": "London"}, "GB", "vercel"},
		{"neutral", map[string]string{"X-Loomarr-Geo-Country": "AU"}, "AU", "proxy"},
	} {
		t.Run(test.name, func(t *testing.T) {
			response, body := locationRequest(t, trusted.Client(), http.MethodGet, trusted.URL+"/v1/locations/suggestion", "", test.headers)
			if response.StatusCode != http.StatusOK || body.Suggestion == nil || body.Suggestion.Country != test.country || body.Suggestion.Source != test.source || !body.Suggestion.Approximate {
				t.Fatalf("suggestion = %d %#v", response.StatusCode, body)
			}
		})
	}

	untrusted := locationServer(t, false)
	defer untrusted.Close()
	response, body := locationRequest(t, untrusted.Client(), http.MethodGet, untrusted.URL+"/v1/locations/suggestion", "", map[string]string{"CF-IPCountry": "CA"})
	if response.StatusCode != http.StatusOK || body.Suggestion != nil {
		t.Fatalf("untrusted header suggestion = %d %#v, want none", response.StatusCode, body)
	}
}

func TestLocationRoutesAreAdminOnly(t *testing.T) {
	server := locationServer(t, true)
	defer server.Close()
	req, err := http.NewRequest(http.MethodGet, server.URL+"/v1/locations?q=London", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+memberToken)
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("member status = %d, want 403", response.StatusCode)
	}
}
