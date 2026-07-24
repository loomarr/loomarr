package binder

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/suggest"
)

// isAutoCurate reports whether a proposal was approved by the scheduled re-curation loop
// (§8.2), as opposed to a human or a per-user auto-approve grant. The audit field is the
// discriminator, so the binder applies additive (non-destructive) lineup semantics only for
// the unattended path — every human-in-the-loop approval keeps full-replace (a person decided).
func isAutoCurate(p store.Proposal) bool {
	return p.ApprovedBy == suggest.AutoCuratedBy
}

// mustExcludeKeys is the set of lower-cased title terms the intent asked to exclude
// (Intent.MustExclude). Used as ONE of the two "clearly off-intent" drop signals — a matching
// existing title may be dropped even though it's still available. Empty when the proposal has
// no intent or no exclusions.
func mustExcludeKeys(p store.Proposal) map[string]struct{} {
	var body suggest.Proposal
	if err := json.Unmarshal([]byte(p.ProposalJSON), &body); err != nil {
		return nil
	}
	out := make(map[string]struct{}, len(body.Intent.MustExclude))
	for _, t := range body.Intent.MustExclude {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
			out[t] = struct{}{}
		}
	}
	return out
}

// dropPredicate builds the LineupAdditive drop test the auto-curate rebind hands to
// schedule.ApplyLineup (§8.2): an existing title the refresh did NOT re-pick is dropped ONLY
// when it is clearly off-intent — gone from the library (unavailable) or named by the intent's
// MustExclude. A still-available title the stochastic LLM merely didn't re-pick is KEPT (no
// churn). The store read (droppable) lives here so ApplyLineup's union logic stays pure; each
// drop is logged. The union/order (existing-kept first, new appended) is ApplyLineup's job.
func (b *Binder) dropPredicate(ctx context.Context, mustExclude map[string]struct{}) func(schedule.LineupEntry) bool {
	return func(e schedule.LineupEntry) bool {
		if !b.droppable(ctx, e, mustExclude) {
			return false
		}
		b.log.Info("auto-curate dropped a clearly-off-intent title", "title", e.Title, "key", e.Key)
		return true
	}
}

// droppable reports whether an existing lineup entry the refresh did NOT re-pick is clearly
// off-intent enough to remove (§8.2 conservative pruning). TWO signals, both conservative:
//   - the intent's MustExclude names it (a deliberate exclusion), or
//   - it is no longer available in the library (genuinely gone — a movie removed, etc.).
//
// Everything else is kept. A store read error is treated as "keep" (fail safe: never drop a
// title on a transient error).
func (b *Binder) droppable(ctx context.Context, e schedule.LineupEntry, mustExclude map[string]struct{}) bool {
	if _, excluded := mustExclude[strings.ToLower(strings.TrimSpace(e.Title))]; excluded {
		return true
	}
	rec, err := b.store.GetTitle(ctx, e.Key)
	if err != nil {
		return false // unreadable → keep (never churn on a transient error)
	}
	// Gone from the library ⇒ droppable. `available` (and any still-in-flight acquisition:
	// wanted/requested/downloading) is NOT droppable — an in-flight title is on its way, not off
	// the channel. Only a title that has left the pipeline entirely (unavailable) is dropped.
	return rec.State == provision.Unavailable
}
