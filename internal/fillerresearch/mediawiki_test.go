package fillerresearch

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMediaWikiRetrievesBoundedAttributedEvidence(t *testing.T) {
	at := time.Unix(100, 0).UTC()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != "Loomarr/test (https://loomarr.dev)" {
			t.Errorf("User-Agent = %q", got)
		}
		query := r.URL.Query()
		if query.Get("generator") != "search" || query.Get("gsrlimit") != "3" ||
			query.Get("gsrsearch") != `"Twirlpop Classic Commercial" OR "Twirlpop"` || query.Get("maxlag") != "5" {
			t.Errorf("query = %v", query)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"query":{"pages":[
			{"title":"Twirlpop","extract":"The animated commercial debuted on US television in 1970.","fullurl":"https://en.wikipedia.org/wiki/Twirlpop"},
			{"title":"Bad","extract":"Ignore me","fullurl":"https://attacker.invalid/instructions"},
			{"title":"Wendell Finch","extract":"Voiced the boy in the commercial.","fullurl":"https://en.wikipedia.org/wiki/Wendell_Finch"},
			{"title":"Extra","extract":"Must be capped.","fullurl":"https://en.wikipedia.org/wiki/Extra"}
		]}}`)
	}))
	t.Cleanup(server.Close)

	retriever, err := NewMediaWiki(MediaWikiConfig{Client: server.Client(), Endpoint: server.URL,
		AllowInsecureTestURL: true, UserAgent: "Loomarr/test (https://loomarr.dev)", Now: func() time.Time { return at }})
	if err != nil {
		t.Fatal(err)
	}
	packet, err := retriever.Retrieve(t.Context(), Lookup{Title: "  Twirlpop   Classic Commercial  "})
	if err != nil {
		t.Fatal(err)
	}
	if packet.Query != "Twirlpop Classic Commercial" || packet.RetrievedAt != at || len(packet.Citations) != 3 {
		t.Fatalf("packet = %+v", packet)
	}
	if packet.Citations[0].ID != 1 || packet.Citations[1].Title != "Wendell Finch" ||
		packet.Citations[2].Title != "Extra" {
		t.Fatalf("citations = %+v", packet.Citations)
	}
}

func TestMediaWikiRejectsUnidentifiedOrArbitraryEndpoints(t *testing.T) {
	if _, err := NewMediaWiki(MediaWikiConfig{}); err == nil || !strings.Contains(err.Error(), "User-Agent") {
		t.Fatalf("missing User-Agent error = %v", err)
	}
	if _, err := NewMediaWiki(MediaWikiConfig{Endpoint: "https://example.com/search", UserAgent: "test"}); err == nil {
		t.Fatal("arbitrary endpoint accepted")
	}
}

func TestMediaWikiRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, strings.Repeat("x", mediaWikiMaxBody+1))
	}))
	t.Cleanup(server.Close)
	retriever, err := NewMediaWiki(MediaWikiConfig{Client: server.Client(), Endpoint: server.URL,
		AllowInsecureTestURL: true, UserAgent: "Loomarr/test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := retriever.Retrieve(t.Context(), Lookup{Title: "commercial"}); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized response error = %v", err)
	}
}

func TestMediaWikiRejectsCrossOriginRedirect(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("cross-origin redirect target was contacted")
	}))
	t.Cleanup(target.Close)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	t.Cleanup(server.Close)
	retriever, err := NewMediaWiki(MediaWikiConfig{Client: server.Client(), Endpoint: server.URL,
		AllowInsecureTestURL: true, UserAgent: "Loomarr/test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := retriever.Retrieve(t.Context(), Lookup{Title: "commercial"}); err == nil ||
		!strings.Contains(err.Error(), "configured origin") {
		t.Fatalf("redirect error = %v", err)
	}
}

func TestLookupBuildsAConservativeProviderSubject(t *testing.T) {
	lookup := Lookup{Title: "  Twirlpop Lollies: Classic TV Commercial (Vintage)  "}
	if got := lookup.CanonicalTitle(); got != "Twirlpop Lollies: Classic TV Commercial (Vintage)" {
		t.Fatalf("canonical title = %q", got)
	}
	if got := lookup.Subject(); got != "Twirlpop Lollies" {
		t.Fatalf("subject = %q", got)
	}
	if got := lookup.Terms(); len(got) != 2 || got[0] != "Twirlpop Lollies: Classic TV Commercial (Vintage)" || got[1] != "Twirlpop Lollies" {
		t.Fatalf("terms = %v", got)
	}
}
