package api

import (
	"context"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/store"
)

// reconcileChannelsForFillerChange is the latency path for §10 V56. The clip state is already
// durable when this runs, so a reconcile failure must not turn a successful catalog decision into
// an HTTP failure. The ordinary channel sweep remains the crash-safe retry.
func (s *Server) reconcileChannelsForFillerChange(ctx context.Context, snapshots ...filler.Clip) {
	if s.channels == nil || s.store == nil {
		return
	}
	if targeted, ok := s.channels.(interface {
		ReconcileFillerChange(context.Context, []filler.Clip) error
	}); ok {
		if err := targeted.ReconcileFillerChange(ctx, snapshots); err != nil && s.log != nil {
			s.log.Warn("filler catalog changed but affected-channel reconcile failed; sweep will retry", "err", err)
		}
		return
	}
	all, err := s.store.ListChannels(ctx)
	if err != nil {
		if s.log != nil {
			s.log.Warn("filler catalog changed but active channels could not be listed; sweep will retry", "err", err)
		}
		return
	}
	for _, ch := range all {
		if !ch.Status.Reconcilable() {
			continue
		}
		if err := s.channels.Reconcile(ctx, ch.ID); err != nil && s.log != nil {
			s.log.Warn("filler catalog changed but channel reconcile failed; sweep will retry",
				"channel", ch.ID, "err", err)
		}
	}
}

func storeClipsToFiller(clips []store.Clip) []filler.Clip {
	out := make([]filler.Clip, len(clips))
	for i := range clips {
		out[i] = clips[i].Clip
	}
	return out
}
