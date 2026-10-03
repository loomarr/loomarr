package app

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/backendtransition"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/setup"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/tunarr/tunarrtest"
)

func TestCurrentBackendTransitionMutatesBeforeResolvingDesired(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	fleetErr := errors.New("fleet remains pending")
	probe := testkit.NewBackendTransitionPhaseProbe()
	probe.FailFleetOnce(fleetErr)
	controller := backendtransition.NewController(st, probe, nil, nil)
	if err := controller.Initialize(context.Background(), func(context.Context) (string, error) {
		return backendtransition.BackendTunarr, nil
	}); err != nil {
		t.Fatal(err)
	}
	desired := backendtransition.BackendTunarr
	refreshed := false
	transition := currentBackendTransition{
		controller: controller,
		refresh: func(context.Context) error {
			refreshed = true
			return nil
		},
		desired: func(context.Context) (string, error) {
			return desired, nil
		},
	}
	mutated := false
	err := transition.ApplyMutation(context.Background(), func(context.Context) bool {
		if !refreshed {
			t.Fatal("mutation ran before settings provenance refresh")
		}
		mutated = true
		desired = backendtransition.BackendInternal
		return true
	})
	if !mutated || !errors.Is(err, fleetErr) {
		t.Fatalf("ApplyMutation = mutated %v, err %v; want mutation followed by fleet error", mutated, err)
	}
	if got := controller.Runtime().Snapshot().Prepared; got != backendtransition.BackendInternal {
		t.Fatalf("prepared backend = %q, want mutation's desired internal", got)
	}
}

func TestBackendPublisherSnapshotsTargetURLsAcrossPhases(t *testing.T) {
	primary := testkit.NewLiveTV()
	rotated := testkit.NewLiveTV()
	librarySnapshots := 0
	connector := setup.NewLiveTVConnector(func() library.LiveTV {
		librarySnapshots++
		if librarySnapshots == 1 {
			return primary
		}
		return rotated
	}, setup.LiveTVURLs{})
	resolves := 0
	publisher := &backendPublisher{
		connector: connector,
		urls: func(context.Context, string) (setup.LiveTVURLs, error) {
			resolves++
			if resolves == 1 {
				return setup.LiveTVURLs{M3U: "http://a/playout/tuner.m3u", XMLTV: "http://a/playout/guide.xml"}, nil
			}
			return setup.LiveTVURLs{M3U: "http://b/playout/tuner.m3u", XMLTV: "http://b/playout/guide.xml"}, nil
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
	if librarySnapshots != 1 {
		t.Fatalf("library snapshots = %d, want one across prepare/refresh/retire", librarySnapshots)
	}
	for _, call := range primary.Calls() {
		if strings.Contains(call, "http://b/") {
			t.Fatalf("later phase re-read changed settings: %v", primary.Calls())
		}
	}
	if calls := rotated.Calls(); len(calls) != 0 {
		t.Fatalf("rotated library received in-flight workflow calls: %v", calls)
	}
}

func TestTransportTunerRescannerUsesPublishedTarget(t *testing.T) {
	liveTV := testkit.NewLiveTV()
	connector := setup.NewLiveTVConnectorFixed(liveTV,
		setup.TunarrURLsFrom("http://applied-tunarr:8000"))
	target := setup.InternalPlayoutURLs("http://prepared-loomarr:8080", "device-token")
	rescanner := transportTunerRescanner{
		c: connector,
		urls: func(context.Context) (setup.LiveTVURLs, error) {
			return target, nil
		},
	}

	if err := rescanner.RescanTuner(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := "rescan-tuner:" + target.M3U
	if calls := liveTV.Calls(); len(calls) != 1 || calls[0] != want {
		t.Fatalf("calls = %v, want [%s]", calls, want)
	}
}

func TestInheritedInternalCutoverStopsOnlyChannelsLeavingInternal(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
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

func TestBuildInitializesMissingCheckpointFromDesiredWithoutRunningNetworkTransition(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st := testkit.MigratedSQLiteStore(t)
	if _, err := st.SaveChannel(ctx, store.Channel{Channel: schedule.Channel{
		ID: "legacy", Name: "Legacy", Number: 7, Status: schedule.StatusLive,
	}}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PLAYOUT_BACKEND", schedule.PlayoutBackendInternal)
	t.Setenv("API_TOKEN", "transition-init-test")
	tunarr := tunarrtest.NewTunarr()
	application, err := Build(ctx, st, slog.New(slog.DiscardHandler), Overrides{Programmer: tunarr})
	if err != nil {
		t.Fatal(err)
	}
	cancel() // this test isolates synchronous initialization from owned scheduler retries.
	if err := application.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	state, err := backendtransition.Load(context.Background(), st, backendtransition.BackendInternal)
	if err != nil {
		t.Fatal(err)
	}
	if state.Applied() != backendtransition.BackendInternal || state.Prepared() != "" {
		t.Fatalf("initialized checkpoint = applied %q prepared %q", state.Applied(), state.Prepared())
	}
	if tunarr.Creates != 0 || tunarr.Pushes != 0 {
		t.Fatalf("Initialize performed remote projection: creates=%d pushes=%d", tunarr.Creates, tunarr.Pushes)
	}
}

func TestTunerPublishingDisabledReadsAgentEnvFlag(t *testing.T) {
	t.Setenv("LOOMARR_AGENT_DISABLE_LIVETV_TUNER", "")
	if tunerPublishingDisabled() {
		t.Fatal("unset env flag must default to false (production behaviour unchanged)")
	}
	t.Setenv("LOOMARR_AGENT_DISABLE_LIVETV_TUNER", "1")
	if !tunerPublishingDisabled() {
		t.Fatal("set env flag must disable tuner publishing")
	}
}

// #1555: an agent dev lane's library.* settings can drift onto the household media server, and
// completing them must never add or retire its Live TV tuner/listing. Both the settings-save
// transition (MutateAndApplyCurrent) and the channel-maintenance repair tick (ApplyCurrent) route
// through the SAME steady-state repair path (Controller.applyLocked -> repairPublished -> the real
// backendPublisher -> setup.LiveTVConnector), so exercising both against one disabled connector
// proves every current caller of AddTuner/RemoveTuner/AddListingProvider/RemoveListingProvider.
func TestLiveTVTunerPublishingDisabledSkipsAddAndRetireOnBothPaths(t *testing.T) {
	run := func(t *testing.T, disabled bool) *testkit.LiveTV {
		t.Helper()
		st := testkit.MigratedSQLiteStore(t)
		lib := testkit.NewLiveTV()
		// A stale Loomarr-owned tuner at a DIFFERENT url: if retirement ran, it would be removed.
		lib.SeedTuner("http://stale-tuner.invalid/old.m3u", "loomarr")
		urls := setup.LiveTVURLs{M3U: "http://lane.invalid/tuner.m3u", XMLTV: "http://lane.invalid/guide.xml"}
		connector := setup.NewLiveTVConnectorFixed(lib, urls).
			WithTunerPublishingDisabled(disabled, slog.New(slog.DiscardHandler))
		publisher := &backendPublisher{
			connector: connector,
			urls:      func(context.Context, string) (setup.LiveTVURLs, error) { return urls, nil },
		}
		fleet := testkit.NewBackendTransitionPhaseProbe()
		controller := backendtransition.NewController(st, fleet, publisher, nil)
		ctx := context.Background()
		desired := func(context.Context) (string, error) { return backendtransition.BackendInternal, nil }
		if err := controller.Initialize(ctx, desired); err != nil {
			t.Fatal(err)
		}

		// Path 1: the settings-save transition (internal/api/settings.go mutateLiveTVSettings).
		refresh := func(context.Context) error { return nil }
		mutate := func(context.Context) bool { return true }
		if err := controller.MutateAndApplyCurrent(ctx, refresh, mutate, desired); err != nil {
			t.Fatalf("settings-save transition: %v", err)
		}

		// Path 2: the channel-maintenance repair publisher (internal/app/buildchannels.go
		// channelMaintenanceJob's backendController.ApplyCurrent on every scheduled tick).
		if err := controller.ApplyCurrent(ctx, desired); err != nil {
			t.Fatalf("channel-maintenance repair tick: %v", err)
		}
		return lib
	}

	t.Run("disabled: neither AddTuner nor retire runs", func(t *testing.T) {
		lib := run(t, true)
		for _, call := range lib.Calls() {
			if strings.HasPrefix(call, "add-tuner") || strings.HasPrefix(call, "remove-tuner") ||
				strings.HasPrefix(call, "add-listing") || strings.HasPrefix(call, "remove-listing") {
				t.Fatalf("tuner publishing disabled, but the library received a mutating call %q: all calls %v", call, lib.Calls())
			}
		}
		if !lib.HasTuner("http://stale-tuner.invalid/old.m3u") {
			// Not a failure by itself (Calls already caught it), but a clearer signal.
			t.Fatal("stale tuner was retired while publishing was disabled")
		}
	})

	t.Run("unset: production behaviour is unchanged", func(t *testing.T) {
		lib := run(t, false)
		var sawAdd, sawRemove bool
		for _, call := range lib.Calls() {
			if strings.HasPrefix(call, "add-tuner") {
				sawAdd = true
			}
			if strings.HasPrefix(call, "remove-tuner") {
				sawRemove = true
			}
		}
		if !sawAdd || !sawRemove {
			t.Fatalf("expected the default (switch unset) path to add and retire as before: calls %v", lib.Calls())
		}
	})
}

// #1407: a busy media server must be logged ONCE with its cause class, and must not stop the
// publisher's other phases from running on later ticks.
func TestBackendPublisherRefreshLogsTransientCauseOnce(t *testing.T) {
	lib := testkit.NewLiveTV()
	urls := setup.LiveTVURLs{M3U: "http://a/playout/tuner.m3u", XMLTV: "http://a/playout/guide.xml"}
	lib.RescanErr = setup.NewTransientError("server-error", 3, errors.New("POST /LiveTv/TunerHosts: status 500: ServiceUnavailable"))
	var logs strings.Builder
	publisher := &backendPublisher{
		connector: setup.NewLiveTVConnectorFixed(lib, urls),
		urls:      func(context.Context, string) (setup.LiveTVURLs, error) { return urls, nil },
		log:       slog.New(slog.NewTextHandler(&logs, nil)),
	}
	ctx := context.Background()
	if _, err := publisher.Prepare(ctx, backendtransition.BackendInternal); err != nil {
		t.Fatal(err)
	}
	if err := publisher.Refresh(ctx, backendtransition.BackendInternal); err == nil {
		t.Fatal("Refresh = nil, want the transient error still returned for the controller to defer")
	}
	if got := strings.Count(logs.String(), "live tv refresh deferred"); got != 1 {
		t.Fatalf("deferral logged %d times, want once:\n%s", got, logs.String())
	}
	if !strings.Contains(logs.String(), "cause_class=server-error") {
		t.Errorf("log lacks the cause class:\n%s", logs.String())
	}
}
