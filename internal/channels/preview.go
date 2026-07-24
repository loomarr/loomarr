package channels

import (
	"context"
	"fmt"
	"time"

	"github.com/mantonx/loomarr/internal/schedule"
)

// CyclePreview computes the channel's desired cycle at a chosen wall-clock `at` for the
// time-travel preview (§8.1). It is PURE and read-only: it loads the channel, rebuilds the
// EXACT schedule.ComputeDesiredAt inputs the reconciler uses (settings-driven window + break
// density, live availability, the same pending policy), and evaluates the pure lineup builder
// at `at` — but it heals nothing, persists nothing, and calls no Tunarr. One code path with
// reconcile, so the preview can never disagree with what actually ships (mirrors the pod
// preview, §10). A zero `at` means "now" (the engine's injected clock).
//
// Returns the resolved cycle's slots (program / pending / break, in play order), the curation
// rule active at `at` (or the base-policy attribution when nothing matched — the same
// pickRule ComputeDesiredAt makes), and the resolved rolling-window horizon at `at`
// (0 = the whole run) which explains why the preview shows ~this much runtime, not all ~800
// episodes. A detached/paused channel is still previewable — the preview is a what-if, not a
// management action.
//
// Break interleaving mirrors reconcile: breaks appear only when the channel actually has a
// filler pool (else there'd be nothing to fill them), so the preview reflects the real guide.
// We deliberately do NOT heal ratings/franchises here — those mutate the lineup and belong to
// reconcile; the preview reads the channel exactly as the last reconcile left it.
func (e *Engine) CyclePreview(ctx context.Context, channelID string, at time.Time) (
	resolvedAt time.Time, slots []schedule.Slot, active schedule.ActiveRuleAttribution, window time.Duration, err error,
) {
	ch, err := e.store.GetChannel(ctx, channelID)
	if err != nil {
		return time.Time{}, nil, schedule.ActiveRuleAttribution{}, 0, fmt.Errorf("load channel %s: %w", channelID, err)
	}
	if at.IsZero() {
		at = e.now()
	}

	// Mirror reconcile's chDomain assembly (reconcile.go step 2): break density only when a
	// filler pool exists, and the settings-driven rolling-window horizon.
	hasFillerPool := false
	if e.pods != nil {
		if ids, ok := e.pods.BuildFillerList(ctx, ch.ID, PodSeed(ch.ID), SelectionForChannel(ch)); ok && len(ids) > 0 {
			hasFillerPool = true
		}
	}
	chDomain := ch.Channel
	chDomain.BreaksPerHour = 0
	if hasFillerPool {
		chDomain.BreaksPerHour = e.breaksPerHour
	}
	chDomain.DefaultWindow = e.defaultWindow

	desired := schedule.ComputeDesiredAt(chDomain, ch.Lineup, e.avail, e.policy, ch.Policy, at)
	active = schedule.ActiveRuleAt(ch.Policy.Rules, at)
	window = schedule.ResolveWindow(chDomain, ch.Policy, at)
	return at, desired.Slots, active, window, nil
}
