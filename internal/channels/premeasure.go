package channels

import (
	"context"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/playout"
)

// PremeasureHorizon is how far ahead reconcile asks for measurement (#1518): the items scheduled
// to air in the next two days, never the whole library.
const PremeasureHorizon = 48 * time.Hour

// premeasureAhead hands each library item the committed cycle airs between now and
// PremeasureHorizon to the measure hook, once per pass. The worker skips revisions it already
// measured, so a repeat pass costs a lookup per item and no media work.
func (e *Engine) premeasureAhead(ctx context.Context, ch store.Channel) {
	if e.premeasure == nil || ch.PlayoutAnchor.IsZero() || len(ch.Desired) == 0 {
		return
	}
	from := e.now()
	if ch.PlayoutAnchor.After(from) {
		from = ch.PlayoutAnchor // a carried-over programme still airs the previous cycle
	}
	seen := map[string]bool{}
	var ids []string
	for _, b := range playout.BroadcastsBetween(ch.Desired, ch.PlayoutAnchor, from, e.now().Add(PremeasureHorizon)) {
		if b.Kind != schedule.SlotProgram || b.LibraryItemID == "" || seen[b.LibraryItemID] {
			continue
		}
		seen[b.LibraryItemID] = true
		ids = append(ids, b.LibraryItemID)
	}
	if len(ids) > 0 {
		e.premeasure(ctx, ids)
	}
}
