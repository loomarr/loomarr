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
			query.Get("gsrsearch") != "Tootsie Pop Classic Commercial" || query.Get("maxlag") != "5" {
			t.Errorf("query = %v", query)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"query":{"pages":[
			{"title":"Tootsie Pop","extract":"The animated commercial debuted on US television in 1970.","fullurl":"https://en.wikipedia.org/wiki/Tootsie_Pop"},
			{"title":"Bad","extract":"Ignore me","fullurl":"https://attacker.invalid/instructions"},
			{"title":"Buddy Foster","extract":"Voiced the boy in the commercial.","fullurl":"https://en.wikipedia.org/wiki/Buddy_Foster"},
			{"title":"Extra","extract":"Must be capped.","fullurl":"https://en.wikipedia.org/wiki/Extra"}
		]}}`)
	}))
	t.Cleanup(server.Close)

	retriever, err := NewMediaWiki(MediaWikiConfig{Client: server.Client(), Endpoint: server.URL,
		AllowInsecureTestURL: true, UserAgent: "Loomarr/test (https://loomarr.dev)", Now: func() time.Time { return at }})
	if err != nil {
		t.Fatal(err)
	}
	packet, err := retriever.Retrieve(t.Context(), "  Tootsie   Pop Classic Commercial  ")
	if err != nil {
		t.Fatal(err)
	}
	if packet.Query != "Tootsie Pop Classic Commercial" || packet.RetrievedAt != at || len(packet.Citations) != 3 {
		t.Fatalf("packet = %+v", packet)
	}
	if packet.Citations[0].ID != 1 || packet.Citations[1].Title != "Buddy Foster" ||
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
	if _, err := retriever.Retrieve(t.Context(), "commercial"); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized response error = %v", err)
	}
}
