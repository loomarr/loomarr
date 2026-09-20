package fillerresearch

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSearXNGRetrievesJSONSnippetsFromConfiguredEndpoint(t *testing.T) {
	var path, query, format, safe string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, query = r.URL.Path, r.URL.Query().Get("q")
		format, safe = r.URL.Query().Get("format"), r.URL.Query().Get("safesearch")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"title":"HP Sauce","url":"https://example.org/hp","content":"A British condiment advert."}]}`))
	}))
	defer server.Close()
	provider, err := NewSearXNG(SearXNGConfig{Client: server.Client(), Endpoint: server.URL + "/meta",
		Now: func() time.Time { return time.Unix(200, 0).UTC() }})
	if err != nil {
		t.Fatal(err)
	}
	packet, err := provider.Retrieve(t.Context(), Lookup{Title: "HP Sauce Advert"})
	if err != nil {
		t.Fatal(err)
	}
	if path != "/meta/search" || !strings.Contains(query, "HP Sauce Advert") || format != "json" || safe != "1" ||
		len(packet.Citations) != 1 || packet.Citations[0].Title != "HP Sauce" {
		t.Fatalf("path=%q query=%q format=%q safe=%q packet=%+v", path, query, format, safe, packet)
	}
}

func TestSearXNGRejectsCredentialsQueriesAndPlainHTTP(t *testing.T) {
	for _, endpoint := range []string{
		"http://search.example.com", "https://user:pass@search.example.com", "https://search.example.com?x=1",
	} {
		if _, err := NewSearXNG(SearXNGConfig{Endpoint: endpoint}); err == nil {
			t.Fatalf("unsafe endpoint %q accepted", endpoint)
		}
	}
}

func TestSearXNGRejectsRedirectOutsideConfiguredOrigin(t *testing.T) {
	target := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("cross-origin redirect target was contacted")
	}))
	defer target.Close()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, target.URL, http.StatusFound)
	}))
	defer server.Close()
	provider, err := NewSearXNG(SearXNGConfig{Client: server.Client(), Endpoint: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Retrieve(t.Context(), Lookup{Title: "clip"}); err == nil ||
		!strings.Contains(err.Error(), "configured origin") {
		t.Fatalf("redirect error = %v", err)
	}
}
