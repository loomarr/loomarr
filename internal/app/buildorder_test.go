package app

import (
	"context"
	"log/slog"
	"testing"

	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

// No generation worker may start before composition finishes. The broadcast-codec backfill reads
// the channel engine (CyclePreview), which the filler subsystem configures later (Engine.WithPods),
// and the backend-transition checkpoint, which a later builder initializes. Run under -race: on a
// restart (the checkpoint already stored) a worker started during Build races that write.
func TestBuild_WorkersStartOnlyAfterTheEngineIsConfigured(t *testing.T) {
	t.Setenv("API_TOKEN", "build-order-test-token")
	st := testkit.MigratedSQLiteStore(t)
	ctx := context.Background()
	for key, value := range map[string]string{"playout.backend": "internal", "playout.encoder": "libx264"} {
		if err := st.SetSetting(ctx, key, value); err != nil {
			t.Fatal(err)
		}
	}
	channel := store.Channel{Channel: schedule.Channel{
		ID: "ch1", Name: "One", Number: 1, Strategy: schedule.Sequential, Status: schedule.StatusLive,
	}}
	channel.Policy.Playout = &schedule.PlayoutPolicy{Backend: schedule.PlayoutBackendInternal}
	if _, err := st.SaveChannel(ctx, channel); err != nil {
		t.Fatal(err)
	}
	for range 2 { // the first generation stores the checkpoint; the second is the restart
		application, err := Build(ctx, st, slog.New(slog.DiscardHandler), Overrides{})
		if err != nil {
			t.Fatal(err)
		}
		if err := application.Shutdown(context.Background()); err != nil { // waits for the backfill
			t.Fatal(err)
		}
	}
}
