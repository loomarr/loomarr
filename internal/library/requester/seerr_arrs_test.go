package requester

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

// Seerr's default non-4K server of each arr is the one read: Seerr's own address first, then the
// one it shows people. An arr Seerr has none of is left out.
func TestSeerrArrServers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "seerr-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/v1/settings/sonarr":
			_, _ = w.Write([]byte(`[
				{"hostname":"sonarr-4k","port":8990,"apiKey":"k4","is4k":true,"isDefault":true},
				{"hostname":"sonarr","port":8989,"useSsl":false,"baseUrl":"/tv/","externalUrl":"https://sonarr.example.invalid/","apiKey":"k1","isDefault":true}
			]`))
		case "/api/v1/settings/radarr":
			_, _ = w.Write([]byte(`[]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	got, err := NewSeerr(srv.URL, "seerr-key").ArrServers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].App != "sonarr" || got[0].APIKey != "k1" ||
		!slices.Equal(got[0].URLs, []string{"http://sonarr:8989/tv", "https://sonarr.example.invalid"}) {
		t.Fatalf("ArrServers = %+v", got)
	}
}

// A login page in front of an arr answers 200 with HTML: reachable, but not the arr.
func TestArrReachableRefusesALoginPage(t *testing.T) {
	login := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<!doctype html><title>Sign in</title>`))
	}))
	t.Cleanup(login.Close)
	if err := ArrReachable(context.Background(), "sonarr", login.URL, "k"); err == nil {
		t.Fatal("a login page must not read as Sonarr")
	}
}
