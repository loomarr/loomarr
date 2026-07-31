// Package filler is the commercials & filler domain (design §10): the clip
// catalog model and pod assembly. Filler is a PARALLEL universe to provisioning
// (§3–§7) — clips are not titles (not in TMDB, no acquisition loop). Their
// identity is the clip's PATH under FILLER_DIR and their duration comes from
// Loomarr's own ffprobe scan (§9.1 moved both; see Clip for why). The media
// server is not in the filler path, and neither is Tunarr any more — an install
// running internal playout with no Tunarr still has a full catalog.
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
	"time"
)

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

// Clip is one filler item in the catalog (§10), scanned from FILLER_DIR.
//
// IDENTITY IS `Path` — the clip's location relative to FILLER_DIR. It was previously the
// Tunarr program uuid, and §9.1 forced the change for two reasons:
//
//  1. Internal playout needs a playable INPUT, and a Tunarr uuid is not one. Loomarr's own
//     encoder takes a path; under the old identity a channel could assemble a pod and then
//     have nothing to hand ffmpeg.
//  2. The dependency ran the wrong way. Clips were DISCOVERED by asking Tunarr to scan
//     FILLER_DIR, so an install running internal playout with no Tunarr had an empty catalog
//     and no commercials — a hard requirement on a service §9.1 makes optional. The files
//     were on Loomarr's own disk the whole time.
type Clip struct {
	// Path is the clip's location RELATIVE to FILLER_DIR ("1994/toys-transformers.mp4") and
	// is the identity.
	//
	// Relative, not absolute, deliberately: FILLER_DIR differs between a host and a container
	// (~/clips vs /data/filler), and absolute paths would invalidate every row the first time
	// someone moves the mount. Relative also keeps pod assembly deterministic across
	// environments, since the pod seed hashes clip ids.
	Path string
	// TunarrProgramID is the Tunarr program uuid, when Tunarr knows this clip. NO LONGER the
	// identity and legitimately empty: Tunarr-backed channels need it to build filler-lists,
	// while an install with no Tunarr simply has none. One catalog serves both backends —
	// internal playout reads Path, Tunarr reads this.
	TunarrProgramID string
	Name            string   // display name (from the filename)
	Kind            Kind     // commercial | bumper | station_id | psa | trailer | interstitial
	Era             int      // decade/year, e.g. 1994; 0 = untagged
	Audience        Audience // kids | family | general | late_night; "" = untagged
	Category        string   // toys | cereal | cars | tech | fast_food | movie_trailer | …; "" = untagged
	DurationMs      int64    // from ffprobe (the core probes now — §14 bundles it for playout)
	Rating          string   // optional content rating
	// Quality is the resolution label ("1080p", "480p"), derived from the probed video height
	// at scan time; "" for an audio-only clip or one scanned before quality existed.
	//
	// DISPLAY ONLY by default — an unconditional "prefer HD" rule would quietly starve the
	// era-accurate 4:3 commercials this whole subsystem exists to play, since the authentic
	// 1990s captures are exactly the low-resolution ones. V17c adds an OPT-IN minimum-quality
	// floor (default off) so an operator can exclude 240p rips without changing that default;
	// until then nothing reads this during selection.
	Quality string
	// License is the licence URL the SOURCE declared, carried from the clip's info-JSON sidecar
	// at scan time (SidecarLicense, V33) — e.g. "https://creativecommons.org/licenses/by-nc-sa/4.0/".
	//
	// ⚠ **"" means UNKNOWN, never "public domain".** About 92% of archive.org items declare no
	// licence at all (667 of 8362 measured in `classic_tv_commercials`, 2026-07-31), so an empty
	// value is the COMMON case and carries no permission. Nothing may treat it as one: this field
	// is a record of what a source claimed, not a rights determination, and it never gates
	// selection or playback.
	License string
	Source  string // provenance: filler-dir | tunarr-local | manual | …
	// Thumbnail is a path RELATIVE to the thumbnail cache dir; "" = not generated yet, which
	// renders as no image rather than a broken one. The bytes live on disk, not in the row —
	// they are regenerable from the source file, and thousands of them in a table that rides
	// the §16 backup would bloat every backup and every V11 migration (see 00017).
	Thumbnail string
	// PlayCount / LastPlayedAt are written from PLAYOUT, never from pod assembly (see 00017).
	//
	// ⚠ Only INTERNAL playout can report these. A Tunarr-backed channel airs its filler
	// through Tunarr, which never tells us, so its clips stay at zero forever — "0 plays" and
	// "not counted here" are different facts and a UI must not conflate them.
	PlayCount    int64
	LastPlayedAt time.Time
	AITagged     bool // whether the era/audience/category came from AI classification
}

// ID returns the clip's identity. A method rather than direct field access at call sites so
// the identity change above stayed a one-line edit in the places that only need "the id".
func (c Clip) ID() string { return c.Path }

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

// KindFromName infers a clip Kind from its filename/folder convention (§10 — the
// cheapest tagging tier, applied by the sync to Tunarr-`local` clips whose scan
// reports no kind). It DEFAULTS TO Commercial (the common case) so an unclassified
// clip is still pod-eligible as an ad — never left as a generic interstitial that
// the pod assembler can't place. AI tagging (§10) refines era/audience/category.
func KindFromName(name string) Kind {
	hay := strings.ToLower(name)
	switch {
	case strings.Contains(hay, "bumper"):
		return Bumper
	case strings.Contains(hay, "station") || strings.Contains(hay, "ident"):
		return StationID
	case strings.Contains(hay, "psa"):
		return PSA
	case strings.Contains(hay, "trailer"):
		return Trailer
	default:
		return Commercial
	}
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
