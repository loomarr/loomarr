package playout

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestInAppPlayableUsesEffectiveBackendAndLifecycle(t *testing.T) {
	checkpoint := httpcore.BackendCheckpoint{Applied: schedule.PlayoutBackendInternal, PublishedInternal: true}

	tests := []struct {
		name   string
		status schedule.ChannelStatus
		policy *schedule.PlayoutPolicy
		want   bool
	}{
		{name: "global internal live", status: schedule.StatusLive, want: true},
		{name: "building can warm while first reconcile finishes", status: schedule.StatusBuilding, want: true},
		{name: "channel override to Tunarr", status: schedule.StatusLive, policy: &schedule.PlayoutPolicy{Backend: "tunarr"}},
		{name: "paused", status: schedule.StatusPaused},
		{name: "detached", status: schedule.StatusDetached},
		{name: "empty", status: schedule.StatusEmpty},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := store.Channel{
				Channel: schedule.Channel{Status: tt.status},
				Policy:  schedule.ChannelPolicy{OperatorPolicy: schedule.OperatorPolicy{Playout: tt.policy}},
			}
			if got := httpcore.InAppPlayableAt(ch, checkpoint); got != tt.want {
				t.Fatalf("inAppPlayable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTransportChannelsReadDurableCheckpointOnce(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	if _, err := st.SaveChannel(context.Background(), store.Channel{Channel: schedule.Channel{
		ID: "inherited", Name: "Inherited", Number: 1, Status: schedule.StatusLive,
	}}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	s := &Handlers{
		log:   testkit.Logger(),
		store: st,
		backend: httpcore.Backend{Source: func(context.Context) (httpcore.BackendCheckpoint, error) {
			calls++
			return httpcore.BackendCheckpoint{
				Applied: schedule.PlayoutBackendTunarr, Prepared: schedule.PlayoutBackendInternal,
				PublishedInternal: true,
			}, nil
		}},
	}

	transport, err := s.playoutChannels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(transport) != 1 || transport[0].ID != "inherited" {
		t.Fatalf("transport = %+v with %d checkpoint reads, want inherited/one", transport, calls)
	}
}

func TestTunerReturnsUnavailableWhenCheckpointReadFails(t *testing.T) {
	want := errors.New("checkpoint unavailable")
	settings := mapSettings{"server.public_url": "http://loomarr.test"}
	s := &Handlers{
		log:          testkit.Logger(),
		liveSettings: settings,
		backend: httpcore.Backend{Settings: settings, Source: func(context.Context) (httpcore.BackendCheckpoint, error) {
			return httpcore.BackendCheckpoint{}, want
		}},
	}
	recorder := httptest.NewRecorder()
	s.tunerHandler(recorder, httptest.NewRequest(http.MethodGet, "/playout/tuner.m3u", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("tuner checkpoint failure = %d, want 503", recorder.Code)
	}
}

func TestPreparedInternalPublicationOpensOnlyInheritedDeviceTransport(t *testing.T) {
	checkpoint := httpcore.BackendCheckpoint{
		Applied: schedule.PlayoutBackendTunarr, Prepared: schedule.PlayoutBackendInternal,
		PublishedInternal: true,
	}
	channel := func(policy *schedule.PlayoutPolicy) store.Channel {
		return store.Channel{
			Channel: schedule.Channel{Status: schedule.StatusLive},
			Policy:  schedule.ChannelPolicy{OperatorPolicy: schedule.OperatorPolicy{Playout: policy}},
		}
	}

	inherited := channel(nil)
	if httpcore.InAppPlayableAt(inherited, checkpoint) {
		t.Fatal("prepared internal target leaked into ordinary in-app routing before cutover")
	}
	if !httpcore.TransportPlayableAt(inherited, checkpoint) {
		t.Fatal("prepared internal target did not publish inherited device transport")
	}
	if httpcore.TransportPlayableAt(channel(&schedule.PlayoutPolicy{Backend: schedule.PlayoutBackendTunarr}), checkpoint) {
		t.Fatal("explicit Tunarr pin was overridden by global internal transport publication")
	}
	if !httpcore.TransportPlayableAt(channel(&schedule.PlayoutPolicy{Backend: schedule.PlayoutBackendInternal}), checkpoint) {
		t.Fatal("explicit internal pin was hidden while global Applied remained Tunarr")
	}
}
