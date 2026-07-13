package reconcile

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/library"
	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/testkit"
)

var now = time.Date(2026, 7, 13, 20, 0, 0, 0, time.UTC)

func setup(t *testing.T) (*Reconciler, store.Store, *testkit.Requester, *testkit.MediaServer) {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/r.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	req := &testkit.Requester{}
	ms := testkit.NewMediaServer(t)
	t.Cleanup(ms.Close)
	lib := library.New(library.Emby, ms.URL, ms.AdminToken, "dev")
	rc := New(st, req, lib, nil, Config{
		RequestTTL: 48 * time.Hour, DownloadingTTL: 12 * time.Hour, Batch: 50, Lease: 2 * time.Minute,
	}, func() time.Time { return now }, slog.New(slog.DiscardHandler))
	return rc, st, req, ms
}

func put(t *testing.T, st store.Store, key provision.Key, state provision.State, tmdb int, deadline time.Time) {
	t.Helper()
	err := st.UpsertTitle(context.Background(), provision.Record{
		Key: key, State: state, Deadline: deadline,
		Title: provision.Title{MediaType: provision.Movie, TMDBID: tmdb},
	})
	if err != nil {
		t.Fatal(err)
	}
}

// A due `wanted` title gets resubmitted; success → requested.
func TestRetryWantedSuccess(t *testing.T) {
	rc, st, req, _ := setup(t)
	put(t, st, "movie:tmdb:1", provision.Wanted, 1, now.Add(-time.Hour))

	n, err := rc.Tick(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("Tick = %d,%v want 1,nil", n, err)
	}
	if req.RequestCount() != 1 {
		t.Errorf("expected one resubmit, got %d", req.RequestCount())
	}
	rec, _ := st.GetTitle(context.Background(), "movie:tmdb:1")
	if rec.State != provision.Requested {
		t.Errorf("wanted+success → %s, want requested", rec.State)
	}
	if !rec.Deadline.After(now) {
		t.Errorf("resubmit should set a future deadline, got %v", rec.Deadline)
	}
}

// A due `wanted` title whose resubmit fails stays wanted (reconciler retries).
func TestRetryWantedFailureStaysWanted(t *testing.T) {
	rc, st, req, _ := setup(t)
	req.RequestErr = errors.New("seerr down")
	put(t, st, "movie:tmdb:2", provision.Wanted, 2, now.Add(-time.Hour))

	if _, err := rc.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	rec, _ := st.GetTitle(context.Background(), "movie:tmdb:2")
	if rec.State != provision.Wanted {
		t.Errorf("failed resubmit → %s, want wanted", rec.State)
	}
	if !rec.Deadline.After(now) {
		t.Error("failed resubmit should push the deadline out for a later retry")
	}
}

// Missed-webhook backstop: a due `requested` title the library now confirms →
// available (even though no Import webhook arrived).
func TestMissedWebhookRecheckToAvailable(t *testing.T) {
	rc, st, _, ms := setup(t)
	ms.PresentTMDB = "16153" // library confirms this title
	put(t, st, "movie:tmdb:16153", provision.Requested, 16153, now.Add(-time.Hour))

	if _, err := rc.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	rec, _ := st.GetTitle(context.Background(), "movie:tmdb:16153")
	if rec.State != provision.Available {
		t.Errorf("missed-webhook re-check with library present → %s, want available", rec.State)
	}
	if rec.LibraryID == "" {
		t.Error("confirmed record should carry the library id")
	}
}

// Deadline give-up: a due in-flight title the library does NOT confirm →
// unavailable + best-effort Cancel.
func TestDeadlineGiveUp(t *testing.T) {
	rc, st, req, _ := setup(t)
	put(t, st, "movie:tmdb:404", provision.Downloading, 404, now.Add(-time.Hour)) // library absent for 404

	if _, err := rc.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	rec, _ := st.GetTitle(context.Background(), "movie:tmdb:404")
	if rec.State != provision.Unavailable {
		t.Errorf("past-deadline unconfirmed → %s, want unavailable", rec.State)
	}
	if req.CancelCount() != 1 {
		t.Errorf("give-up should best-effort Cancel, got %d cancels", req.CancelCount())
	}
}

// A not-yet-due title isn't claimed, so the reconciler leaves it alone.
func TestNotDueUntouched(t *testing.T) {
	rc, st, req, _ := setup(t)
	put(t, st, "movie:tmdb:5", provision.Requested, 5, now.Add(time.Hour)) // future deadline

	n, _ := rc.Tick(context.Background())
	if n != 0 {
		t.Errorf("claimed %d not-due titles, want 0", n)
	}
	if req.RequestCount() != 0 || req.CancelCount() != 0 {
		t.Error("not-due title should not be acted on")
	}
}

// Terminal titles are never claimed even if their (stale) deadline is past.
func TestTerminalNotClaimed(t *testing.T) {
	rc, st, _, _ := setup(t)
	put(t, st, "movie:tmdb:6", provision.Available, 6, now.Add(-time.Hour))
	put(t, st, "movie:tmdb:7", provision.Unavailable, 7, now.Add(-time.Hour))

	n, _ := rc.Tick(context.Background())
	if n != 0 {
		t.Errorf("claimed %d terminal titles, want 0", n)
	}
}
