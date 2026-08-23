package recurate

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/suggest"
)

// dropped is one rejected acquisition, kept for the audit log.
//
// ⚠ The NAME and CONFIDENCE are the point. A bare count ("dropped_below_bar=8") says a
// decision happened and nothing about whether it was right — diagnosing one such run meant
// reading the filter source and reconstructing the values by hand, because the stored
// proposal is post-filter and the rejected titles are gone. Logging what was dropped, and
// what score it was dropped for, makes "is the bar tuned correctly?" answerable from a log
// line rather than from an investigation.
type dropped struct {
	Name       string
	Confidence float64
}

// retirement is one title rotated OUT to make room for a better one (§8.2a).
type retirement struct {
	Out      string  // the title leaving the lineup
	OutScore float64 // its confidence when it was last scored
	In       string  // the title taking its place
	InScore  float64
}

// filterResult carries the rewritten proposal plus what it discarded and why.
type filterResult struct {
	Proposal store.Proposal
	BelowBar []dropped
	OverCap  []dropped
	// Retired are the keys the caller must remove from the channel's lineup, paired with
	// what replaced them for the audit log.
	Retired    []retirement
	RetiredKey []provision.Key
}

// filterAcquisitions rewrites a re-curation proposal so its acquisition list contains ONLY the
// net-new titles that (a) clear the quality bar (per-title Confidence ≥ minScorePct/100) and
// (b) fit within the growth cap (channel's current title count + kept acquisitions ≤ maxTitles).
// The in-library lineup is left untouched (already available, no acquisition, no score gate).
// Returns the rewritten proposal plus counts of how many acquisitions were dropped below the
// bar and dropped for the cap — for the audit log. Deterministic: acquisitions are ranked by
// Confidence desc (Name as a stable tiebreaker) before the cap is applied, so the BEST titles
// fill the remaining room. A maxTitles of 0 means "no cap" (inherit-none / unbounded).
func filterAcquisitionsProtected(p store.Proposal, ch store.Channel, minScorePct, maxTitles int, protected map[provision.Key]bool) (filterResult, error) {
	var body suggest.Proposal
	if uerr := json.Unmarshal([]byte(p.ProposalJSON), &body); uerr != nil {
		return filterResult{}, fmt.Errorf("recurate: proposal %s malformed: %w", p.ID, uerr)
	}
	var res filterResult

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
		if effectiveConfidence(a) < barFrac {
			res.BelowBar = append(res.BelowBar, dropped{Name: a.Name, Confidence: a.Confidence})
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

	// ROTATION (§8.2a). A channel above its rotation target trades rather than only grows: a
	// better candidate displaces the stalest retirable title even while free slots remain.
	//
	// Retiring ONLY at the cap was the wrong trigger. It made the lineup a ratchet — every run
	// appended and nothing ever left until the channel hit 100% and froze — so a channel that
	// had been curated for weeks still led with its original picks, which is exactly what
	// "why do I still see the same old movies" describes. A real station retires a film once it
	// has had its run; it does not wait until the shelf is full.
	rotating := maxTitles > 0 && len(committed) >= rotationTarget(maxTitles)
	// The turnstile (§8.2a): at the cap, a better candidate retires the weakest RETIRABLE
	// title rather than being discarded. Built once, consumed as room runs out.
	bench := retirableByWeakest(ch, protected)

	for _, c := range survivors {
		if room == 0 || rotating {
			// Try to rotate: find the weakest unscheduled title this candidate beats.
			out, ok := bench.weakestBelow(effectiveConfidence(c.item))
			if !ok {
				// Nothing retirable is weaker (or everything left is airing). Below the cap
				// the newcomer simply takes a free slot — rotation is a preference, not a
				// gate. At the cap there is nowhere to put it, so it drops.
				if room == 0 {
					res.OverCap = append(res.OverCap, dropped{Name: c.item.Name, Confidence: c.item.Confidence})
					continue
				}
				kept = append(kept, c.item)
				committed[c.key] = struct{}{}
				if room > 0 {
					room--
				}
				continue
			}
			res.Retired = append(res.Retired, retirement{
				Out: out.title, OutScore: out.confidence, In: c.item.Name, InScore: c.item.Confidence,
			})
			res.RetiredKey = append(res.RetiredKey, out.key)
			kept = append(kept, c.item)
			committed[c.key] = struct{}{}
			continue
		}
		kept = append(kept, c.item)
		committed[c.key] = struct{}{} // occupy the slot (dedupes if the model repeated a title)
		if room > 0 {
			room--
		}
	}

	body.Acquisitions = kept
	// ⚠ The retirements ride the PROPOSAL to the binder, which is the only writer of a
	// channel's lineup (§8.2a). This subsystem used to trim `ch.Lineup` and persist the channel
	// itself, which made it a second writer racing the binder's additive union — ordered against
	// each other by a comment rather than by anything the compiler or a test could check.
	body.Retired = res.RetiredKey
	blob, merr := json.Marshal(body)
	if merr != nil {
		return filterResult{}, fmt.Errorf("recurate: re-marshal proposal %s: %w", p.ID, merr)
	}
	p.ProposalJSON = string(blob)
	res.Proposal = p
	return res, nil
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

// adjacencyUnscoredFloor is the confidence an ADJACENCY pick is credited when the model
// returned none for it (§8.3).
//
// The bar reads a confidence the MODEL assigns, and an omitted one unmarshals to 0 — which
// filterAcquisitions deliberately treats as "never clears a positive bar", the right call for
// a title the model searched for and then declined to stand behind. An adjacency pick is not
// that: it was HANDED to the model with a consensus the model neither computed nor can see,
// and a model that scores only what it found itself would silently zero out the entire second
// corpus. That failure is invisible — the count looks like "the bar is working".
//
// So an UNSCORED adjacency pick is credited exactly at the default bar, and no higher: enough
// to be considered on the strength of its consensus, never enough to outrank a title the model
// actually endorsed. A SCORED adjacency pick keeps the model's number in both directions — if
// the model looked and judged it weak, that judgement stands.
//
// ⚠ This is a floor on an UNSCORED pick, never a bypass. Everything downstream is unchanged:
// the title cap still applies, the per-channel opt-in still gates the run, and the acquisition
// still routes through the one suggest.Approver gate (§8.2). It cannot make a pick the operator
// never authorized, and it cannot admit a title the model marked as a poor fit.
const adjacencyUnscoredFloor = 0.60

// effectiveConfidence is the score the bar judges an acquisition on.
//
// Identity for everything except an adjacency pick the model left unscored — see
// adjacencyUnscoredFloor for why that one case is not the same as a zero.
func effectiveConfidence(a suggest.ProposalItem) float64 {
	if a.Confidence == 0 && a.Source == string(catalog.ScopeAdjacent) {
		return adjacencyUnscoredFloor
	}
	return a.Confidence
}

// benchEntry is one lineup title that MAY be retired, with the score it is judged on.
type benchEntry struct {
	key        provision.Key
	title      string
	confidence float64
}

// bench is the retirable set, weakest first.
type bench struct{ entries []benchEntry }

// weakestBelow takes the weakest retirable title STRICTLY below `incoming`, removing it so a
// single run cannot retire the same title twice.
//
// Strictly below, never equal: retiring a title for one of identical confidence is a coin flip
// that churns the lineup every week — precisely the failure additive binding (§8.2) exists to
// prevent. A tie means "no better", so nothing moves.
func (b *bench) weakestBelow(incoming float64) (benchEntry, bool) {
	if len(b.entries) == 0 || b.entries[0].confidence >= incoming {
		return benchEntry{}, false
	}
	out := b.entries[0]
	b.entries = b.entries[1:]
	return out, true
}

// retirableByWeakest builds the retirable set for a channel: every lineup title that is NOT
// currently scheduled, ordered weakest-confidence first (§8.2a).
//
// ⚠ THE SCHEDULED GUARD IS THE POINT. A title in ch.Desired is airing in the current window —
// someone may be planning to watch it today, and pulling it out from under them is a worse
// outcome than a stale channel. Desired is already persisted per channel, so this costs no new
// dependency and no scheduler call.
//
// A lineup entry carries no stored confidence (the proposal that added it is long gone), so an
// untracked title sorts as 0 — the weakest, and therefore the first to go. That is the intended
// reading: a title nobody has scored since it was added has the least evidence for its place.
func retirableByWeakest(ch store.Channel, protected map[provision.Key]bool) *bench {
	// ⚠ FAIL CLOSED on an unknown schedule. An empty Desired means "we do not know what is
	// airing" — a channel that has never reconciled, or one whose desired state was cleared —
	// NOT "nothing is airing". Treating unknown as all-retirable would let one run churn an
	// entire lineup, which is the opposite of the guard's purpose. No schedule ⇒ nothing
	// retires, and the channel simply behaves as it did before the turnstile existed.
	if len(ch.Desired) == 0 {
		return &bench{}
	}
	scheduled := make(map[provision.Key]struct{}, len(ch.Desired))
	for _, s := range ch.Desired {
		if s.Key != "" {
			scheduled[s.Key] = struct{}{}
		}
	}
	out := make([]benchEntry, 0, len(ch.Lineup))
	for _, e := range ch.Lineup {
		if protected[e.Key] {
			continue // explicit keep is stronger than automatic lineup rotation
		}
		if _, airing := scheduled[e.Key]; airing {
			continue // never retire something on the air
		}
		out = append(out, benchEntry{key: e.Key, title: e.Title, confidence: 0})
	}
	// Weakest first; Title as a stable tiebreaker so a run is reproducible (§7).
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].confidence != out[j].confidence {
			return out[i].confidence < out[j].confidence
		}
		return out[i].title < out[j].title
	})
	return &bench{entries: out}
}

// rotationFraction is how full a channel must be before it starts TRADING titles rather than
// only accumulating them (§8.2a).
//
// Three quarters, not the whole cap. Retiring only at 100% made the lineup a ratchet: a channel
// grew for weeks, never dropped anything, and then froze — so its oldest picks stayed at the
// front forever. Starting the trade earlier means a mature channel keeps circulating stock while
// still leaving genuine headroom, so a burst of good candidates can be absorbed without
// immediately displacing anything.
//
// Below the target nothing is retired at all: a young channel should fill up, not churn.
const rotationFraction = 3.0 / 4.0

// rotationTarget is the lineup size at which rotation begins for a given cap.
func rotationTarget(maxTitles int) int {
	if maxTitles <= 0 {
		return 0 // unbounded ⇒ no cap ⇒ no rotation pressure
	}
	t := int(float64(maxTitles) * rotationFraction)
	if t < 1 {
		t = 1
	}
	return t
}
