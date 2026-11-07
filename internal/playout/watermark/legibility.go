package watermark

import "image"

// Picking a legible logo variant (maintainer: "make that a measured rule, not a list").
//
// TMDB lists several logos per network. Its primary can be the wrong one for a bug: Nickelodeon's
// 2023 "splat" has 1–2 px strokes and an 11 px wordmark at bug size, and in the samples it broke
// up on a bright sky and vanished on a busy wall, while the network's plain wordmark read
// everywhere (PR #1532, nick.png vs nickw.png). What failed is stroke width at the size the bug
// airs, so that is what is measured: render the silhouette at its real size and erode it by one
// pixel. A stroke 1–2 px wide disappears; a 4 px stroke keeps half its pixels; a bold mark keeps
// most of them.

// legibleSurvival is the share of opaque pixels that must survive the erosion: strokes of about
// 4 px or more at the airing size.
const legibleSurvival = 0.5

// Legibility is the share of the silhouette's opaque pixels (alpha ≥ 50%) that stay opaque after a
// one-pixel erosion (all eight neighbours opaque), at the size it airs on a frameHeight picture.
func Legibility(mask *image.Alpha, frameHeight int, look Look) float64 {
	bug := Render(mask, frameHeight, Look{Size: look.Size, Opacity: 1})
	a := bug.Straight
	w, h := a.Rect.Dx(), a.Rect.Dy()
	opaque := func(x, y int) bool {
		return x >= 0 && y >= 0 && x < w && y < h && a.Pix[a.PixOffset(x, y)+3] >= 128
	}
	var n, kept int
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if !opaque(x, y) {
				continue
			}
			n++
			survives := true
			for dy := -1; dy <= 1 && survives; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if !opaque(x+dx, y+dy) {
						survives = false
						break
					}
				}
			}
			if survives {
				kept++
			}
		}
	}
	if n == 0 {
		return 0
	}
	return float64(kept) / float64(n)
}

// PickVariant chooses which of a network's logo silhouettes airs: the first, in TMDB's order, that
// is legible at the airing size; when none is, the most legible. It returns -1 for no variants.
func PickVariant(masks []*image.Alpha, frameHeight int, look Look) int {
	best, bestScore := -1, -1.0
	for i, m := range masks {
		s := Legibility(m, frameHeight, look)
		if s >= legibleSurvival {
			return i
		}
		if s > bestScore {
			best, bestScore = i, s
		}
	}
	return best
}
