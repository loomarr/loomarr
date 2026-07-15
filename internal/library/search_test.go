package library

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The search adapter must REQUEST OfficialRating (the ChannelPolicy audience input,
// programming-design §4) and PARSE it off the real /Items shape. The pinned
// search_matrix fixture predates this field, so this uses an authored response that
// carries it — testing the parser against the real Emby item shape, not remembered
// field names.
func TestSearch_ParsesOfficialRating(t *testing.T) {
	var gotFields string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotFields = r.URL.Query().Get("Fields")
		_, _ = w.Write([]byte(`{"Items":[
			{"Id":"lib-1","Name":"Cartoon Hour","Type":"Series","ProductionYear":1993,
			 "Genres":["Animation"],"Overview":"kids show","OfficialRating":"TV-Y7",
			 "ProviderIds":{"Tmdb":"456"}},
			{"Id":"lib-2","Name":"Late Movie","Type":"Movie","ProductionYear":1999,
			 "OfficialRating":"R","ProviderIds":{"Tmdb":"603"}}
		],"TotalRecordCount":2}`))
	}))
	defer srv.Close()

	c := New(Emby, srv.URL, "tok", "dev-1")
	got, err := c.Search(context.Background(), "anything", 20)
	if err != nil {
		t.Fatal(err)
	}
	// The adapter must ask the server for OfficialRating (else Emby omits it).
	if !strings.Contains(gotFields, "OfficialRating") {
		t.Errorf("Fields param = %q, must request OfficialRating", gotFields)
	}
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2", len(got))
	}
	if got[0].OfficialRating != "TV-Y7" {
		t.Errorf("result[0].OfficialRating = %q, want TV-Y7", got[0].OfficialRating)
	}
	if got[1].OfficialRating != "R" {
		t.Errorf("result[1].OfficialRating = %q, want R", got[1].OfficialRating)
	}
}
