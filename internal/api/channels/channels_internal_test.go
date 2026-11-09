package channels

import (
	"context"
	"errors"
	"testing"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestChannelListReadsDurableCheckpointOnce(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	if _, err := st.SaveChannel(context.Background(), channelstore.Channel{Channel: schedule.Channel{
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
}

func TestChannelListFailsClosedWhenCheckpointReadFails(t *testing.T) {
	want := errors.New("checkpoint unavailable")
	s := &Handlers{backend: httpcore.Backend{Source: func(context.Context) (httpcore.BackendCheckpoint, error) {
		return httpcore.BackendCheckpoint{}, want
	}}}
	if _, err := s.listChannels(context.Background(), &struct{}{}); httpcore.StatusFromError(err, 0) != 503 {
		t.Fatalf("list error = %v, want 503 checkpoint failure", err)
	}
}

func TestChannelDTOCarriesServerResolvedInAppPlayable(t *testing.T) {
	s := &Handlers{}
	got := s.channelDTOAt(channelstore.Channel{Channel: schedule.Channel{Status: schedule.StatusLive}}, nil, nil, nil,
		httpcore.BackendCheckpoint{Applied: schedule.PlayoutBackendInternal})
	if !got.InAppPlayable {
		t.Fatal("ChannelDTO.InAppPlayable = false for a live internally played channel")
	}
}

// #2767: the channel's decade is on the wire, the same one the on-screen logo's palette uses, and
// absent for a channel with no single-decade era.
func TestChannelDTOCarriesTheEraDecade(t *testing.T) {
	s := &Handlers{}
	eighties := channelstore.Channel{Policy: schedule.ChannelPolicy{
		OperatorPolicy: schedule.OperatorPolicy{},
		Scope:          schedule.ScopePolicy{Dates: schedule.EraDates(schedule.Range{From: 1980, To: 1989})},
	}}
	if got := s.channelDTOAt(eighties, nil, nil, nil, httpcore.BackendCheckpoint{}).Decade; got != 1980 {
		t.Fatalf("Decade = %d for a 1980s channel, want 1980", got)
	}
	if got := s.channelDTOAt(channelstore.Channel{}, nil, nil, nil, httpcore.BackendCheckpoint{}).Decade; got != 0 {
		t.Fatalf("Decade = %d for a channel with no era, want 0 (absent)", got)
	}
}
