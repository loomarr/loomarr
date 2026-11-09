package httpcore

import (
	"strings"

	"github.com/loomarr/loomarr/internal/media/images"
)

// Resolving a channel's `logo` URL back to an image record (§22, V52).
//
// A channel's logo is a URL STRING. It can point at this instance's image
// service (an upload, or an adopted remote), or it can be an arbitrary external
// URL an operator pasted. The frontend's `<Image>` primitive needs the whole record (real
// width/height, the ThumbHash, both srcsets), which a URL alone cannot provide, so the channel
// resource carries an optional `logoImage` alongside the URL.
//
// ⚠ **Optional, not replacing `logo`.** Dropping `logo` for a hash would break the external case
// permanently, and the external case is not a legacy state: pasting a URL is a supported way to set
// a channel icon. `logo` stays the source of truth; `logoImage` is the enrichment that lets the
// frontend render a service-hosted logo properly.

// ChannelLogoWidth is the rendition width stored on the channel — the top of the icon ladder
// (images.RoleIcon.Widths() = 92, 185, 500).
const ChannelLogoWidth = 500

// logoHashPrefix is the path segment that marks a logo as one of ours. It matches URLFor's shape
// (`{base}/v1/images/{hash}/w{width}.{ext}`) with or without an absolute base.
const logoHashPrefix = "/v1/images/"

// ImageHashFromLogo returns the content hash a logo URL points at, or "" when the URL is not one
// of this instance's image URLs.
//
// ⚠ **Validated as a hash, never merely extracted.** The returned value is handed to the image
// store as a lookup key, and this input is operator-controlled (`PATCH /v1/channels/{id}` accepts
// any logo string). A bare "take the segment after /v1/images/" would forward arbitrary text —
// including traversal like `../../etc`. Requiring 64 lowercase hex characters means anything that
// is not literally a sha256 is treated as an external URL, which is the safe reading anyway.
func ImageHashFromLogo(logo string) string {
	i := strings.Index(logo, logoHashPrefix)
	if i < 0 {
		return ""
	}
	rest := logo[i+len(logoHashPrefix):]
	// The hash runs to the next separator: `/w500.jpg`, or a bare `?`/`#` if someone stored the
	// record URL rather than a rendition URL.
	if j := strings.IndexAny(rest, "/?#"); j >= 0 {
		rest = rest[:j]
	}
	if !isSHA256Hex(rest) {
		return ""
	}
	return rest
}

// BrowserLogoURL keeps an image-service logo on the in-app page's own origin while preserving an
// operator-pasted external URL verbatim. The stored logo stays absolute because Tunarr is a
// machine client with no page origin; only the browser projection changes.
func BrowserLogoURL(logo string, pathFor func(string, int, images.Format) string) string {
	hash := ImageHashFromLogo(logo)
	if hash == "" || pathFor == nil {
		return logo
	}
	return pathFor(hash, ChannelLogoWidth, images.FormatJPEG)
}

// isSHA256Hex reports whether s is exactly 64 lowercase hex characters.
func isSHA256Hex(s string) bool {
	if len(s) != 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
