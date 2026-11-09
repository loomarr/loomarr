package app

import (
	"context"
	"log/slog"

	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// fillerChannelWake is the shared post-commit latency path for every non-HTTP eligibility change.
// It deliberately depends on only Reconcile: pipeline code does not need the API's wider channel
// management surface merely to announce that pod eligibility changed.
type fillerChannelWake struct {
	st       store.Store
	channels interface {
		Reconcile(context.Context, string) error
	}
	log *slog.Logger
}

func (w *fillerChannelWake) Run(ctx context.Context, snapshots []clips.Clip) {
	if w == nil || w.st == nil || w.channels == nil {
		return
	}
	if targeted, ok := w.channels.(interface {
		ReconcileFillerChange(context.Context, []clips.Clip) error
	}); ok {
		if err := targeted.ReconcileFillerChange(ctx, snapshots); err != nil && w.log != nil {
			w.log.Warn("filler catalog changed but affected-channel reconcile failed; sweep will retry", "err", err)
		}
		return
	}
	all, err := w.st.ListChannels(ctx)
	if err != nil {
		if w.log != nil {
			w.log.Warn("filler catalog changed but active channels could not be listed; sweep will retry", "err", err)
		}
		return
	}
	for _, ch := range all {
		if !ch.Status.Reconcilable() {
			continue
		}
		if err := w.channels.Reconcile(ctx, ch.ID); err != nil && w.log != nil {
			w.log.Warn("filler catalog changed but channel reconcile failed; sweep will retry",
				"channel", ch.ID, "err", err)
		}
	}
}
