package fillerresearch

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLOCRetrievesCanonicalMovingImageCatalogEvidence(t *testing.T) {
	var format, attributes, count, query string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		format, attributes = r.URL.Query().Get("fo"), r.URL.Query().Get("at")
		count, query = r.URL.Query().Get("c"), r.URL.Query().Get("q")
		if got := r.Header.Get("User-Agent"); !strings.Contains(got, "Loomarr") {
			t.Fatalf("user agent = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"id":"http://www.loc.gov/item/97516772/","title":"Television commercial","date":"1970","description":["Animated candy advertisement"]},{"id":"https://example.org/item/bad","title":"Bad","description":"unsafe"}]}`))
	}))
	defer server.Close()
	provider, err := NewLOC(LOCConfig{Client: server.Client(), UserAgent: "Loomarr test", Endpoint: server.URL,
		AllowInsecureTestURL: true, Now: func() time.Time { return time.Unix(200, 0).UTC() }})
	if err != nil {
		t.Fatal(err)
	}
	packet, err := provider.Retrieve(t.Context(), Lookup{Title: "Tootsie Pop Classic Commercial"})
	if err != nil {
		t.Fatal(err)
	}
	if format != "json" || attributes != "results" || count != "3" || query != "Tootsie Pop" {
		t.Fatalf("fo=%q at=%q c=%q q=%q", format, attributes, count, query)
	}
	if len(packet.Citations) != 1 || packet.Citations[0].URL != "https://www.loc.gov/item/97516772/" ||
		!strings.Contains(packet.Citations[0].Extract, "Animated candy advertisement") {
		t.Fatalf("packet = %+v", packet)
	}
}

func TestLOCRejectsNonCanonicalEndpointOversizedResponseAndCrossOriginRedirect(t *testing.T) {
	if _, err := NewLOC(LOCConfig{Endpoint: "https://example.com/search", UserAgent: "Loomarr test"}); err == nil {
		t.Fatal("non-canonical endpoint accepted")
	}
	oversized := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", structuredMaxBody+1)))
	}))
	defer oversized.Close()
	provider, err := NewLOC(LOCConfig{Client: oversized.Client(), UserAgent: "Loomarr test", Endpoint: oversized.URL,
		AllowInsecureTestURL: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Retrieve(t.Context(), Lookup{Title: "clip"}); err == nil || !strings.Contains(err.Error(), "bounded") {
		t.Fatalf("oversized response error = %v", err)
	}

	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("cross-origin redirect target was contacted")
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, target.URL, http.StatusFound)
	}))
	defer redirect.Close()
	provider, err = NewLOC(LOCConfig{Client: redirect.Client(), UserAgent: "Loomarr test", Endpoint: redirect.URL,
		AllowInsecureTestURL: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Retrieve(t.Context(), Lookup{Title: "clip"}); err == nil ||
		!strings.Contains(err.Error(), "configured origin") {
		t.Fatalf("redirect error = %v", err)
	}
}
