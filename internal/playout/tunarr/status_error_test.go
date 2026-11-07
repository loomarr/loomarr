package tunarr_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/httpx"
	"github.com/loomarr/loomarr/internal/playout/tunarr"
)

func statusServer(t *testing.T, status int) *tunarr.Tunarr {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"error":"nope"}`))
	}))
	t.Cleanup(srv.Close)
	return tunarr.New(srv.URL, "cfg")
}

func requireStatusError(t *testing.T, err error, status int, class httpx.Class) {
	t.Helper()
	var se *httpx.StatusError
	if !errors.As(err, &se) || se.Status != status {
		t.Fatalf("error = %v, want *httpx.StatusError with status %d", err, status)
	}
	if se.Body != `{"error":"nope"}` {
		t.Errorf("body = %q, want Tunarr's own reason kept", se.Body)
	}
	if got := httpx.Classify(err); got != class {
		t.Errorf("Classify = %v, want %v", got, class)
	}
}

// A non-2xx Tunarr answer is an httpx.StatusError carrying Tunarr's body, classified by httpx.
func TestNon2xxIsHTTPXStatusError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	t.Run("list channels 401 is auth", func(t *testing.T) {
		t.Parallel()
		_, err := statusServer(t, http.StatusUnauthorized).ListChannels(ctx)
		requireStatusError(t, err, http.StatusUnauthorized, httpx.ClassAuth)
	})
	t.Run("get channel 422 is permanent", func(t *testing.T) {
		t.Parallel()
		_, _, err := statusServer(t, http.StatusUnprocessableEntity).GetChannel(ctx, "c1")
		requireStatusError(t, err, http.StatusUnprocessableEntity, httpx.ClassPermanent)
	})
	t.Run("delete channel 500 is transient", func(t *testing.T) {
		t.Parallel()
		err := statusServer(t, http.StatusInternalServerError).DeleteChannel(ctx, "c1")
		requireStatusError(t, err, http.StatusInternalServerError, httpx.ClassTransient)
	})
	t.Run("get lineup 403 is auth", func(t *testing.T) {
		t.Parallel()
		_, err := statusServer(t, http.StatusForbidden).GetLineup(ctx, "c1")
		requireStatusError(t, err, http.StatusForbidden, httpx.ClassAuth)
	})
	t.Run("guide 401 is auth", func(t *testing.T) {
		t.Parallel()
		_, err := statusServer(t, http.StatusUnauthorized).Guide(ctx, time.Now(), time.Now().Add(time.Hour))
		requireStatusError(t, err, http.StatusUnauthorized, httpx.ClassAuth)
	})
}

// The statuses Tunarr uses to say "nothing here" stay answers, not failures.
func TestToleratedStatusesAreNotErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	if _, ok, err := statusServer(t, http.StatusNotFound).GetChannel(ctx, "c1"); err != nil || ok {
		t.Errorf("GetChannel 404 = %v, %v; want false, nil", ok, err)
	}
	if err := statusServer(t, http.StatusNotFound).DeleteChannel(ctx, "c1"); err != nil {
		t.Errorf("DeleteChannel 404 = %v; want nil (already gone)", err)
	}
	for _, status := range []int{http.StatusBadRequest, http.StatusNotFound} {
		if slots, err := statusServer(t, status).GetLineup(ctx, "c1"); err != nil || len(slots) != 0 {
			t.Errorf("GetLineup %d = %v, %v; want empty, nil", status, slots, err)
		}
		if guide, err := statusServer(t, status).Guide(ctx, time.Now(), time.Now().Add(time.Hour)); err != nil || len(guide) != 0 {
			t.Errorf("Guide %d = %v, %v; want empty, nil", status, guide, err)
		}
	}
}
