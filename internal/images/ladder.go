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
	// backdropWidthLadder is 16:9 — episode stills, hero images. Fewer rungs because these are
	// rendered at a small number of sizes (a hover chip, a timeline strip, a hero) rather than
	// across a fluid grid.
	backdropWidthLadder = []int{300, 780, 1280}
	// iconWidthLadder covers channel icons and logos. Small by nature: the largest consumer is a
	// settings preview, and Tunarr fetches one for a guide tile.
	iconWidthLadder = []int{92, 185, 500}
)

// Widths returns the ladder for a role, smallest first.
//
// An unknown role falls back to the icon ladder rather than to something generous: Role is a
// closed set, so reaching the fallback means a caller invented a value, and the failure should be
// visibly too small rather than quietly plausible.
func (r Role) Widths() []int {
	switch r {
	case RolePoster:
		return slices.Clone(posterWidthLadder)
	case RoleBackdrop, RoleThumb:
		return slices.Clone(backdropWidthLadder)
	case RoleIcon:
		return slices.Clone(iconWidthLadder)
	}
	return slices.Clone(iconWidthLadder)
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
	ladder := r.Widths()
	for _, w := range ladder {
		if requested <= w {
			return w
		}
	}
	return ladder[len(ladder)-1]
}
