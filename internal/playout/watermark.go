package playout

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// The channel watermark ("bug"), #1512 phase 1d.
//
// A programme item is encoded with the channel's bug burned in; breaks (commercials, bumpers, IDs)
// are encoded without it. The bug is blended on the GPU and NEVER on the CPU (maintainer): a host
// whose GPU overlay failed the boot self-check (WatermarkCheck), or a family without one, airs
// programmes bug-free and declares it in Pipeline.Fallbacks. The programme's frames never leave the
// GPU for it; only the bug's own single frame is prepared on the CPU, once, before its upload.
//
// The image is pre-rendered by internal/watermark at its final pixel size with the opacity and
// drop shadow baked into its alpha, so the graph only decodes it once, uploads it once and blends
// it: no per-frame scale or alpha maths. Every family blends STRAIGHT alpha, each from its own
// pixel format; the convention is measured per family on real hardware, never assumed:
//
//   - overlay_cuda: a yuva420p bug onto a yuv420p main only (spike #1532, finding 1). It also emits
//     the decoder's aligned surface (1920x1088 at 1080p) with an SPS that has no cropping, so
//     scale_cuda passthrough=0 restores the output geometry; the SPS is then byte-identical to a
//     bug-off item's (finding 2), which the self-check asserts.
//   - VAAPI: Loomarr's own OpenCL kernel (watermark.cl) through ffmpeg's program_opencl, on the
//     NV12 surface mapped from VAAPI and back, zero-copy (#1613). No stock filter works on the Arc:
//     overlay_vaapi draws a correct bug at ~30 fps (798 ms per 1 s fragment, #1595), overlay_opencl
//     reads its alpha from the bug's V plane over an NV12 main, and overlay_qsv and overlay_vulkan
//     cannot map the surface. The kernel is a straight-alpha mix per plane, so the bug and its
//     alpha are prepared once as full NV12 frames (kernelBug); against ffmpeg's CPU overlay it lands
//     within ±1 in Y and inside the bug's chroma (TestLive_BlendKernelMatchesTheCPUOverlay).
//
// Neither family needs a range step: swscale converts the bug to limited range (white at 235), and
// the matrix measured overlay_cuda's blend at alpha 0.651 and white 234.8. The self-check's bug
// assertions (coded luma and chroma, ±6) re-prove each family's convention, range and colour on
// every host, so a driver that disagrees disables the bug rather than airing it dim, super-white
// or tinted.

// blendKernel is the VAAPI family's blend (watermark.cl), shipped inside the binary.
//
//go:embed watermark.cl
var blendKernel string

// WriteBlendKernel writes the blend kernel into dir, named by its content, and returns its path:
// program_opencl reads its source from a file. A kernel already there is left alone, so a running
// ffmpeg never reads a half-written file, and a new release's kernel never shadows an old one.
func WriteBlendKernel(dir string) (string, error) {
	sum := sha256.Sum256([]byte(blendKernel))
	path := filepath.Join(dir, "blend-bug-"+hex.EncodeToString(sum[:4])+".cl")
	if b, err := os.ReadFile(path); err == nil && string(b) == blendKernel {
		return path, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, ".blend-bug-*")
	if err != nil {
		return "", err
	}
	if _, err := tmp.WriteString(blendKernel); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return "", err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return "", err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
		return "", err
	}
	return path, nil
}

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
	Straight string
	// Kernel is the VAAPI family's blend kernel on disk (WriteBlendKernel); without it the VAAPI
	// family airs the programme bug-free.
	Kernel        string
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
	case b.host.Family == FamilyVAAPI && b.wm.Kernel == "":
		why = "no blend kernel file for the VAAPI overlay"
	case b.host.Family == FamilyVAAPI && !safeGraphPath.MatchString(b.wm.Kernel):
		why = "the blend kernel's path would need escaping in the filter graph"
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

// bugPTS is where the kernel's bug inputs sit in time: before any programme frame. program_opencl's
// framesync holds a one-frame input forever after its EOF, but drops every main frame earlier than
// an input's first frame (before=EXT_STOP), and a seek can start the main a few frames below 0.
const bugPTS = "-86400/TB"

// kernelBug is the VAAPI kernel's bug inputs from the bug's one decoded frame, on the CPU once
// (never per programme frame), each uploaded to OpenCL once:
//
//   - [wm], the bug placed on a transparent frame of the output size (the kernel takes no position),
//     in NV12, BT.709 limited range (swscale converts RGB with BT.601 unless told);
//   - [wa], its alpha, NV12-shaped: Y is the alpha, and both chroma channels are the 2x2 mean, the
//     chroma alpha ffmpeg's CPU overlay uses for 4:2:0.
//
// Each is labelled and re-timed like the main (program_opencl negotiates colour in common, and its
// output keeps input 0's time base but carries framesync's pts, which an AVTB-only graph makes equal).
func (b *builder) kernelBug() string {
	x, y := b.wm.position(b.src, b.out)
	upload := conformColour + ",hwupload=derive_device=opencl,settb=AVTB,setpts=" + bugPTS
	return fmt.Sprintf("movie=filename=%s,format=rgba,pad=w=%d:h=%d:x=%d:y=%d:color=black@0,split[wmc][wma];",
		b.wm.Straight, b.out.Width, b.out.Height, x, y) +
		"[wmc]scale=out_color_matrix=bt709:out_range=tv,format=nv12," + upload + "[wm];" +
		"[wma]alphaextract,format=gray,split[wmay][wmah];[wmah]scale=w=iw/2:h=ih/2:flags=area,split[wmau][wmav];" +
		"[wmay][wmau][wmav]mergeplanes=map0s=0:map0p=0:map1s=1:map1p=0:map2s=2:map2p=0:format=yuv420p,format=nv12," + upload + "[wa]"
}

func (b *builder) bugPosition() string {
	x, y := b.wm.position(b.src, b.out)
	return fmt.Sprintf("x=%d:y=%d", x, y)
}
