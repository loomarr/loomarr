package filler

import "time"

// Auto-confirm for split proposals (§10 V43) — the gate behind
// `filler.autosplit.enabled`.
//
// ⚠ **The whole reel qualifies or none of it does.** Confirming the good segments and surfacing
// the rest would split one reel's decision across two places and hand the operator fragments to
// judge without the picture. It also matches how the 69% case actually fails: a badly-split reel
// is not uniformly slightly-wrong, it has obvious tells — one 6-minute block the detector could
// not resolve, sitting beside perfectly good 30-second cuts. One doubtful segment is evidence
// about the REEL, not just about itself.

// AutoSplitPolicy is what the gate reads, resolved per call so a settings change takes effect on
// the next run rather than at the next restart (the same hot-apply contract `AutoFilePolicy` uses).
type AutoSplitPolicy struct {
	// Enabled reports whether auto-confirm is on at all.
	//
	// ⚠ Must be backed by a FAIL-CLOSED read (`boolv`, not `boolOn`), like AutoFilePolicy. When
	// the settings service cannot answer, the safe answer is "don't cut" — failing open would
	// consume compilations unattended precisely when the install is degraded.
	Enabled func() bool
	// MinConfidence is the score every segment must reach. Bounded 50-95 by the registry.
	MinConfidence func() int
	// MaxDuration is the longest a segment may be and still be advert-shaped.
	MaxDuration func() time.Duration
}

// AutoSplitReject names why a proposal was not auto-confirmed. Returned rather than logged so
// the caller can report it — an unattended decision that leaves no trace is not one an appliance
// gets to make (§10), and "it just didn't" is unactionable for an operator tuning the threshold.
type AutoSplitReject string

const (
	AutoSplitOK AutoSplitReject = ""
	// RejectDisabled is the ordinary state: the operator has not opted in.
	RejectDisabled   AutoSplitReject = "auto-split is off"
	RejectNoSegments AutoSplitReject = "the proposal has no segments"
	// RejectUnsplittable is the signal this gate exists for. The detector already knows it
	// failed on this span; auto-confirming around that admission is the 69% failure.
	RejectUnsplittable AutoSplitReject = "a segment could not be split and needs a human"
	RejectTooLong      AutoSplitReject = "a segment is longer than one advert"
	RejectTooShort     AutoSplitReject = "a segment is shorter than the clip floor"
	// RejectDuplicate: the dedup pass flagged a match against the catalog. §10 says a match is
	// a FLAG and never a silent drop, so it is also never a silent keep.
	RejectDuplicate AutoSplitReject = "a segment already exists in the catalog"
	// RejectUngrounded is the grounding rule, reused rather than re-derived.
	RejectUngrounded AutoSplitReject = "a segment's era is a guess rather than something Loomarr read"
	RejectUntagged   AutoSplitReject = "a segment could not be classified"
)

// minConfidence resolves the operator's threshold, defaulting to the registry's 85 when the
// policy carries no reader. Clamped to the same ceiling the tagger uses: `filler.Score` caps an
// ungrounded era strictly below it, and a value above would silently break that guarantee.
func minConfidence(pol *AutoSplitPolicy) int {
	if pol == nil || pol.MinConfidence == nil {
		return 85
	}
	n := pol.MinConfidence()
	if n <= 0 || n > MaxAutoFileConfidence {
		return MaxAutoFileConfidence
	}
	return n
}

// AutoConfirmable reports whether a proposal may be confirmed with no human, and why not.
//
// ⚠ **There is no per-segment confidence NUMBER to read, and this is better than one.**
// `SplitSegment` carries no score — by §10's era rule a grounded era lands in `Era` and an
// ungrounded guess lands in `SuggestedEra` instead. So "confident" is not a threshold comparison
// here, it is the grounding outcome the classifier already committed to: a segment carrying
// `SuggestedEra` is by definition one the rule refused to trust. Inventing a confidence field to
// compare against would launder that refusal into a number, which is the exact move the era rule
// exists to prevent.
//
// ⚠ `MinConfidence` gates the ERA specifically, and that is the honest use for it. §10 caps an
// ungrounded era strictly below `MaxAutoFileConfidence` (95), so the operator's threshold is
// meaningful in exactly one place here: at 95 nothing auto-confirms without a grounded era, and
// lowering it does NOT admit guesses — `SuggestedEra` disqualifies at any setting. It tunes how
// much classification a reel must carry, not whether a guess can pass.
func AutoConfirmable(p SplitProposal, pol *AutoSplitPolicy, minClipDuration time.Duration) AutoSplitReject {
	if pol == nil || pol.Enabled == nil || !pol.Enabled() {
		return RejectDisabled
	}
	if len(p.Segments) == 0 {
		return RejectNoSegments
	}

	maxDur := 120 * time.Second
	if pol.MaxDuration != nil {
		if d := pol.MaxDuration(); d > 0 {
			maxDur = d
		}
	}

	for _, s := range p.Segments {
		if s.Unsplittable {
			return RejectUnsplittable
		}
		if s.DupOf != "" {
			return RejectDuplicate
		}
		span := time.Duration(s.EndMs-s.StartMs) * time.Millisecond
		if span > maxDur {
			return RejectTooLong
		}
		// ⚠ The floor is the SAME `filler.min_duration` the scan boundary rejects on (V40), not
		// a second number. A segment this gate would confirm and the scan would then reject is
		// a clip cut out of a compilation and immediately thrown away — work done to produce
		// nothing, and a source file consumed for it.
		if minClipDuration > 0 && span < minClipDuration {
			return RejectTooShort
		}
		// The grounding outcome, not a score. An era Loomarr GUESSED is exactly the case a human
		// should see, and it is already marked.
		if s.SuggestedEra > 0 {
			return RejectUngrounded
		}
		// ⚠ Audience and category are what pod assembly MATCHES on (§10). A segment with
		// neither is a clip that can only ever be a fallback-ladder pick, which is not something
		// to create unattended out of a file the operator still had.
		if s.Audience == "" && s.Category == "" {
			return RejectUntagged
		}
		// ⚠ At the ceiling (95) a grounded ERA is required too, not merely tags. That is what
		// makes the threshold do real work rather than sit decorative: an operator who wants
		// only unambiguous reels cut sets it to the top and gets segments Loomarr READ a year
		// for. Below the ceiling, tags alone suffice — but a guessed era still disqualifies
		// above, because `SuggestedEra` is checked at every setting.
		if minConfidence(pol) >= MaxAutoFileConfidence && s.Era == 0 {
			return RejectUngrounded
		}
	}
	return AutoSplitOK
}
