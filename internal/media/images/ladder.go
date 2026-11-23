package images

import "slices"

// Width ladders, per role (§22).
//
// ⚠ **These are code, not configuration, and that is deliberate.** A single flat `images.widths`
// setting could not express this: the ladder is per-role, so one value would be necessarily wrong
// for two of the three shapes. An operator knob here would be a control whose correct setting
// depends on information the operator does not have.
//
// The poster and backdrop rungs deliberately MIRROR TMDB's own size tokens. When a requested rung
// matches a token TMDB already offers, a future fetcher can take their rendition unmodified rather
// than pulling `original` and resizing — and when it does resize, starting from the same numbers
// keeps our output comparable to theirs rather than subtly different at every breakpoint.
//
// Every rung costs an AVIF encode as well as a WebP one, which is the real reason the ladders are
// short rather than generous: five rungs across three formats is fifteen artifacts per poster.

var (
	// posterWidthLadder is 2:3 artwork — channel tiles, title art.
	posterWidthLadder = []int{154, 185, 342, 500, 780}
	// thumbWidthLadder is a 16:9 still, an episode's or a filler clip's. Fewer rungs because these
	// are rendered at a small number of sizes (an episode row, a hover chip, a timeline strip)
	// rather than across a fluid grid.
	thumbWidthLadder = []int{300, 780, 1280}
	// backdropWidthLadder is 16:9 fanart, which is also drawn full screen: the player's cold-start
	// hold, the station ident and full-width Home art need at least the screen's width, 1440 to
	// 2880 device pixels on a laptop and 1920 or 3840 on a TV (#2774). 1920 mirrors TMDB's `w1920`
	// habit and 3840 is the TV rung (TMDB's `original`). Only the first four rungs are encoded as
	// AVIF (see maxAVIFWidth); the 3840 rung is WebP and JPEG, produced when a TV asks for it.
	backdropWidthLadder = []int{300, 780, 1280, 1920, 3840}
	// iconWidthLadder covers channel icons and logos. Small by nature: the largest consumer is a
	// settings preview, and Tunarr fetches one for a guide tile.
	iconWidthLadder = []int{92, 185, 500}
	// logoWidthLadder is a title's clear logo, wide and short. Clear logos top out near 800px wide
	// (the Kodi convention's size), and the largest consumer is a player overlay.
	logoWidthLadder = []int{300, 500, 800}
)

// Formats is the formats a role is rendered in, best first. A logo's alpha is its shape, so it
// has no JPEG rung: a JPEG would draw it on a black box.
func (r Role) Formats() []Format {
	if r == RoleLogo {
		return []Format{FormatAVIF, FormatWebP}
	}
	return DefaultFormats()
}

// Fallback is the format of the plain <img> fallback: JPEG, the floor every browser decodes, or
// WebP for a role with no JPEG rung.
func (r Role) Fallback() Format {
	formats := r.Formats()
	return formats[len(formats)-1]
}

// Widths returns the ladder for a role, smallest first.
//
// An unknown role falls back to the icon ladder rather than to something generous: Role is a
// closed set, so reaching the fallback means a caller invented a value, and the failure should be
// visibly too small rather than quietly plausible.
func (r Role) Widths() []int {
	switch r {
	case RolePoster:
		return slices.Clone(posterWidthLadder)
	case RoleBackdrop:
		return slices.Clone(backdropWidthLadder)
	case RoleThumb:
		return slices.Clone(thumbWidthLadder)
	case RoleIcon:
		return slices.Clone(iconWidthLadder)
	case RoleLogo:
		return slices.Clone(logoWidthLadder)
	}
	return slices.Clone(iconWidthLadder)
}

// maxAVIFWidth is the widest rung encoded as AVIF. A 3840-wide AVIF encode is the most expensive
// rendition the box makes, and a 4K client takes WebP at that size without a visible loss.
const maxAVIFWidth = 1920

// fullScreenFrom is the first fanart rung drawn full screen, where an upscale shows.
const fullScreenFrom = 1920

// WidthsFor is the ladder for an image whose original is sourceWidth pixels wide. A full-screen
// fanart rung (1920 and up) is only made when the original can fill it, so the server never
// upscales one (#2774): a 1280 original has no 1920 rung and a 1920 one has no 3840. A poster's
// and a still's (RoleThumb) rung is offered only when the original nearly fills it: at least
// nearFitPercent of the rung's width (#4476). A rung far wider than the original would be the
// same pixels upscaled at several times the bytes, while one the original nearly fills is
// stretched by at most about 15%, which is not visible, and leaving it out would serve a 680-wide
// poster (common for TVDB art) at 500 and make it soft on a 2x screen. The smallest rung is
// always offered, so an image always has one: a poster narrower than 154 is served at 154 and a
// still narrower than 300 at 300. A 185-wide poster's set is 154 and 185, and a grid asking for
// 342 takes the 185 rung and scales it in the browser. A catalogue still arrives 300 wide, so its
// set is the one 300 rung; a filler clip's catalogue still is made 320 wide (tools.PreviewWidth),
// so its set is the 300 rung too. A rendition such an image already holds
// above its set is not listed, and budget eviction reclaims it when the cache is over budget. A
// split proposal's segment still is a frame up to 1920 wide and keeps the rungs it fills. Icons
// and logos keep the whole ladder: they are drawn at card sizes, where a rung slightly wider than
// a small original is not visible. A sourceWidth of 0 (unknown, for an adopted image whose bytes
// have not arrived) is the whole ladder.
func (r Role) WidthsFor(sourceWidth int) []int {
	ladder := r.Widths()
	switch {
	case sourceWidth <= 0:
		return ladder
	case r == RoleBackdrop:
		return slices.DeleteFunc(ladder, func(w int) bool { return w >= fullScreenFrom && w > sourceWidth })
	case r == RolePoster, r == RoleThumb:
		return withinSource(ladder, sourceWidth)
	}
	return ladder
}

// nearFitPercent is how much of a rung's width an original fills for that rung to be offered.
const nearFitPercent = 85

// withinSource drops every rung the original fills less than nearFitPercent of, except the
// smallest, which is always kept.
func withinSource(ladder []int, sourceWidth int) []int {
	first := ladder[0]
	return slices.DeleteFunc(ladder, func(w int) bool {
		return w > first && sourceWidth*100 < w*nearFitPercent
	})
}

// AVIFWidthsFor is WidthsFor without the rungs wider than maxAVIFWidth.
func (r Role) AVIFWidthsFor(sourceWidth int) []int {
	widths := r.WidthsFor(sourceWidth)
	for i, w := range widths {
		if i > 0 && w > maxAVIFWidth {
			return widths[:i]
		}
	}
	return widths
}

// AspectRatio is width divided by height for the role, or 0 when the role has no fixed shape.
//
// Used to serve real `width`/`height` attributes to the frontend so an <img> contributes zero
// cumulative layout shift. ⚠ It is a FALLBACK: the actual stored dimensions are always preferred,
// because an icon is whatever shape the operator uploaded and pretending otherwise would letterbox
// their logo.
func (r Role) AspectRatio() float64 {
	switch r {
	case RolePoster:
		return 2.0 / 3.0
	case RoleBackdrop, RoleThumb:
		return 16.0 / 9.0
	}
	return 0
}

// NearestWidth maps an arbitrary requested width onto the role's ladder.
//
// ⚠ **Requests are snapped to the ladder rather than honoured literally, and this is a security
// property as much as a caching one.** The serve route takes a width from the URL; without
// snapping, an attacker could request ten thousand distinct widths and make the box encode ten
// thousand renditions — an unauthenticated CPU and disk amplification bug. Snapping means the set
// of files an image can ever produce is fixed and small.
//
// Rounds UP to the next rung so a layout never receives fewer pixels than it asked for (which
// would look soft); the largest rung is the ceiling.
func (r Role) NearestWidth(requested int) int {
	return nearest(r.Widths(), requested)
}

// NearestWidthFor is NearestWidth over WidthsFor(sourceWidth): the serve route snaps to the largest
// rung the original supports, so a 1280 original asked for 1920 is served at 1280, not upscaled.
func (r Role) NearestWidthFor(requested, sourceWidth int) int {
	return nearest(r.WidthsFor(sourceWidth), requested)
}

func nearest(ladder []int, requested int) int {
	for _, w := range ladder {
		if requested <= w {
			return w
		}
	}
	return ladder[len(ladder)-1]
}
