package app

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/mantonx/loomarr/internal/backendtransition"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/setup"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/testkit"
)

func TestBackendPublisherSnapshotsTargetURLsAcrossPhases(t *testing.T) {
	liveTV := testkit.NewLiveTV()
	connector := setup.NewLiveTVConnectorFixed(liveTV, setup.TunarrURLs{})
	resolves := 0
	publisher := &backendPublisher{
		connector: connector,
		urls: func(string) setup.TunarrURLs {
			resolves++
			if resolves == 1 {
				return setup.TunarrURLs{M3U: "http://a/playout/tuner.m3u", XMLTV: "http://a/playout/guide.xml"}
			}
			return setup.TunarrURLs{M3U: "http://b/playout/tuner.m3u", XMLTV: "http://b/playout/guide.xml"}
		},
	}
	ctx := context.Background()
	if _, err := publisher.Prepare(ctx, backendtransition.BackendInternal); err != nil {
		t.Fatal(err)
	}
	if err := publisher.Refresh(ctx, backendtransition.BackendInternal); err != nil {
		t.Fatal(err)
	}
	if err := publisher.RetireStale(ctx, backendtransition.BackendInternal); err != nil {
		t.Fatal(err)
	}
	if resolves != 1 {
		t.Fatalf("target URL resolver calls = %d, want one snapshot", resolves)
	}
	for _, call := range liveTV.Calls() {
		if strings.Contains(call, "http://b/") {
			t.Fatalf("later phase re-read changed settings: %v", liveTV.Calls())
		}
	}
}

func TestInheritedInternalCutoverStopsOnlyChannelsLeavingInternal(t *testing.T) {
	st := testkit.SQLiteStore(t)
	ctx := context.Background()
	seed := func(id string, policy *schedule.PlayoutPolicy) {
		t.Helper()
		_, err := st.SaveChannel(ctx, store.Channel{
			Channel: schedule.Channel{ID: id, Name: id, Number: len(id), Status: schedule.StatusLive},
			Policy:  schedule.ChannelPolicy{OperatorPolicy: schedule.OperatorPolicy{Playout: policy}},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	seed("inherited", nil)
	seed("pinned-internal", &schedule.PlayoutPolicy{Backend: schedule.PlayoutBackendInternal})
	seed("pinned-tunarr", &schedule.PlayoutPolicy{Backend: schedule.PlayoutBackendTunarr})

	play := &testkit.Playout{}
	cutover := inheritedInternalCutover{channels: st, playout: play}
	if err := cutover.BeforePublish(ctx, backendtransition.BackendInternal, backendtransition.BackendTunarr); err != nil {
		t.Fatal(err)
	}
	if got := play.StoppedChannels(); len(got) != 1 || got[0] != "inherited" {
		t.Fatalf("stopped channels = %v, want only inherited", got)
	}
}

func TestBuildHandlerInitializesLegacyFleetWithoutRunningNetworkTransition(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st := testkit.SQLiteStore(t)
	if _, err := st.SaveChannel(ctx, store.Channel{Channel: schedule.Channel{
		ID: "legacy", Name: "Legacy", Number: 7, Status: schedule.StatusLive,
	}}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PLAYOUT_BACKEND", schedule.PlayoutBackendInternal)
	t.Setenv("API_TOKEN", "transition-init-test")
	tunarr := testkit.NewTunarr()
	if _, err := BuildHandler(ctx, st, slog.New(slog.DiscardHandler), Overrides{Programmer: tunarr}); err != nil {
		t.Fatal(err)
	}
	cancel() // this test isolates synchronous initialization from owned scheduler retries.
	state, err := backendtransition.Load(context.Background(), st, backendtransition.BackendInternal)
	if err != nil {
		t.Fatal(err)
	}
	if state.Applied() != backendtransition.BackendTunarr || state.Prepared() != "" {
		t.Fatalf("initialized checkpoint = applied %q prepared %q", state.Applied(), state.Prepared())
	}
	if tunarr.Creates != 0 || tunarr.Pushes != 0 {
		t.Fatalf("Initialize performed remote projection: creates=%d pushes=%d", tunarr.Creates, tunarr.Pushes)
	}
}
