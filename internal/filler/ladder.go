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
		if c.Kind == Commercial && durationEligible(c, policy) {
			commercials = append(commercials, c)
		}
	}

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
// TODO(learning): implement the fill loop. You have `pools` (tightest first),
// the window `w` (GapMs, PodMax), `policy`, `used` (no-repeat-in-window set), and
// `rng` (seeded — use rng.Shuffle / rng.Intn, never Math.random-style, so pods
// are reproducible §19). Requirements:
//   - Pick the tightest pool with any un-used clips; that pool's level is the
//     MatchLevel to return.
//   - Fill up to w.PodMax clips whose total duration fits w.GapMs (leave a little
//     headroom for the bumpers the caller adds — subtract a bumper budget or just
//     stop when adding the next clip would exceed GapMs).
//   - CATEGORY VARIETY: never place two clips of the same Category consecutively
//     (§10 — "so it doesn't play three car ads back to back"). If the only
//     remaining candidate repeats the last category, prefer a different one; only
//     repeat if there's no alternative.
//   - NO-REPEAT: skip any clip already in `used` (the caller threads `used` across
//     a window). Don't add to `used` here — Assemble does that as it appends.
//   - Deterministic: seed all randomness from rng. Sort candidates by
//     TunarrProgramID before any random pick so the input order can't leak in.
//   - Return ("", nil) if no pool has eligible clips (Assemble then uses the
//     bumper card).
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
			if used[c.TunarrProgramID] || contains(out, c.TunarrProgramID) {
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
		if !used[c.TunarrProgramID] {
			return true
		}
	}
	return false
}

// contains reports whether a clip id is already in the pod-in-progress.
func contains(out []Clip, id string) bool {
	for _, c := range out {
		if c.TunarrProgramID == id {
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
	sort.Slice(out, func(i, j int) bool { return out[i].TunarrProgramID < out[j].TunarrProgramID })
	return out
}
