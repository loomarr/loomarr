package library

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/demolibrary"
	"github.com/loomarr/loomarr/internal/testkit"
)

func resultFor(t *testing.T, got []SearchResult, id string) SearchResult {
	t.Helper()
	for _, r := range got {
		if r.LibraryItemID == id {
			return r
		}
	}
	t.Fatalf("search results %+v do not include %q", got, id)
	return SearchResult{}
}

// The search adapter must REQUEST OfficialRating (the ChannelPolicy audience input,
// programming-design §4) and PARSE it off the /Items shape the stand-in serves.
func TestSearch_ParsesOfficialRatingAndRuntime(t *testing.T) {
	c, demo := demoClient(t, Emby)
	for _, title := range []demolibrary.Title{testkit.FirstSeries(), testkit.FirstMovie()} {
		got, err := c.Search(context.Background(), title.Name, 20)
		if err != nil {
			t.Fatal(err)
		}
		// The adapter must ask the server for OfficialRating (else Emby omits it).
		fields := demo.LastQuery(t).Get("Fields")
		if !strings.Contains(fields, "OfficialRating") {
			t.Errorf("Fields param = %q, must request OfficialRating", fields)
		}
		if !strings.Contains(fields, "RunTimeTicks") {
			t.Errorf("Fields param = %q, must request RunTimeTicks", fields)
		}
		r := resultFor(t, got, title.ID())
		if r.OfficialRating != title.Rating {
			t.Errorf("%s OfficialRating = %q, want %q", title.Name, r.OfficialRating, title.Rating)
		}
		f, _ := demolibrary.FormatByID(title.Format)
		if r.RuntimeMinutes != f.Duration/60 {
			t.Errorf("%s RuntimeMinutes = %d, want %d", title.Name, r.RuntimeMinutes, f.Duration/60)
		}
	}
}

// The approval summary shows an owned title's file size (#1817), read from the search
// response already in hand. Emby's top-level Size is MediaSources[0].Size (measured live:
// +134 bytes on a 20-item page, where Fields=MediaSources added ~97 KB of streams).
// A series has no media source of its own: its size is absent, never 0-as-a-fact.
func TestSearch_ParsesEmbySize(t *testing.T) {
	c, demo := demoClient(t, Emby)
	film, show := testkit.FirstMovie(), testkit.FirstSeries()
	path := demo.Layout.FilmFile(film)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, 4096), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := c.Search(context.Background(), film.Name, 20)
	if err != nil {
		t.Fatal(err)
	}
	if fields := demo.LastQuery(t).Get("Fields"); !strings.Contains(","+fields+",", ",Size,") {
		t.Errorf("Fields = %q, must request Size", fields)
	}
	if r := resultFor(t, got, film.ID()); r.SizeBytes != 4096 {
		t.Errorf("film SizeBytes = %d, want the file's 4096 bytes", r.SizeBytes)
	}

	got, err = c.Search(context.Background(), show.Name, 20)
	if err != nil {
		t.Fatal(err)
	}
	if r := resultFor(t, got, show.ID()); r.SizeBytes != 0 {
		t.Errorf("series SizeBytes = %d, want 0 (unavailable)", r.SizeBytes)
	}
}

// Jellyfin has no top-level Size field, so it asks for MediaSources and the FIRST source
// answers. The stand-in serves one source per item, so this authored response is the only
// way to show that a second source never contributes.
func TestSearch_ParsesJellyfinFirstSourceSize(t *testing.T) {
	var gotFields string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotFields = r.URL.Query().Get("Fields")
		_, _ = w.Write([]byte(`{"Items":[
			{"Id":"lib-1","Name":"The Quiet Ledger","Type":"Movie","MediaSources":[{"Id":"src-1","Size":21914038799},{"Id":"src-2","Size":5}],"ProviderIds":{"Tmdb":"90157851"}},
			{"Id":"lib-2","Name":"Kestrel One","Type":"Series","Size":null,"ProviderIds":{"Tmdb":"90063639"}}
		]}`))
	}))
	defer srv.Close()

	got, err := New(Jellyfin, srv.URL, "tok", "dev-1").Search(context.Background(), "the", 20)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(","+gotFields+",", ",MediaSources,") {
		t.Errorf("Fields = %q, must request MediaSources", gotFields)
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
}
