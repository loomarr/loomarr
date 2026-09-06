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
// IDENTITY IS `Hash` (see the field below). This has moved three times, and the reasons
// are worth keeping because each move was forced by a real failure:
//
//  1. It was the Tunarr program uuid. §9.1 killed that for two reasons: internal playout
//     needs a playable INPUT and a uuid is not one (a channel could assemble a pod and then
//     have nothing to hand ffmpeg); and the dependency ran the wrong way — clips were
//     DISCOVERED by asking Tunarr to scan FILLER_DIR, so an install running internal playout
//     with no Tunarr had an empty catalog. The files were on Loomarr's own disk the whole time.
//  2. It became `Path`, relative to FILLER_DIR.
//  3. V38c moved it to the content hash: a path is unique only WITHIN a folder, so once many
//     watched folders were allowed, two clips at `ads/coke.mp4` collided and silently
//     overwrote one another.
//
// ⚠ This comment read "IDENTITY IS `Path`" until 2026-08-10 — one architecture behind the
// field it introduces, which is the failure mode a long doc comment is most prone to.
type Clip struct {
	// Hash is the identity: the clip's sparse content hash (§10 V38c) — 64 hex characters.
	// Read it through `ID()` rather than directly; see that method.
	//
	// ⚠ **Identity moved off the path in V38c**, the third such change (§15's migration note
	// records all three). A path is only unique WITHIN a folder, so allowing many watched folders
	// made two clips at `ads/coke.mp4` collide and silently overwrite one another. Hashing the
	// bytes fixes that AND answers what a path cannot — *is this the same advert?* — which is
	// what lets intake skip duplicates instead of airing one twice in a break.
	Hash string
	// Path is the clip's LOCATION relative to the clip folder: `a3/f9/<hash>.mp4`, as intake
	// files it. Data, not identity.
	//
	// Relative, not absolute, deliberately: the clip folder differs between a host and a
	// container (~/clips vs /data/clips), and absolute paths would invalidate every row the first
	// time someone moves the mount.
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
	GeographicScope GeographicScope
	Country         string // ISO 3166-1 alpha-2 when known
	Market          string // local broadcast market; meaningful only for local scope
	Network         string // grounded broadcast network context
	Station         string // grounded station/callsign context
	AirDate         string // grounded YYYY-MM-DD broadcast date
	GeoEvidence     string // literal source evidence or "operator" correction attribution
	// Category is the DERIVED SHADOW of the taxonomy tags (§10 V45a): the primary product leaf of
	// `Tags`, computed at the single tag-write site (forest.PrimaryProductLeaf) and stored so the
	// hot pod-selection read path stays a plain column read. ⚠ Never an input any more — it is
	// always `PrimaryProductLeaf(Tags)`; the conformance suite pins the invariant. "" = no product
	// tag (a clip tagged only `psa`/`christmas` has no product category, which is honest).
	Category string
	// Tags is the clip's DENORMALISED taxonomy tag set (§10 V45a): the full leaf+rollup expansion
	// from `clip_tags`, loaded alongside the row. This is the source of truth for curation matching
	// and selection; `Category` is derived from it. Empty for a clip the tagger has not reached.
	Tags []string
	// AssertedTags is the operator/classifier-authored subset of Tags. The remaining tags are
	// rollups derived from the taxonomy graph. Editors MUST round-trip this set, never Tags: saving
	// the full expansion would promote derived ancestors into assertions that survive graph edits.
	AssertedTags []string
	DurationMs   int64  // from ffprobe (the core probes now — §14 bundles it for playout)
	Rating       string // optional content rating
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
	// Preview is a path RELATIVE to the preview cache dir; "" = not generated yet, which renders
	// as the still thumbnail rather than as a broken image (V39).
	//
	// ⚠ Stored rather than derived from Thumbnail. The two are almost always the same stem with a
	// different extension, which is what makes derivation tempting — and wrong, because the two
	// ffmpeg passes fail independently. A derived path would assert a file exists whenever the
	// STILL succeeded, so a clip whose preview render failed would render a broken image on hover
	// instead of simply not having one.
	Preview string
	// ThumbImageHash / HoverImageHash are the image-service identities of the same two assets
	// (§22, V52 phase 6); "" = not ingested yet.
	//
	// ⚠ **They coexist with Thumbnail/Preview during the migration window rather than replacing
	// them**, because those still point at real files the existing /v1/filler routes still serve,
	// and the phase-8 backfill reads them to find artwork that predates this. Dropping them now
	// would strand every clip that has not re-rendered — which is all of them the moment this
	// lands. The pair retires in phase 8, once nothing reads it.
	//
	// ⚠ The two hashes are SEPARATE for the same reason Preview is stored rather than derived
	// from Thumbnail: the still and the animation are ingested independently, so either can be
	// present without the other. Deriving one from the other would assert bytes exist that do not.
	ThumbImageHash string
	HoverImageHash string
	// Language is what the detection job heard, and it has THREE meaningful states (§10 V40,
	// migration 00036):
	//
	//	""      NOT YET CHECKED — the job has not reached this clip. Never a reason to reject.
	//	"none"  CHECKED, and there is no speech to judge. A wordless visual spot. ALWAYS KEPT.
	//	"en"/…  CHECKED, and this is what was heard.
	//
	// ⚠ The ""-vs-"none" distinction is load-bearing. Both mean "we are not rejecting this", which
	// is exactly what makes them tempting to merge — and merging them makes a clip whose detection
	// FAILED (whisper unrunnable, hosted key missing) indistinguishable from one that genuinely has
	// no dialogue. A silent advert is often the best filler; a failed detection is a retry.
	Language string
	// PlayCount / LastPlayedAt are written from PLAYOUT, never from pod assembly (see 00017).
	//
	// ⚠ Only INTERNAL playout can report these. A Tunarr-backed channel airs its filler
	// through Tunarr, which never tells us, so its clips stay at zero forever — "0 plays" and
	// "not counted here" are different facts and a UI must not conflate them.
	PlayCount    int64
	LastPlayedAt time.Time
	// Brand is the advertiser the clip is FOR — "Kellogg's", "Ford" (§10 V44). Free text, not an
	// enum: the set of advertisers is open in a way the category set is not.
	//
	// ⚠ GROUNDED like era, and for the same reason. A brand is accepted only when it appears
	// literally in a text signal (filename, sidecar, or the persisted Transcript) or in the
	// VisibleText a vision pass read off the frame — never inferred from a category or a tone. An
	// ungrounded brand is dropped, not persisted: "" is the honest common case, and a fabricated
	// advertiser is exactly the confidently-wrong metadata §8's grounding rule exists to keep out.
	Brand string
	// Transcript is the clip's spoken text, when it has been transcribed (§10 V44) — persisted here
	// rather than discarded after tagging (the splitter produced it and threw it away pre-V44).
	//
	// It is BOTH a searchable metadata field AND the richest input the tagger gets: a cereal advert
	// with an empty source description still SAYS "Kellogg's", which is the only place a brand or
	// era can be grounded for such a clip. "" has two meanings the transcribe job distinguishes the
	// same way the language gate does — not-yet-transcribed vs transcribed-and-wordless — but the
	// clip row does not need to; a wordless clip simply carries no transcript and is tagged from its
	// other signals.
	Transcript string
	// VisibleText is the on-screen text a vision pass read off the keyframes (§10 V44) — a logo, a
	// product name, a "1987" burned into the corner.
	//
	// ⚠ This is what makes vision AUDITABLE, and it is load-bearing, not display trivia. A brand or
	// era the vision model asserts is grounded only if it is supported by text the model also says
	// it can SEE here — reading "KELLOGG'S" off a box grounds the brand exactly as a year in the
	// filename grounds the era. A vision-proposed tag with no VisibleText backing does not persist.
	VisibleText string
	// VisionTagged records that a VISION pass (keyframes → multimodal model) contributed to this
	// clip's tags, distinct from AITagged (text-only classification). Separate because the two have
	// different trust and different cost: a human reviewing Incoming wants to know a tag came from
	// pixels, and a re-run wants to avoid paying for vision twice.
	VisionTagged bool
	AITagged     bool // whether the era/audience/category came from AI classification (text signals)
	// SuggestedEra is an AI-proposed era whose year did NOT appear in the clip's
	// text signals (§10 era grounding, V34) — demoted from Era by validateTags so
	// an inferred-from-tone year is never persisted as fact.
	//
	// ⚠ MATCHING NEVER READS THIS. It is not a tag: it is a question for the
	// operator, rendered on the clip and confirmed by PATCHing Era (which clears
	// it). 0 = no suggestion. Only the tagger writes it.
	SuggestedEra int
	// RemovedAt tombstones a clip the operator removed from the catalog (V35). Zero = present.
	//
	// ⚠ A tombstone rather than a DELETE because `clips` is a synced CACHE of FILLER_DIR: the
	// next scan finds the file still there and puts the row back, so a removed clip would
	// reappear minutes later. The file itself is never touched — nothing in Loomarr deletes an
	// operator's media, and the action says remove from the CATALOG.
	//
	// Removed clips are excluded from listings and from pod assembly by DEFAULT (opt-in to see
	// them), which is the safe polarity: pod assembly loads the catalog with a zero filter.
	RemovedAt time.Time
	// Held marks a clip that is recorded but NOT in the playable catalog (§10 V38). It is not
	// matched into a pod, not attached to a filler-list, and not counted as coverage — it is
	// waiting for processing and terminal admission or rejection. Every new non-composite clip
	// starts held, including a hand copy: placement is acquisition intent, not safety authority.
	//
	// Held clips are excluded from listings and pod assembly by DEFAULT (opt-in to see them),
	// the same polarity as RemovedAt and for the same reason: pod assembly loads the catalog
	// with a zero filter, so the safe state has to be the zero value.
	Held bool
	// Confidence (0-100) is the grounding-CAPPED diagnostic tagging score (§10 V38).
	//
	// ⚠ NEVER the model's self-assessment — see filler.TagSuggestion.Score. 0 = never scored,
	// which means the classifier has not measured it.
	Confidence int
	// IsComposite marks a clip that is a RECORDED BREAK — many adverts in one file, like
	// "KCPQ/Fox commercials, 5/28/1996" (§10 V45). A composite is NOT airable: it is excluded from
	// pod assembly exactly like Held/RemovedAt, because airing a 16-minute block as one "commercial"
	// is the bug this flag removes. Its SEGMENTS (produced by splitting) are the airable clips.
	//
	// ⚠ A distinct axis from Kind, deliberately. A composite's segments are commercials/bumpers/PSAs;
	// the composite itself is a CONTAINER. Overloading Kind with a `composite` value would force every
	// `filterKinds` call site to special-case it — which is how a container leaks into a pod. A
	// boolean the pod filter excludes ONCE (the same polarity as Held) is the safe shape.
	IsComposite bool
	// ParentHash is the identity of the COMPOSITE this clip was split out of (§10 V45), or "" for a
	// clip that is not a split segment (a hand-dropped single advert, or a composite itself).
	//
	// ⚠ This is the lineage V45 keeps that V34 threw away. V34 deleted the compilation on confirm
	// ("its identity is a path that now means twenty clips"); V45 keeps the parent as a composite and
	// points each segment back at it. That is what makes "which break did this advert air in?"
	// answerable (channel theming needs it), a re-split possible (detection improves), and the
	// parent's broadcast context (network/market/date) inheritable by every segment for free.
	ParentHash string
}

// IsSegment reports whether a clip was split out of a composite (§10 V45) — it has a parent.
func (c Clip) IsSegment() bool { return c.ParentHash != "" }

// ID returns the clip's identity.
//
// ⚠ **This method is why V38c's identity change was one line.** It was written as a method rather
// than direct field access precisely so identity could move without touching every call site —
// and it has now paid for itself twice: `Path` when identity was the location, `Hash` since
// content addressing (§10 V38c). Call sites that only need "the id" never had to know.
func (c Clip) ID() string { return c.Hash }

// Tagged reports whether a clip has the metadata pod matching needs (§10). An
// untagged clip can't be era/audience-matched, so it's only usable as a generic
// bumper/flex fill, never as a matched commercial.
//
// ⚠ `Category` here is the DERIVED product-leaf shadow of the taxonomy tags (§10 V45a), so
// `Category != ""` means "has at least one product tag" — the same completeness the flat category
// string used to assert, now sourced from the graph. A clip tagged only on non-product axes
// (`psa`, `christmas`) is correctly NOT "tagged" for matched-commercial purposes.
func (c Clip) Tagged() bool {
	return c.Era > 0 && c.Audience != "" && c.Category != ""
}

// IsBumper reports whether a clip can serve as pod bookend bumper (§10 pod
// structure: intro bumper → commercials → return bumper).
func (c Clip) IsBumper() bool { return c.Kind == Bumper || c.Kind == StationID }

// ⚠ `Clip.decade()` was deleted here (V51f). It bucketed a clip to its containing decade for the
// ladder's widened rung, and that rule could not generalise to a RANGE: a range already spanning
// 1990–1999 snaps to itself, so the widened rung would collapse into `exact` exactly when the
// operator asked for a decade. `EraRange.Widened` — ten years at each end — replaces it and is
// strictly wider than `exact` for every range, which is what a fallback rung has to be.

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
