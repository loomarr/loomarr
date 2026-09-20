package fillerresearch

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBraveRetrievesBoundedSnippetsWithoutLeakingTheKey(t *testing.T) {
	const secret = "brave-secret-that-must-not-leak"
	var query string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Subscription-Token"); got != secret {
			t.Fatalf("subscription token = %q", got)
		}
		if got := r.Header.Get("User-Agent"); !strings.Contains(got, "Loomarr") {
			t.Fatalf("user agent = %q", got)
		}
		query = r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"web":{"results":[{"title":"Tootsie Pop","url":"https://example.com/tootsie","description":"The animated campaign began in 1970."}]}}`))
	}))
	defer server.Close()

	provider, err := NewBrave(BraveConfig{Client: server.Client(), APIKey: secret, Endpoint: server.URL,
		AllowInsecureTestURL: true, Now: func() time.Time { return time.Unix(100, 0).UTC() }})
	if err != nil {
		t.Fatal(err)
	}
	packet, err := provider.Retrieve(t.Context(), Lookup{Title: "Tootsie Pop Classic Commercial"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(query, "Tootsie Pop Classic Commercial") || len(packet.Citations) != 1 || packet.Citations[0].Extract == "" {
		t.Fatalf("query=%q packet=%+v", query, packet)
	}

	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, secret, http.StatusUnauthorized)
	}))
	defer failing.Close()
	provider, err = NewBrave(BraveConfig{Client: failing.Client(), APIKey: secret, Endpoint: failing.URL,
		AllowInsecureTestURL: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Retrieve(t.Context(), Lookup{Title: "clip"})
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("safe upstream error = %v", err)
	}
}

func TestBraveRejectsNonCanonicalProductionEndpointAndOversizedResponse(t *testing.T) {
	if _, err := NewBrave(BraveConfig{APIKey: "key", Endpoint: "https://example.com/search"}); err == nil {
		t.Fatal("non-canonical Brave endpoint accepted")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", webSearchMaxBody+1)))
	}))
	defer server.Close()
	provider, err := NewBrave(BraveConfig{Client: server.Client(), APIKey: "key", Endpoint: server.URL,
		AllowInsecureTestURL: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Retrieve(t.Context(), Lookup{Title: "clip"}); err == nil || !strings.Contains(err.Error(), "bounded") {
		t.Fatalf("oversized response error = %v", err)
	}
}

func TestBraveRejectsRedirectOutsideProviderOrigin(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("cross-origin redirect target was contacted")
	}))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, target.URL, http.StatusFound)
	}))
	defer server.Close()
	provider, err := NewBrave(BraveConfig{Client: server.Client(), APIKey: "key", Endpoint: server.URL,
		AllowInsecureTestURL: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Retrieve(t.Context(), Lookup{Title: "clip"}); err == nil ||
		!strings.Contains(err.Error(), "configured origin") {
		t.Fatalf("redirect error = %v", err)
	}
}
