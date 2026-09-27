// Package watermark renders a channel's bug (#1512 phase 1d): the small mark burned into a
// channel's programmes by the GPU overlay (internal/playout, watermark.go).
//
// Rendering is deterministic and happens once per channel, output size and source image: shapes are
// drawn supersampled from bundled data (Geist Bold, SIL OFL 1.1, fonts/OFL.txt) and reduced, with
// no system fonts and no randomness. The LLM may choose a callsign's TEXT; it never draws.
//
// The look is the maintainer's, approved from real-encoder sample sheets (PR #1532, 2026-09-26),
// and this file ports the sample renderer exactly:
//
//   - a white silhouette: RGB is white and only alpha carries the shape, so the 4:2:0 overlay
//     cannot fringe (white has neutral chroma);
//   - equal visual weight: every bug covers the same area, 2·H², H = frame height × size (6%),
//     so a wide mark gets shorter and a square one reaches H (never taller);
//   - 65% opacity baked into alpha, over a soft black drop shadow at 35% of that opacity, offset
//     H/40 and blurred H/30, on a canvas padded 12% of the mark's height.
package watermark

import (
	_ "embed"
	"errors"
	"image"
	"image/color"
	"math"
	"strings"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// The approved default look.
const (
	DefaultSize    = 0.06 // of the frame height
	DefaultOpacity = 0.65
	DefaultMargin  = 0.05 // of the frame width (x) and height (y)
)

//go:embed fonts/Geist-Bold.ttf
var geistBold []byte

// supersample is the factor shapes are drawn at before reduction.
const supersample = 8

// Bug is a rendered bug. Every GPU overlay blends straight alpha (playout/watermark.go).
type Bug struct {
	// Straight is ordinary (non-premultiplied) RGBA.
	Straight *image.NRGBA
}

// Size is the bug's pixel size; both dimensions are even (yuva420p).
func (b Bug) Size() (int, int) { return b.Straight.Rect.Dx(), b.Straight.Rect.Dy() }

// Look is one channel's bug settings.
type Look struct {
	Size, Opacity float64
	Shadow        bool
}

// DefaultLook is the approved default.
func DefaultLook() Look { return Look{Size: DefaultSize, Opacity: DefaultOpacity, Shadow: true} }

// ErrEmpty means the source image has no visible pixels.
var ErrEmpty = errors.New("watermark: the image has no visible shape")

// PlateMask is the "Plate" typographic bug: the callsign knocked out of a rounded plate.
func PlateMask(callsign string) (*image.Alpha, error) {
	callsign = strings.TrimSpace(callsign)
	if callsign == "" {
		return nil, errors.New("watermark: empty callsign")
	}
	h := 100 * supersample
	txt, err := textMask(callsign, float64(h)*0.52)
	if err != nil {
		return nil, err
	}
	padX := int(float64(h) * 0.30)
	w := txt.Rect.Dx() + 2*padX
	m := image.NewAlpha(image.Rect(0, 0, w, h))
	fillRoundedRect(m, float64(h)*0.22)
	ox, oy := padX, (h-txt.Rect.Dy())/2
	for y := 0; y < txt.Rect.Dy(); y++ {
		for x := 0; x < txt.Rect.Dx(); x++ {
			k := int(txt.AlphaAt(x, y).A)
			i := m.PixOffset(ox+x, oy+y)
			m.Pix[i] = uint8(int(m.Pix[i]) * (255 - k) / 255)
		}
	}
	return m, nil
}

// LogoMask is a logo's silhouette: its own alpha, trimmed to the visible shape.
func LogoMask(logo image.Image) (*image.Alpha, error) {
	b := logo.Bounds()
	m := image.NewAlpha(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := logo.At(x, y).RGBA()
			m.Pix[m.PixOffset(x-b.Min.X, y-b.Min.Y)] = uint8(a >> 8)
		}
	}
	t := trim(m)
	if t == nil {
		return nil, ErrEmpty
	}
	return t, nil
}

// Render sizes a silhouette mask for a frame of frameHeight lines and finishes it with the look.
func Render(mask *image.Alpha, frameHeight int, look Look) Bug {
	w, h := sizeFor(mask.Rect.Dx(), mask.Rect.Dy(), frameHeight, look.Size)
	small := image.NewAlpha(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(small, small.Rect, mask, mask.Rect, xdraw.Src, nil)
	fg := image.NewNRGBA(small.Rect)
	for i, a := range small.Pix {
		fg.Pix[4*i], fg.Pix[4*i+1], fg.Pix[4*i+2], fg.Pix[4*i+3] = 255, 255, 255, a
	}
	return finish(fg, small, look)
}

// RenderImage is Render for a custom upload: the image keeps its own colours.
func RenderImage(img image.Image, frameHeight int, look Look) (Bug, error) {
	mask, err := LogoMask(img)
	if err != nil {
		return Bug{}, err
	}
	b := img.Bounds()
	// Crop the colour image to the silhouette's trimmed box (LogoMask trims from the same pixels).
	box := visibleBox(img)
	w, h := sizeFor(mask.Rect.Dx(), mask.Rect.Dy(), frameHeight, look.Size)
	fg := image.NewNRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(fg, fg.Rect, img, box.Add(b.Min).Intersect(b), xdraw.Src, nil)
	small := image.NewAlpha(fg.Rect)
	for i := range small.Pix {
		small.Pix[i] = fg.Pix[4*i+3]
	}
	return finish(fg, small, look), nil
}

// sizeFor is the equal-area rule: area 2·H², height capped at H.
func sizeFor(w, h, frameHeight int, size float64) (int, int) {
	H := float64(frameHeight) * size
	ar := float64(w) / float64(h)
	hh := math.Min(H, H*math.Sqrt(2/ar))
	return max(1, int(math.Round(hh*ar))), max(1, int(math.Round(hh)))
}

// finish bakes the opacity into alpha, adds the shadow and pads to even dimensions. fg is
// straight-alpha colour at final size; shape is its alpha.
func finish(fg *image.NRGBA, shape *image.Alpha, look Look) Bug {
	w, h := fg.Rect.Dx(), fg.Rect.Dy()
	pad, off := 0, 0
	var shadow []float64
	cw, ch := w, h
	if look.Shadow {
		pad = max(4, int(math.Round(float64(h)*0.12)))
		off = max(1, int(math.Round(float64(h)/40)))
		cw, ch = w+2*pad, h+2*pad
		shadow = make([]float64, cw*ch)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				shadow[(y+pad+off)*cw+x+pad+off] = float64(shape.Pix[shape.PixOffset(x, y)]) / 255
			}
		}
		gaussianBlur(shadow, cw, ch, math.Max(1, float64(h)/30))
	}
	cw, ch = cw+cw%2, ch+ch%2 // yuva420p needs even dimensions; the extra line is transparent
	straight := image.NewNRGBA(image.Rect(0, 0, cw, ch))
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			var r, g, b, af, as float64
			if fx, fy := x-pad, y-pad; fx >= 0 && fy >= 0 && fx < w && fy < h {
				c := fg.NRGBAAt(fx, fy)
				r, g, b = float64(c.R), float64(c.G), float64(c.B)
				af = float64(c.A) / 255 * look.Opacity
			}
			if shadow != nil && x < w+2*pad && y < h+2*pad {
				as = shadow[y*(w+2*pad)+x] * look.Opacity * 0.35
			}
			// The bug over its black shadow: premultiplied colour is the bug's alone.
			a := af + as*(1-af)
			if a <= 0 {
				continue
			}
			pr, pg, pb := r*af, g*af, b*af
			straight.SetNRGBA(x, y, color.NRGBA{u8(pr / a), u8(pg / a), u8(pb / a), u8(a * 255)})
		}
	}
	return Bug{Straight: straight}
}

func u8(v float64) uint8 { return uint8(math.Max(0, math.Min(255, math.Round(v)))) }

// gaussianBlur blurs a w×h plane in place, separably, with standard deviation sigma.
func gaussianBlur(p []float64, w, h int, sigma float64) {
	r := int(math.Ceil(3 * sigma))
	k := make([]float64, 2*r+1)
	var sum float64
	for i := range k {
		d := float64(i - r)
		k[i] = math.Exp(-d * d / (2 * sigma * sigma))
		sum += k[i]
	}
	for i := range k {
		k[i] /= sum
	}
	tmp := make([]float64, len(p))
	pass := func(src, dst []float64, n, stride, lines, lineStride int) {
		for l := 0; l < lines; l++ {
			for i := 0; i < n; i++ {
				var v float64
				for j, kv := range k {
					if s := i + j - r; s >= 0 && s < n {
						v += kv * src[l*lineStride+s*stride]
					}
				}
				dst[l*lineStride+i*stride] = v
			}
		}
	}
	pass(p, tmp, w, 1, h, w)
	pass(tmp, p, h, w, w, 1)
}

// textMask draws text in Geist Bold at cap height ~px (the em is 1.45·px), trimmed.
func textMask(text string, px float64) (*image.Alpha, error) {
	f, err := opentype.Parse(geistBold)
	if err != nil {
		return nil, err
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: px * 1.45, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return nil, err
	}
	defer face.Close()
	bounds, _ := font.BoundString(face, text)
	w := (bounds.Max.X - bounds.Min.X).Ceil() + 4
	h := (bounds.Max.Y - bounds.Min.Y).Ceil() + 4
	m := image.NewAlpha(image.Rect(0, 0, w, h))
	d := font.Drawer{Dst: m, Src: image.Opaque, Face: face,
		Dot: fixed.Point26_6{X: fixed.I(2) - bounds.Min.X, Y: fixed.I(2) - bounds.Min.Y}}
	d.DrawString(text)
	t := trim(m)
	if t == nil {
		return nil, ErrEmpty
	}
	return t, nil
}

// fillRoundedRect fills the whole mask with a rounded rectangle of corner radius r.
func fillRoundedRect(m *image.Alpha, r float64) {
	w, h := float64(m.Rect.Dx()), float64(m.Rect.Dy())
	for y := 0; y < m.Rect.Dy(); y++ {
		for x := 0; x < m.Rect.Dx(); x++ {
			cx, cy := float64(x)+0.5, float64(y)+0.5
			dx := math.Max(0, math.Max(r-cx, cx-(w-r)))
			dy := math.Max(0, math.Max(r-cy, cy-(h-r)))
			if dx*dx+dy*dy <= r*r {
				m.Pix[m.PixOffset(x, y)] = 255
			}
		}
	}
}

// trim crops a mask to its visible pixels, or nil when there are none.
func trim(m *image.Alpha) *image.Alpha {
	box := image.Rectangle{}
	for y := m.Rect.Min.Y; y < m.Rect.Max.Y; y++ {
		for x := m.Rect.Min.X; x < m.Rect.Max.X; x++ {
			if m.AlphaAt(x, y).A > 0 {
				box = box.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	if box.Empty() {
		return nil
	}
	out := image.NewAlpha(image.Rect(0, 0, box.Dx(), box.Dy()))
	for y := 0; y < box.Dy(); y++ {
		copy(out.Pix[y*out.Stride:y*out.Stride+box.Dx()], m.Pix[m.PixOffset(box.Min.X, box.Min.Y+y):])
	}
	return out
}

// visibleBox is the bounding box of an image's non-transparent pixels, relative to its origin.
func visibleBox(img image.Image) image.Rectangle {
	b := img.Bounds()
	box := image.Rectangle{}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a > 0 {
				box = box.Union(image.Rect(x-b.Min.X, y-b.Min.Y, x-b.Min.X+1, y-b.Min.Y+1))
			}
		}
	}
	return box
}
