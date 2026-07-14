// Package filler is the commercials & filler domain (design §10): the clip
// catalog model and pod assembly. Filler is a PARALLEL universe to provisioning
// (§3–§7) — clips are not titles (not in TMDB, no acquisition loop); their
// identity is the media-server item id and their duration comes from the server,
// so the core never downloads or probes media. Pod assembly is pure and
// SEEDED-DETERMINISTIC (seed = channel + window) so tests reproduce exactly and
// the same break rebuilds identically across reconciles (§10/§19). The scheduler
// (§9) inserts the assembled pods via Tunarr flex + filler lists; this package
// only decides *what plays in the breaks*.
package filler

import "strings"

// Kind is what a clip is (§10). A clip is NEVER a program — the scheduler places
// clips only as SlotFiller, never SlotProgram (the filler-never-a-program gate).
type Kind string

const (
	Commercial   Kind = "commercial"
	Bumper       Kind = "bumper"
	StationID    Kind = "station_id"
	PSA          Kind = "psa"
	Trailer      Kind = "trailer"
	Interstitial Kind = "interstitial"
)

// Audience is who a clip suits (§10). Matched to the channel (Saturday-morning
// cartoons → kids ads, not car insurance).
type Audience string

const (
	Kids      Audience = "kids"
	Family    Audience = "family"
	General   Audience = "general"
	LateNight Audience = "late_night"
)

// Clip is one filler item synced from the media server's filler library (§10).
// Identity is LibraryItemID (the media-server item id) — "library is source of
// truth" (§4). Duration comes from the server; the core never probes it.
type Clip struct {
	LibraryItemID string   // media-server item id — the identity
	Name          string   // display name (from the server / filename)
	Kind          Kind     // commercial | bumper | station_id | psa | trailer | interstitial
	Era           int      // decade/year, e.g. 1994; 0 = untagged
	Audience      Audience // kids | family | general | late_night; "" = untagged
	Category      string   // toys | cereal | cars | tech | fast_food | movie_trailer | …; "" = untagged
	DurationMs    int64    // from the media server (it already probes media)
	Rating        string   // optional content rating
	Source        string   // provenance: archive | youtube | manual | …
	AITagged      bool     // whether the era/audience/category came from AI classification
}

// Tagged reports whether a clip has the metadata pod matching needs (§10). An
// untagged clip can't be era/audience-matched, so it's only usable as a generic
// bumper/flex fill, never as a matched commercial.
func (c Clip) Tagged() bool {
	return c.Era > 0 && c.Audience != "" && c.Category != ""
}

// IsBumper reports whether a clip can serve as pod bookend bumper (§10 pod
// structure: intro bumper → commercials → return bumper).
func (c Clip) IsBumper() bool { return c.Kind == Bumper || c.Kind == StationID }

// decade returns the clip's decade (1994 → 1990) for era-widening in the
// fallback ladder (§10).
func (c Clip) decade() int {
	if c.Era <= 0 {
		return 0
	}
	return (c.Era / 10) * 10
}

// KindFromString parses a Kind, defaulting to interstitial for an unknown value
// (a clip with a weird kind is still placeable as generic filler, never a program).
func KindFromString(s string) Kind {
	switch Kind(strings.ToLower(s)) {
	case Commercial, Bumper, StationID, PSA, Trailer, Interstitial:
		return Kind(strings.ToLower(s))
	default:
		return Interstitial
	}
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
