package filler

import "fmt"

// Per-clip fit (§10, V35 item 1.7): where ONE clip lands on ONE channel's ladder, and — when
// it lands nowhere — which predicate rejected it.
//
// ⚠ **This asks a different question from Coverage.** `Coverage` answers "how well does the
// catalog cover this channel's breaks"; this answers "would this clip ever be picked for this
// channel, and if not, why". The override picker needs the second: an operator ticking a
// channel is deciding about one clip, and "won't be picked" with no reason sends them hunting
// through channel settings for a rule they cannot see.
//
// ⚠ **Every answer comes from the SAME predicates assembly uses** — `filterKinds`,
// `durationEligible`, `qualityEligible`, `filterCategories`, `filterAudience`, `filterEra`,
// `filterDecade` — applied to a one-clip slice rather than re-implemented. That is the whole
// design constraint: `channelpreview.go` records the v2 mock's own meter recomputing its
// buckets inline "with five mutually inconsistent era/audience predicates", and a fit note
// that disagrees with what actually airs is a confident wrong answer about why a channel
// looks the way it does.

// FitReason names the predicate that rejected a clip, or "" when it was not rejected.
//
// ⚠ A closed vocabulary rather than a prose string, because the FRONTEND owns the wording
// (§11's refusal-code precedent): a server sentence cannot be translated, cannot link to the
// setting that caused it, and hard-codes our copy into an API response.
type FitReason string

const (
	FitKind     FitReason = "kind"     // the channel does not use this kind of clip
	FitDuration FitReason = "duration" // too long or too short for this channel's breaks
	FitQuality  FitReason = "quality"  // below the channel's minimum-quality floor
	FitCategory FitReason = "category" // the channel narrowed to categories this clip is not in
	FitAudience FitReason = "audience" // wrong audience, and not a general-audience clip
	FitExcluded FitReason = "excluded" // the operator excluded it from this channel
)

// Fit reports how one clip relates to one channel's selection.
type Fit struct {
	// Level is the tightest ladder rung this clip would be drawn from, or MatchBumperCard when
	// it would never be picked at all.
	//
	// ⚠ MatchBumperCard here means "this clip is not a candidate", NOT "this channel falls back
	// to the card" — the same constant carries both meanings because it is the bottom of one
	// ladder, but the sentence a UI writes differs. Reason disambiguates: a rejected clip always
	// carries one.
	//
	// ⚠ A PINNED clip reports MatchBumperCard with NO reason, and that is honest rather than a
	// gap: a pin is placed as its own top-priority pool ahead of the ladder, so it has no rung.
	// `Pinned` is the field that says it will play. Inventing a rung for it would claim the
	// ladder admitted a clip the ladder never saw.
	Level MatchLevel
	// Reason is why the clip is not a candidate; "" when it is one.
	Reason FitReason
	// Pinned / Excluded report the operator's explicit override for this clip on this channel.
	//
	// ⚠ Both can be true, and Excluded WINS — `Assemble` seeds the excluded ids into `used`
	// before pin runs, so a pinned-and-excluded clip never plays. Reported as-is rather than
	// normalised, or the picker would show a state the store does not hold.
	Pinned   bool
	Excluded bool
}

// FitFor computes a clip's fit against one channel's window + policy.
//
// ⚠ Bumpers and station IDs are answered honestly rather than specially: they are never drawn
// from the commercial ladder, so a bumper reports MatchBumperCard with no reason — it IS
// eligible (as a bookend), just not via the rungs. Callers distinguish by kind, which they
// already have.
func FitFor(c Clip, w Window, policy Policy) Fit {
	fit := Fit{Level: MatchBumperCard}
	for _, id := range w.Pinned {
		if id == c.ID() {
			fit.Pinned = true
		}
	}
	for _, id := range w.Excluded {
		if id == c.ID() {
			fit.Excluded = true
		}
	}

	// EXCLUDED first, mirroring Assemble: excluded ids are seeded into `used` before anything
	// picks, so no later predicate can rescue the clip. Reporting a rung here would describe a
	// pool the clip is removed from.
	if fit.Excluded {
		fit.Reason = FitExcluded
		return fit
	}

	one := []Clip{c}

	// KIND and DURATION gate everything, pins included: `pickPinned` reads the kind-filtered
	// catalog and checks `durationEligible` itself, so these two reject a pinned clip as surely
	// as an unpinned one.
	if len(filterKinds(one, w.Kinds)) == 0 {
		fit.Reason = FitKind
		return fit
	}
	if c.Kind == Commercial && !durationEligible(c, policy) {
		fit.Reason = FitDuration
		return fit
	}

	// ⚠ **A PIN BYPASSES THE REST OF THE LADDER — that is what pinning IS.** `pickPinned`'s own
	// comment says so ("Pins bypass the era/audience/category ladder entirely"), and it places
	// the clip as a top-priority pool BEFORE any rung is consulted.
	//
	// This block is a correction, recorded because the wrong version was plausible: a first cut
	// reasoned "the audience filter drops it, so the pin cannot rescue it" and reported a
	// `pinned-but-never-played` state. That state does not exist — a wrong-audience pin plays
	// fine — and the note would have told operators their explicit choice was being ignored when
	// it was being honoured. Caught by asserting against `Assemble` rather than re-asserting the
	// predicates this function calls.
	if fit.Pinned {
		return fit
	}

	// Only commercials run the ladder. A bumper that passed the kind filter is eligible as a
	// bookend and has no rung — see the doc comment.
	if c.Kind != Commercial {
		return fit
	}

	if !qualityEligible(c, policy) {
		fit.Reason = FitQuality
		return fit
	}
	if len(filterCategories(one, w.Categories)) == 0 {
		fit.Reason = FitCategory
		return fit
	}
	// ⚠ The BOTTOM rung's predicate, not the strict one — otherwise this reports "wrong audience"
	// for a clip `candidatePools` actually places on the audience rung, and the note tells an
	// operator to fix a setting that is not the problem. `TestFitFor_AgreesWithTheLadder` is what
	// catches this pair drifting apart.
	if len(filterAudienceWithUngrounded(one, w.Audience)) == 0 {
		fit.Reason = FitAudience
		return fit
	}

	// It is a candidate. Which rung? The same order candidatePools builds them in, so a fit
	// note cannot claim a tighter rung than the pool the clip actually lands in.
	switch {
	case len(filterEra(one, w.Era)) > 0:
		fit.Level = MatchExact
	case len(filterEra(one, w.Era.Widened())) > 0:
		fit.Level = MatchWidened
	default:
		fit.Level = MatchAudience
	}
	return fit
}

// String renders a Fit for logs and tests. Not the UI's wording — see FitReason.
func (f Fit) String() string {
	if f.Reason != "" {
		return fmt.Sprintf("%s (%s)", f.Level, f.Reason)
	}
	return string(f.Level)
}
