package fillerresearch

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestArchiveRetrievesBoundedItemMetadata(t *testing.T) {
	at := time.Unix(200, 0).UTC()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != "Loomarr/test" {
			t.Errorf("User-Agent = %q", got)
		}
		query := r.URL.Query()
		if query.Get("q") != `mediatype:movies AND (title:"Twirlpop Classic Commercial" OR description:"Twirlpop Classic Commercial" OR title:"Twirlpop" OR description:"Twirlpop")` ||
			query.Get("rows") != "3" || query.Get("output") != "json" || len(query["fl[]"]) != 7 {
			t.Errorf("query = %v", query)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"response":{"docs":[
			{"identifier":"twirlpop-ad","title":"Twirlpop commercial","description":["Animated television advertisement","Campaign collection"],"year":1970,"creator":"Agency","subject":["Candy","Advertising"]},
			{"identifier":"bad/path","title":"Bad","description":"Ignored"},
			{"identifier":"twirlpop-two","title":["Alternate title"],"date":"1971-01-01","description":"Second item"}
		]}}`)
	}))
	t.Cleanup(server.Close)

	retriever, err := NewArchive(ArchiveConfig{Client: server.Client(), Endpoint: server.URL,
		AllowInsecureTestURL: true, UserAgent: "Loomarr/test", Now: func() time.Time { return at }})
	if err != nil {
		t.Fatal(err)
	}
	packet, err := retriever.Retrieve(t.Context(), Lookup{Title: "Twirlpop Classic Commercial"})
	if err != nil {
		t.Fatal(err)
	}
	if packet.Query != "Twirlpop Classic Commercial" || packet.RetrievedAt != at || len(packet.Citations) != 2 {
		t.Fatalf("packet = %+v", packet)
	}
	if packet.Citations[0].URL != "https://archive.org/details/twirlpop-ad" ||
		!strings.Contains(packet.Citations[0].Extract, "Year: 1970") ||
		!strings.Contains(packet.Citations[0].Extract, "Subject: Candy; Advertising") {
		t.Fatalf("citation = %+v", packet.Citations[0])
	}
}

func TestArchiveRejectsArbitraryEndpointAndOversizedResponse(t *testing.T) {
	if _, err := NewArchive(ArchiveConfig{Endpoint: "https://example.com/search", UserAgent: "test"}); err == nil {
		t.Fatal("arbitrary endpoint accepted")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, strings.Repeat("x", archiveMaxBody+1))
	}))
	t.Cleanup(server.Close)
	retriever, err := NewArchive(ArchiveConfig{Client: server.Client(), Endpoint: server.URL,
		AllowInsecureTestURL: true, UserAgent: "Loomarr/test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := retriever.Retrieve(t.Context(), Lookup{Title: "commercial"}); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized response error = %v", err)
	}
}

func TestArchiveRejectsCrossOriginRedirect(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("cross-origin redirect target was contacted")
	}))
	t.Cleanup(target.Close)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	t.Cleanup(server.Close)
	retriever, err := NewArchive(ArchiveConfig{Client: server.Client(), Endpoint: server.URL,
		AllowInsecureTestURL: true, UserAgent: "Loomarr/test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := retriever.Retrieve(t.Context(), Lookup{Title: "commercial"}); err == nil ||
		!strings.Contains(err.Error(), "configured origin") {
		t.Fatalf("redirect error = %v", err)
	}
}
