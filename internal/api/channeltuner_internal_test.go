package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestInAppPlayableUsesEffectiveBackendAndLifecycle(t *testing.T) {
	checkpoint := BackendCheckpoint{Applied: schedule.PlayoutBackendInternal, PublishedInternal: true}

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
			if got := inAppPlayableAt(ch, checkpoint); got != tt.want {
				t.Fatalf("inAppPlayable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChannelOperationsReadDurableCheckpointOnce(t *testing.T) {
	st := testkit.SQLiteStore(t)
	if _, err := st.SaveChannel(context.Background(), store.Channel{Channel: schedule.Channel{
		ID: "inherited", Name: "Inherited", Number: 1, Status: schedule.StatusLive,
	}}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	s := &Server{
		log:   testkit.Logger(),
		store: st,
		backendCheckpoint: func(context.Context) (BackendCheckpoint, error) {
			calls++
			return BackendCheckpoint{
				Applied: schedule.PlayoutBackendTunarr, Prepared: schedule.PlayoutBackendInternal,
				PublishedInternal: true,
			}, nil
		},
	}

	listed, err := s.listChannels(context.Background(), &struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("channel list checkpoint reads = %d, want one", calls)
	}
	if listed.Body.Channels[0].InAppPlayable {
		t.Fatal("prepared target leaked into in-app list before cutover")
	}

	calls = 0
	transport, err := s.playoutChannels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(transport) != 1 || transport[0].ID != "inherited" {
		t.Fatalf("transport = %+v with %d checkpoint reads, want inherited/one", transport, calls)
	}
}

func TestChannelOperationsFailClosedWhenCheckpointReadFails(t *testing.T) {
	want := errors.New("checkpoint unavailable")
	s := &Server{backendCheckpoint: func(context.Context) (BackendCheckpoint, error) {
		return BackendCheckpoint{}, want
	}}
	if _, err := s.listChannels(context.Background(), &struct{}{}); statusFromError(err, 0) != 503 {
		t.Fatalf("list error = %v, want 503 checkpoint failure", err)
	}
}

func TestTunerReturnsUnavailableWhenCheckpointReadFails(t *testing.T) {
	want := errors.New("checkpoint unavailable")
	s := &Server{
		log: testkit.Logger(),
		liveConfig: func(key string) string {
			if key == "server.public_url" {
				return "http://loomarr.test"
			}
			return ""
		},
		backendCheckpoint: func(context.Context) (BackendCheckpoint, error) {
			return BackendCheckpoint{}, want
		},
	}
	recorder := httptest.NewRecorder()
	s.tunerHandler(recorder, httptest.NewRequest(http.MethodGet, "/playout/tuner.m3u", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("tuner checkpoint failure = %d, want 503", recorder.Code)
	}
}

func TestChannelDTOCarriesServerResolvedInAppPlayable(t *testing.T) {
	s := &Server{}
	got := s.channelDTOAt(store.Channel{Channel: schedule.Channel{Status: schedule.StatusLive}}, nil, nil,
		BackendCheckpoint{Applied: schedule.PlayoutBackendInternal})
	if !got.InAppPlayable {
		t.Fatal("ChannelDTO.InAppPlayable = false for a live internally played channel")
	}
}

func TestPreparedInternalPublicationOpensOnlyInheritedDeviceTransport(t *testing.T) {
	checkpoint := BackendCheckpoint{
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
	if inAppPlayableAt(inherited, checkpoint) {
		t.Fatal("prepared internal target leaked into ordinary in-app routing before cutover")
	}
	if !transportPlayableAt(inherited, checkpoint) {
		t.Fatal("prepared internal target did not publish inherited device transport")
	}
	if transportPlayableAt(channel(&schedule.PlayoutPolicy{Backend: schedule.PlayoutBackendTunarr}), checkpoint) {
		t.Fatal("explicit Tunarr pin was overridden by global internal transport publication")
	}
	if !transportPlayableAt(channel(&schedule.PlayoutPolicy{Backend: schedule.PlayoutBackendInternal}), checkpoint) {
		t.Fatal("explicit internal pin was hidden while global Applied remained Tunarr")
	}
}
