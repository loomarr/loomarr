package requester

import (
	"context"
	"net/http"
	"testing"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/testkit"
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

// Reachable is the "test my Seerr" probe: ok with a key, error without (bad key),
// error on an unreachable host (§8 setup).
func TestSeerrReachable(t *testing.T) {
	ms := testkit.NewSeerr(t)
	t.Cleanup(ms.Close)

	if err := NewSeerr(ms.URL, "secret-key").Reachable(context.Background()); err != nil {
		t.Errorf("Reachable with a key → %v, want ok", err)
	}
	if err := NewSeerr(ms.URL, "").Reachable(context.Background()); err == nil {
		t.Error("Reachable with no key → ok, want a rejected-key error")
	}
	if err := NewSeerr("http://127.0.0.1:0", "k").Reachable(context.Background()); err == nil {
		t.Error("Reachable against an unreachable host → ok, want a transport error")
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

// Series requests map to mediaType "tv" and ALWAYS carry `seasons` — "all" when no
// specific seasons are asked for, else the array. Jellyseerr 500s if `seasons` is omitted
// (its TV handler filters over it), so the body is asserted, not just the no-error path.
func TestSeerrSeries(t *testing.T) {
	cases := []struct {
		name    string
		seasons []int
		want    string // expected JSON body
	}{
		// The common path — "acquire all seasons". This is the one that broke in production:
		// the old omitempty []int dropped `seasons` entirely and Jellyseerr returned 500.
		{"all seasons (nil)", nil, `{"mediaType":"tv","mediaId":555,"seasons":"all"}`},
		{"all seasons (empty)", []int{}, `{"mediaType":"tv","mediaId":555,"seasons":"all"}`},
		{"specific seasons", []int{1, 2}, `{"mediaType":"tv","mediaId":555,"seasons":[1,2]}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ms := testkit.NewSeerr(t)
			t.Cleanup(ms.Close)
			r := NewSeerr(ms.URL, "k")
			err := r.Request(context.Background(), provision.Title{MediaType: provision.Series, TMDBID: 555, Seasons: c.seasons})
			if err != nil {
				t.Fatalf("series request: %v", err)
			}
			if got := string(ms.LastBody); got != c.want {
				t.Errorf("body\n got %s\nwant %s", got, c.want)
			}
		})
	}
}

// A movie request omits `seasons` entirely (Jellyseerr only wants it for TV).
func TestSeerrMovieOmitsSeasons(t *testing.T) {
	ms := testkit.NewSeerr(t)
	t.Cleanup(ms.Close)
	r := NewSeerr(ms.URL, "k")
	if err := r.Request(context.Background(), provision.Title{MediaType: provision.Movie, TMDBID: 42}); err != nil {
		t.Fatalf("movie request: %v", err)
	}
	if got, want := string(ms.LastBody), `{"mediaType":"movie","mediaId":42}`; got != want {
		t.Errorf("movie body\n got %s\nwant %s", got, want)
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

// QueueStatus maps Seerr's /media processing enum to coarse status (§18.1): PROCESSING and
// PARTIALLY_AVAILABLE are Grabbed (promote requested→downloading) with a label and NO
// percentage; PENDING and titles absent from the set are not grabbed. Progress stays 0 —
// Seerr has no byte count, the label carries the meaning.
func TestSeerrQueueStatus(t *testing.T) {
	ms := testkit.NewSeerr(t)
	t.Cleanup(ms.Close)
	// 82728 downloading, 65733 partly available, 3022 still pending; 999 not in the set at all.
	ms.Processing = map[int]int{82728: 3, 65733: 4, 3022: 2}
	r := NewSeerr(ms.URL, "k")

	titles := []provision.Title{
		{MediaType: provision.Series, TMDBID: 82728},
		{MediaType: provision.Series, TMDBID: 65733},
		{MediaType: provision.Series, TMDBID: 3022},
		{MediaType: provision.Series, TMDBID: 999},
	}
	items, err := r.QueueStatus(context.Background(), titles)
	if err != nil {
		t.Fatalf("QueueStatus: %v", err)
	}
	byTMDB := map[int]QueueItem{}
	for i, it := range items {
		byTMDB[titles[i].TMDBID] = it
	}

	want := []struct {
		tmdb    int
		grabbed bool
		label   string
	}{
		{82728, true, "Downloading"},
		{65733, true, "Partly available"},
		{3022, false, ""}, // pending — not grabbed
		{999, false, ""},  // absent from the processing set — not grabbed
	}
	for _, w := range want {
		got := byTMDB[w.tmdb]
		if got.Grabbed != w.grabbed || got.Status != w.label {
			t.Errorf("tmdb %d: got grabbed=%v status=%q, want grabbed=%v status=%q",
				w.tmdb, got.Grabbed, got.Status, w.grabbed, w.label)
		}
		if got.Progress != 0 {
			t.Errorf("tmdb %d: Progress=%v, want 0 (Seerr has no percentage)", w.tmdb, got.Progress)
		}
	}
}
