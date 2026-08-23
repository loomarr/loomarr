package tmdb_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/tmdb"
)

// movieDetail is the /movie/{id} shape the exists-check re-validates against (§8
// grounding). Exists() itself is STATUS-ONLY today — a 200 means the id is real, a
// 404 means the LLM proposed a non-existent id (tmdb.go Exists). This struct pins
// the real BODY shape the endpoint returns, so a future body-reading enhancement
// (e.g. cross-checking title/year against the suggestion) has a contract to build
// on rather than remembered field names.
type movieDetail struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	ReleaseDate string `json:"release_date"`
}

// TestExists_MovieBody_ContractPin parses the pinned live capture
// (fixtures/tmdb/movie_603.json) — a minimal /movie/603 200 body (The Matrix). Per
// that dir's FINDINGS.md the capture is intentionally minimal (the adapter only
// reads status), so it carries id + title but NOT release_date; we assert the
// present fields decode and that the declared release_date field is a clean
// zero-value (absent, not a decode error), which pins the shape without inventing
// data not in the pinned bytes.
func TestExists_MovieBody_ContractPin(t *testing.T) {
	raw := testkit.Fixture(t, "tmdb/movie_603.json")

	var m movieDetail
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("pinned /movie/{id} 200 body must decode into the movieDetail shape: %v", err)
	}
	if m.ID != 603 {
		t.Errorf("id = %d, want 603 (The Matrix real id, the exists-check target)", m.ID)
	}
	if m.Title != "The Matrix" {
		t.Errorf("title = %q, want \"The Matrix\" (movie title field is `title`, per FINDINGS.md)", m.Title)
	}
	// release_date is absent in this minimal capture; the field must still be a
	// declared part of the /movie shape (it parses to "" rather than erroring). If a
	// future fixture pins the full body, this guards the field name.
	if m.ReleaseDate != "" {
		t.Errorf("release_date = %q, want empty (the minimal pinned capture omits it)", m.ReleaseDate)
	}
}

// TestExists_DrivesRealAdapterOn200 exercises the REAL Exists path: it serves a 200
// for /movie/603 (the pinned exists-check success) and asserts the adapter reports
// the id as real. This is the status-based contract exists() relies on today — a
// 200 = real, mirroring the live capture (movie 603 → 200).
func TestExists_DrivesRealAdapterOn200(t *testing.T) {
	raw := testkit.Fixture(t, "tmdb/movie_603.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/movie/603" {
			t.Errorf("unexpected path %s, want /movie/603", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("bearer auth not set (key must not ride in the URL): %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write(raw)
	}))
	defer srv.Close()

	c := tmdb.NewWithBase(srv.URL, "test-key")
	ok, err := c.Exists(context.Background(), provision.Movie, 603)
	if err != nil {
		t.Fatalf("Exists(603): %v", err)
	}
	if !ok {
		t.Fatal("Exists(603) must report the pinned real id as existing (200 = real)")
	}
}
