package api

import (
	"context"
	"strings"
)

// Resolving a channel's `logo` URL back to an image record (§22, V52 phase 5).
//
// A channel's logo is a URL STRING and always has been. It can point at this instance's image
// service (an upload, or — from phase 7 — an adopted remote), or it can be an arbitrary external
// URL an operator pasted. The frontend's `<Image>` primitive needs the whole record (real
// width/height, the ThumbHash, both srcsets), which a URL alone cannot provide, so the channel
// resource carries an optional `logoImage` alongside the URL.
//
// ⚠ **Optional, not replacing `logo`.** Dropping `logo` for a hash would break the external case
// permanently, and the external case is not a legacy state: pasting a URL is a supported way to set
// a channel icon. `logo` stays the source of truth; `logoImage` is the enrichment that lets the
// frontend render a service-hosted logo properly.

// logoHashPrefix is the path segment that marks a logo as one of ours. It matches URLFor's shape
// (`{base}/v1/images/{hash}/w{width}.{ext}`) with or without an absolute base.
const logoHashPrefix = "/v1/images/"

// imageHashFromLogo returns the content hash a logo URL points at, or "" when the URL is not one
// of this instance's image URLs.
//
// ⚠ **Validated as a hash, never merely extracted.** The returned value is handed to the image
// store as a lookup key, and this input is operator-controlled (`PATCH /v1/channels/{id}` accepts
// any logo string). A bare "take the segment after /v1/images/" would forward arbitrary text —
// including traversal like `../../etc`. Requiring 64 lowercase hex characters means anything that
// is not literally a sha256 is treated as an external URL, which is the safe reading anyway.
func imageHashFromLogo(logo string) string {
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

// logoImageResolver pre-resolves the image records for a set of channels and returns a lookup by
// logo URL. A logo that is external, unresolvable, or points at a hash that no longer exists
// yields nil, and the frontend falls back to the plain URL.
//
// ⚠ **Pre-resolved rather than looked up per channel, and that is the whole reason this is a
// separate function.** `channelToDTO` is called once per channel in the LIST handler, so a lookup
// inside it is an N+1 — the exact shape a profile here has already caught once, and the reason
// `LineupEntryDTO.State` is documented as list-omitted. Distinct hashes are fetched ONCE each even
// when twenty channels share an icon.
//
// ⚠ A failed lookup is a nil entry, never an error. A channel whose image row was garbage-collected
// must still render — with its logo URL, degrading to the plain <img> path — rather than failing
// the whole list. §22's Durability section makes a missing upload an accepted state, so the read
// path has to survive it.
func (s *Server) logoImageResolver(ctx context.Context, logos []string) func(string) *ImageDTO {
	if s.images == nil {
		return nil
	}
	byHash := make(map[string]*ImageDTO, len(logos))
	for _, logo := range logos {
		hash := imageHashFromLogo(logo)
		if hash == "" {
			continue
		}
		if _, seen := byHash[hash]; seen {
			continue
		}
		// Recorded even on failure, so a shared broken hash is looked up once rather than once
		// per channel that references it.
		byHash[hash] = nil
		rec, err := s.images.Get(ctx, hash)
		if err != nil {
			continue
		}
		dto := s.imageToDTO(ctx, rec)
		byHash[hash] = &dto
	}
	if len(byHash) == 0 {
		return nil
	}
	return func(logo string) *ImageDTO {
		hash := imageHashFromLogo(logo)
		if hash == "" {
			return nil
		}
		return byHash[hash]
	}
}
