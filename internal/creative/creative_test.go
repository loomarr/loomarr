package creative

import (
	"math"
	"math/bits"
	"math/rand/v2"
	"testing"
)

// advert builds a fingerprint the way a real spot reads: shots of held pictures, each frame of a
// shot within a bit or two of the shot's picture, and a soundtrack hash per audio hop.
type advert struct {
	shots []uint64
	audio []uint32
}

const shotFrames = 10 // 2.5 s shots at VideoFPS

func newAdvert(r *rand.Rand, seconds int) advert {
	shots := make([]uint64, seconds*VideoFPS/shotFrames)
	for i := range shots {
		shots[i] = informative(r)
	}
	audio := make([]uint32, seconds*1000/audioHopMs)
	for i := range audio {
		audio[i] = r.Uint32() & (1<<hashBits - 1)
	}
	return advert{shots: shots, audio: audio}
}

func informative(r *rand.Rand) uint64 {
	for {
		h := r.Uint64()
		if !solid(h) {
			return h
		}
	}
}

// copyOf renders the advert as one encode would: every frame and hop with fresh noise of up to
// noiseBits flipped bits, then trimFrames dropped from the head and tail.
func (a advert) copyOf(r *rand.Rand, noiseBits, headFrames, tailFrames int) Fingerprint {
	var video []uint64
	for _, shot := range a.shots {
		for range shotFrames {
			video = append(video, flip64(r, shot, noiseBits))
		}
	}
	video = video[headFrames : len(video)-tailFrames]
	headHops := headFrames * videoFrameMs / audioHopMs
	tailHops := tailFrames * videoFrameMs / audioHopMs
	audio := make([]uint32, 0, len(a.audio))
	for _, h := range a.audio[headHops : len(a.audio)-tailHops] {
		audio = append(audio, flip32(r, h, 2))
	}
	return Fingerprint{DurationMs: int64(len(video) * videoFrameMs), Video: video, Audio: audio}
}

// cutDown keeps the given shots, in order, as a shorter spot built from the same footage.
func (a advert) cutDown(r *rand.Rand, shots ...int) Fingerprint {
	var video []uint64
	for _, s := range shots {
		for range shotFrames {
			video = append(video, flip64(r, a.shots[s], 1))
		}
	}
	return Fingerprint{DurationMs: int64(len(video) * videoFrameMs), Video: video}
}

func flip64(r *rand.Rand, h uint64, n int) uint64 {
	for range r.IntN(n + 1) {
		h ^= 1 << uint(r.IntN(64))
	}
	return h
}

func flip32(r *rand.Rand, h uint32, n int) uint32 {
	for range r.IntN(n + 1) {
		h ^= 1 << uint(r.IntN(hashBits))
	}
	return h
}

func TestCompareResolvesCopiesTrimsCutDownsAndStrangers(t *testing.T) {
	r := rand.New(rand.NewPCG(2033, 1))
	spot := newAdvert(r, 30)
	other := newAdvert(r, 30)
	master := spot.copyOf(r, 1, 0, 0)

	silent := spot.copyOf(r, 2, 0, 0)
	silent.Audio = nil
	dubbed := spot.copyOf(r, 1, 0, 0)
	dubbed.Audio = other.copyOf(r, 1, 0, 0).Audio

	for _, tc := range []struct {
		name string
		b    Fingerprint
		want Relation
	}{
		{"re-encode", spot.copyOf(r, 3, 0, 0), Same},
		{"trimmed a second at each edge", spot.copyOf(r, 2, 4, 4), Same},
		{"trimmed 1.5 s at the head only", spot.copyOf(r, 2, 6, 0), Same},
		{"a copy without its sound", silent, Same},
		// An alternate voice-over or price tag over the same footage is a separate, linked ad.
		{"the picture over another soundtrack", dubbed, Variant},
		{"15 s cut-down from its shots", spot.cutDown(r, 0, 3, 4, 7, 9, 11), Variant},
		{"cut-down too short to stand alone", spot.cutDown(r, 0), Different},
		{"a different advert", other.copyOf(r, 1, 0, 0), Different},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Compare(master, tc.b); got != tc.want {
				t.Fatalf("Compare(master, %s) = %s, want %s", tc.name, got, tc.want)
			}
			if got := Compare(tc.b, master); got != tc.want {
				t.Fatalf("Compare is not symmetric for %s: %s", tc.name, got)
			}
		})
	}
}

// Two different adverts that both open and close on black must not match through those frames.
func TestCompareIgnoresSolidFramesBothCopiesShare(t *testing.T) {
	r := rand.New(rand.NewPCG(2033, 2))
	a := newAdvert(r, 30).copyOf(r, 1, 0, 0)
	b := newAdvert(r, 30).copyOf(r, 1, 0, 0)
	for _, fp := range []*Fingerprint{&a, &b} {
		for i := range fp.Video {
			if i < len(fp.Video)/2 {
				fp.Video[i] = 0
			}
		}
		fp.Audio = nil
	}
	if got := Compare(a, b); got != Different {
		t.Fatalf("adverts sharing only black frames = %s, want different", got)
	}
}

func TestCompareRefusesAVerdictOnTooLittleEvidence(t *testing.T) {
	r := rand.New(rand.NewPCG(2033, 3))
	tiny := newAdvert(r, 1).copyOf(r, 0, 0, 0)
	if got := Compare(tiny, tiny); got != Different {
		t.Fatalf("a one-second clip against itself = %s, want different", got)
	}
	if got := Compare(Fingerprint{DurationMs: 30_000}, tiny); got != Different {
		t.Fatalf("an unmeasured clip = %s, want different", got)
	}
}

func TestHashAudioIsGainInvariant(t *testing.T) {
	// Broadband like a real soundtrack: a harmonic-rich melody over seeded noise. Pure tones leave
	// most bands empty, and the sign of an empty band's difference is quantisation noise.
	melody := func(gain float64) []int16 {
		r := rand.New(rand.NewPCG(2033, 4))
		samples := make([]int16, 5*AudioSampleRate)
		notes := []float64{220, 330, 262, 392, 196, 523, 294, 440}
		for i := range samples {
			note := notes[i/(AudioSampleRate/2)%len(notes)]
			x := 0.3 * r.NormFloat64()
			for h := 1.0; h <= 12; h++ {
				x += math.Sin(2*math.Pi*h*note*float64(i)/AudioSampleRate) / h
			}
			samples[i] = int16(gain * 6000 * x)
		}
		return samples
	}
	loud, quiet := HashAudio(melody(1)), HashAudio(melody(0.5)) // −6 dB
	if len(loud) == 0 || len(loud) != len(quiet) {
		t.Fatalf("hashed %d and %d hops", len(loud), len(quiet))
	}
	errs, compared := 0, 0
	for i := range loud {
		if loud[i]&QuietHop != 0 || quiet[i]&QuietHop != 0 {
			continue
		}
		compared++
		errs += bits.OnesCount32(loud[i] ^ quiet[i])
	}
	if compared < minComparedHops {
		t.Fatalf("only %d audible hops", compared)
	}
	if ber := float64(errs) / float64(compared*hashBits); ber > 0.05 {
		t.Fatalf("−6 dB copy bit-error rate = %.3f, want ≤ 0.05", ber)
	}
	if silent := HashAudio(make([]int16, 2*AudioSampleRate)); len(silent) == 0 || silent[len(silent)-1] != QuietHop {
		t.Fatalf("digital silence hashed as audible: %v", silent)
	}
}

func TestFFTFindsAToneInItsBin(t *testing.T) {
	re := make([]float64, AudioWindow)
	im := make([]float64, AudioWindow)
	const bin = 64
	for i := range re {
		re[i] = math.Cos(2 * math.Pi * bin * float64(i) / AudioWindow)
	}
	fft(re, im)
	peak := 0
	for k := 1; k < AudioWindow/2; k++ {
		if math.Hypot(re[k], im[k]) > math.Hypot(re[peak], im[peak]) {
			peak = k
		}
	}
	if peak != bin {
		t.Fatalf("tone peaked in bin %d, want %d", peak, bin)
	}
}
