package tmdb_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/loomarr/loomarr/internal/library/tmdb"
	"github.com/loomarr/loomarr/internal/platform/httpx"
	"github.com/loomarr/loomarr/internal/provision"
)

// A non-2xx TMDB answer is an httpx.StatusError (status machine-readable through
// errors.As) and tmdb.FailureClass buckets it by that status.
func TestNon2xxIsHTTPXStatusError(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		status int
		want   string
	}{
		{http.StatusBadRequest, tmdb.FailureClientError},
		{http.StatusUnauthorized, tmdb.FailureUnauthorized},
		{http.StatusForbidden, tmdb.FailureUnauthorized},
		{http.StatusUnprocessableEntity, tmdb.FailureClientError},
		{http.StatusServiceUnavailable, tmdb.FailureServerError},
	} {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "upstream says no", tc.status)
			}))
			defer srv.Close()
			c := tmdb.NewWithBase(srv.URL, "key")

			_, err := c.Exists(context.Background(), provision.Movie, 90000603)
			var se *httpx.StatusError
			if !errors.As(err, &se) || se.Status != tc.status {
				t.Fatalf("Exists error = %v, want *httpx.StatusError with status %d", err, tc.status)
			}
			if got := tmdb.FailureClass(err); got != tc.want {
				t.Errorf("FailureClass = %q, want %q", got, tc.want)
			}
			if want := fmt.Sprintf("tmdb GET /movie/90000603: status %d: upstream says no", tc.status); err.Error() != want {
				t.Errorf("error text = %q, want %q", err.Error(), want)
			}
		})
	}
}

// 404 stays "not found, not a failure" for the two callers that tolerate it.
func TestNotFoundIsNotAFailureWhereTolerated(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	c := tmdb.NewWithBase(srv.URL, "key")

	if ok, err := c.Exists(context.Background(), provision.Series, 90000001); err != nil || ok {
		t.Errorf("Exists on 404 = %v, %v; want false, nil", ok, err)
	}
	if u, err := c.EpisodeStillURL(context.Background(), 90000001, 1, 1); err != nil || u != "" {
		t.Errorf("EpisodeStillURL on 404 = %q, %v; want \"\", nil", u, err)
	}
	// A call that does not tolerate 404 reports it, as a client error.
	_, err := c.CollectionID(context.Background(), provision.Movie, 90000001)
	if got := tmdb.FailureClass(err); got != tmdb.FailureClientError {
		t.Errorf("FailureClass(404 from CollectionID) = %q (err %v), want %q", got, err, tmdb.FailureClientError)
	}
}
