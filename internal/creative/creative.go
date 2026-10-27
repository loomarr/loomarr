// Package creative decides whether two filler clips are the same advert (#2033). Identity is the
// creative, never the file or the title: a re-upload, a re-encode, or the same spot cut from a
// different broadcast is one ad, and a 15 s cut-down of a 30 s spot is a separate ad linked to it.
//
// A fingerprint is two sequences measured from the playback file: a dHash per video frame sampled
// at VideoFPS, and a 15-bit band-energy hash per audio hop. Compare aligns them over a small window
// so a second or so trimmed at either edge still matches. Everything here is pure; the frames and
// samples come from mediatools.
package creative

import (
	"math"
	"math/bits"
)

// Algorithm versions every choice that makes a stored fingerprint comparable. A change to sampling,
// hashing or band layout gets a new key, and stored fingerprints under the old key are recomputed
// rather than read under new semantics.
const Algorithm = "creative-v1-video4fps-gray9x8-audio8k-hk15"

const (
	// VideoFPS is the frame sampling rate. At 4 fps an edge trim lands at most 125 ms from a sampled
	// frame, which a typical advert shot does not change across.
	VideoFPS = 4
	// AudioSampleRate, AudioWindow and AudioHop shape the audio hash: 128 ms windows every 64 ms.
	AudioSampleRate = 8000
	AudioWindow     = 1024
	AudioHop        = 512

	videoFrameMs = 1000 / VideoFPS
	audioHopMs   = 1000 * AudioHop / AudioSampleRate
)

// The thresholds were set against the identity corpus (testkit.FillerIdentityMedia) and the dHash
// separation measured for §10 V34: a re-encoded copy reads about 1 bit per frame, a different
// advert 28-32.
const (
	// SameMaxDurationGapMs bounds how different two copies' lengths may be. A cut-down fails it.
	SameMaxDurationGapMs = 2500
	// MaxEdgeTrimMs is how far one copy's content may be shifted against the other's.
	MaxEdgeTrimMs = 1500
	// Unrelated pictures differ in about half of a dHash's 64 bits, while heavy compression spreads
	// one picture's frames well below that: a CRF 38 re-encode at 160x90 reads 8-20. A copy compares
	// each frame once, aligned, where a random pair reaches 20 bits about 2 times in 1,000. A variant
	// searches every frame of the longer clip for each of the shorter's, which multiplies those odds
	// by its length, so it takes the stricter bar, which a random pair reaches about 3 in 100,000.
	sameFrameBits    = 20
	variantFrameBits = 16
	// sameMatchedFraction is the share of compared frames that must match for a copy.
	sameMatchedFraction = 0.85
	// sameMeanBits bounds the mean distance over the compared frames for a copy.
	sameMeanBits = 12.0
	// sameMaxAudioBER bounds the audio bit-error rate for a copy. Unrelated audio reads near 0.5.
	sameMaxAudioBER = 0.30
	// VariantMinDurationMs is the shortest clip that can be a linked variant.
	VariantMinDurationMs = 5000
	// variantMatchedFraction is the share of the shorter clip's frames found in the longer one.
	variantMatchedFraction = 0.50
	// minComparedFrames is the fewest informative frame pairs a verdict may rest on. Fewer is
	// Different: an undecidable pair must never merge two ads.
	minComparedFrames = 8
	// minComparedHops is the fewest audible audio hops an audio verdict may rest on; fewer leaves
	// the decision to the picture.
	minComparedHops = 32
	// solidBits bounds a near-solid frame (black, white, a flat slate): its hash has almost every
	// or almost no bit set, and two such frames match whatever adverts they came from.
	solidBits = 4
)

// QuietHop marks an audio hop too quiet to hash. Its other bits are meaningless.
const QuietHop = uint32(1) << 31

// Fingerprint is one clip's measured creative. Audio is nil for a clip without sound.
type Fingerprint struct {
	DurationMs int64    `json:"durationMs"`
	Video      []uint64 `json:"video"`
	Audio      []uint32 `json:"audio,omitempty"`
}

// Relation is how two clips' creatives relate.
type Relation string

const (
	// Same is one advert: one copy survives and the others are its inferior copies.
	Same Relation = "same"
	// Variant is a separate advert built from the same footage, such as a cut-down. Variants are
	// linked so a break never carries two of them.
	Variant Relation = "variant"
	// Different is unrelated, or too little evidence to say otherwise.
	Different Relation = "different"
)

// Compare relates two fingerprints. It is symmetric.
func Compare(a, b Fingerprint) Relation {
	if len(a.Video) == 0 || len(b.Video) == 0 {
		return Different
	}
	gap := a.DurationMs - b.DurationMs
	if gap < 0 {
		gap = -gap
	}
	if gap <= SameMaxDurationGapMs && sameVideo(a.Video, b.Video) && sameAudio(a.Audio, b.Audio) {
		return Same
	}
	shorter, longer := a, b
	if shorter.DurationMs > longer.DurationMs {
		shorter, longer = longer, shorter
	}
	if shorter.DurationMs >= VariantMinDurationMs && containedVideo(shorter.Video, longer.Video) {
		return Variant
	}
	return Different
}

// sameVideo reports whether some alignment within MaxEdgeTrimMs makes the two sequences one
// picture: most compared frames match and the mean distance is low.
func sameVideo(a, b []uint64) bool {
	maxShift := MaxEdgeTrimMs / videoFrameMs
	for shift := -maxShift; shift <= maxShift; shift++ {
		compared, matched, total := 0, 0, 0
		for i := range a {
			j := i + shift
			if j < 0 || j >= len(b) || (solid(a[i]) && solid(b[j])) {
				continue
			}
			d := bits.OnesCount64(a[i] ^ b[j])
			compared++
			total += d
			if d <= sameFrameBits {
				matched++
			}
		}
		if compared >= minComparedFrames &&
			float64(matched) >= sameMatchedFraction*float64(compared) &&
			float64(total) <= sameMeanBits*float64(compared) {
			return true
		}
	}
	return false
}

// sameAudio reports whether the soundtracks agree at their best alignment. It leaves the decision
// to the picture when either clip is silent or too little audio is audible in both; a copy whose
// sound is missing is still the same advert.
func sameAudio(a, b []uint32) bool {
	maxShift := (MaxEdgeTrimMs + videoFrameMs) / audioHopMs
	decided := false
	for shift := -maxShift; shift <= maxShift; shift++ {
		compared, errs := 0, 0
		for i := range a {
			j := i + shift
			if j < 0 || j >= len(b) || a[i]&QuietHop != 0 || b[j]&QuietHop != 0 {
				continue
			}
			compared++
			errs += bits.OnesCount32(a[i] ^ b[j])
		}
		if compared < minComparedHops {
			continue
		}
		decided = true
		if float64(errs) <= sameMaxAudioBER*float64(compared*hashBits) {
			return true
		}
	}
	return !decided
}

// containedVideo reports whether most of the shorter clip's informative frames appear somewhere in
// the longer one, in any order: a cut-down reuses the original's shots, re-sequenced.
func containedVideo(shorter, longer []uint64) bool {
	compared, matched := 0, 0
	for _, f := range shorter {
		if solid(f) {
			continue
		}
		compared++
		for _, g := range longer {
			if bits.OnesCount64(f^g) <= variantFrameBits {
				matched++
				break
			}
		}
	}
	return compared >= minComparedFrames && float64(matched) >= variantMatchedFraction*float64(compared)
}

func solid(h uint64) bool {
	n := bits.OnesCount64(h)
	return n <= solidBits || n >= 64-solidBits
}

// HashFrame hashes one 9x8 grayscale frame to 64 bits: a bit is set where a pixel is brighter than
// its right neighbour.
func HashFrame(frame []byte) uint64 {
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

// HashFrames hashes a frame sequence, one hash per sampled frame.
func HashFrames(frames [][]byte) []uint64 {
	out := make([]uint64, len(frames))
	for i, f := range frames {
		out[i] = HashFrame(f)
	}
	return out
}

const (
	hashBands    = 16
	hashBits     = hashBands - 1
	bandLowHz    = 300.0
	bandHighHz   = 3000.0
	quietEnergy  = 1e-6
	sampleToUnit = 1.0 / 32768.0
)

// HashAudio hashes mono 16-bit samples at AudioSampleRate: per hop, bit m is set when the energy
// difference between bands m and m+1 grew since the previous hop (Haitsma-Kalker). Comparing signs
// of differences makes the hash independent of gain, so a quieter copy hashes the same.
func HashAudio(samples []int16) []uint32 {
	if len(samples) < AudioWindow {
		return nil
	}
	edges := bandEdges()
	window := hann()
	re := make([]float64, AudioWindow)
	im := make([]float64, AudioWindow)
	var out []uint32
	var prev [hashBands]float64
	havePrev := false
	for start := 0; start+AudioWindow <= len(samples); start += AudioHop {
		for i := range re {
			re[i] = float64(samples[start+i]) * sampleToUnit * window[i]
			im[i] = 0
		}
		fft(re, im)
		var energy [hashBands]float64
		total := 0.0
		for band := 0; band < hashBands; band++ {
			for k := edges[band]; k < edges[band+1]; k++ {
				energy[band] += re[k]*re[k] + im[k]*im[k]
			}
			total += energy[band]
		}
		if total < quietEnergy {
			out = append(out, QuietHop)
			havePrev = false
			continue
		}
		if havePrev {
			var h uint32
			for m := 0; m < hashBits; m++ {
				if (energy[m]-energy[m+1])-(prev[m]-prev[m+1]) > 0 {
					h |= 1 << uint(m)
				}
			}
			out = append(out, h)
		} else {
			out = append(out, QuietHop)
		}
		prev, havePrev = energy, true
	}
	return out
}

// bandEdges are FFT bin boundaries for hashBands log-spaced bands between bandLowHz and bandHighHz.
func bandEdges() [hashBands + 1]int {
	var edges [hashBands + 1]int
	binHz := float64(AudioSampleRate) / AudioWindow
	ratio := math.Pow(bandHighHz/bandLowHz, 1.0/hashBands)
	for i := range edges {
		edges[i] = int(math.Round(bandLowHz * math.Pow(ratio, float64(i)) / binHz))
	}
	for i := 1; i < len(edges); i++ {
		if edges[i] <= edges[i-1] {
			edges[i] = edges[i-1] + 1
		}
	}
	return edges
}

func hann() []float64 {
	w := make([]float64, AudioWindow)
	for i := range w {
		w[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(AudioWindow-1))
	}
	return w
}

// fft is an in-place iterative radix-2 transform; len(re) is a power of two.
func fft(re, im []float64) {
	n := len(re)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			re[i], re[j] = re[j], re[i]
			im[i], im[j] = im[j], im[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		angle := -2 * math.Pi / float64(size)
		wr, wi := math.Cos(angle), math.Sin(angle)
		for start := 0; start < n; start += size {
			cr, ci := 1.0, 0.0
			for k := 0; k < size/2; k++ {
				a, b := start+k, start+k+size/2
				tr := re[b]*cr - im[b]*ci
				ti := re[b]*ci + im[b]*cr
				re[b], im[b] = re[a]-tr, im[a]-ti
				re[a], im[a] = re[a]+tr, im[a]+ti
				cr, ci = cr*wr-ci*wi, cr*wi+ci*wr
			}
		}
	}
}
