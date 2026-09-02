package filler

import (
	"math/rand"
	"sort"
	"time"
)

// This file builds the fallback ladder (§10): the ordered sequence of candidate
// pools, from tightest (exact era + audience) to loosest (any appropriate-
// audience clip), with the embedded bumper card below that. Commercials are drawn
// from the tightest non-empty pool so a break degrades gracefully instead of
// going dead.

// pool is one rung of the ladder: a candidate set + the MatchLevel it represents.
type pool struct {
	level MatchLevel
	clips []Clip
}

// candidatePools builds the ladder rungs for a window (§10 fallback ladder):
//
//  1. exact    — kind=commercial, inside the Era RANGE, matching Audience
//  2. widened  — kind=commercial, a decade either side of that range, matching Audience
//  3. audience — kind=commercial, any era, matching Audience
//
// ⚠ **All three rungs are always built (V51f).** `policy.EraStrict` used to drop rung 2, and it
// was set in tests and nowhere else — no settings key, no policy field, no way for an operator to
// reach it — while `coverage.go`, `fit.go` and `coverage-meter.tsx` all carried special copy for
// the branch. A real range subsumes it anyway: a channel that wants strictness sets a narrow
// range, which is a control an operator can actually see and change.
//
// Each rung is filtered to eligible durations (density, §10). Clips already used
// in the window are excluded at fill time (no-repeat), not here.
func candidatePools(catalog []Clip, w Window, policy Policy) []pool {
	catalog = filterGeography(catalog, effectiveGeography(w.Geography, policy.Geography))
	commercials := make([]Clip, 0, len(catalog))
	for _, c := range catalog {
		// ⚠ The quality floor applies to COMMERCIALS only, alongside the duration bounds —
		// deliberately not catalog-wide. Bumpers are the ladder's floor and the station's own
		// bookends; excluding them on resolution could leave a break with nothing to open or
		// close on, trading a cosmetic complaint for a structural one.
		if c.Kind == Commercial && durationEligible(c, policy) && qualityEligible(c, policy) {
			commercials = append(commercials, c)
		}
	}
	// Category narrowing (§10) applies to the commercial pool only — bumpers are
	// bookends, not themed by category.
	commercials = filterCategories(commercials, w.Categories)

	audienceMatch := filterAudience(commercials, w.Audience)

	// ⚠ The era rungs draw from the STRICT audience pool and only the bottom rung widens to
	// ungrounded clips — that ordering is the whole point. An unclassified clip with a perfect
	// era must not outrank a clip Loomarr actually knows is right for this channel.
	return []pool{
		{MatchExact, filterEra(audienceMatch, w.Era)},
		{MatchWidened, filterEra(audienceMatch, w.Era.Widened())},
		{MatchAudience, filterAudienceWithUngrounded(commercials, w.Audience)},
	}
}

// fillCommercials draws matched commercials from the tightest non-empty pool,
// enforcing category variety (no same category back-to-back) and no-repeat, up to
// PodMax clips and the flex gap. Deterministic under rng. Returns the MatchLevel
// reached and the ordered clips.
//
// The invariants it holds, each with a test — change one and the pod stops being
// reproducible or starts playing three car ads in a row:
//   - Takes the TIGHTEST pool with any un-used clips; that pool's level is the
//     MatchLevel returned (the ladder's whole point — never widen further than needed).
//   - Fills up to w.PodMax clips within w.GapMs, reserving `bumperBudgetMs` for the
//     intro/return bumpers the caller appends.
//   - CATEGORY VARIETY: never two clips of the same Category consecutively (§10 —
//     "so it doesn't play three car ads back to back"). Implemented as the two-pass
//     place(false)/place(true): the second pass tops the pod up and accepts a repeat
//     rather than returning a short pod.
//   - NO-REPEAT: skips anything already in `used`, which the caller threads across the
//     whole window. Nothing is added to `used` here — Assemble does that as it appends.
//   - Deterministic: all randomness comes from the seeded rng, and candidates are sorted
//     by Path (the identity) before any pick so catalog input order can't leak into output.
//   - Returns ("", nil) when no pool has eligible clips; Assemble then uses the bumper card.
func fillCommercials(pools []pool, w Window, policy Policy, used map[string]bool, rng *rand.Rand) (MatchLevel, []Clip) {
	// Reserve a little of the gap for the intro+return bumpers the caller adds.
	const bumperBudgetMs = 12000 // ~two 6s bumpers
	budget := w.GapMs - bumperBudgetMs
	if budget < 0 {
		budget = w.GapMs
	}
	podMax := w.PodMax
	if podMax <= 0 {
		podMax = 4
	}

	// Pick the tightest pool that has any un-used clips; that's our MatchLevel.
	var chosen pool
	for _, p := range pools {
		if hasUnused(p.clips, used) {
			chosen = p
			break
		}
	}
	if chosen.level == "" {
		return "", nil // nothing eligible → caller uses the bumper card
	}

	// Deterministic candidate order, then a seeded shuffle so selection varies by
	// window but reproduces exactly for a given seed (§19).
	cands := sortByID(chosen.clips)
	rng.Shuffle(len(cands), func(i, j int) { cands[i], cands[j] = cands[j], cands[i] })
	// Stable sorting after the seeded shuffle preserves varied tie-breaking while making durable
	// rotation authoritative: new first, then oldest outside cooldown, then oldest recent clip.
	sort.SliceStable(cands, func(i, j int) bool { return rotationLess(cands[i], cands[j], w, policy) })

	var out []Clip
	var totalMs int64
	lastCat := ""
	// Two passes: first honor category variety strictly; the fallback pass allows
	// a repeat only if the pod would otherwise be short of clips. Variety wins
	// WITHIN the chosen pool; we don't descend the ladder just to avoid a repeat
	// (§10 — the ladder is the duration fallback, variety is a placement rule).
	//
	// ⚠ V45a: `c.Category` is now the DERIVED primary product leaf, so "no two back-to-back with the
	// same Category" reads as "don't stack two clips of the same product family" — two beer ads are a
	// repeat, a beer and a car ad are not. That is the right granularity and the rule is unchanged; a
	// clip with no product tag ("" Category) is exempt from the repeat check, as an untagged one was.
	place := func(allowRepeat bool) {
		for _, c := range cands {
			if len(out) >= podMax {
				return
			}
			// ⚠ Keyed by HASH (Clip.ID()), not Path — `used` is seeded by hash-keyed pinned/excluded
			// and reserved by hash everywhere (§10 V45a). Path keying here meant a pinned/excluded
			// clip was never recognised as used, so it could be double-placed or ignore its exclusion.
			if used[c.ID()] || contains(out, c.ID()) {
				continue
			}
			if !allowRepeat && c.Category != "" && c.Category == lastCat {
				continue // would repeat a category back-to-back
			}
			if totalMs+c.DurationMs > budget {
				continue // does not fit; caller's fallback card is the bounded never-dead-air answer
			}
			out = append(out, c)
			totalMs += c.DurationMs
			lastCat = c.Category
		}
	}
	place(false)
	if len(out) < podMax {
		place(true) // top up, accepting a category repeat rather than a shorter pod
	}
	if len(out) == 0 {
		return "", nil
	}
	return chosen.level, out
}

const (
	rotationNever = iota
	rotationReady
	rotationRecent
)

// rotationRank is the domain-owned anti-repeat ordering (§10 V58). The returned timestamp is
// meaningful only for aired clips; lower tiers and older times rank first.
func rotationRank(c Clip, w Window, policy Policy) (int, time.Time) {
	exposure, ok := w.Exposures[c.ID()]
	if !ok || exposure.PlayCount <= 0 || exposure.LastPlayedAt.IsZero() {
		return rotationNever, time.Time{}
	}
	if policy.Cooldown <= 0 || w.SnapshotAt.IsZero() ||
		!exposure.LastPlayedAt.After(w.SnapshotAt.Add(-policy.Cooldown)) {
		return rotationReady, exposure.LastPlayedAt
	}
	return rotationRecent, exposure.LastPlayedAt
}

func rotationLess(a, b Clip, w Window, policy Policy) bool {
	at, aa := rotationRank(a, w, policy)
	bt, ba := rotationRank(b, w, policy)
	if at != bt {
		return at < bt
	}
	if !aa.Equal(ba) {
		return aa.Before(ba)
	}
	// The caller establishes a deterministic seeded order before this stable comparison.
	return false
}

// hasUnused reports whether any clip in the pool isn't already used this window.
func hasUnused(clips []Clip, used map[string]bool) bool {
	for _, c := range clips {
		if !used[c.ID()] {
			return true
		}
	}
	return false
}

// contains reports whether a clip id (its HASH) is already in the pod-in-progress.
func contains(out []Clip, id string) bool {
	for _, c := range out {
		if c.ID() == id {
			return true
		}
	}
	return false
}

// --- pure filters (deterministic, no rng) ---

// EraRange is the year window a channel draws its breaks from — §10's "a year range", which the
// wire has always carried and the domain, until V51f, threw half of away.
//
// ⚠ **The zero value means ANY, and the ambiguity that costs is resolved one layer up, not here.**
// `SelectionForChannel` owns the three states the operator sees (unset = inherit the channel's
// programming era, `{0,0}` = explicitly any, a set range = that window); by the time a range
// reaches the ladder the inheritance has already been applied, so the only question left is
// "which years". Keeping the third state OUT of this type is what stops every filter below from
// having to know about channel scope.
type EraRange struct {
	From int
	To   int
}

// Any reports whether the range constrains nothing.
func (r EraRange) Any() bool { return r.From <= 0 && r.To <= 0 }

// Year is the single-year range — what a channel targeting "1992" means, and the shape every
// call site had before V51f when the field was one int.
func Year(y int) EraRange { return EraRange{From: y, To: y} }

// Contains reports whether a clip's year falls in the window. A zero bound is unbounded on that
// end, so {1990, 0} is "1990 onwards" and {0, 1999} is "up to 1999".
//
// ⚠ An untagged clip (year 0) is IN no range but in "any" — the same shape as the audience rule.
// A clip whose era Loomarr could not ground must not silently satisfy an era the operator set.
func (r EraRange) Contains(year int) bool {
	if r.Any() {
		return true
	}
	if year <= 0 {
		return false
	}
	if r.From > 0 && year < r.From {
		return false
	}
	if r.To > 0 && year > r.To {
		return false
	}
	return true
}

// Widened is the ladder's second rung: a decade either side of the range.
//
// ⚠ **This deliberately replaces the old decade-BUCKET rule, and the difference matters for
// ranges.** `filterDecade` snapped a single year to its containing decade (1994 → 1990–1999),
// which cannot generalise: a range that already spans 1990–1999 snaps to itself, so the widened
// rung would be identical to `exact` and the ladder would silently lose a rung exactly when the
// operator asked for a decade. Growing by ten years at each end is strictly wider than `exact`
// for EVERY range, which is the property a fallback rung has to have to be one.
func (r EraRange) Widened() EraRange {
	if r.Any() {
		return r
	}
	w := r
	if w.From > 0 {
		w.From -= 10
	}
	if w.To > 0 {
		w.To += 10
	}
	return w
}

func filterEra(clips []Clip, era EraRange) []Clip {
	if era.Any() {
		return clips
	}
	out := make([]Clip, 0, len(clips))
	for _, c := range clips {
		if era.Contains(c.Era) {
			out = append(out, c)
		}
	}
	return out
}

func filterAudience(clips []Clip, aud Audience) []Clip {
	if aud == "" {
		return clips
	}
	out := make([]Clip, 0, len(clips))
	for _, c := range clips {
		// A general-audience clip fits any channel; otherwise require an exact
		// audience match (kids ads on the kids channel, not late-night).
		if c.Audience == aud || c.Audience == General {
			out = append(out, c)
		}
	}
	return out
}

// admitsUngroundedAudience reports whether a channel targeting `aud` may draw clips whose
// audience Loomarr could not work out.
//
// ⚠ **An ALLOWLIST, deliberately, and this is the one rule here that is about safety.** A
// denylist (`aud != Kids`) would hand every audience added later the permissive default —
// exactly the wrong direction. Only `general` and `late_night` admit an unclassified clip.
// `family` is excluded alongside `kids`: family channels are watched by children, and the
// asymmetry the §10 audience ceiling encodes is that "we could not tell who this is for" must
// never resolve to "so show it to children".
//
// An empty `aud` (the channel expressed no audience) never reaches here — `filterAudience`
// returns everything, untagged clips included, before this is consulted.
func admitsUngroundedAudience(aud Audience) bool {
	return aud == General || aud == LateNight
}

// filterAudienceWithUngrounded is the BOTTOM rung's pool: everything `filterAudience` admits,
// plus — on a channel allowed to take them — the clips whose audience could not be grounded.
//
// ⚠ **This rung is why picking an Audience stopped emptying the whole ladder (§10 V51f).**
// `filler.ai_tagging` defaulted off for most of this project's life, so a real catalog is full of
// clips carrying `""`, which equals no audience and matches `General` either. The moment an
// operator chose an audience, every rung went empty and the channel fell to its bumper card —
// while the meter said "nothing in the catalog fits", which reads as a catalog problem rather
// than a tagging one. Admitting them at the BOTTOM means a grounded match always wins when one
// exists, and the untagged clips are a floor rather than a competitor.
func filterAudienceWithUngrounded(clips []Clip, aud Audience) []Clip {
	if aud == "" || !admitsUngroundedAudience(aud) {
		return filterAudience(clips, aud)
	}
	out := make([]Clip, 0, len(clips))
	for _, c := range clips {
		if c.Audience == aud || c.Audience == General || c.Audience == "" {
			out = append(out, c)
		}
	}
	return out
}

// filterKinds keeps only clips whose kind is in the selected set (§10 per-channel
// "kinds to include"). Empty selection = the default kinds (commercial + the two
// bumper kinds), which is what a channel with no kind preference should draw: ads
// with bumper bookends. Applied catalog-wide BEFORE pickBumper/candidatePools, so a
// "bumpers only" selection empties the commercial pool and a "no bumpers" one leaves
// pickBumper nothing — the kind choice shapes the whole pod, not just the middle.
func filterKinds(clips []Clip, kinds []string) []Clip {
	allow := map[Kind]bool{}
	if len(kinds) == 0 {
		allow[Commercial], allow[Bumper], allow[StationID] = true, true, true
	} else {
		for _, k := range kinds {
			allow[Kind(k)] = true
		}
	}
	out := make([]Clip, 0, len(clips))
	for _, c := range clips {
		if allow[c.Kind] {
			out = append(out, c)
		}
	}
	return out
}

// filterCategories keeps only clips whose TAXONOMY TAG SET intersects the selected categories (§10
// per-channel "categories", V45a). Empty selection = any category.
//
// ⚠ **Rollup-set intersection, not a single-string equality** (the flat-`category` model this
// replaces). A clip's `Tags` is the full leaf+rollup expansion, so selecting `food` matches a clip
// tagged `cereal` (cereal rolls up to food) — the question the flat string could NOT answer and the
// whole reason the taxonomy exists. A clip with NO tags is dropped under a non-empty selection: an
// untagged clip can't be known to match, and the selection is an explicit narrowing (unchanged from
// the flat model's polarity). This is deterministic set membership — eligibility, not fuzzy ranking
// (§10: structured filters stay deterministic; theme ranking never decides eligibility).
func filterCategories(clips []Clip, cats []string) []Clip {
	if len(cats) == 0 {
		return clips
	}
	allow := make(map[string]bool, len(cats))
	for _, c := range cats {
		allow[c] = true
	}
	out := make([]Clip, 0, len(clips))
	for _, c := range clips {
		for _, tag := range c.Tags {
			if allow[tag] {
				out = append(out, c)
				break
			}
		}
	}
	return out
}

// qualityEligible reports whether a clip clears the opt-in minimum-quality floor (V17c).
//
// ⚠ Two things are deliberately permissive here, and both are the difference between a floor
// and a catalog-emptier:
//
//   - A floor of 0 (the default) admits everything, so an install that sets nothing behaves
//     exactly as it did before this existed. `TestFloorOffIsByteIdentical` pins that.
//   - A clip whose quality is UNKNOWN is admitted. Audio-only clips have no height, and every
//     clip scanned before `00014_clips_quality` has an empty label until the next sync
//     re-probes it. Excluding unknowns would make setting a floor look like the catalog
//     vanished, which is the opposite of the diagnosis an operator needs.
func qualityEligible(c Clip, policy Policy) bool {
	if policy.MinQualityHeight <= 0 {
		return true
	}
	h := heightFromQuality(c.Quality)
	if h == 0 {
		return true // unknown quality is not evidence of LOW quality
	}
	return h >= policy.MinQualityHeight
}

// heightFromQuality maps a stored quality LABEL back to the pixel height it represents.
//
// ⚠ The label is what `clips.quality` holds — `QualityFromHeight` bucketed the probed height
// at scan time and the original number was never stored. So this is the inverse of that
// bucketing, and it returns each bucket's LOWER bound: "480p" covers heights 460-699, and
// treating it as 460 means a floor of 480 admits it. Rounding to the nominal 480 instead would
// exclude a 470-line clip that the label already called 480p, which is not a distinction an
// operator setting "480" is trying to draw.
func heightFromQuality(label string) int {
	switch label {
	case "4K":
		return 2000
	case "1080p":
		return 1000
	case "720p":
		return 700
	case "480p":
		return 460
	case "360p":
		return 340
	case "240p":
		return 1 // a real, known-low height: any floor above 0 excludes it
	default:
		return 0 // "" and anything unrecognised — treated as unknown, see qualityEligible
	}
}

func durationEligible(c Clip, policy Policy) bool {
	if policy.MinClipMs > 0 && c.DurationMs < policy.MinClipMs {
		return false
	}
	if policy.MaxClipMs > 0 && c.DurationMs > policy.MaxClipMs {
		return false
	}
	return true
}

// sortByID returns a deterministically-ordered copy (so a random pick can't leak
// catalog input order — §19 determinism).
func sortByID(clips []Clip) []Clip {
	out := make([]Clip, len(clips))
	copy(out, clips)
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}
