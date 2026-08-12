package mediatools

import (
	"bytes"
	"image"
	// Registers the JPEG decoder for image.Decode — Keyframes hands us JPEG bytes.
	_ "image/jpeg"
)

// Frame heuristics (§10 V44): deterministic, LLM-free era HINTS read off a clip's
// decoded keyframes. Two signals, both cheap and both computed from the same JPEGs
// the vision tier would send a model — so this tier runs FIRST and for free:
//
//   - black-and-white: a monochrome transfer (near-zero colour, every pixel a grey)
//     is a strong "this is old" signal a modern re-broadcast almost never has.
//   - aspect ratio: a 4:3 frame is pre-widescreen; 16:9 is not.
//
// ⚠ **These are HINTS, never facts, and never override a grounded tag.** They seed
// SuggestedEra for a clip that has NO other era signal — exactly the demotion the
// era-grounding rule (tag.go validateTags) applies to an LLM-proposed year: a
// human confirms it by PATCHing Era, which clears the suggestion. They are the
// weakest tier for a reason. A B&W 4:3 spot is *probably* pre-1970s, but a modern
// stylised advert can be shot B&W on purpose and a 4:3 upload can be a cropped
// export — so the mapping is deliberately COARSE (a decade sentinel, not a year)
// and only ever produces a suggestion the operator can reject in one click.
//
// The whole point of the tier is that it needs no model and no network: a clip
// nobody narrates and no text describes still has pixels, and a monochrome 4:3
// frame is signal a keyword search will never find.

// FrameHint is what the heuristic tier learns about a clip's frames. Every field
// is a measurement, not a conclusion — SuggestedEraFrom turns them into a hint.
type FrameHint struct {
	// BlackAndWhite is true when the sampled frames are monochrome: their per-pixel
	// colour spread is at or below bwChromaThreshold. A colour clip trips this only
	// if it happens to be an all-grey frame, which is why several frames are sampled
	// and the clip counts as B&W only if MOST of them are (see AnalyzeFrames).
	BlackAndWhite bool
	// AspectRatio is width/height of the sampled frames (4:3 ≈ 1.333, 16:9 ≈ 1.778).
	// 0 when no frame decoded — a clip with no video, which yields no hint at all.
	AspectRatio float64
	// Frames is how many keyframes actually decoded, so a caller can tell "measured
	// and found nothing" (Frames>0, no hint) from "had nothing to measure" (0).
	Frames int
}

// bwChromaThreshold is the per-pixel colour-spread ceiling below which a frame is
// called monochrome. A truly grey pixel has R==G==B, so max(R,G,B)-min(R,G,B)==0;
// JPEG's lossy chroma subsampling leaves a few points of spread even on a genuine
// black-and-white transfer, so the ceiling is a small tolerance rather than zero.
//
// ⚠ Measured against JPEG artefacts, not chosen for elegance. 12 (of 255) admits
// the ringing a real B&W scan shows around high-contrast edges while still
// rejecting any frame with actual colour in it — a skin tone or a red logo spreads
// far past this. It is a per-pixel mean, so one coloured object cannot by itself
// flip a mostly-grey frame; the FRAME is B&W only if the average pixel is grey.
const bwChromaThreshold = 12.0

// bwFrameFraction is how many of the sampled frames must read monochrome for the
// CLIP to count as black-and-white. A single grey frame (a fade, a title card on
// black) is common in a colour advert, so one is not enough — a majority is. Set
// just above half so a 2-of-3 sample qualifies but a 1-of-2 does not.
const bwFrameFraction = 0.6

// Era-hint sentinels: the era the two signals combine into. Coarse ON PURPOSE
// (see the file comment) — a decade, and only when the frames genuinely support it.
const (
	// hintPre1970s: black-and-white AND 4:3. Colour broadcast was widespread in the
	// US by the early 1970s, so a monochrome transfer that is also pre-widescreen is
	// the strongest "very old" combination the frames can show. A decade sentinel,
	// not a year — the operator narrows it, or rejects it.
	hintPre1970s = 1960
)

// AnalyzeFrames measures the heuristic signals across decoded keyframes (the JPEG
// bytes MediaTools.Keyframes returns). It decodes what it can and IGNORES frames it
// cannot — a corrupt frame in the middle of a clip must not sink the whole hint.
//
// ⚠ Pure and deterministic: same frames in, same measurement out, no seed, no
// clock, no network. That is what lets it run in the free tier and be table-tested.
func AnalyzeFrames(jpegs [][]byte) FrameHint {
	var (
		decoded int
		bwCount int
		arSum   float64
	)
	for _, raw := range jpegs {
		img, _, err := image.Decode(bytes.NewReader(raw))
		if err != nil {
			// A single unreadable frame is skipped, not fatal — see the doc comment.
			continue
		}
		b := img.Bounds()
		w, h := b.Dx(), b.Dy()
		if w == 0 || h == 0 {
			continue
		}
		decoded++
		arSum += float64(w) / float64(h)
		if frameIsMonochrome(img) {
			bwCount++
		}
	}
	var hint FrameHint
	hint.Frames = decoded
	if decoded == 0 {
		return hint
	}
	hint.AspectRatio = arSum / float64(decoded)
	// A MAJORITY of frames must be grey for the clip to read as B&W — one grey fade
	// in a colour spot is not the clip being monochrome.
	hint.BlackAndWhite = float64(bwCount) >= bwFrameFraction*float64(decoded)
	return hint
}

// frameIsMonochrome reports whether a decoded frame's average pixel carries
// essentially no colour — the per-pixel mean of max(R,G,B)-min(R,G,B) is at or
// below bwChromaThreshold.
//
// ⚠ Samples a grid rather than every pixel. A 320-wide keyframe is ~70k pixels and
// this runs per frame per clip in a background job; a fixed grid of ~gridSteps²
// samples is the same measurement (the colourfulness of a frame is not a
// high-frequency property) at a bounded cost that does not grow with resolution.
func frameIsMonochrome(img image.Image) bool {
	const gridSteps = 32 // ≤1024 samples per frame regardless of its size.
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return false
	}
	stepX, stepY := w/gridSteps, h/gridSteps
	if stepX < 1 {
		stepX = 1
	}
	if stepY < 1 {
		stepY = 1
	}
	var (
		spreadSum float64
		samples   int
	)
	for y := b.Min.Y; y < b.Max.Y; y += stepY {
		for x := b.Min.X; x < b.Max.X; x += stepX {
			// RGBA() returns 16-bit premultiplied components; shift to 8-bit so the
			// threshold reads in the familiar 0–255 range.
			r, g, bl, _ := img.At(x, y).RGBA()
			r8, g8, b8 := float64(r>>8), float64(g>>8), float64(bl>>8)
			maxc := r8
			if g8 > maxc {
				maxc = g8
			}
			if b8 > maxc {
				maxc = b8
			}
			minc := r8
			if g8 < minc {
				minc = g8
			}
			if b8 < minc {
				minc = b8
			}
			spreadSum += maxc - minc
			samples++
		}
	}
	if samples == 0 {
		return false
	}
	return spreadSum/float64(samples) <= bwChromaThreshold
}

// isFourThree reports whether an aspect ratio is 4:3 (≈1.333) rather than 16:9
// (≈1.778). The midpoint between the two standard ratios is ~1.555; anything below
// it is treated as the narrower 4:3 family (which also catches 5:4 and 1:1 title
// cards), anything above as widescreen. A tolerance band rather than an exact
// match, because a scaled/letterboxed export lands a few hundredths off nominal.
func isFourThree(ar float64) bool {
	if ar <= 0 {
		return false
	}
	const fourThreeToSixteenNineMidpoint = 1.555
	return ar < fourThreeToSixteenNineMidpoint
}

// splitJPEGs splits a stream of concatenated JPEGs (ffmpeg's image2pipe output)
// into individual frames on the SOI marker (0xFF 0xD8). Each element is one
// decodable JPEG; a stream with no marker (no video decoded) yields nothing.
//
// ⚠ Splitting on SOI rather than parsing EOI is deliberate and correct here: an
// EOI byte pair (0xFF 0xD9) can legitimately appear inside entropy-coded scan data,
// so scanning for it risks cutting a frame in half. SOI only ever appears at a true
// frame start in mjpeg's output, so "everything from one SOI up to the next" is
// always exactly one frame.
func splitJPEGs(stream []byte) [][]byte {
	soi := []byte{0xFF, 0xD8}
	var frames [][]byte
	for {
		start := bytes.Index(stream, soi)
		if start < 0 {
			break
		}
		// The next frame begins at the following SOI; search past THIS one's marker.
		next := bytes.Index(stream[start+2:], soi)
		if next < 0 {
			frames = append(frames, stream[start:])
			break
		}
		end := start + 2 + next
		frames = append(frames, stream[start:end])
		stream = stream[end:]
	}
	return frames
}

// SuggestedEraFrom turns a FrameHint into a SuggestedEra sentinel, or 0 for no hint.
//
// ⚠ **This is the ONLY place the measurements become a suggestion, and it produces
// ONLY a suggestion — never a grounded Era.** The caller writes the result to
// Clip.SuggestedEra (the operator-confirms field), exactly as validateTags demotes
// an ungrounded LLM year. Matching never reads it. Conservative by construction: it
// fires only on the strongest combination (B&W AND 4:3) and returns a decade
// sentinel, so the worst case is an easily-dismissed "pre-1970s?" prompt on a clip
// that had no era signal at all — never a wrong fact in the catalog.
//
// The caller is responsible for the "no other era signal" precondition (§10 V44):
// this must seed a suggestion only for a clip with no grounded Era and no other
// SuggestedEra, so a frame hint never overwrites a stronger one.
func SuggestedEraFrom(hint FrameHint) int {
	if hint.Frames == 0 {
		return 0
	}
	// Both signals required. B&W alone can be an artistic choice in any decade, and
	// 4:3 alone is far too common (every SD upload) to date a clip. Together they are
	// the one combination that reliably means "old broadcast".
	if hint.BlackAndWhite && isFourThree(hint.AspectRatio) {
		return hintPre1970s
	}
	return 0
}
