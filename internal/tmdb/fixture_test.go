package tmdb_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/testkit"
	"github.com/mantonx/loomarr/internal/tmdb"
)

// TestSearch_ParsesRealFixture pins the adapter against the LIVE-captured TMDB
// /search/multi shape (fixtures/tmdb/search_multi_matrix.json, Phase-11 follow-up
// capture). This is the "parsers against fixtures, not remembered field names"
// rule (AGENTS.md) applied to TMDB: if TMDB renames a field, this fails.
func TestSearch_ParsesRealFixture(t *testing.T) {
	body := testkit.Fixture(t, "tmdb/search_multi_matrix.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/multi" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("bearer auth not set (key must not ride in the URL): %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := tmdb.NewWithBase(srv.URL, "test-key")
	got, err := c.Search(context.Background(), "matrix", 20)
	if err != nil {
		t.Fatal(err)
	}

	// The person result is dropped; movie + tv are kept.
	for _, cand := range got {
		if cand.Name == "Keanu Reeves" {
			t.Fatal("person result not filtered out (grounding must keep only movie/tv)")
		}
		if cand.TMDBID == 0 {
			t.Errorf("candidate missing real TMDB id: %+v", cand)
		}
	}

	// The Matrix (movie, id 603, year from release_date 1999-03-31).
	var foundMatrix bool
	for _, cand := range got {
		if cand.TMDBID == 603 {
			foundMatrix = true
			if cand.MediaType != provision.Movie {
				t.Errorf("Matrix media type = %s, want movie", cand.MediaType)
			}
			if cand.Name != "The Matrix" {
				t.Errorf("Matrix name = %q (should read the `title` field)", cand.Name)
			}
			if cand.Year != 1999 {
				t.Errorf("Matrix year = %d, want 1999 (from release_date)", cand.Year)
			}
		}
	}
	if !foundMatrix {
		t.Fatal("The Matrix (603) missing from parsed results")
	}

	// The tv results read the `name`/`first_air_date` fields.
	var foundTV bool
	for _, cand := range got {
		if cand.MediaType == provision.Series {
			foundTV = true
			if cand.Name == "" {
				t.Error("tv result missing name (should read the `name` field)")
			}
		}
	}
	if !foundTV {
		t.Error("no tv result parsed (should keep media_type=tv)")
	}
}
