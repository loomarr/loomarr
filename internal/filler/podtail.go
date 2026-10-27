package filler

import "math/rand"

// maxTailClips bounds how many clips the top-up may add after the ads. It exists so a catalog of
// one-second idents cannot turn a break's last seconds into a slideshow; time is the real limit.
const maxTailClips = 8

// tailCandidate is a clip the top-up may still place: an ad from a ladder rung, or a bookend
// (rung -1) such as a station ID.
type tailCandidate struct {
	Clip
	rung int
}

// fillTail spends the time the ads could not: it adds whole clips that fit the remaining
// milliseconds until nothing else does. Nothing is cut and nothing exceeds the slot.
//
// ⚠ **Why this is separate from fillCommercials.** The ads are picked in ladder order, which is
// the right order for choosing WHICH ads air, but ladder order leaves a tail whenever the next
// ad in line is longer than what is left. The household asked for fuller breaks, and a 2 minute
// break of 30s ads and two 6s bumpers aired 102 seconds and then 18 seconds of card. The only
// things that fit 18 seconds are not ads: a station ID, a short promo, a bumper. So the
// top-up also draws on bookend clips the break has not used.
//
// Rules held identical to the ads' own: the candidates are the same ladder pools (so audience
// ceiling, era rungs, geography, quality and duration bounds all apply), the same `used` set (no
// repeat in a window, exclusions win), and rotation held strictly: a clip resting inside its
// cooldown is never used here — a short tail beats a repeat — then category variety, relaxed
// rather than leaving time unused. Ads respect the pod's clip ceiling; bookends do not count against it.
//
// Among the clips that fit, rotation picks first, exactly as it does for the ads: a clip the
// household has never seen, then the least recently aired. Equals go to the LONGEST, so the
// leftover shrinks as fast as it can. Either way the loop runs until nothing unused fits, so the
// remainder still ends under the shortest unused clip.
//
// ⚠ Length used to come first, so the same few long clips topped up every break they fitted and
// a never-aired clip lost to one aired a few hours ago (#2017).
func (p *Pod) fillTail(catalog []Clip, pools []pool, w Window, policy Policy, used map[string]bool, rng *rand.Rand, remainingMs int64, adsPlaced int) {
	if remainingMs <= 0 {
		return
	}
	podMax := w.PodMax
	if podMax <= 0 {
		podMax = 4
	}

	var cands []tailCandidate
	seen := map[string]bool{}
	for rung, pl := range pools {
		for _, c := range sortByID(pl.clips) {
			if used[c.ID()] || seen[c.ID()] {
				continue
			}
			seen[c.ID()] = true
			cands = append(cands, tailCandidate{Clip: c, rung: rung})
		}
	}
	for _, c := range sortByID(bookendCandidates(catalog, w, used)) {
		if !seen[c.ID()] {
			seen[c.ID()] = true
			cands = append(cands, tailCandidate{Clip: c, rung: -1})
		}
	}
	if len(cands) == 0 {
		return
	}
	// Seeded shuffle first so equal candidates vary by break but reproduce for a seed (§19).
	rng.Shuffle(len(cands), func(i, j int) { cands[i], cands[j] = cands[j], cands[i] })

	lastCat := ""
	if n := len(p.Entries); n > 0 {
		lastCat = p.Entries[n-1].Category
	}
	for placed := 0; placed < maxTailClips && remainingMs > 0; placed++ {
		pick := -1
		for _, allowRepeat := range []bool{false, true} {
			for i, c := range cands {
				if used[c.ID()] || c.DurationMs <= 0 || c.DurationMs > remainingMs {
					continue
				}
				if c.rung >= 0 && adsPlaced >= podMax {
					continue
				}
				// A few spare seconds never justify re-airing a clip still inside its cooldown.
				if resting(c.Clip, w, policy) {
					continue
				}
				if !allowRepeat && c.Category != "" && c.Category == lastCat {
					continue
				}
				if pick < 0 || tailBetter(c, cands[pick], w, policy) {
					pick = i
				}
			}
			if pick >= 0 {
				break
			}
		}
		if pick < 0 {
			return
		}
		c := cands[pick]
		p.append(clipToEntry(c.Clip, w, policy), used)
		remainingMs -= c.DurationMs
		lastCat = c.Category
		if c.rung >= 0 {
			adsPlaced++
			p.widenTo(pools, c.rung)
		}
	}
}

// widenTo records that a clip from a looser rung joined the pod.
func (p *Pod) widenTo(pools []pool, rung int) {
	current := -1
	for i, pl := range pools {
		if pl.level == p.MatchLevel {
			current = i
		}
	}
	if rung > current {
		p.MatchLevel = pools[rung].level
	}
}

// tailBetter orders two rested clips that both fit: the durable rotation order, then the longer,
// then an ad before a bookend. Equal clips keep their (seeded) position. Like the rung, the region
// preference (#2035) does not rank here: the tail fills spare seconds with whatever fits best.
func tailBetter(a, b tailCandidate, w Window, policy Policy) bool {
	if rotationLess(a.Clip, b.Clip, w, policy) {
		return true
	}
	if rotationLess(b.Clip, a.Clip, w, policy) {
		return false
	}
	if a.DurationMs != b.DurationMs {
		return a.DurationMs > b.DurationMs
	}
	return a.rung >= 0 && b.rung < 0
}

func resting(c Clip, w Window, policy Policy) bool {
	tier, _ := rotationRank(c, w, policy)
	return tier == rotationRecent
}
