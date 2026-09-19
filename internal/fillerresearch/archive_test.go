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
		if query.Get("q") != `mediatype:movies AND (title:"Tootsie Pop Classic Commercial" OR description:"Tootsie Pop Classic Commercial" OR title:"Tootsie Pop" OR description:"Tootsie Pop")` ||
			query.Get("rows") != "3" || query.Get("output") != "json" || len(query["fl[]"]) != 7 {
			t.Errorf("query = %v", query)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"response":{"docs":[
			{"identifier":"tootsie-pop-ad","title":"Tootsie Pop commercial","description":["Animated television advertisement","Campaign collection"],"year":1970,"creator":"Agency","subject":["Candy","Advertising"]},
			{"identifier":"bad/path","title":"Bad","description":"Ignored"},
			{"identifier":"tootsie-two","title":["Alternate title"],"date":"1971-01-01","description":"Second item"}
		]}}`)
	}))
	t.Cleanup(server.Close)

	retriever, err := NewArchive(ArchiveConfig{Client: server.Client(), Endpoint: server.URL,
		AllowInsecureTestURL: true, UserAgent: "Loomarr/test", Now: func() time.Time { return at }})
	if err != nil {
		t.Fatal(err)
	}
	packet, err := retriever.Retrieve(t.Context(), Lookup{Title: "Tootsie Pop Classic Commercial"})
	if err != nil {
		t.Fatal(err)
	}
	if packet.Query != "Tootsie Pop Classic Commercial" || packet.RetrievedAt != at || len(packet.Citations) != 2 {
		t.Fatalf("packet = %+v", packet)
	}
	if packet.Citations[0].URL != "https://archive.org/details/tootsie-pop-ad" ||
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
