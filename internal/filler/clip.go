// Package filler is the commercials & filler domain (design §10): the clip
// catalog model and pod assembly. Filler is a PARALLEL universe to provisioning
// (§3–§7) — clips are not titles (not in TMDB, no acquisition loop). Their
// identity is the clip's sparse content HASH (§10 V38c; see Clip.Hash) and their
// duration comes from Loomarr's own ffprobe scan.
//
// ⚠ **Loomarr discovers its own clips.** Tunarr does not: it is optional, and when
// present it only supplies program uuids for clips it already knows (see
// TunarrClipSource) — an install running internal playout with no Tunarr has a full
// catalog. The MEDIA SERVER, however, IS one of the scan sources: `library` is a
// source kind alongside `folder`, `youtube` and `archive`, read via
// library.ListFillerClips. This comment claimed "the media server is not in the
// filler path" until 2026-08-10, which had not been true since sources became
// pluggable.
//
// Pod assembly is pure and
// SEEDED-DETERMINISTIC (seed = channel + window start) so tests reproduce
// exactly and the same break rebuilds identically across reconciles (§10/§19) —
// which is also what lets the guide promise the clips that will really air.
// Tunarr-backed channels still get pods via flex + filler lists; internal
// playout resolves each break itself. This package only decides *what plays in
// the breaks*.
package filler

import (
	"strconv"
	"strings"

	"github.com/loomarr/loomarr/internal/clipcatalog"
)

// The clip catalog's value types are defined in internal/clipcatalog, which the core store persists
// without importing this package (#1747). Filler names them here, as aliases of the same types, so
// the domain that discovers, prepares and schedules clips speaks one vocabulary in its own terms.
// Their documentation lives on the definitions.
type (
	// Clip is one filler item in the catalog. Identity is Hash; see clipcatalog.Clip.
	Clip = clipcatalog.Clip
	// Kind is what a clip is. A clip is never a program.
	Kind = clipcatalog.Kind
	// Placement says where a Ready clip may be scheduled, separate from Kind.
	Placement = clipcatalog.Placement
	// Audience is who a clip suits.
	Audience = clipcatalog.Audience
)

const (
	Unclassified = clipcatalog.Unclassified
	Commercial   = clipcatalog.Commercial
	Bumper       = clipcatalog.Bumper
	StationID    = clipcatalog.StationID
	PSA          = clipcatalog.PSA
	Trailer      = clipcatalog.Trailer
	Interstitial = clipcatalog.Interstitial

	PlacementNotPlayable = clipcatalog.PlacementNotPlayable
	PlacementBreakBody   = clipcatalog.PlacementBreakBody
	PlacementBookend     = clipcatalog.PlacementBookend

	Kids      = clipcatalog.Kids
	Family    = clipcatalog.Family
	General   = clipcatalog.General
	LateNight = clipcatalog.LateNight
)

// PlacementForRole derives scheduling placement without inventing a descriptive role. An enrolled
// unknown is ordinary break-body filler; composites remain containers and can never be scheduled.
func PlacementForRole(kind Kind, enrolled, composite bool) Placement {
	if composite || !enrolled {
		return PlacementNotPlayable
	}
	switch kind {
	case Bumper, StationID:
		return PlacementBookend
	case Commercial, PSA, Trailer, Interstitial, Unclassified:
		return PlacementBreakBody
	default:
		return PlacementNotPlayable
	}
}

// KindFromName records an explicit filler-role token from the filename convention (§10 — the
// cheapest tagging tier). Absence is Unclassified, never a placement default: acquisition and
// unrelated grounding do not prove that unknown bytes are a commercial.
func KindFromName(name string) Kind {
	tokens := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return r < 'a' || r > 'z'
	})
	for index, token := range tokens {
		switch token {
		case "bumper", "bumpers":
			return Bumper
		case "ident", "idents":
			return StationID
		case "station":
			if index+1 < len(tokens) && (tokens[index+1] == "id" || tokens[index+1] == "ident") {
				return StationID
			}
		case "psa", "psas":
			return PSA
		case "trailer", "trailers":
			return Trailer
		case "interstitial", "interstitials":
			return Interstitial
		case "commercial", "commercials", "advert", "adverts", "ad", "ads":
			return Commercial
		}
	}
	return Unclassified
}

// EraFromName best-effort extracts a 4-digit year (1930–2035) from a filename
// ("Total-Cereal 1985.mp4" → 1985), an initial era tag before AI refinement.
// Returns 0 if none.
func EraFromName(name string) int {
	for i := 0; i+4 <= len(name); i++ {
		if y, err := strconv.Atoi(name[i : i+4]); err == nil && y >= 1930 && y <= 2035 {
			return y
		}
	}
	return 0
}

// AudienceFromString parses an Audience; unknown/empty → "" (untagged).
func AudienceFromString(s string) Audience {
	switch Audience(strings.ToLower(s)) {
	case Kids, Family, General, LateNight:
		return Audience(strings.ToLower(s))
	default:
		return ""
	}
}

// ClipQuery is filler's view of a clip filter — deliberately tiny, because its callers want
// "everything currently in the catalog" and do their own filtering.
//
// ⚠ It stays small on purpose. `language = ”` and `transcript = ”` are not concepts the store
// should learn: they are the "not yet checked" sentinels of individual pipeline rungs, and pushing
// them down would make the store's filter grow a clause per stage.
//
// (It lived in `languagejob.go` until V51b retired that job. The type outlived it because the
// store adapters and the split stage read through it.)
type ClipQuery struct {
	// IncludeHeld covers clips still awaiting review. A held clip is a fine candidate for most
	// work: knowing a clip's language, transcript or tags BEFORE a human looks is strictly more
	// useful than after.
	IncludeHeld bool
}
