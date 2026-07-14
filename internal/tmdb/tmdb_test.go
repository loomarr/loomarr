package tmdb_test

import (
	"context"
	"testing"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/testkit"
	"github.com/mantonx/loomarr/internal/tmdb"
)

func TestSearch_MapsMovieAndTV_DropsPerson(t *testing.T) {
	mock := testkit.NewTMDB(t)
	c := tmdb.NewWithBase(mock.URL, "key")

	got, err := c.Search(context.Background(), "the", 20) // matches The Rock, The Matrix
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("expected results")
	}
	for _, cand := range got {
		if cand.TMDBID == 0 {
			t.Errorf("candidate has no TMDB id (grounding requires real ids): %+v", cand)
		}
		if cand.MediaType != provision.Movie && cand.MediaType != provision.Series {
			t.Errorf("person/unknown result not filtered: %+v", cand)
		}
	}
}

func TestExists_RealID200_FabricatedID404(t *testing.T) {
	mock := testkit.NewTMDB(t)
	c := tmdb.NewWithBase(mock.URL, "key")

	// Real movie id (100 = Speed) exists.
	ok, err := c.Exists(context.Background(), provision.Movie, 100)
	if err != nil || !ok {
		t.Fatalf("Exists(movie,100) = %v,%v want true,nil", ok, err)
	}
	// Fabricated id → does not exist (the LLM-hallucination path §8).
	ok, err = c.Exists(context.Background(), provision.Movie, 88888888)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("fabricated TMDB id must report not-exists (grounding drops it)")
	}
	// Real series id.
	ok, _ = c.Exists(context.Background(), provision.Series, 1396)
	if !ok {
		t.Error("Breaking Bad (tv:1396) should exist")
	}
}

func TestExists_ZeroIDIsFalse(t *testing.T) {
	c := tmdb.NewWithBase("http://unused", "key")
	ok, err := c.Exists(context.Background(), provision.Movie, 0)
	if err != nil || ok {
		t.Errorf("Exists(_,0) = %v,%v want false,nil", ok, err)
	}
}
