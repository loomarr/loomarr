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
func TestSearch_ParsesOfficialRatingAndRuntime(t *testing.T) {
	var gotFields string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotFields = r.URL.Query().Get("Fields")
		_, _ = w.Write([]byte(`{"Items":[
			{"Id":"lib-1","Name":"Cartoon Hour","Type":"Series","ProductionYear":1993,
			 "Genres":["Animation"],"Overview":"kids show","OfficialRating":"TV-Y7","RunTimeTicks":54000000000,
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
	if !strings.Contains(gotFields, "RunTimeTicks") {
		t.Errorf("Fields param = %q, must request RunTimeTicks", gotFields)
	}
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2", len(got))
	}
	if got[0].OfficialRating != "TV-Y7" {
		t.Errorf("result[0].OfficialRating = %q, want TV-Y7", got[0].OfficialRating)
	}
	if got[0].RuntimeMinutes != 90 {
		t.Errorf("result[0].RuntimeMinutes = %d, want 90", got[0].RuntimeMinutes)
	}
	if got[1].OfficialRating != "R" {
		t.Errorf("result[1].OfficialRating = %q, want R", got[1].OfficialRating)
	}
}

// The approval summary shows an owned title's file size (#1817), read from the search
// response already in hand. Emby's top-level Size is MediaSources[0].Size (measured live:
// +134 bytes on a 20-item page, where Fields=MediaSources added ~97 KB of streams).
// Jellyfin has no Size field, so it asks for MediaSources and the first source answers.
// A series has no media source of its own: its size is absent, never 0-as-a-fact.
func TestSearch_ParsesPrimarySourceSize(t *testing.T) {
	cases := []struct {
		flavor     Flavor
		wantField  string
		movieBytes string
	}{
		{Emby, "Size", `"Size":21914038799`},
		{Jellyfin, "MediaSources", `"MediaSources":[{"Id":"src-1","Size":21914038799},{"Id":"src-2","Size":5}]`},
	}
	for _, tc := range cases {
		t.Run(string(tc.flavor), func(t *testing.T) {
			var gotFields string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotFields = r.URL.Query().Get("Fields")
				_, _ = w.Write([]byte(`{"Items":[
					{"Id":"lib-1","Name":"The Zero Theorem","Type":"Movie",` + tc.movieBytes + `,"ProviderIds":{"Tmdb":"157851"}},
					{"Id":"lib-2","Name":"The Expanse","Type":"Series","Size":null,"ProviderIds":{"Tmdb":"63639"}}
				]}`))
			}))
			defer srv.Close()

			got, err := New(tc.flavor, srv.URL, "tok", "dev-1").Search(context.Background(), "the", 20)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(","+gotFields+",", ","+tc.wantField+",") {
				t.Errorf("Fields = %q, must request %s", gotFields, tc.wantField)
			}
			if len(got) != 2 {
				t.Fatalf("got %d results, want 2", len(got))
			}
			if got[0].SizeBytes != 21914038799 {
				t.Errorf("movie SizeBytes = %d, want 21914038799", got[0].SizeBytes)
			}
			if got[1].SizeBytes != 0 {
				t.Errorf("series SizeBytes = %d, want 0 (unavailable)", got[1].SizeBytes)
			}
		})
	}
}
