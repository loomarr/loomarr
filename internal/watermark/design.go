package watermark

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"slices"
	"strconv"
	"strings"

	xdraw "golang.org/x/image/draw"
)

// The approved design (#1659, project/prototypes/watermark.html, maintainer's answers 2026-10-13):
// six more looks, colour from the design tokens, and a holiday picture (motif) beside the look. Each
// is still ONE still PNG with straight alpha, drawn supersampled and reduced like the four built
// looks, so the GPU overlay does not change.

// The six looks added by the approved design. Their ids are the mock's.
const (
	StyleTint    Style = "tint"    // Colour plate: the callsign knocked out of a coloured plate
	StyleTwotone Style = "twotone" // Number + name: the number in a coloured tab beside a white name plate
	StyleBadge   Style = "badge"   // Number badge: the number knocked out of a coloured rounded square
	StyleLockup  Style = "lockup"  // Number | name: white number, thin divider, white name
	StyleGlass   Style = "glass"   // Glass: a frosted see-through plate with a bright edge
	StyleStripe  Style = "stripe"  // Era stripe: the callsign over a three-colour stripe
)

// DesignStyles is every look RenderDesign draws: the four built ones then the six new ones.
var DesignStyles = append(slices.Clone(Styles), StyleTint, StyleTwotone, StyleBadge, StyleLockup, StyleGlass, StyleStripe)

// Coloured reports whether the look takes a colour; the others are white whatever the channel's colour.
func (s Style) Coloured() bool {
	return s == StyleTint || s == StyleTwotone || s == StyleBadge || s == StyleStripe
}

// Colour is a design-token colour by its plain name (web/packages/tokens, tokens.ts: the "on" value,
// the one drawn on dark surfaces, which the mock uses).
type Colour string

const (
	Amber  Colour = "amber"  // signal
	Cyan   Colour = "cyan"   // tune
	Green  Colour = "green"  // lock
	Red    Colour = "red"    // onair
	Pink   Colour = "pink"   // suggest
	Yellow Colour = "yellow" // caution
)

// Colours is every colour, in the order the API lists them.
var Colours = []Colour{Amber, Cyan, Green, Red, Pink, Yellow}

var colourRGB = map[Colour]color.NRGBA{
	Amber: {0xFF, 0xB0, 0x20, 0xFF}, Cyan: {0x4C, 0xC9, 0xE8, 0xFF}, Green: {0x3D, 0xD6, 0x8C, 0xFF},
	Red: {0xE8, 0x5A, 0x5F, 0xFF}, Pink: {0xDC, 0x5B, 0xAC, 0xFF}, Yellow: {0xF5, 0xD9, 0x0A, 0xFF},
}

// RGB is the colour's sRGB value.
func (c Colour) RGB() color.NRGBA { return colourRGB[c] }

var white = color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF}

// Palette is three colours: a coloured look's fill is the first, the Era stripe uses all three.
type Palette [3]Colour

// eraPalettes is "Match the era" (#1659 Q8): the decade a channel's date window centres on picks
// the palette. It is the ONE table; a decade missing here (the 1960s, the 2000s, ...) gets
// NoEraPalette, the same as a channel with no date window.
var eraPalettes = map[int]Palette{
	1970: {Yellow, Amber, Red},
	1980: {Red, Amber, Cyan},
	1990: {Cyan, Pink, Green},
}

// NoEraPalette is a channel without a defined era's: amber, then the app's cyan and green.
var NoEraPalette = Palette{Amber, Cyan, Green}

// EraPalette is the palette for the decade (1980 for the 1980s), or NoEraPalette.
func EraPalette(decade int) Palette {
	if p, ok := eraPalettes[decade]; ok {
		return p
	}
	return NoEraPalette
}

// Motif is a holiday picture drawn beside the look.
type Motif string

const (
	MotifPumpkin   Motif = "pumpkin"
	MotifBat       Motif = "bat"
	MotifGhost     Motif = "ghost"
	MotifLeaf      Motif = "leaf"
	MotifHolly     Motif = "holly"
	MotifSnowflake Motif = "snowflake"
	MotifFirework  Motif = "firework"
	MotifHeart     Motif = "heart"
)

// Motifs is every motif, in the order the API lists them.
var Motifs = []Motif{MotifPumpkin, MotifBat, MotifGhost, MotifLeaf, MotifHolly, MotifSnowflake, MotifFirework, MotifHeart}

// HolidayLook is how a holiday colours a channel whose colour is "Match the era": the mock's fill
// colour and palette per holiday.
type HolidayLook struct {
	Colour  Colour
	Palette Palette
}

// HolidayLooks is keyed by the schedule calendar's holiday ids (internal/schedule, seasonal.go).
var HolidayLooks = map[string]HolidayLook{
	"halloween":    {Amber, Palette{Amber, Pink, Green}},
	"thanksgiving": {Amber, Palette{Amber, Red, Yellow}},
	"christmas":    {Green, Palette{Red, Green, Yellow}},
	"newyear":      {Cyan, Palette{Cyan, Yellow, Pink}},
	"valentines":   {Pink, Palette{Pink, Red, Amber}},
}

// Design is one generated bug: the look, its text, its colours and an optional holiday motif.
type Design struct {
	Style    Style
	Callsign string
	Number   int // the channel number, for the number looks
	Colour   Colour
	Palette  Palette
	Motif    Motif // "" draws none
	// White draws every colour as white: the host's coloured self-check failed (#1659 Q2).
	White bool
}

// motifSize and motifGap are the motif's height and its gap from the look, in units (the mock's 84, 6).
const motifSize, motifGap = 84.0, 6.0

// unit is a mock unit in canvas pixels: the bug is 100 units tall at the supersampled size.
const unit = float64(supersample)

// RenderDesign draws the design and finishes it for a frame of frameHeight lines. A built look
// without a motif is exactly Render of its CallsignMask, so existing bugs do not change.
func RenderDesign(d Design, frameHeight int, look Look) (Bug, error) {
	if !slices.Contains(DesignStyles, d.Style) {
		return Bug{}, fmt.Errorf("watermark: unknown style %q", d.Style)
	}
	if d.Motif != "" && !slices.Contains(Motifs, d.Motif) {
		return Bug{}, fmt.Errorf("watermark: unknown motif %q", d.Motif)
	}
	if d.Number <= 0 { // the number looks need a number; without one they draw the callsign
		switch d.Style {
		case StyleTwotone, StyleBadge:
			d.Style = StyleTint
		case StyleLockup:
			d.Style = StyleText
		}
	}
	if slices.Contains(Styles, d.Style) && d.Motif == "" {
		mask, err := CallsignMask(d.Callsign, d.Style)
		if err != nil {
			return Bug{}, err
		}
		return Render(mask, frameHeight, d.Style.Adjust(look)), nil
	}
	c, err := d.draw()
	if err != nil {
		return Bug{}, err
	}
	fg := c.straight()
	w, h := sizeFor(fg.Rect.Dx(), fg.Rect.Dy(), frameHeight, d.Style.Adjust(look).Size)
	small := image.NewNRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(small, small.Rect, fg, fg.Rect, xdraw.Src, nil)
	shape := image.NewAlpha(small.Rect)
	for i := range shape.Pix {
		shape.Pix[i] = small.Pix[4*i+3]
	}
	return finish(small, shape, d.Style.Adjust(look)), nil
}

// colour is c, or white when the design is white.
func (d Design) colour(c Colour) color.NRGBA {
	if d.White {
		return white
	}
	return c.RGB()
}

// draw lays out the look (and the motif to its left) on a supersampled canvas 100 units tall.
func (d Design) draw() (*canvas, error) {
	call := strings.TrimSpace(d.Callsign)
	if call == "" {
		return nil, errors.New("watermark: empty callsign")
	}
	num := strconv.Itoa(d.Number)
	x0 := 0.0
	if d.Motif != "" {
		x0 = motifSize + motifGap
	}
	// width measures text in units at cap height fs (the mock's font size; the built Plate's 0.52).
	width := func(s string, fs float64) (float64, error) {
		m, err := textMask(s, fs*unit)
		if err != nil {
			return 0, err
		}
		return float64(m.Rect.Dx()) / unit, nil
	}
	var ops []func(c *canvas, t xform) error
	var w float64
	text := func(s string, fs, cx, cy float64, col color.NRGBA, knock bool) func(c *canvas, t xform) error {
		return func(c *canvas, t xform) error {
			p := t.at(cx, cy)
			l, err := textLayer(s, fs*unit, p.x, p.y)
			if err != nil {
				return err
			}
			if knock {
				c.erase(l)
			} else {
				c.paint(l, col, 1)
			}
			return nil
		}
	}
	fill := func(s func(t xform) shape, col color.NRGBA, a float64) func(c *canvas, t xform) error {
		return func(c *canvas, t xform) error { c.paint(raster(s(t)), col, a); return nil }
	}
	main := d.colour(d.Colour)
	switch d.Style {
	case StylePlate, StyleText, StyleOutline, StyleSmallPlate:
		mask, err := CallsignMask(call, d.Style)
		if err != nil {
			return nil, err
		}
		w = float64(mask.Rect.Dx()) / unit
		ops = append(ops, func(c *canvas, t xform) error {
			c.paint(layer{m: mask, x: int(math.Round(t.ox)), y: int(math.Round(t.oy))}, white, 1)
			return nil
		})
	case StyleTint:
		tw, err := width(call, 52)
		if err != nil {
			return nil, err
		}
		w = tw + 60
		ops = append(ops, fill(func(t xform) shape { return roundRect(0, 0, w, 100, 22, t) }, main, 1),
			text(call, 52, w/2, 50, white, true))
	case StyleTwotone:
		nw, err := width(num, 60)
		if err != nil {
			return nil, err
		}
		cw2, err := width(call, 44)
		if err != nil {
			return nil, err
		}
		cw, pw := nw+44, cw2+50
		w = cw + 4 + pw
		ops = append(ops, fill(func(t xform) shape { return roundRect(0, 0, cw, 100, 22, t) }, main, 1),
			text(num, 60, cw/2, 50, white, true),
			fill(func(t xform) shape { return roundRect(cw+4, 0, pw, 100, 22, t) }, white, 1),
			text(call, 44, cw+4+pw/2, 50, white, true))
	case StyleBadge:
		nw, err := width(num, 62)
		if err != nil {
			return nil, err
		}
		w = math.Max(100, nw+40)
		ops = append(ops, fill(func(t xform) shape { return roundRect(0, 0, w, 100, 28, t) }, main, 1),
			text(num, 62, w/2, 50, white, true))
	case StyleLockup:
		a, err := width(num, 62)
		if err != nil {
			return nil, err
		}
		b, err := width(call, 44)
		if err != nil {
			return nil, err
		}
		w = a + b + 52
		ops = append(ops, text(num, 62, 12+a/2, 50, white, false),
			fill(func(t xform) shape { return roundRect(a+24, 22, 3, 56, 1.5, t) }, white, 1),
			text(call, 44, a+40+b/2, 50, white, false))
	case StyleGlass:
		tw, err := width(call, 50)
		if err != nil {
			return nil, err
		}
		w = tw + 60
		ops = append(ops, func(c *canvas, t xform) error {
			// The frosted fill: white whose alpha falls from 0.6 at the top to 0.16 at the bottom.
			body := raster(roundRect(1, 1, w-2, 98, 22, t))
			top, bottom := t.at(0, 1).y, t.at(0, 99).y
			c.paintFn(body, white, func(y float64) float64 { return 0.6 + (0.16-0.6)*math.Max(0, math.Min(1, (y-top)/(bottom-top))) })
			// The bright edge: a 2-unit stroke on the plate's edge at 0.75.
			c.paint(subtract(raster(roundRect(0, 0, w, 100, 23, t)), raster(roundRect(2, 2, w-4, 96, 21, t))), white, 0.75)
			// The sheen across the top.
			c.paint(raster(roundRect(5, 5, w-10, 38, 18, t)), white, 0.18)
			p := t.at(w/2, 50)
			l, err := textLayer(call, 50*unit, p.x, p.y)
			if err != nil {
				return err
			}
			// The letters' own shadow (the mock's feDropShadow dy 1.6, σ 1, black at 0.45).
			c.paint(blurred(l, 1.6*unit, unit), color.NRGBA{A: 255}, 0.45)
			c.paint(l, white, 1)
			return nil
		})
	case StyleStripe:
		tw, err := width(call, 48)
		if err != nil {
			return nil, err
		}
		w = tw + 24
		ops = append(ops, text(call, 48, w/2, 40, white, false))
		for i, col := range d.Palette {
			ops = append(ops, fill(func(t xform) shape { return roundRect(12, 74+float64(i)*8, w-24, 5, 2, t) }, d.colour(col), 1))
		}
	}
	c := newCanvas(int(math.Ceil((x0+w)*unit)), int(100*unit))
	t := xform{s: unit, ox: x0 * unit}
	for _, op := range ops {
		if err := op(c, t); err != nil {
			return nil, err
		}
	}
	if d.Motif != "" {
		d.drawMotif(c, xform{s: motifSize / 100 * unit, oy: (100 - motifSize) / 2 * unit})
	}
	return c, nil
}

// drawMotif draws the mock's motif (a 100×100 box) through t. Holes are real alpha holes.
func (d Design) drawMotif(c *canvas, t xform) {
	switch d.Motif {
	case MotifPumpkin:
		body := union(raster(ellipse(50, 60, 40, 30, t)), raster(ellipse(29, 60, 20, 29, t)), raster(ellipse(71, 60, 20, 29, t)))
		face := raster(svgPath("M26 50L38 50L32 38ZM62 50L74 50L68 38ZM30 64L40 72L46 66L54 66L60 72L70 64L68 76Q50 86 32 76Z", t))
		c.paint(subtract(body, face), d.colour(Amber), 1)
		c.paint(raster(svgPath("M45 34L44 18Q50 10 58 16L55 34Z", t)), d.colour(Green), 1)
	case MotifBat:
		c.paint(raster(svgPath("M50 38L45 22L41 38C32 26 18 24 4 30C12 38 12 48 18 56C26 48 34 50 40 62C44 56 48 56 50 64C52 56 56 56 60 62C66 50 74 48 82 56C88 48 88 38 96 30C82 24 68 26 59 38L55 22Z", t)), white, 1)
	case MotifGhost:
		body := raster(svgPath("M16 94V46C16 22 31 6 50 6S84 22 84 46V94L72 83L61 94L50 83L39 94L28 83Z", t))
		face := union(raster(ellipse(38, 42, 7, 7, t)), raster(ellipse(62, 42, 7, 7, t)), raster(ellipse(50, 62, 7, 9, t)))
		c.paint(subtract(body, face), white, 1)
	case MotifSnowflake:
		var arms shape
		for _, a := range []float64{0, 60, 120} {
			for _, l := range [][4]float64{{50, 6, 50, 94}, {50, 24, 38, 12}, {50, 24, 62, 12}, {50, 76, 38, 88}, {50, 76, 62, 88}} {
				x1, y1 := rotate(l[0], l[1], a, 50, 50)
				x2, y2 := rotate(l[2], l[3], a, 50, 50)
				arms = append(arms, line(x1, y1, x2, y2, 8, t)...)
			}
		}
		c.paint(raster(arms), d.colour(Cyan), 1)
	case MotifHolly:
		c.paint(raster(append(svgPath("M50 74C24 76 6 60 2 36C26 32 44 46 50 74Z", t), svgPath("M50 74C76 76 94 60 98 36C74 32 56 46 50 74Z", t)...)), d.colour(Green), 1)
		c.paint(raster(append(append(ellipse(40, 78, 11, 11, t), ellipse(60, 78, 11, 11, t)...), ellipse(50, 62, 11, 11, t)...)), d.colour(Red), 1)
	case MotifHeart:
		c.paint(raster(svgPath("M50 90C8 60 4 30 27 21C40 17 48 25 50 34C52 25 60 17 73 21C96 30 92 60 50 90Z", t)), d.colour(Pink), 1)
	case MotifFirework:
		var rays shape
		for i := range 8 {
			x1, y1 := rotate(50, 10, float64(i)*45, 50, 50)
			x2, y2 := rotate(50, 32, float64(i)*45, 50, 50)
			rays = append(rays, line(x1, y1, x2, y2, 12, t)...)
		}
		c.paint(raster(append(rays, ellipse(50, 50, 10, 10, t)...)), d.colour(Yellow), 1)
	case MotifLeaf:
		blade := raster(svgPath("M50 4C14 26 8 62 50 86C92 62 86 26 50 4Z", t))
		c.paint(subtract(blade, raster(line(50, 84, 50, 30, 5, t))), d.colour(Amber), 1)
		c.paint(raster(line(50, 84, 50, 97, 8, t)), d.colour(Amber), 1)
	}
}

// canvas is premultiplied RGBA in [0, 1], composited source-over.
type canvas struct {
	w, h int
	p    []float64 // r, g, b, a per pixel
}

func newCanvas(w, h int) *canvas { return &canvas{w: w, h: h, p: make([]float64, 4*w*h)} }

// paintFn draws colour col through the layer's coverage at alpha a(y), y in canvas pixels.
func (c *canvas) paintFn(l layer, col color.NRGBA, a func(y float64) float64) {
	r, g, b := float64(col.R)/255, float64(col.G)/255, float64(col.B)/255
	for y := 0; y < l.m.Rect.Dy(); y++ {
		cy := l.y + y
		if cy < 0 || cy >= c.h {
			continue
		}
		ay := a(float64(cy)+0.5) * float64(col.A) / 255
		for x := 0; x < l.m.Rect.Dx(); x++ {
			cx := l.x + x
			k := float64(l.m.Pix[y*l.m.Stride+x]) / 255 * ay
			if cx < 0 || cx >= c.w || k == 0 {
				continue
			}
			i := 4 * (cy*c.w + cx)
			c.p[i] = r*k + c.p[i]*(1-k)
			c.p[i+1] = g*k + c.p[i+1]*(1-k)
			c.p[i+2] = b*k + c.p[i+2]*(1-k)
			c.p[i+3] = k + c.p[i+3]*(1-k)
		}
	}
}

func (c *canvas) paint(l layer, col color.NRGBA, a float64) {
	c.paintFn(l, col, func(float64) float64 { return a })
}

// erase cuts the layer's coverage out of everything beneath (a knockout).
func (c *canvas) erase(l layer) {
	for y := 0; y < l.m.Rect.Dy(); y++ {
		for x := 0; x < l.m.Rect.Dx(); x++ {
			cx, cy := l.x+x, l.y+y
			if cx < 0 || cy < 0 || cx >= c.w || cy >= c.h {
				continue
			}
			k := 1 - float64(l.m.Pix[y*l.m.Stride+x])/255
			i := 4 * (cy*c.w + cx)
			c.p[i], c.p[i+1], c.p[i+2], c.p[i+3] = c.p[i]*k, c.p[i+1]*k, c.p[i+2]*k, c.p[i+3]*k
		}
	}
}

// straight is the canvas as straight-alpha NRGBA.
func (c *canvas) straight() *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, c.w, c.h))
	for i := 0; i < c.w*c.h; i++ {
		a := c.p[4*i+3]
		if a <= 0 {
			continue
		}
		out.Pix[4*i], out.Pix[4*i+1], out.Pix[4*i+2] = u8(255*c.p[4*i]/a), u8(255*c.p[4*i+1]/a), u8(255*c.p[4*i+2]/a)
		out.Pix[4*i+3] = u8(255 * a)
	}
	return out
}

// union is the layers' combined coverage (the maximum), on a mask covering all of them.
func union(ls ...layer) layer {
	box := image.Rectangle{}
	for _, l := range ls {
		box = box.Union(l.m.Rect.Add(image.Pt(l.x, l.y)))
	}
	out := layer{m: image.NewAlpha(image.Rect(0, 0, box.Dx(), box.Dy())), x: box.Min.X, y: box.Min.Y}
	for _, l := range ls {
		for y := 0; y < l.m.Rect.Dy(); y++ {
			for x := 0; x < l.m.Rect.Dx(); x++ {
				i := out.m.PixOffset(l.x-out.x+x, l.y-out.y+y)
				out.m.Pix[i] = max(out.m.Pix[i], l.m.Pix[y*l.m.Stride+x])
			}
		}
	}
	return out
}

// subtract is a's coverage with b's cut out (the mock's black-in-a-mask holes).
func subtract(a, b layer) layer {
	out := layer{m: image.NewAlpha(a.m.Rect), x: a.x, y: a.y}
	copy(out.m.Pix, a.m.Pix)
	for y := 0; y < b.m.Rect.Dy(); y++ {
		for x := 0; x < b.m.Rect.Dx(); x++ {
			ax, ay := b.x-a.x+x, b.y-a.y+y
			if ax < 0 || ay < 0 || ax >= a.m.Rect.Dx() || ay >= a.m.Rect.Dy() {
				continue
			}
			i := out.m.PixOffset(ax, ay)
			out.m.Pix[i] = uint8(int(out.m.Pix[i]) * (255 - int(b.m.Pix[y*b.m.Stride+x])) / 255)
		}
	}
	return out
}

// blurred is the layer moved down by dy pixels and blurred with standard deviation sigma.
func blurred(l layer, dy, sigma float64) layer {
	pad := int(math.Ceil(3 * sigma))
	w, h := l.m.Rect.Dx()+2*pad, l.m.Rect.Dy()+2*pad
	p := make([]float64, w*h)
	for y := 0; y < l.m.Rect.Dy(); y++ {
		for x := 0; x < l.m.Rect.Dx(); x++ {
			p[(y+pad)*w+x+pad] = float64(l.m.Pix[y*l.m.Stride+x]) / 255
		}
	}
	gaussianBlur(p, w, h, sigma)
	out := layer{m: image.NewAlpha(image.Rect(0, 0, w, h)), x: l.x - pad, y: l.y - pad + int(math.Round(dy))}
	for i, v := range p {
		out.m.Pix[i] = u8(255 * v)
	}
	return out
}
