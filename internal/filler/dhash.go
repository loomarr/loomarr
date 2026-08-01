package filler

import "math/bits"

// dHash duplicate detection (§10 V34): the same advert recurs across
// compilations, and re-encoding/downscaling changes every byte while changing
// almost none of the STRUCTURE. Measured (plan §6.4): mean per-frame Hamming
// distance 1.1 for a re-encoded, downscaled duplicate vs 27.6–32.2 for
// different adverts — a 25× separation, so any threshold in the teens works.

// DupHashThreshold is the mean per-frame Hamming distance at or under which two
// spans are the same advert. Deliberately in the teens' low end: the measured
// margin makes false duplicates the failure that matters (a false duplicate
// hides a genuinely new advert behind a "already have it" flag).
const DupHashThreshold = 12.0

// dHashFrame hashes one 9x8 grayscale frame to 64 bits: bit set when the left
// pixel is brighter than its right neighbour. Pure — the frames come from
// MediaTools.GrayFrames and tests construct them directly.
func dHashFrame(frame []byte) uint64 {
	var h uint64
	bit := uint(0)
	for row := 0; row < 8; row++ {
		for col := 0; col < 8; col++ {
			if frame[row*9+col] > frame[row*9+col+1] {
				h |= 1 << bit
			}
			bit++
		}
	}
	return h
}

// dHashFrames hashes a frame sequence (one hash per sampled frame).
func dHashFrames(frames [][]byte) []uint64 {
	out := make([]uint64, len(frames))
	for i, f := range frames {
		out[i] = dHashFrame(f)
	}
	return out
}

// meanHamming is the average per-frame Hamming distance over the frames two
// spans have in common. Two encodes of one advert differ in length by a frame
// or two at the tail (the 1/3fps sample phase shifts), so the comparison runs
// over the shorter sequence rather than demanding equal lengths. ok=false when
// either side has no frames — an undecodable span is NOT a duplicate of
// anything (that would be a silent drop, the failure the flag exists to avoid).
func meanHamming(a, b []uint64) (mean float64, ok bool) {
	n := min(len(a), len(b))
	if n == 0 {
		return 0, false
	}
	total := 0
	for i := 0; i < n; i++ {
		total += bits.OnesCount64(a[i] ^ b[i])
	}
	return float64(total) / float64(n), true
}
