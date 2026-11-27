package app

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
)

type noDueChannels struct{}

func (noDueChannels) ClaimDueChannels(context.Context, time.Time, time.Duration, int) ([]channelstore.Channel, error) {
	return nil, nil
}

// Channel maintenance converges Tunarr's media source every tick, so a library token that
// changed without a successful auto-wire still reaches Tunarr, and its failure is the job's.
func TestChannelMaintenanceConvergesTheTunarrMediaSource(t *testing.T) {
	t.Parallel()
	runner := channels.NewRunner(nil, noDueChannels{}, time.Minute, 0, 0, nil, slog.New(slog.DiscardHandler))
	unreachable := errors.New("tunarr unreachable")
	calls := 0
	job := channelMaintenanceJob(runner, slog.New(slog.DiscardHandler), nil, func(context.Context) error {
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

// A run that uses a third of its one-minute budget says at Info where the time went, step by
// step, so a slow overnight run can be told apart from logs (#4428). A quick one stays at Debug.
func TestChannelMaintenanceTimingNamesEachStep(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		run  maintenanceRun
		want slog.Level
	}{
		"slow": {run: maintenanceRun{MediaSource: 2 * time.Second, Backend: 3 * time.Second, Sweep: channels.SweepReport{
			Claimed: 4, Reconciled: 3, Claim: 11 * time.Second, Reconcile: 9 * time.Second,
			Slowest: 6 * time.Second, SlowestChannel: "ch_slow",
		}}, want: slog.LevelInfo},
		"quick": {run: maintenanceRun{MediaSource: time.Second}, want: slog.LevelDebug},
	} {
		t.Run(name, func(t *testing.T) {
			var got []slog.Record
			tc.run.log(slog.New(recordHandler{records: &got}))
			if len(got) != 1 || got[0].Level != tc.want {
				t.Fatalf("logged %+v, want one %s record", got, tc.want)
			}
			attrs := map[string]string{}
			got[0].Attrs(func(a slog.Attr) bool { attrs[a.Key] = a.Value.String(); return true })
			if name == "slow" && (attrs["total_ms"] != "25000" || attrs["claim_ms"] != "11000" ||
				attrs["media_source_ms"] != "2000" || attrs["backend_ms"] != "3000" ||
				attrs["reconcile_ms"] != "9000" || attrs["slowest_channel"] != "ch_slow") {
				t.Fatalf("timing attrs = %v, want each step's share of 25 s", attrs)
			}
		})
	}
}

type recordHandler struct{ records *[]slog.Record }

func (recordHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h recordHandler) Handle(_ context.Context, r slog.Record) error {
	*h.records = append(*h.records, r)
	return nil
}
func (h recordHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h recordHandler) WithGroup(string) slog.Handler      { return h }
