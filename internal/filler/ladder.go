package filler

import (
	"math/rand"
	"sort"
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
//  1. exact    — kind=commercial, exact Era, matching Audience
//  2. widened  — kind=commercial, same decade as Era, matching Audience
//     (skipped when policy.EraStrict — the "strict" era setting)
//  3. audience — kind=commercial, any era, matching Audience
//
// Each rung is filtered to eligible durations (density, §10). Clips already used
// in the window are excluded at fill time (no-repeat), not here.
func candidatePools(catalog []Clip, w Window, policy Policy) []pool {
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

	pools := []pool{
		{MatchExact, filterEra(audienceMatch, w.Era)},
	}
	if !policy.EraStrict {
		pools = append(pools, pool{MatchWidened, filterDecade(audienceMatch, w.Era)})
	}
	pools = append(pools, pool{MatchAudience, audienceMatch})
	return pools
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

	var out []Clip
	var totalMs int64
	lastCat := ""
	// Two passes: first honor category variety strictly; the fallback pass allows
	// a repeat only if the pod would otherwise be short of clips. Variety wins
	// WITHIN the chosen pool; we don't descend the ladder just to avoid a repeat
	// (§10 — the ladder is the duration fallback, variety is a placement rule).
	place := func(allowRepeat bool) {
		for _, c := range cands {
			if len(out) >= podMax {
				return
			}
			if used[c.Path] || contains(out, c.Path) {
				continue
			}
			if !allowRepeat && c.Category != "" && c.Category == lastCat {
				continue // would repeat a category back-to-back
			}
			if totalMs+c.DurationMs > budget && len(out) > 0 {
				continue // doesn't fit (but always allow at least one clip)
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

// hasUnused reports whether any clip in the pool isn't already used this window.
func hasUnused(clips []Clip, used map[string]bool) bool {
	for _, c := range clips {
		if !used[c.Path] {
			return true
		}
	}
	return false
}

// contains reports whether a clip id is already in the pod-in-progress.
func contains(out []Clip, id string) bool {
	for _, c := range out {
		if c.Path == id {
			return true
		}
	}
	return false
}

// --- pure filters (deterministic, no rng) ---

func filterEra(clips []Clip, era int) []Clip {
	if era <= 0 {
		return clips
	}
	out := make([]Clip, 0, len(clips))
	for _, c := range clips {
		if c.Era == era {
			out = append(out, c)
		}
	}
	return out
}

func filterDecade(clips []Clip, era int) []Clip {
	if era <= 0 {
		return clips
	}
	dec := (era / 10) * 10
	out := make([]Clip, 0, len(clips))
	for _, c := range clips {
		if c.decade() == dec {
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

// filterCategories keeps only clips whose category is in the selected set (§10
// per-channel "categories"). Empty = any category. A clip with no category ("") is
// dropped when a non-empty selection is given — an untagged clip can't be known to
// match "toys", and the selection is an explicit narrowing.
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
		if allow[c.Category] {
			out = append(out, c)
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
