package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/demolibrary"
	mp "github.com/loomarr/loomarr/internal/metadataprovider"
	provider "github.com/loomarr/loomarr/internal/metadataprovider/tmdb"
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

// The demo plays what Loomarr's own scan finds (#2151 step 16), and the scan names each title from
// its catalog record: the TMDB provider, pointed at the stand-in, must get a film's and a show's
// details (name, overview, genres, rating, runtime, a show's TVDB id) and a season's episodes by
// number, or the guide shows file names.
func TestTheScansTMDBProviderReadsTheDemoCatalogue(t *testing.T) {
	lib, err := demolibrary.Load(demolibrary.ContentInvented)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(withTMDB(demolibrary.Server{Layout: demolibrary.Layout{Dir: t.TempDir()}, Library: lib, Token: DemoToken}, lib))
	t.Cleanup(srv.Close)
	p := provider.New(tmdb.NewWithBase(srv.URL+TMDBPrefix, "unused"), nil)
	ctx := context.Background()

	var film, show demolibrary.Title
	for _, ti := range lib.Titles {
		if ti.Kind == demolibrary.Movie && film.Name == "" {
			film = ti
		}
		if ti.Kind == demolibrary.Series && show.Name == "" {
			show = ti
		}
	}
	tmdbRef := func(kind mp.Kind, value string) mp.Ref {
		return mp.Ref{Kind: kind, ID: mp.ExternalID{Namespace: mp.NamespaceTMDB, Value: value}}
	}

	rec, err := p.Fetch(ctx, tmdbRef(mp.KindMovie, strconv.Itoa(film.ProviderID())))
	if err != nil {
		t.Fatalf("film: %v", err)
	}
	if d := rec.Descriptive; d == nil || d.Title != film.Name || d.Overview != film.Overview || d.Runtime == 0 || len(d.Genres) == 0 {
		t.Errorf("film details = %+v, want %q with its overview, genres and runtime", rec.Descriptive, film.Name)
	}
	if rec.Certification == nil {
		t.Errorf("film has no rating, want %q", film.Rating)
	}

	rec, err = p.Fetch(ctx, tmdbRef(mp.KindSeries, strconv.Itoa(show.ProviderID())))
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	if d := rec.Descriptive; d == nil || d.Title != show.Name || d.Overview != show.Overview {
		t.Errorf("show details = %+v, want %q with its overview", rec.Descriptive, show.Name)
	}
	if !slices.Contains(rec.IDs, mp.ExternalID{Namespace: mp.NamespaceTVDB, Value: strconv.Itoa(show.ProviderID())}) {
		t.Errorf("show ids = %v, want its TVDB id %d", rec.IDs, show.ProviderID())
	}

	season, err := p.Fetch(ctx, tmdbRef(mp.KindSeason, strconv.Itoa(show.ProviderID())+"/1"))
	if err != nil {
		t.Fatalf("season 1: %v", err)
	}
	eps := lib.EpisodesOf(show)
	if len(season.Children) != len(eps) {
		t.Fatalf("season 1 lists %d episodes, want %d", len(season.Children), len(eps))
	}
	if got := season.Children[0]; got.Numbering == nil || got.Numbering.Episode != 1 || got.Descriptive.Title != eps[0].Name {
		t.Errorf("episode 1 = %+v, want %q numbered 1", got, eps[0].Name)
	}
}
