package tmdb

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/loomarr/loomarr/internal/httpx"
)

func TestNetworkExportNon2xxIsStatusError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	c := &Client{networkExportBaseURL: srv.URL, http: srv.Client()}

	_, err := c.fetchNetworkExport(context.Background(), "tv_network_ids.json.gz")
	var se *httpx.StatusError
	if !errors.As(err, &se) || se.Status != http.StatusForbidden {
		t.Fatalf("fetchNetworkExport error = %v, want *httpx.StatusError 403", err)
	}
	if got := httpx.Classify(err); got != httpx.ClassAuth {
		t.Errorf("Classify = %v, want %v", got, httpx.ClassAuth)
	}
}
