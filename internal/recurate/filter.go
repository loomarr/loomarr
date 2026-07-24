package recurate

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/suggest"
)

// filterAcquisitions rewrites a re-curation proposal so its acquisition list contains ONLY the
// net-new titles that (a) clear the quality bar (per-title Confidence ≥ minScorePct/100) and
// (b) fit within the growth cap (channel's current title count + kept acquisitions ≤ maxTitles).
// The in-library lineup is left untouched (already available, no acquisition, no score gate).
// Returns the rewritten proposal plus counts of how many acquisitions were dropped below the
// bar and dropped for the cap — for the audit log. Deterministic: acquisitions are ranked by
// Confidence desc (Name as a stable tiebreaker) before the cap is applied, so the BEST titles
// fill the remaining room. A maxTitles of 0 means "no cap" (inherit-none / unbounded).
func filterAcquisitions(p store.Proposal, ch store.Channel, minScorePct, maxTitles int) (
	out store.Proposal, droppedBelowBar, droppedOverCap int, err error,
) {
	var body suggest.Proposal
	if uerr := json.Unmarshal([]byte(p.ProposalJSON), &body); uerr != nil {
		return store.Proposal{}, 0, 0, fmt.Errorf("recurate: proposal %s malformed: %w", p.ID, uerr)
	}

	// The bar: a per-title Confidence (0..1) at or above minScorePct/100. A title the model gave
	// no confidence (0) never clears a positive bar — the conservative direction for spending.
	barFrac := float64(minScorePct) / 100.0

	// Distinct keys already committed to the channel (its current lineup) — the cap counts
	// EXISTING titles too, so a full channel grows no further. In-library adds in this same
	// proposal also count against the cap.
	committed := map[provision.Key]struct{}{}
	for _, e := range ch.Lineup {
		committed[e.Key] = struct{}{}
	}
	for _, it := range body.Lineup { // in-library picks this proposal adds
		if k, kerr := it.Key(); kerr == nil {
			committed[k] = struct{}{}
		}
	}

	// Rank candidate acquisitions by confidence (desc), keep only those over the bar.
	type cand struct {
		item suggest.ProposalItem
		key  provision.Key
	}
	survivors := make([]cand, 0, len(body.Acquisitions))
	for _, a := range body.Acquisitions {
		k, kerr := a.Key()
		if kerr != nil {
			continue // an unkeyable acquisition can't be requested; drop silently (shouldn't happen post-grounding)
		}
		if _, dup := committed[k]; dup {
			continue // already on the channel / already an in-library add — not net-new
		}
		if a.Confidence < barFrac {
			droppedBelowBar++
			continue
		}
		survivors = append(survivors, cand{item: a, key: k})
	}
	sort.SliceStable(survivors, func(i, j int) bool {
		if survivors[i].item.Confidence != survivors[j].item.Confidence {
			return survivors[i].item.Confidence > survivors[j].item.Confidence
		}
		return survivors[i].item.Name < survivors[j].item.Name
	})

	// Apply the growth cap: room = maxTitles − (already committed). maxTitles 0 ⇒ unbounded.
	kept := make([]suggest.ProposalItem, 0, len(survivors))
	room := -1 // -1 = unbounded
	if maxTitles > 0 {
		room = maxTitles - len(committed)
		if room < 0 {
			room = 0
		}
	}
	for _, c := range survivors {
		if room == 0 {
			droppedOverCap++
			continue
		}
		kept = append(kept, c.item)
		committed[c.key] = struct{}{} // occupy the slot (dedupes if the model repeated a title)
		if room > 0 {
			room--
		}
	}

	body.Acquisitions = kept
	blob, merr := json.Marshal(body)
	if merr != nil {
		return store.Proposal{}, 0, 0, fmt.Errorf("recurate: re-marshal proposal %s: %w", p.ID, merr)
	}
	p.ProposalJSON = string(blob)
	return p, droppedBelowBar, droppedOverCap, nil
}

// effectiveMinScore resolves the quality bar for a channel: its per-channel override if set
// (> 0), else the global recurate.min_score_pct.
func effectiveMinScore(ctx context.Context, ac *schedule.AutoCurate, th Thresholds) int {
	if ac != nil && ac.MinScorePct > 0 {
		return ac.MinScorePct
	}
	if th == nil {
		return 0
	}
	return th.MinScorePct(ctx)
}

// effectiveMaxTitles resolves the growth cap for a channel: its per-channel override if set
// (> 0), else the global recurate.max_titles.
func effectiveMaxTitles(ctx context.Context, ac *schedule.AutoCurate, th Thresholds) int {
	if ac != nil && ac.MaxTitles > 0 {
		return ac.MaxTitles
	}
	if th == nil {
		return 0
	}
	return th.MaxTitles(ctx)
}
