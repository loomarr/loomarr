package images

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"io"
	"net/http"
	"slices"
	"strings"

	// Decoders register themselves via init(). JPEG/PNG/GIF are stdlib; WebP decode comes from
	// x/image. ⚠ We deliberately do NOT register an AVIF decoder: nothing re-processes our own
	// AVIF output, and an accepted input format is an attack surface we would then have to
	// justify. Inputs are what a browser or TMDB hands us, which is this set.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"github.com/gen2brain/webp"
	"go.n16f.net/thumbhash"

	"image/jpeg"
)

// allowedInputTypes is the MIME allowlist for anything entering the service — RASTER ONLY.
//
// ⚠ SVG is excluded, and this is a security boundary rather than a format preference. The serve
// endpoint is publicly reachable for some images and returns bytes with an image content type;
// an uploaded SVG can carry <script> that executes when the URL is opened, which is stored XSS
// in Loomarr's own origin. Raster-only removes the class instead of trying to sanitize it —
// the same call the channel-icon path made, kept rather than re-argued.
var allowedInputTypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/webp": true,
	"image/gif":  true,
}

// SniffType returns the media type from a BYTE SIGNATURE, ignoring every client claim — the
// declared Content-Type, the filename, the extension, all of it.
//
// ⚠ This is THE check. Huma's multipart validator trusts the Content-Type the client declares
// and only sniffs when that header is absent, so a caller can label anything `image/png` and
// pass it. An SVG sniffs as text/xml and is refused here regardless of what it claimed to be.
func SniffType(data []byte) string {
	ct := http.DetectContentType(data)
	// DetectContentType returns e.g. "text/plain; charset=utf-8" for some types; the parameter
	// is never part of the allowlist comparison.
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	return ct
}

// AllowedInput reports whether sniffed bytes are a format we accept.
func AllowedInput(mime string) bool { return allowedInputTypes[mime] }

// Decode reads an image and reports the MIME type its BYTES say it is.
//
// The returned MIME comes from the sniff, not from the decoder's own format name, so one
// function answers both "can we decode this" and "is it on the allowlist" — two questions that
// must never be able to disagree about the same bytes.
func Decode(data []byte) (image.Image, string, error) {
	mime := SniffType(data)
	if !AllowedInput(mime) {
		return nil, mime, fmt.Errorf("images: unsupported input type %q", mime)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		// A type that sniffed as an image but will not decode is corrupt or truncated, which is
		// a different operator problem from an unsupported format — say which.
		return nil, mime, fmt.Errorf("images: decode %s: %w", mime, err)
	}
	return img, mime, nil
}

// isAnimatedWebP reports whether data is a RIFF WebP carrying animation chunks.
//
// image.Decode intentionally returns only frame zero, so successful decode cannot answer this
// question. Walking the RIFF chunk table does: ANIM declares the loop and ANMF carries a frame.
// Parse chunk boundaries rather than bytes.Contains so pixel data that happens to contain the
// four letters "ANIM" cannot turn a still into an animation record.
func isAnimatedWebP(data []byte, mime string) bool {
	if mime != "image/webp" || len(data) < 20 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return false
	}

	riffSize := uint64(binary.LittleEndian.Uint32(data[4:8])) + 8
	end := min(riffSize, uint64(len(data)))
	for offset := uint64(12); offset+8 <= end; {
		chunkSize := uint64(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		// RIFF chunks are padded to an even byte boundary. Check the addition before advancing so
		// a corrupt size cannot wrap around and turn this validation loop infinite.
		next := offset + 8 + chunkSize + chunkSize%2
		if next <= offset || next > end {
			return false
		}
		chunk := string(data[offset : offset+4])
		// ANIM's fixed payload is six bytes; ANMF's is sixteen before its nested frame data. A
		// matching fourcc with no complete payload is corrupt trailing data, not motion.
		if (chunk == "ANIM" && chunkSize >= 6) || (chunk == "ANMF" && chunkSize >= 16) {
			return true
		}
		offset = next
	}
	return false
}

// Resize scales img down to exactly width, preserving aspect ratio.
//
// ⚠ **Stepped, not one-shot, and that is a measured cost rather than a style preference.**
// A kernel interpolator's cost scales with BOTH source and destination extent, so resampling a
// 2000px original straight to 154px with CatmullRom does far more work than halving first. This
// halves with a cheap filter until within 2× of the target — box-halving is close to ideal for
// downscale, since it is essentially averaging — then spends the one expensive high-quality pass
// on the final step, where it is actually visible.
//
// Upscaling is refused by returning the source untouched: every ladder rung is smaller than its
// original by construction, so a request to enlarge means the caller got the ladder wrong, and
// inventing pixels would hide that.
func Resize(img image.Image, width int) image.Image {
	b := img.Bounds()
	if width <= 0 || b.Dx() <= width {
		return img
	}

	src := img
	// Halve while the source is still at least twice the target.
	for src.Bounds().Dx() >= width*2 {
		sb := src.Bounds()
		half := image.NewRGBA(image.Rect(0, 0, sb.Dx()/2, sb.Dy()/2))
		xdraw.ApproxBiLinear.Scale(half, half.Bounds(), src, sb, draw.Over, nil)
		src = half
	}

	sb := src.Bounds()
	height := int(float64(width) * float64(sb.Dy()) / float64(sb.Dx()))
	if height < 1 {
		height = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, sb, draw.Over, nil)
	return dst
}

// ResizeLadder produces every requested width from one source, reusing each result as the source
// for the next rung down.
//
// ⚠ **Not a loop over Resize, and the difference is not marginal.** Calling Resize(src, w) once
// per rung re-walks the halving chain from the full-size original every time: for a 2000×3000
// poster and the five-rung poster ladder that is ~15 redundant halvings of large buffers.
// Measured on an i9-12900K (BenchmarkResizeLadder{,Naive}): **231ms naive vs 100ms stepped**,
// a 2.3× difference for byte-comparable output. §22 states the rule; this is where it is
// actually enforced, because a rule that lives only in prose gets re-broken by the next caller
// who writes the obvious loop.
//
// Widths are sorted descending internally, so callers may pass a ladder in any order.
func ResizeLadder(img image.Image, widths []int) map[int]image.Image {
	out := make(map[int]image.Image, len(widths))
	if len(widths) == 0 {
		return out
	}

	desc := append([]int(nil), widths...)
	slices.SortFunc(desc, func(a, b int) int { return b - a })

	src := img
	for _, w := range desc {
		// A rung at or above the original is served by the original itself — the same
		// no-upscale rule Resize applies, kept consistent so a ladder and a single resize can
		// never disagree about what "wider than the source" means.
		if src.Bounds().Dx() <= w {
			out[w] = src
			continue
		}
		r := Resize(src, w)
		out[w] = r
		// The next rung is smaller, so it starts from this result rather than the original.
		src = r
	}
	return out
}

// webpQuality is the lossy quality for WebP renditions.
//
// 80 rather than the library's default 75: these are posters and stills that a person looks at
// directly on a large tile, not thumbnails in a feed. The extra bytes are cheap next to the AVIF
// rendition that most clients will actually take.
const webpQuality = 80

// webpMethod is the compression effort, 0–6. 4 is the library default and the knee of the
// curve — 6 costs roughly twice the CPU for low single-digit percent of size, which is a bad
// trade on a lazily-generated rendition sitting in a request.
const webpMethod = 4

// EncodeWebP writes img as lossy WebP.
//
// ⚠ The binary MUST be built with `-tags nodynamic`. Without it this library will dlopen a
// system libwebp when one happens to be present, so the same source image encodes to different
// bytes depending on the base layer — which would make our content-addressed derivative paths
// non-reproducible across the two architectures we ship.
func EncodeWebP(w io.Writer, img image.Image) error {
	return webp.Encode(w, img, webp.Options{Quality: webpQuality, Method: webpMethod})
}

// jpegQuality is deliberately higher than the WebP setting. JPEG is the fallback taken only by
// clients that support neither AVIF nor WebP, so it is already the least efficient path; making
// it visibly worse as well would punish exactly the oldest, slowest devices twice.
const jpegQuality = 85

// EncodeJPEG writes img as baseline JPEG using the standard library.
//
// ⚠ Stdlib rather than jpegli, reversing the §14 note written before measurement. jpegli's ~20%
// density win is real, but it arrives on a wazero WASM runtime — a second, heavier interpreter
// in the binary — to improve the rendition that the fewest clients take and that is already the
// compatibility floor rather than the quality target. If JPEG ever becomes a primary format that
// trade flips; today it does not earn a runtime.
func EncodeJPEG(w io.Writer, img image.Image) error {
	return jpeg.Encode(w, img, &jpeg.Options{Quality: jpegQuality})
}

// Encode dispatches to the right in-process encoder.
//
// ⚠ AVIF is absent ON PURPOSE and returns an error if asked for. It costs 300–1200ms per image,
// which is an order of magnitude past WebP, so it is produced by a background job through ffmpeg
// (see avif.go) and never inline. Routing it here would put that cost on a request the first
// time anyone forgot.
func Encode(w io.Writer, img image.Image, f Format) error {
	switch f {
	case FormatWebP:
		return EncodeWebP(w, img)
	case FormatJPEG:
		return EncodeJPEG(w, img)
	case FormatAVIF:
		return fmt.Errorf("images: AVIF is produced by the images-avif job, not inline")
	}
	return fmt.Errorf("images: unknown format %q", f)
}

// Placeholder returns a base64 ThumbHash — the ~25-byte blur preview stored on the row and
// rendered as an LQIP while the real image loads.
//
// ThumbHash rather than BlurHash because it carries ALPHA. Channel logos are routinely
// transparent PNGs, and BlurHash renders transparency as black, so choosing it would have meant
// shipping black placeholder boxes for exactly the images most likely to be transparent — or
// adding a composite-onto-dominant-colour step purely to work around the format.
//
// ⚠ ThumbHash encodes at most 100×100; anything larger wastes work for an identical result, so
// the caller's image is pre-shrunk here rather than at every call site.
func Placeholder(img image.Image) string {
	small := Resize(img, 100)
	return base64.StdEncoding.EncodeToString(thumbhash.EncodeImage(small))
}

// DominantHex returns the image's average colour as "#rrggbb", used to tint a skeleton before
// even the placeholder has decoded.
//
// Computed by resampling the whole image to a single pixel, which IS the average — cheaper and
// less arbitrary than sampling a grid, and it cannot pick an unrepresentative corner.
func DominantHex(img image.Image) string {
	one := image.NewRGBA(image.Rect(0, 0, 1, 1))
	xdraw.CatmullRom.Scale(one, one.Bounds(), img, img.Bounds(), draw.Over, nil)
	c := color.NRGBAModel.Convert(one.At(0, 0)).(color.NRGBA)
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}
