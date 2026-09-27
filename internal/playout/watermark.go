package playout

import (
	"fmt"
	"regexp"
	"strings"
)

// The channel watermark ("bug"), #1512 phase 1d.
//
// A programme item is encoded with the channel's bug burned in; breaks (commercials, bumpers, IDs)
// are encoded without it. The bug is drawn by the family's GPU overlay filter and NEVER on the CPU
// (maintainer): a host whose GPU overlay failed the boot self-check (WatermarkCheck), or a family
// without one, airs programmes bug-free and declares it in Pipeline.Fallbacks.
//
// The image is pre-rendered by internal/watermark at its final pixel size with the opacity and
// drop shadow baked into its alpha, so the graph only decodes it once, uploads it once and blends
// it: no scale, no per-frame alpha maths. Every family's overlay blends STRAIGHT alpha, each from
// its own pixel format; the convention is measured per family on real hardware, never assumed:
//
//   - overlay_cuda: a yuva420p bug onto a yuv420p main only (spike #1532, finding 1). It also emits
//     the decoder's aligned surface (1920x1088 at 1080p) with an SPS that has no cropping, so
//     scale_cuda passthrough=0 restores the output geometry; the SPS is then byte-identical to a
//     bug-off item's (finding 2), which the self-check asserts.
//   - overlay_vaapi: a bgra bug. ffmpeg flags any alpha format VA_BLEND_PREMULTIPLIED_ALPHA, but
//     the household Arc (iHD, ffmpeg n8.1.2) blends it as straight: over a Y 71 patch a
//     premultiplied bug read 129 where 65% white is 178, and the straight bug read 178
//     (scripts/watermark-vaapi-matrix.sh). The self-check's bug-luma assertion re-proves the
//     convention on each host, so a driver that disagrees disables the bug rather than airing it
//     dim.

// Corner is where the bug sits, relative to the active picture.
type Corner string

const (
	CornerTopRight    Corner = "top-right"
	CornerTopLeft     Corner = "top-left"
	CornerBottomRight Corner = "bottom-right"
	CornerBottomLeft  Corner = "bottom-left"
)

// Corners is every corner, in the order the API lists them.
var Corners = []Corner{CornerTopRight, CornerTopLeft, CornerBottomRight, CornerBottomLeft}

// Rect is a rectangle in pixels.
type Rect struct {
	X, Y, W, H int
}

// Watermark is one programme item's bug, rendered for this channel's output.
type Watermark struct {
	// Straight is the rendered PNG, straight (non-premultiplied) alpha: final pixel size, opacity
	// and shadow baked in, even dimensions (yuva420p).
	Straight      string
	Width, Height int
	Corner        Corner
	// MarginX and MarginY are output pixels from the active picture's edges.
	MarginX, MarginY int
}

// position is the bug's top-left in output pixels: the corner of the ACTIVE picture (the source's
// measured picture area, letterbox and pillarbox bars excluded, after the fit and centring pad),
// inset by the margins, rounded down to even for 4:2:0 chroma, and clamped inside the frame.
func (w Watermark) position(src MediaFormat, out OutputProfile) (int, int) {
	a := activeInOutput(src, out)
	x, y := a.X+w.MarginX, a.Y+w.MarginY
	if w.Corner == CornerTopRight || w.Corner == CornerBottomRight {
		x = a.X + a.W - w.MarginX - w.Width
	}
	if w.Corner == CornerBottomLeft || w.Corner == CornerBottomRight {
		y = a.Y + a.H - w.MarginY - w.Height
	}
	x = even(max(0, min(x, out.Width-w.Width)))
	y = even(max(0, min(y, out.Height-w.Height)))
	return x, y
}

// activeInOutput maps the source's active picture into the output frame: the fitted picture
// (force_original_aspect_ratio=decrease), centred by the pad, and the measured active area scaled
// with it. Unknown geometry is the whole frame.
func activeInOutput(src MediaFormat, out OutputProfile) Rect {
	fw, fh, ok := fitSize(src.Width, src.Height, out.Width, out.Height)
	if !ok {
		return Rect{W: out.Width, H: out.Height}
	}
	ox, oy := (out.Width-fw)/2, (out.Height-fh)/2
	a := src.Active
	if a.W <= 0 || a.H <= 0 || a.X < 0 || a.Y < 0 || a.X+a.W > src.Width || a.Y+a.H > src.Height {
		return Rect{X: ox, Y: oy, W: fw, H: fh}
	}
	return Rect{
		X: ox + a.X*fw/src.Width, Y: oy + a.Y*fh/src.Height,
		W: a.W * fw / src.Width, H: a.H * fh / src.Height,
	}
}

// safeGraphPath is what may be spliced into a filter graph unescaped. Rendered bugs live under the
// data directory with generated names, so anything else is refused rather than escaped.
var safeGraphPath = regexp.MustCompile(`^[A-Za-z0-9/._-]+$`)

// overlay reports whether this item carries the bug on this host, declaring why not when one was
// asked for. The software and generic families never do: their filters run on the CPU.
func (b *builder) overlay() bool {
	if b.wm == nil {
		return false
	}
	why := ""
	switch {
	case b.host.Family == FamilySoftware || b.host.Family == FamilyGeneric:
		why = "no GPU overlay in the " + string(b.host.Family) + " family; never drawn on the CPU"
	case b.host.Family == FamilyVideoToolbox:
		// overlay_videotoolbox exists, but the family has never run on a real Mac; its graph comes
		// with the family's certification rather than untested here.
		why = "no VideoToolbox overlay until the family is certified on a real Mac"
	case b.out.HDR:
		// overlay_cuda, like pad_cuda, takes 8-bit frames only; a 10-bit bug stage is not built.
		why = "no 10-bit GPU overlay for an HDR10 output yet"
	case !b.host.Overlay:
		why = "the GPU overlay failed this host's self-check"
	case b.wm.Width <= 0 || b.wm.Height <= 0 || b.wm.Width%2 != 0 || b.wm.Height%2 != 0:
		why = fmt.Sprintf("bad bug size %dx%d", b.wm.Width, b.wm.Height)
	case !safeGraphPath.MatchString(b.wm.Straight):
		why = "the bug's path would need escaping in the filter graph"
	}
	if why != "" {
		b.fallback("watermark", "disabled: "+why)
		return false
	}
	return true
}

// overlaid joins the main chain with the bug: main[main]; bug[wm]; [main][wm]blend,after. The main
// chain's input and the last chain's output are the simple graph's own, so ItemArgs keeps -vf.
func overlaid(main []string, bug, blend string, after ...string) string {
	return strings.Join(main, ",") + "[main];" + bug + "[wm];[main][wm]" + strings.Join(append([]string{blend}, after...), ",")
}

func (b *builder) bugPosition() string {
	x, y := b.wm.position(b.src, b.out)
	return fmt.Sprintf("x=%d:y=%d", x, y)
}
