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

// mergeLineupAdditive computes the auto-curate lineup (§8.2): the refreshed proposal's picks
// UNIONED onto the channel's existing lineup, dropping an existing title ONLY when it is
// "clearly off-intent" — genuinely gone from the library (not available) OR named by the
// intent's MustExclude. A still-available title the LLM merely didn't re-pick this run is
// KEPT (no churn). Order: existing kept-entries first (preserve the channel's shape), then
// the genuinely-new picks appended.
//
// This is the non-destructive counterpart to a full replace: re-curation grows a channel
// toward its intent and prunes only the clearly-wrong, never the merely-unrepicked.
func (b *Binder) mergeLineupAdditive(
	ctx context.Context,
	existing, fresh []schedule.LineupEntry,
	mustExclude map[string]struct{},
) []schedule.LineupEntry {
	freshByKey := make(map[provision.Key]struct{}, len(fresh))
	for _, e := range fresh {
		freshByKey[e.Key] = struct{}{}
	}

	out := make([]schedule.LineupEntry, 0, len(existing)+len(fresh))
	kept := make(map[provision.Key]struct{}, len(existing))
	for _, e := range existing {
		// Re-picked by the refresh → definitely keep.
		if _, repicked := freshByKey[e.Key]; !repicked {
			// Not re-picked: keep UNLESS it's clearly off-intent (gone from the library, or
			// explicitly excluded). A still-available, non-excluded title is kept — the LLM
			// omitting it is not evidence it should leave (§8.2 "never drop for churn").
			if b.droppable(ctx, e, mustExclude) {
				b.log.Info("auto-curate dropped a clearly-off-intent title",
					"title", e.Title, "key", e.Key)
				continue
			}
		}
		out = append(out, e)
		kept[e.Key] = struct{}{}
	}
	// Append the genuinely-new picks (not already kept).
	for _, e := range fresh {
		if _, dup := kept[e.Key]; dup {
			continue
		}
		out = append(out, e)
		kept[e.Key] = struct{}{}
	}
	return out
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
