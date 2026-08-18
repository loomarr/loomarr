package app

import (
	"context"
	"testing"

	"github.com/mantonx/loomarr/internal/backendtransition"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/testkit"
)

func TestDurableInternalTransportPlayableRejectsEveryOffAirShape(t *testing.T) {
	t.Parallel()
	st := testkit.MigratedSQLiteStore(t)
	checkpoint := testkit.Snapshotter[backendtransition.Snapshot]{Result: backendtransition.Snapshot{
		Applied: schedule.PlayoutBackendInternal, PublishedInternal: true,
	}}
	tests := []struct {
		id      string
		status  schedule.ChannelStatus
		backend string
		want    bool
	}{
		{id: "live-internal", status: schedule.StatusLive, backend: schedule.PlayoutBackendInternal, want: true},
		{id: "paused", status: schedule.StatusPaused, backend: schedule.PlayoutBackendInternal},
		{id: "detached", status: schedule.StatusDetached, backend: schedule.PlayoutBackendInternal},
		{id: "empty", status: schedule.StatusEmpty, backend: schedule.PlayoutBackendInternal},
		{id: "tunarr", status: schedule.StatusLive, backend: schedule.PlayoutBackendTunarr},
	}
	for i, tc := range tests {
		channel := store.Channel{Channel: schedule.Channel{
			ID: tc.id, Name: tc.id, Number: i + 1, Strategy: schedule.Sequential, Status: tc.status,
		}}
		channel.Policy.Playout = &schedule.PlayoutPolicy{Backend: tc.backend}
		if _, err := st.SaveChannel(context.Background(), channel); err != nil {
			t.Fatal(err)
		}
		got, err := durableInternalTransportPlayable(context.Background(), st, checkpoint, tc.id)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("%s playable = %v, want %v", tc.id, got, tc.want)
		}
	}
}
