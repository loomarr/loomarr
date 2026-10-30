package playout

import (
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"time"
)

// A Blu-ray subtitle track (PGS, `hdmv_pgs_subtitle`) decoded into what the viewer sees: the whole
// canvas at every moment it changes (#2088 step 8). The burn-in (burnin.go) blends these stills on
// the GPU; ffmpeg's own route ([0:s:N] into an overlay) re-sends and re-converts the full canvas on
// every video frame, about a quarter of a core per stream, where a still per change costs nothing
// between changes.

// PGSState is the canvas from At until the next state: Image is nil when nothing shows.
type PGSState struct {
	At    time.Duration
	Image *image.NRGBA
}

// PGS segment types.
const (
	pgsPalette     = 0x14
	pgsObject      = 0x15
	pgsComposition = 0x16
	pgsWindow      = 0x17
	pgsEnd         = 0x80
)

// pgsEpochStart is the composition state that discards every object and palette before it.
const pgsEpochStart = 0x80

type pgsPlacement struct {
	object     uint16
	x, y       int
	cropped    bool
	crop       image.Rectangle
	paletteSet bool
}

type pgsObjectData struct {
	width, height int
	data          []byte
}

// ParsePGS decodes a .sup stream (ffmpeg's `-c:s copy -f sup`) into canvas states in time order.
// A display set's composition takes effect at its presentation time; consecutive identical
// "nothing shows" states are folded. Width and height are the canvas the composition declares.
func ParsePGS(b []byte) (states []PGSState, width, height int, err error) {
	palettes := map[uint8][256]color.NRGBA{}
	objects := map[uint16]*pgsObjectData{}
	var (
		at        time.Duration
		paletteID uint8
		placed    []pgsPlacement
		pending   bool
	)
	for len(b) > 0 {
		if len(b) < 13 || b[0] != 'P' || b[1] != 'G' {
			return nil, 0, 0, errors.New("pgs: bad segment header")
		}
		pts := binary.BigEndian.Uint32(b[2:6])
		kind := b[10]
		size := int(binary.BigEndian.Uint16(b[11:13]))
		if len(b) < 13+size {
			return nil, 0, 0, errors.New("pgs: truncated segment")
		}
		seg := b[13 : 13+size]
		b = b[13+size:]
		switch kind {
		case pgsComposition:
			if len(seg) < 11 {
				return nil, 0, 0, errors.New("pgs: short composition")
			}
			width, height = int(binary.BigEndian.Uint16(seg[0:2])), int(binary.BigEndian.Uint16(seg[2:4]))
			if seg[7]&pgsEpochStart != 0 {
				clear(palettes)
				clear(objects)
			}
			at = time.Duration(pts) * time.Second / 90000
			paletteID = seg[9]
			placed, pending = placed[:0], true
			n, rest := int(seg[10]), seg[11:]
			for range n {
				if len(rest) < 8 {
					return nil, 0, 0, errors.New("pgs: short composition object")
				}
				p := pgsPlacement{object: binary.BigEndian.Uint16(rest[0:2]), cropped: rest[3]&0x80 != 0,
					x: int(binary.BigEndian.Uint16(rest[4:6])), y: int(binary.BigEndian.Uint16(rest[6:8]))}
				rest = rest[8:]
				if p.cropped {
					if len(rest) < 8 {
						return nil, 0, 0, errors.New("pgs: short crop")
					}
					cx, cy := int(binary.BigEndian.Uint16(rest[0:2])), int(binary.BigEndian.Uint16(rest[2:4]))
					cw, ch := int(binary.BigEndian.Uint16(rest[4:6])), int(binary.BigEndian.Uint16(rest[6:8]))
					p.crop = image.Rect(cx, cy, cx+cw, cy+ch)
					rest = rest[8:]
				}
				placed = append(placed, p)
			}
		case pgsPalette:
			if len(seg) < 2 {
				return nil, 0, 0, errors.New("pgs: short palette")
			}
			pal := palettes[seg[0]]
			for e := seg[2:]; len(e) >= 5; e = e[5:] {
				pal[e[0]] = pgsColor(e[1], e[2], e[3], e[4])
			}
			palettes[seg[0]] = pal
		case pgsObject:
			if len(seg) < 4 {
				return nil, 0, 0, errors.New("pgs: short object")
			}
			id, flags := binary.BigEndian.Uint16(seg[0:2]), seg[3]
			if flags&0x80 != 0 { // the first fragment carries the size
				if len(seg) < 11 {
					return nil, 0, 0, errors.New("pgs: short first object fragment")
				}
				objects[id] = &pgsObjectData{width: int(binary.BigEndian.Uint16(seg[7:9])),
					height: int(binary.BigEndian.Uint16(seg[9:11])), data: append([]byte(nil), seg[11:]...)}
			} else if o := objects[id]; o != nil {
				o.data = append(o.data, seg[4:]...)
			}
		case pgsWindow:
			// Windows bound where objects may draw; the composition's placements already say where.
		case pgsEnd:
			if !pending {
				continue
			}
			pending = false
			img, err := pgsRender(width, height, placed, objects, palettes[paletteID])
			if err != nil {
				return nil, 0, 0, err
			}
			if img == nil && (len(states) == 0 || states[len(states)-1].Image == nil) {
				continue
			}
			states = append(states, PGSState{At: at, Image: img})
		}
	}
	return states, width, height, nil
}

// pgsRender draws one composition onto a transparent canvas; nil when nothing shows.
func pgsRender(width, height int, placed []pgsPlacement, objects map[uint16]*pgsObjectData, pal [256]color.NRGBA) (*image.NRGBA, error) {
	if len(placed) == 0 || width <= 0 || height <= 0 {
		return nil, nil
	}
	canvas := image.NewNRGBA(image.Rect(0, 0, width, height))
	drawn := false
	for _, p := range placed {
		o := objects[p.object]
		if o == nil {
			continue // an acquisition point can name an object a later set redefines; skip it
		}
		idx, err := pgsDecodeRLE(o.data, o.width, o.height)
		if err != nil {
			return nil, err
		}
		src := image.Rect(0, 0, o.width, o.height)
		if p.cropped {
			src = p.crop.Intersect(src)
		}
		for y := src.Min.Y; y < src.Max.Y; y++ {
			for x := src.Min.X; x < src.Max.X; x++ {
				c := pal[idx[y*o.width+x]]
				if c.A == 0 {
					continue
				}
				cx, cy := p.x+x-src.Min.X, p.y+y-src.Min.Y
				if cx < 0 || cy < 0 || cx >= width || cy >= height {
					continue
				}
				canvas.SetNRGBA(cx, cy, c)
				drawn = true
			}
		}
	}
	if !drawn {
		return nil, nil
	}
	return canvas, nil
}

// pgsDecodeRLE expands an object's run-length coded rows into palette indices.
func pgsDecodeRLE(data []byte, width, height int) ([]byte, error) {
	out := make([]byte, width*height)
	x, y := 0, 0
	put := func(n int, c byte) {
		for ; n > 0 && x < width; n-- {
			out[y*width+x] = c
			x++
		}
	}
	for i := 0; i < len(data) && y < height; {
		b := data[i]
		i++
		if b != 0 {
			put(1, b)
			continue
		}
		if i >= len(data) {
			break
		}
		flag := data[i]
		i++
		if flag == 0 { // end of line
			x, y = 0, y+1
			continue
		}
		n := int(flag & 0x3F)
		if flag&0x40 != 0 {
			if i >= len(data) {
				return nil, errors.New("pgs: truncated run")
			}
			n = n<<8 | int(data[i])
			i++
		}
		c := byte(0)
		if flag&0x80 != 0 {
			if i >= len(data) {
				return nil, errors.New("pgs: truncated run colour")
			}
			c = data[i]
			i++
		}
		put(n, c)
	}
	return out, nil
}

// pgsColor converts a palette entry (BT.709 limited range, as Blu-ray's HD subtitles are) to
// straight-alpha RGB. The burn-in converts it back with the same matrix, so a colour round-trips.
func pgsColor(y, cr, cb, a byte) color.NRGBA {
	yf := (float64(y) - 16) * 255 / 219
	crf := (float64(cr) - 128) * 255 / 224
	cbf := (float64(cb) - 128) * 255 / 224
	return color.NRGBA{R: clamp8(yf + 1.5748*crf), G: clamp8(yf - 0.1873*cbf - 0.4681*crf), B: clamp8(yf + 1.8556*cbf), A: a}
}

func clamp8(v float64) uint8 {
	switch {
	case v <= 0:
		return 0
	case v >= 255:
		return 255
	}
	return uint8(v + 0.5)
}
