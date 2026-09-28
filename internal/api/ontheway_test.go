package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/store"
)

// Home's On the way (#1667): a downloading series carries "8 of 36 episodes" next to its
// progress; a movie carries no counts at all.
func TestOnTheWayEpisodeCountsOverHTTP(t *testing.T) {
	t.Parallel()
	h := newAPIHarness(t)
	ctx := context.Background()
	series := provision.Title{MediaType: provision.Series, TVDBID: 90001, Name: "A sitcom"}
	movie := provision.Title{MediaType: provision.Movie, TMDBID: 90002, Name: "A film"}
	for _, tt := range []provision.Title{series, movie} {
		key, _ := tt.Key()
		if err := h.Store.UpsertTitle(ctx, provision.Record{Key: key, Title: tt, State: provision.Downloading}); err != nil {
			t.Fatal(err)
		}
	}
	seriesKey, _ := series.Key()
	movieKey, _ := movie.Key()
	if err := h.Store.UpdateTitleProgress(ctx, seriesKey, store.TitleProgress{Progress: 0.3, EpisodesHave: 8, EpisodesWanted: 36}); err != nil {
		t.Fatal(err)
	}
	if err := h.Store.UpdateTitleProgress(ctx, movieKey, store.TitleProgress{Progress: 0.6}); err != nil {
		t.Fatal(err)
	}

	var body struct {
		Titles []struct {
			Key            string `json:"key"`
			EpisodesHave   *int   `json:"episodesHave"`
			EpisodesWanted *int   `json:"episodesWanted"`
		} `json:"titles"`
	}
	decodeOK(t, h.Do(http.MethodGet, "/v1/titles?state=downloading", memberToken, ""), &body)
	got := map[string][2]*int{}
	for _, tt := range body.Titles {
		got[tt.Key] = [2]*int{tt.EpisodesHave, tt.EpisodesWanted}
	}
	if c := got[string(seriesKey)]; c[0] == nil || c[1] == nil || *c[0] != 8 || *c[1] != 36 {
		t.Errorf("series counts = %v, want 8 of 36", c)
	}
	if c := got[string(movieKey)]; c[0] != nil || c[1] != nil {
		t.Errorf("movie carries episode counts %v, want both absent", c)
	}
}
