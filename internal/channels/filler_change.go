package channels

import (
	"context"
	"errors"
	"fmt"

	"github.com/loomarr/loomarr/internal/filler"
)

// fillerFitter is the eligibility half of the production PodAdapter. Keeping it narrower than
// PodFiller lets older backends retain their existing assembly interface while this latency path
// reuses the exact predicates behind coverage, preview and playout (§10 V57).
type fillerFitter interface {
	FitForChannel(channelID string, sel filler.Selection, clip filler.Clip) filler.Fit
}

// ReconcileFillerChange immediately converges only channels that could use at least one before or
// after snapshot. It is best-effort and idempotent; callers have already committed the catalog
// mutation, and the ordinary sweep remains the crash-safe retry.
func (e *Engine) ReconcileFillerChange(ctx context.Context, snapshots []filler.Clip) error {
	all, err := e.store.ListChannels(ctx)
	if err != nil {
		return fmt.Errorf("list channels for filler eligibility change: %w", err)
	}
	fitter, targeted := e.pods.(fillerFitter)
	var errs []error
	for _, ch := range all {
		if !ch.Status.Reconcilable() {
			continue
		}
		affected := !targeted || len(snapshots) == 0
		if targeted {
			sel := SelectionForChannel(ch)
			for _, clip := range snapshots {
				if fitter.FitForChannel(ch.ID, sel, clip).Reason == "" {
					affected = true
					break
				}
			}
		}
		if !affected {
			continue
		}
		if err := e.Reconcile(ctx, ch.ID); err != nil {
			errs = append(errs, fmt.Errorf("reconcile channel %s: %w", ch.ID, err))
			if ctx.Err() != nil {
				break
			}
		}
	}
	return errors.Join(errs...)
}
