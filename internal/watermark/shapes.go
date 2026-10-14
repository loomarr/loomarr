package watermark

import (
	"fmt"
	"image"
	"math"
	"strconv"
	"strings"

	"golang.org/x/image/vector"
)

// The approved design's looks and motifs (#1659) are drawn from the mock's own SVG geometry
// (project/prototypes/watermark.html): a look is laid out in units of a 100-unit-tall bug, and a motif
// in a 100×100 box. shape is that geometry in canvas pixels, rasterised anti-aliased by
// golang.org/x/image/vector at the supersampled size, so the PNG is drawn, not traced from a render.

// pt is a canvas point in pixels.
type pt struct{ x, y float64 }

// seg is one path command in pixels: a move, a line or a cubic curve (quadratics are raised to
// cubics); close ends the current subpath.
type seg struct {
	op        byte // 'M', 'L', 'C' or 'Z'
	p1, p2, p pt
}

// shape is a set of closed subpaths whose union is filled. Every builder below winds its subpaths
// the same way, so overlapping parts fill once (the rasteriser clamps coverage, it does not cancel).
type shape []seg

// xform maps the mock's units to canvas pixels: scale s, then offset (ox, oy).
type xform struct{ s, ox, oy float64 }

func (t xform) at(x, y float64) pt { return pt{t.ox + x*t.s, t.oy + y*t.s} }

// rotate turns (x, y) by deg degrees about (cx, cy), the mock's rotate(deg cx cy).
func rotate(x, y, deg, cx, cy float64) (float64, float64) {
	r := deg * math.Pi / 180
	sin, cos := math.Sincos(r)
	dx, dy := x-cx, y-cy
	return cx + dx*cos - dy*sin, cy + dx*sin + dy*cos
}

// svgPath parses the absolute-command subset the mock uses (M L H V C S Q Z).
func svgPath(d string, t xform) shape {
	toks := pathTokens(d)
	var s shape
	var cur, start, lastCtl pt
	var lastOp byte
	num := func(i *int) float64 {
		v, err := strconv.ParseFloat(toks[*i], 64)
		if err != nil {
			panic(fmt.Sprintf("watermark: bad path number %q in %q", toks[*i], d))
		}
		*i++
		return v
	}
	for i := 0; i < len(toks); {
		op := toks[i][0]
		i++
		switch op {
		case 'M':
			x, y := num(&i), num(&i)
			cur = t.at(x, y)
			start = cur
			s = append(s, seg{op: 'M', p: cur})
		case 'L':
			x, y := num(&i), num(&i)
			cur = t.at(x, y)
			s = append(s, seg{op: 'L', p: cur})
		case 'H':
			cur = pt{t.ox + num(&i)*t.s, cur.y}
			s = append(s, seg{op: 'L', p: cur})
		case 'V':
			cur = pt{cur.x, t.oy + num(&i)*t.s}
			s = append(s, seg{op: 'L', p: cur})
		case 'C', 'S':
			var c1 pt
			if op == 'C' {
				c1 = t.at(num(&i), num(&i))
			} else if lastOp == 'C' || lastOp == 'S' {
				c1 = pt{2*cur.x - lastCtl.x, 2*cur.y - lastCtl.y}
			} else {
				c1 = cur
			}
			c2 := t.at(num(&i), num(&i))
			p := t.at(num(&i), num(&i))
			s = append(s, seg{op: 'C', p1: c1, p2: c2, p: p})
			cur, lastCtl = p, c2
		case 'Q':
			q := t.at(num(&i), num(&i))
			p := t.at(num(&i), num(&i))
			s = append(s, seg{op: 'C', p1: pt{cur.x + 2.0/3*(q.x-cur.x), cur.y + 2.0/3*(q.y-cur.y)},
				p2: pt{p.x + 2.0/3*(q.x-p.x), p.y + 2.0/3*(q.y-p.y)}, p: p})
			cur = p
		case 'Z':
			s = append(s, seg{op: 'Z'})
			cur = start
		default:
			panic(fmt.Sprintf("watermark: unsupported path command %q in %q", op, d))
		}
		lastOp = op
	}
	return s
}

// pathTokens splits a path into commands and numbers ("M26 50L38 50" → M 26 50 L 38 50).
func pathTokens(d string) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			out = append(out, b.String())
			b.Reset()
		}
	}
	for _, r := range d {
		switch {
		case strings.ContainsRune("MLHVCSQZ", r):
			flush()
			out = append(out, string(r))
		case r == ' ' || r == ',':
			flush()
		case r == '-' && b.Len() > 0:
			flush()
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	flush()
	return out
}

// kappa places a cubic's control points to approximate a quarter ellipse.
const kappa = 0.5522847498

// ellipse is the mock's <ellipse>/<circle>, clockwise on screen.
func ellipse(cx, cy, rx, ry float64, t xform) shape {
	p := func(x, y float64) pt { return t.at(x, y) }
	kx, ky := rx*kappa, ry*kappa
	return shape{
		{op: 'M', p: p(cx+rx, cy)},
		{op: 'C', p1: p(cx+rx, cy+ky), p2: p(cx+kx, cy+ry), p: p(cx, cy+ry)},
		{op: 'C', p1: p(cx-kx, cy+ry), p2: p(cx-rx, cy+ky), p: p(cx-rx, cy)},
		{op: 'C', p1: p(cx-rx, cy-ky), p2: p(cx-kx, cy-ry), p: p(cx, cy-ry)},
		{op: 'C', p1: p(cx+kx, cy-ry), p2: p(cx+rx, cy-ky), p: p(cx+rx, cy)},
		{op: 'Z'},
	}
}

// roundRect is the mock's <rect rx>, clockwise on screen.
func roundRect(x, y, w, h, r float64, t xform) shape {
	r = math.Min(r, math.Min(w, h)/2)
	p := func(x, y float64) pt { return t.at(x, y) }
	k := r * (1 - kappa)
	return shape{
		{op: 'M', p: p(x+r, y)},
		{op: 'L', p: p(x+w-r, y)},
		{op: 'C', p1: p(x+w-k, y), p2: p(x+w, y+k), p: p(x+w, y+r)},
		{op: 'L', p: p(x+w, y+h-r)},
		{op: 'C', p1: p(x+w, y+h-k), p2: p(x+w-k, y+h), p: p(x+w-r, y+h)},
		{op: 'L', p: p(x+r, y+h)},
		{op: 'C', p1: p(x+k, y+h), p2: p(x, y+h-k), p: p(x, y+h-r)},
		{op: 'L', p: p(x, y+r)},
		{op: 'C', p1: p(x, y+k), p2: p(x+k, y), p: p(x+r, y)},
		{op: 'Z'},
	}
}

// line is a stroke from (x1, y1) to (x2, y2) of width w with round caps (stroke-linecap="round").
func line(x1, y1, x2, y2, w float64, t xform) shape {
	dx, dy := x2-x1, y2-y1
	l := math.Hypot(dx, dy)
	nx, ny := -dy/l*w/2, dx/l*w/2
	body := shape{{op: 'M', p: t.at(x1+nx, y1+ny)}, {op: 'L', p: t.at(x1-nx, y1-ny)},
		{op: 'L', p: t.at(x2-nx, y2-ny)}, {op: 'L', p: t.at(x2+nx, y2+ny)}, {op: 'Z'}}
	if signedArea(body) < 0 {
		body = shape{{op: 'M', p: body[0].p}, {op: 'L', p: body[3].p}, {op: 'L', p: body[2].p}, {op: 'L', p: body[1].p}, {op: 'Z'}}
	}
	return append(append(body, ellipse(x1, y1, w/2, w/2, t)...), ellipse(x2, y2, w/2, w/2, t)...)
}

// signedArea is a polygon subpath's area, positive when clockwise on screen (y down).
func signedArea(s shape) float64 {
	var a float64
	var pts []pt
	for _, g := range s {
		if g.op != 'Z' {
			pts = append(pts, g.p)
		}
	}
	for i, p := range pts {
		q := pts[(i+1)%len(pts)]
		a += p.x*q.y - q.x*p.y
	}
	return a / 2
}

// layer is a rasterised shape: an anti-aliased coverage mask placed at (x, y) on the canvas.
type layer struct {
	m    *image.Alpha
	x, y int
}

// raster fills a shape anti-aliased into a mask the size of its bounds.
func raster(s shape) layer {
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, g := range s {
		for _, p := range []pt{g.p1, g.p2, g.p} {
			if g.op == 'Z' || (g.op != 'C' && p != g.p) {
				continue
			}
			minX, minY, maxX, maxY = math.Min(minX, p.x), math.Min(minY, p.y), math.Max(maxX, p.x), math.Max(maxY, p.y)
		}
	}
	x0, y0 := int(math.Floor(minX)), int(math.Floor(minY))
	w, h := int(math.Ceil(maxX))-x0+1, int(math.Ceil(maxY))-y0+1
	r := vector.NewRasterizer(w, h)
	f := func(p pt) (float32, float32) { return float32(p.x - float64(x0)), float32(p.y - float64(y0)) }
	for _, g := range s {
		switch g.op {
		case 'M':
			r.MoveTo(f(g.p))
		case 'L':
			r.LineTo(f(g.p))
		case 'C':
			ax, ay := f(g.p1)
			bx, by := f(g.p2)
			cx, cy := f(g.p)
			r.CubeTo(ax, ay, bx, by, cx, cy)
		case 'Z':
			r.ClosePath()
		}
	}
	m := image.NewAlpha(image.Rect(0, 0, w, h))
	r.Draw(m, m.Rect, image.Opaque, image.Point{})
	return layer{m: m, x: x0, y: y0}
}

// textLayer is text in Geist Bold at cap height px, centred on (cx, cy) in canvas pixels.
func textLayer(text string, px, cx, cy float64) (layer, error) {
	m, err := textMask(text, px)
	if err != nil {
		return layer{}, err
	}
	return layer{m: m, x: int(math.Round(cx - float64(m.Rect.Dx())/2)), y: int(math.Round(cy - float64(m.Rect.Dy())/2))}, nil
}
