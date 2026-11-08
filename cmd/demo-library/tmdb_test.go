package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/demolibrary"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/tmdb"
)

// #1941: the real TMDB client, pointed at the demo server's /tmdb/3, finds the demo catalogue's
// invented titles with their genres, confirms their ids, and refuses an id the catalogue lacks.
// The media-server routes still answer beside it.
func TestServeAnswersAsTMDBOverTheDemoCatalogue(t *testing.T) {
	lib, err := demolibrary.Load(demolibrary.ContentInvented)
	if err != nil {
		t.Fatal(err)
	}
	media := demolibrary.Server{Layout: demolibrary.Layout{Dir: t.TempDir()}, Library: lib, Token: DemoToken}
	srv := httptest.NewServer(withTMDB(media, lib))
	t.Cleanup(srv.Close)
	client := tmdb.NewWithBase(srv.URL+TMDBPrefix, "unused")
	ctx := context.Background()

	var film, series demolibrary.Title
	for _, ti := range lib.Titles {
		if ti.Kind == demolibrary.Movie && film.Name == "" {
			film = ti
		}
		if ti.Kind == demolibrary.Series && series.Name == "" {
			series = ti
		}
	}

	got, err := client.Search(ctx, film.Name, 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	i := slices.IndexFunc(got, func(c catalog.Candidate) bool { return c.TMDBID == film.ProviderID() })
	if i < 0 {
		t.Fatalf("search %q = %+v, want tmdb id %d", film.Name, got, film.ProviderID())
	}
	if got[i].Name != film.Name || got[i].Year != film.Year || !slices.Equal(got[i].Genres, film.Genres) {
		t.Errorf("candidate = %+v, want %q (%d) %v", got[i], film.Name, film.Year, film.Genres)
	}

	for _, c := range []struct {
		mt   provision.MediaType
		id   int
		want bool
	}{
		{provision.Movie, film.ProviderID(), true},
		{provision.Series, series.ProviderID(), true},
		{provision.Movie, demolibrary.ProviderIDBase + 9999, false},
	} {
		ok, err := client.Exists(ctx, c.mt, c.id)
		if err != nil || ok != c.want {
			t.Errorf("exists %s %d = %v, %v; want %v", c.mt, c.id, ok, err, c.want)
		}
	}

	res, err := http.Get(srv.URL + "/System/Info/Public")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Errorf("media server beside the stand-in: %s", res.Status)
	}
}
