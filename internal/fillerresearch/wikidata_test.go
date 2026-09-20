package fillerresearch

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWikidataRetrievesCanonicalEntityEvidence(t *testing.T) {
	var action, search, language, resultType, limit string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action, search = r.URL.Query().Get("action"), r.URL.Query().Get("search")
		language, resultType, limit = r.URL.Query().Get("language"), r.URL.Query().Get("type"), r.URL.Query().Get("limit")
		if got := r.Header.Get("User-Agent"); !strings.Contains(got, "Loomarr") {
			t.Fatalf("user agent = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"search":[{"id":"Q154950","label":"Tootsie Pop","description":"brand of candy"},{"id":"javascript:bad","label":"Bad","description":"unsafe"}]}`))
	}))
	defer server.Close()
	provider, err := NewWikidata(WikidataConfig{Client: server.Client(), UserAgent: "Loomarr test",
		Endpoint: server.URL, AllowInsecureTestURL: true, Now: func() time.Time { return time.Unix(100, 0).UTC() }})
	if err != nil {
		t.Fatal(err)
	}
	packet, err := provider.Retrieve(t.Context(), Lookup{Title: "Tootsie Pop Classic Commercial"})
	if err != nil {
		t.Fatal(err)
	}
	if action != "wbsearchentities" || search != "Tootsie Pop" || language != "en" || resultType != "item" || limit != "3" {
		t.Fatalf("action=%q search=%q language=%q type=%q limit=%q", action, search, language, resultType, limit)
	}
	if len(packet.Citations) != 1 || packet.Citations[0].URL != "https://www.wikidata.org/wiki/Q154950" ||
		!strings.Contains(packet.Citations[0].Extract, "brand of candy") {
		t.Fatalf("packet = %+v", packet)
	}
}

func TestWikidataRejectsNonCanonicalEndpointAndCrossOriginRedirect(t *testing.T) {
	if _, err := NewWikidata(WikidataConfig{Endpoint: "https://example.com/api", UserAgent: "Loomarr test"}); err == nil {
		t.Fatal("non-canonical endpoint accepted")
	}
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("cross-origin redirect target was contacted")
	}))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, target.URL, http.StatusFound)
	}))
	defer server.Close()
	provider, err := NewWikidata(WikidataConfig{Client: server.Client(), UserAgent: "Loomarr test",
		Endpoint: server.URL, AllowInsecureTestURL: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Retrieve(t.Context(), Lookup{Title: "clip"}); err == nil ||
		!strings.Contains(err.Error(), "configured origin") {
		t.Fatalf("redirect error = %v", err)
	}
}
