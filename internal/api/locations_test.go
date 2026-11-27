package api_test

import (
	"encoding/json"
	"net/http"
	"net/netip"
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
	ID          string `json:"id"`
	Place       string `json:"place"`
	Region      string `json:"region"`
	Source      string `json:"source"`
	Approximate bool   `json:"approximate"`
	Attribution string `json:"attribution"`
}

// newLocationHarness serves the API with the given trusted proxy ranges. The harness's clients
// connect from loopback, so listing 127.0.0.0/8 makes them a proxy and listing anything else
// leaves them an ordinary client.
func newLocationHarness(t *testing.T, trusted ...string) *apiHarness {
	t.Helper()
	var proxies []netip.Prefix
	for _, cidr := range trusted {
		proxies = append(proxies, netip.MustParsePrefix(cidr))
	}
	return startAPIHarness(t, func(defaults apiHarnessDefaults) http.Handler {
		return api.Router(defaults.Log, api.Options{
			Store: defaults.Store, Auth: defaults.Auth, Log: defaults.Log,
			TrustedProxies: proxies,
		})
	})
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
	server := newLocationHarness(t).Server

	response, body := locationRequest(t, server.Client(), http.MethodGet,
		server.URL+"/v1/locations?q="+url.QueryEscape("New York City")+"&limit=3", "", nil)
	if response.StatusCode != http.StatusOK || len(body.Locations) == 0 || body.Locations[0].Country != "US" {
		t.Fatalf("search = %d %#v", response.StatusCode, body)
	}

	response, body = locationRequest(t, server.Client(), http.MethodPost, server.URL+"/v1/locations/resolve",
		`{"latitude":40.7128,"longitude":-74.006}`, nil)
	if response.StatusCode != http.StatusOK || body.Location.Country != "US" || body.Location.Place == "" ||
		body.Location.Place != body.Location.ID || body.Location.Source != "device" || body.Location.Approximate {
		t.Fatalf("resolve = %d %#v", response.StatusCode, body)
	}

	// A saved place reads back by id, so a form can show what it holds (#2062).
	saved := body.Location
	response, body = locationRequest(t, server.Client(), http.MethodGet, server.URL+"/v1/locations/"+saved.Place, "", nil)
	if response.StatusCode != http.StatusOK || body.Location.Label != saved.Label || body.Location.Place != saved.Place {
		t.Fatalf("get saved place = %d %#v, want %#v", response.StatusCode, body.Location, saved)
	}
	response, _ = locationRequest(t, server.Client(), http.MethodGet, server.URL+"/v1/locations/999999999999", "", nil)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("get unknown place = %d, want 404", response.StatusCode)
	}
}

func TestLocationsSearchIncludesPopulatedMunicipalityContext(t *testing.T) {
	server := newLocationHarness(t).Server

	response, body := locationRequest(t, server.Client(), http.MethodGet,
		server.URL+"/v1/locations?q="+url.QueryEscape("North Greenbush")+"&limit=3", "", nil)
	if response.StatusCode != http.StatusOK || len(body.Locations) == 0 ||
		body.Locations[0].Label != "North Greenbush, United States" || body.Locations[0].Region != "New York" {
		t.Fatalf("search municipality = %d %#v", response.StatusCode, body)
	}
}

func TestLocationSuggestionAcceptsKnownAndNeutralHeadersOnlyBehindTrustProxy(t *testing.T) {
	trusted := newLocationHarness(t, "127.0.0.0/8", "::1/128").Server

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

	// No proxy listed, and a proxy listed that is not the peer (documentation ranges, IPv4 and
	// IPv6): the geography header is the client's own claim in every case.
	for name, harness := range map[string]*apiHarness{
		"nothing listed":               newLocationHarness(t),
		"another IPv4 proxy listed":    newLocationHarness(t, "192.0.2.10/32"),
		"another IPv6 range listed":    newLocationHarness(t, "2001:db8::/32"),
		"loopback range one-off v4":    newLocationHarness(t, "127.0.0.2/32"),
		"IPv6 loopback only is listed": newLocationHarness(t, "::1/128"),
	} {
		t.Run("untrusted/"+name, func(t *testing.T) {
			server := harness.Server
			for _, headers := range []map[string]string{
				{"CF-IPCountry": "CA"},
				{"X-Loomarr-Geo-Country": "AU"},
				{"X-Vercel-IP-Country": "GB"},
				{"CloudFront-Viewer-Country": "US"},
			} {
				response, body := locationRequest(t, server.Client(), http.MethodGet, server.URL+"/v1/locations/suggestion", "", headers)
				if response.StatusCode != http.StatusOK || body.Suggestion != nil {
					t.Fatalf("untrusted header %v suggestion = %d %#v, want none", headers, response.StatusCode, body)
				}
			}
		})
	}
}

func TestLocationRoutesAreAdminOnly(t *testing.T) {
	server := newLocationHarness(t, "127.0.0.0/8", "::1/128").Server
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
