package split

import "sort"

// CutSound is a cut's loudness where it matters (#4409). Ads are mastered to open and close on
// near-silence: a cut between two spots meets a dip in loudness at each edge, a cut inside a spot
// meets a running sound bed, and a cut holding two spots goes quiet where they meet. Levels are dB
// relative to the cut's own median, so a quiet ad and a loud one are judged alike.
//
// Measured on one install's ads set inside reels (TestSplitWholeCutEval, 20 cases of each kind):
// the sound passed none of the 40 cuts that were not one spot and 12 of the 20 that were, where
// the vision look alone called 13 of those 40 `one`.
type CutSound struct {
	// MedianDb is the cut's median loudness, absolute. At SoundSilentDb the cut has no sound to
	// judge.
	MedianDb float64 `json:"medianDb"`
	// StartDb and EndDb are the quietest window within a window of each edge.
	StartDb float64 `json:"startDb"`
	EndDb   float64 `json:"endDb"`
	// InsideDb is the quietest window more than InsideMarginMs inside both edges; 0 when the cut is
	// too short to have an inside.
	InsideDb float64 `json:"insideDb"`
}

const (
	// SoundSilentDb is the level of digital silence.
	SoundSilentDb = -90.0
	// SoundWindowMs is the length of one loudness window.
	SoundWindowMs = 200
	// SoundPadMs is how much of the reel either side of a cut is measured, so an edge window can
	// look across the cut.
	SoundPadMs = 400
	// InsideMarginMs keeps the inside clear of the edges' own fades.
	InsideMarginMs = 1_500
	// EdgeQuietDb: an edge is quiet when its quietest window is at least this far below the median.
	// The measured gap: whole ads' edges reached -16 dB or quieter but for one; cuts inside a spot
	// stayed above -10 dB but for one.
	EdgeQuietDb = -12.0
	// InsideQuietDb: a cut goes quiet inside when a window falls this far below the median. Two
	// spots joined dipped to -25.7 dB or quieter at the join, every one.
	InsideQuietDb = -20.0
)

// MeasureCutSound reads a cut's sound from windowed levels measured from originMs. A cut with no
// windows inside it measures as silent, which is never clean.
func MeasureCutSound(levels []float64, originMs, startMs, endMs int64) CutSound {
	at := func(ms int64) int { return int((ms - originMs) / SoundWindowMs) }
	clamp := func(i int) int { return max(0, min(len(levels), i)) }
	inside := levels[clamp(at(startMs)):clamp(at(endMs))]
	if len(inside) == 0 {
		return CutSound{MedianDb: SoundSilentDb}
	}
	median := medianDb(inside)
	quietest := func(from, to int) (float64, bool) {
		from, to = clamp(from), clamp(to+1)
		if from >= to {
			return 0, false
		}
		q := levels[from]
		for _, l := range levels[from+1 : to] {
			q = min(q, l)
		}
		return q - median, true
	}
	c := CutSound{MedianDb: median}
	c.StartDb, _ = quietest(at(startMs)-1, at(startMs)+1)
	c.EndDb, _ = quietest(at(endMs)-1, at(endMs)+1)
	if mid, ok := quietest(at(startMs+InsideMarginMs), at(endMs-InsideMarginMs)); ok && endMs-startMs > 2*InsideMarginMs {
		c.InsideDb = mid
	}
	return c
}

// Clean is whether the sound says one whole spot: there is sound, both edges are quiet, and
// nothing inside is.
func (c *CutSound) Clean() bool {
	return c != nil && c.MedianDb > SoundSilentDb &&
		c.StartDb <= EdgeQuietDb && c.EndDb <= EdgeQuietDb && c.InsideDb > InsideQuietDb
}

func medianDb(xs []float64) float64 {
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	return s[len(s)/2]
}
