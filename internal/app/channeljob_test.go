package app

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/provisionbackstop"
	"github.com/loomarr/loomarr/internal/store"
)

type noDueChannels struct{}

func (noDueChannels) ClaimDueChannels(context.Context, time.Time, time.Duration, int) ([]store.Channel, error) {
	return nil, nil
}

// Channel maintenance converges Tunarr's media source every tick, so a library token that
// changed without a successful auto-wire still reaches Tunarr, and its failure is the job's.
func TestChannelMaintenanceConvergesTheTunarrMediaSource(t *testing.T) {
	t.Parallel()
	runner := channels.NewRunner(nil, noDueChannels{}, time.Minute, 0, 0, nil, slog.New(slog.DiscardHandler))
	unreachable := errors.New("tunarr unreachable")
	calls := 0
	job := channelMaintenanceJob(runner, &provisionbackstop.EpisodeRefresh{}, nil, func(context.Context) error {
		calls++
		return unreachable
	})

	err := job.Run(context.Background())
	if calls != 1 {
		t.Fatalf("media-source convergence ran %d times in one maintenance tick, want 1", calls)
	}
	if !errors.Is(err, unreachable) {
		t.Fatalf("maintenance error = %v, want the media-source failure reported", err)
	}
}
