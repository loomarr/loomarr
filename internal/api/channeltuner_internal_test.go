package api

import (
	"testing"

	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
)

func TestInAppPlayableUsesEffectiveBackendAndLifecycle(t *testing.T) {
	s := &Server{liveConfig: func(key string) string {
		if key == "playout.backend" {
			return schedule.PlayoutBackendInternal
		}
		return ""
	}}

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
			if got := s.inAppPlayable(ch); got != tt.want {
				t.Fatalf("inAppPlayable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChannelDTOCarriesServerResolvedInAppPlayable(t *testing.T) {
	s := &Server{liveConfig: func(string) string { return schedule.PlayoutBackendInternal }}
	got := s.channelDTO(store.Channel{Channel: schedule.Channel{Status: schedule.StatusLive}}, nil, nil)
	if !got.InAppPlayable {
		t.Fatal("ChannelDTO.InAppPlayable = false for a live internally played channel")
	}
}

func TestPreparedInternalPublicationOpensOnlyInheritedDeviceTransport(t *testing.T) {
	s := &Server{
		appliedBackend:    func() string { return schedule.PlayoutBackendTunarr },
		publishedInternal: func() bool { return true },
	}
	channel := func(policy *schedule.PlayoutPolicy) store.Channel {
		return store.Channel{
			Channel: schedule.Channel{Status: schedule.StatusLive},
			Policy:  schedule.ChannelPolicy{OperatorPolicy: schedule.OperatorPolicy{Playout: policy}},
		}
	}

	inherited := channel(nil)
	if s.inAppPlayable(inherited) {
		t.Fatal("prepared internal target leaked into ordinary in-app routing before cutover")
	}
	if !s.transportPlayable(inherited) {
		t.Fatal("prepared internal target did not publish inherited device transport")
	}
	if s.transportPlayable(channel(&schedule.PlayoutPolicy{Backend: schedule.PlayoutBackendTunarr})) {
		t.Fatal("explicit Tunarr pin was overridden by global internal transport publication")
	}
	if !s.transportPlayable(channel(&schedule.PlayoutPolicy{Backend: schedule.PlayoutBackendInternal})) {
		t.Fatal("explicit internal pin was hidden while global Applied remained Tunarr")
	}
}
