package watermark

import (
	"fmt"
	"html"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// candidates are the bug looks offered for the maintainer's pick (#1617), all at the approved
// opacity: the approved Plate as the control and three alternatives. None is selectable by a
// channel until one is approved.
var candidates = []struct {
	name, label string
	design      Design
	look        Look
}{
	{"a-plate", "A · Plate (approved look, the control)", DesignPlate, Look{Size: 0.06, Opacity: 0.40, Shadow: true}},
	{"b-text", "B · Text: the Plate's letters, no plate, soft shadow", DesignText, Look{Size: 0.06, Opacity: 0.40, Shadow: true}},
	{"c-outline", "C · Outline: the Plate's letters as a hollow stroke, soft shadow", DesignOutline, Look{Size: 0.06, Opacity: 0.40, Shadow: true}},
	{"d-small-plate", "D · Small Plate: 75% size (0.045), no shadow", DesignPlate, Look{Size: 0.045, Opacity: 0.40, Shadow: false}},
}

// Design review sheets for the candidates. Set WATERMARK_PREVIEW_DIR to write, per candidate, the
// aired bug PNGs (1080 and 2160 lines) and the bug composited over synthetic bright, dark and busy
// frames (corner crops at 1:1, 1080p full frames), with index.html. WATERMARK_PREVIEW_CALLSIGN
// sets the text (default RETRO). The composite is a straight-alpha blend in RGB, the blend every
// GPU overlay performs (playout/watermark.go), placed like playout places it: the active picture's
// top-right corner inset by the default margin (0.05), rounded down to even.
func TestPreviewCandidates(t *testing.T) {
	dir := os.Getenv("WATERMARK_PREVIEW_DIR")
	if dir == "" {
		t.Skip("WATERMARK_PREVIEW_DIR not set")
	}
	callsign := os.Getenv("WATERMARK_PREVIEW_CALLSIGN")
	if callsign == "" {
		callsign = "RETRO"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name string, img image.Image) {
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	scenes := []string{"bright", "dark", "busy"}
	var idx strings.Builder
	fmt.Fprintf(&idx, `<!doctype html><meta charset=utf-8><title>Bug look candidates</title>
<style>body{font:14px system-ui;margin:16px;background:#222;color:#eee}img{display:block;max-width:100%%}
.chk{background:repeating-conic-gradient(#888 0 25%%,#bbb 0 50%%) 0 0/16px 16px;display:inline-block}
td{vertical-align:top;padding:4px}h2{margin-top:32px}small{color:#aaa}</style>
<h1>Bug look candidates (#1617): for the maintainer's approval</h1>
<p>CANDIDATES, not a decision. Callsign %q, opacity 0.40, rendered by internal/watermark; composited in RGB over
synthetic frames at the default placement (top-right, margin 0.05). Crops are 1:1 pixels of the frame's top-right third.</p>`,
		callsign)
	for _, res := range []int{1080, 2160} {
		fw, fh := res*16/9, res
		frames := map[string]*image.NRGBA{}
		for _, s := range scenes {
			frames[s] = syntheticFrame(s, fw, fh)
		}
		fmt.Fprintf(&idx, "<h2>%dp: top-right crops</h2><table><tr><th></th>", res)
		for _, s := range scenes {
			fmt.Fprintf(&idx, "<th>%s</th>", s)
		}
		idx.WriteString("</tr>")
		for _, c := range candidates {
			mask, err := CallsignMask(callsign, c.design)
			if err != nil {
				t.Fatal(err)
			}
			bug := Render(mask, res, c.look)
			bw, bh := bug.Size()
			bugName := fmt.Sprintf("bug-%s-%d.png", c.name, res)
			write(bugName, bug.Straight)
			var peakA, peakW uint8
			for i := 0; i < len(bug.Straight.Pix); i += 4 {
				p := bug.Straight.Pix[i : i+4]
				peakA, peakW = max(peakA, p[3]), max(peakW, uint8((int(p[0])*int(p[3])+127)/255))
			}
			fmt.Fprintf(&idx, `<tr><td><b>%s</b><br><small>bug %dx%d px, peak alpha %.2f, peak white %.2f</small><br>
<span class=chk><img src=%q></span></td>`, html.EscapeString(c.label), bw, bh, float64(peakA)/255, float64(peakW)/255, bugName)
			mx, my := evenDown(0.05*float64(fw)), evenDown(0.05*float64(fh))
			x, y := evenDown(float64(fw-mx-bw)), my
			for _, s := range scenes {
				out := composite(frames[s], bug.Straight, x, y)
				crop := out.SubImage(image.Rect(fw*2/3, 0, fw, fh/4))
				name := fmt.Sprintf("crop-%s-%s-%d.png", c.name, s, res)
				write(name, crop)
				if res == 1080 {
					write(fmt.Sprintf("full-%s-%s-1080.png", c.name, s), out)
				}
				fmt.Fprintf(&idx, "<td><img src=%q></td>", name)
			}
			idx.WriteString("</tr>")
		}
		idx.WriteString("</table>")
	}
	idx.WriteString("<h2>1080p full frames</h2><table>")
	for _, c := range candidates {
		fmt.Fprintf(&idx, "<tr><td><b>%s</b></td>", html.EscapeString(c.label))
		for _, s := range scenes {
			fmt.Fprintf(&idx, "<td><img src=%q></td>", fmt.Sprintf("full-%s-%s-1080.png", c.name, s))
		}
		idx.WriteString("</tr>")
	}
	idx.WriteString("</table>")
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(idx.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", filepath.Join(dir, "index.html"))
}

func evenDown(v float64) int { return int(v) &^ 1 }

// composite blends a straight-alpha bug onto a copy of the frame at (x, y).
func composite(frame, bug *image.NRGBA, x, y int) *image.NRGBA {
	out := image.NewNRGBA(frame.Rect)
	copy(out.Pix, frame.Pix)
	for by := 0; by < bug.Rect.Dy(); by++ {
		for bx := 0; bx < bug.Rect.Dx(); bx++ {
			b := bug.NRGBAAt(bx, by)
			a := float64(b.A) / 255
			o := out.NRGBAAt(x+bx, y+by)
			mix := func(fg, bg uint8) uint8 { return u8(float64(fg)*a + float64(bg)*(1-a)) }
			out.SetNRGBA(x+bx, y+by, color.NRGBA{mix(b.R, o.R), mix(b.G, o.G), mix(b.B, o.B), 255})
		}
	}
	return out
}

// syntheticFrame is a deterministic test picture: "bright" is an overcast sky, "dark" a night
// scene, "busy" saturated high-detail texture with white highlights (the bug's worst case).
func syntheticFrame(scene string, w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		fy := float64(y) / float64(h)
		for x := 0; x < w; x++ {
			fx := float64(x) / float64(w)
			var r, g, b float64
			switch scene {
			case "bright":
				r, g, b = 228-40*fy, 234-36*fy, 242-30*fy
				sun := math.Exp(-(sq(fx-0.8) + sq(fy-0.15)) / 0.02)
				r, g, b = r+(255-r)*sun, g+(255-g)*sun, b+(250-b)*sun
			case "dark":
				r, g, b = 14+22*fy, 16+16*fy, 24+14*fy
				lamp := math.Exp(-(sq(fx-0.2) + sq(fy-0.7)) / 0.01)
				r, g, b = r+120*lamp, g+90*lamp, b+40*lamp
			default: // busy
				// Resolution-independent: frequencies in cycles per frame height.
				u, v := fx*float64(w)/float64(h), fy
				s1 := math.Sin(2 * math.Pi * (u*38 + v*11))
				s2 := math.Sin(2 * math.Pi * math.Hypot(u-1.5, v-0.3) * 60)
				s3 := math.Sin(2 * math.Pi * (u*7 - v*23))
				r = 128 + 90*s1 + 30*s3
				g = 128 + 90*s2 - 20*s1
				b = 128 + 90*s3*s2
				if s1 > 0.92 && s2 > 0.6 {
					r, g, b = 250, 250, 250
				}
			}
			img.SetNRGBA(x, y, color.NRGBA{u8(r), u8(g), u8(b), 255})
		}
	}
	return img
}

func sq(v float64) float64 { return v * v }
