package app

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/testkit"
)

// A media-server-only install is the default §9.1 path: Loomarr derives and plays the
// channel itself, so an empty Tunarr connection must not leave the durable row in `building`.
// This exercises the real composition root and HTTP gate together. The channels package's nil-
// Programmer test is the hard no-call proof; the shared Tunarr double here additionally proves
// the real app wiring made no remote writes while it persisted the local desired state.
func TestBuildHandler_InternalChannelReconcilesWithoutTunarr(t *testing.T) {
	t.Setenv("API_TOKEN", "internal-reconcile-token")
	// Empty means no environment override, so the registry's actual default (internal) wins.
	// This guards the shortest supported install path rather than a test-only explicit choice.
	t.Setenv("PLAYOUT_BACKEND", "")
	t.Setenv("TUNARR_URL", "")
	t.Setenv("CHANNEL_RECONCILE_EVERY", "9999h")

	mediaServer := testkit.NewMediaServer(t)
	t.Cleanup(mediaServer.Close)
	t.Setenv("LIBRARY_FLAVOR", "emby")
	t.Setenv("LIBRARY_URL", mediaServer.URL)
	t.Setenv("LIBRARY_TOKEN", mediaServer.AdminToken)

	ctx, cancel := context.WithCancel(context.Background())
	st, err := store.Open(ctx, "sqlite://"+t.TempDir()+"/internal-reconcile.db", true)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	// BuildHandler owns background work under ctx. Stop it before closing the store or its
	// shutdown waits on workers that can no longer finish their final database operation.
	t.Cleanup(func() {
		cancel()
		_ = st.Close()
	})

	tunarr := testkit.NewTunarr()
	handler, err := BuildHandler(ctx, st, slog.New(slog.DiscardHandler), Overrides{Programmer: tunarr})
	if err != nil {
		t.Fatalf("BuildHandler: %v", err)
	}

	// Seed after BuildHandler so the one-time codec backfill observes the intentionally empty
	// store. The channel then reaches reconciliation only through the HTTP request below.
	title := provision.Title{MediaType: provision.Movie, TMDBID: 603, Name: "The Matrix", Year: 1999}
	key, err := title.Key()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertTitle(ctx, provision.Record{
		Key: key, Title: title, State: provision.Available, LibraryID: "lib-matrix",
	}); err != nil {
		t.Fatal(err)
	}
	ch := store.Channel{Lineup: []schedule.LineupEntry{{
		Key: key, Title: title.Name, Year: title.Year, DurationMs: int64((136 * time.Minute) / time.Millisecond),
	}}}
	ch.ID, ch.Name, ch.Number = "internal-only", "Internal only", 42
	ch.Strategy, ch.Status = schedule.Sequential, schedule.StatusBuilding
	if _, err := st.SaveChannel(ctx, ch); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		srv.URL+"/v1/channels/internal-only/reconcile", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer internal-reconcile-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST reconcile with no Tunarr = %d, want 200", resp.StatusCode)
	}

	got, err := st.GetChannel(ctx, ch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != schedule.StatusLive {
		t.Errorf("status = %q, want live", got.Status)
	}
	if len(got.Desired) != 1 || got.Desired[0].Kind != schedule.SlotProgram ||
		got.Desired[0].LibraryItemID != "lib-matrix" {
		t.Errorf("desired = %+v, want one persisted Matrix program", got.Desired)
	}
	if got.TunarrID != "" {
		t.Errorf("TunarrID = %q, want empty on an internal-only channel", got.TunarrID)
	}
	if tunarr.Creates != 0 || tunarr.Updates != 0 || tunarr.Pushes != 0 ||
		tunarr.Deletes != 0 || tunarr.FillerWrites != 0 {
		t.Errorf("internal reconcile wrote to Tunarr: creates=%d updates=%d pushes=%d deletes=%d filler=%d",
			tunarr.Creates, tunarr.Updates, tunarr.Pushes, tunarr.Deletes, tunarr.FillerWrites)
	}
}
