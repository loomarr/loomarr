package requester

import (
	"context"
	"net/http"
	"testing"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/testkit"
)

func TestSeerrRequestMovie(t *testing.T) {
	ms := testkit.NewSeerr(t)
	t.Cleanup(ms.Close)
	r := NewSeerr(ms.URL, "secret-key")

	err := r.Request(context.Background(), provision.Title{MediaType: provision.Movie, TMDBID: 1111867})
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if ms.LastAPIKey != "secret-key" {
		t.Errorf("X-Api-Key = %q, want secret-key (§6 header auth)", ms.LastAPIKey)
	}
}

// §6 idempotency: both 201 and 409 are success.
func TestSeerrIdempotentStatuses(t *testing.T) {
	for _, code := range []int{http.StatusCreated, http.StatusOK, http.StatusConflict} {
		ms := testkit.NewSeerr(t)
		ms.Status = code
		r := NewSeerr(ms.URL, "k")
		if err := r.Request(context.Background(), provision.Title{MediaType: provision.Movie, TMDBID: 1}); err != nil {
			t.Errorf("status %d should be success (§6), got %v", code, err)
		}
		ms.Close()
	}
}

// A 5xx is a real failure — the record stays wanted for the reconciler (§6).
func TestSeerrServerErrorFails(t *testing.T) {
	ms := testkit.NewSeerr(t)
	t.Cleanup(ms.Close)
	ms.Status = http.StatusInternalServerError
	r := NewSeerr(ms.URL, "k")
	if err := r.Request(context.Background(), provision.Title{MediaType: provision.Movie, TMDBID: 1}); err == nil {
		t.Error("500 should be an error so the reconciler retries")
	}
}

// Series requests map to mediaType "tv" and carry seasons.
func TestSeerrSeries(t *testing.T) {
	ms := testkit.NewSeerr(t)
	t.Cleanup(ms.Close)
	r := NewSeerr(ms.URL, "k")
	err := r.Request(context.Background(), provision.Title{MediaType: provision.Series, TMDBID: 555, Seasons: []int{1, 2}})
	if err != nil {
		t.Fatalf("series request: %v", err)
	}
}

// A title without a TMDB id cannot be requested (Seerr keys on TMDB — §6).
func TestSeerrNoTMDBID(t *testing.T) {
	ms := testkit.NewSeerr(t)
	t.Cleanup(ms.Close)
	r := NewSeerr(ms.URL, "k")
	if err := r.Request(context.Background(), provision.Title{MediaType: provision.Movie}); err == nil {
		t.Error("expected error for a title with no TMDB id")
	}
}
