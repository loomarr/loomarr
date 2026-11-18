package pod

import (
	"math/rand"
	"slices"
	"sort"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
)

// This file builds the fallback ladder (§10): the ordered sequence of candidate
// pools, from tightest (exact era + audience) to loosest (any appropriate-
// audience clip), with the embedded bumper card below that. Commercials are drawn
// from the tightest non-empty pool so a break degrades gracefully instead of
// going dead.

// pool is one rung of the ladder: a candidate set + the MatchLevel it represents.
type pool struct {
	level MatchLevel
	clips []Clip
}

// candidatePools builds the ladder rungs for a window (§10 fallback ladder):
//
//  1. exact    — break-body Placement, inside the Era RANGE, inside the Audience ceiling
//  2. widened  — break-body Placement, a decade either side of that range, inside the ceiling
//
// Nothing further away airs (#3838 decision 21): era-correct repeats, least recently aired first,
// are the rotation's job within these two rungs, so there is no any-era rung.
//
// ⚠ **Both rungs are always built (V51f).** A channel that wants strictness sets a narrow
// era range, which is a control an operator can actually see and change; there is no separate
// strict flag to special-case in `coverage.go`, `fit.go` or the coverage meter.
//
// Each rung is filtered to eligible durations (density, §10). Clips already used
// in the window are excluded at fill time (no-repeat), not here.
func candidatePools(catalog []Clip, w Window, policy Policy) []pool {
	catalog = filterGeography(catalog, policy.area(w.Geography))
	catalog = filterSeason(catalog, w)
	catalog = filterLanguage(catalog, w, policy)
	body := make([]Clip, 0, len(catalog))
	for _, c := range catalog {
		// ⚠ The quality floor applies to break-body clips only, alongside the duration bounds —
		// deliberately not catalog-wide. Bumpers are the ladder's floor and the station's own
		// bookends; excluding them on resolution could leave a break with nothing to open or
		// close on, trading a cosmetic complaint for a structural one.
		if c.EffectivePlacement() == PlacementBreakBody && durationEligible(c, policy) && qualityEligible(c, policy) {
			body = append(body, c)
		}
	}
	// Category narrowing (§10) applies to the break-body pool only — bumpers are
	// bookends, not themed by category.
	body = filterCategories(body, w.Categories)

	audienceMatch := filterAudience(body, w.Audience)

	return []pool{
		{MatchExact, filterEraWindows(audienceMatch, w.eraWindows())},
		{MatchWidened, filterEraWindows(audienceMatch, widenEraWindows(w.eraWindows()))},
	}
}

// fillCommercials draws matched commercials down the ladder, tightest rung first,
// enforcing category variety (no same category back-to-back) and no-repeat, up to
// PodMax clips and the flex gap. Deterministic under rng. Returns the MatchLevel
// reached (the loosest rung a placed clip came from) and the ordered clips.
//
// The invariants it holds, each with a test — change one and the pod stops being
// reproducible or starts playing three car ads in a row:
//   - LADDER ORDER: a clip ranks by its tightest rung, so exact-era material always leads
//     and a looser rung only TOPS UP what the tighter rungs cannot fill.
//   - COOLDOWN BEFORE RUNG: a clip resting inside its cooldown ranks behind every rested
//     clip on any rung. Cooldown relaxes only when the whole ladder is resting, which is the
//     "never leave a break empty" promise the setting makes, and nothing looser.
//   - Fills up to w.PodMax clips within w.GapMs, which the caller has already reduced by the
//     intro/return bumpers it places. A clip that does not fit whole is skipped for a shorter
//     one; an ad is never cut to fit.
//
// ⚠ #1684: every rung with an un-used clip feeds the break, not only the tightest. `used` is per
// break, so a tight pool is never exhausted by one break, and drawing from the tightest alone
// would air a one-clip exact rung alone in every break while readiness reported the clips on the
// widened rung. Both rungs are already inside the channel's audience ceiling (`candidatePools`),
// so drawing from both admits nothing the channel could not already air.
//   - CATEGORY VARIETY: never two clips of the same Category consecutively (§10 —
//     "so it doesn't play three car ads back to back"). Implemented as the two-pass
//     place(false)/place(true): the second pass tops the pod up and accepts a repeat
//     rather than returning a short pod.
//   - NO-REPEAT: skips anything already in `used`, which the caller threads across the
//     whole window. Nothing is added to `used` here — Assemble does that as it appends.
//   - Deterministic: all randomness comes from the seeded rng, and candidates are sorted
//     by Path (the identity) before any pick so catalog input order can't leak into output.
//   - SEASON SHARE: in-season clips stay at most half of the break's commercials (`share`, which
//     already counts the pins), and with none yet the best rested in-season clip is placed first,
//     then moved to a seeded spot among the commercials (placeLead, season.go).
//   - Returns ("", nil) when no pool has eligible clips; Assemble then uses the bumper card.
func fillCommercials(pools []pool, w Window, policy Policy, used map[string]bool, rng *rand.Rand, share *seasonShare) (MatchLevel, []Clip) {
	// w.GapMs is already net of the intro and return bumpers: Assemble subtracts their real
	// lengths before it calls here, so the ads below fill exactly what the break has left.
	budget := w.GapMs
	podMax := w.PodMax
	if podMax <= 0 {
		podMax = 4
	}

	// Every un-used clip once, at the tightest rung that holds it (the rungs nest).
	var cands []rungClip
	seen := map[string]bool{}
	regionOf := effectiveGeography(w.Geography, policy.Geography).regionRanker()
	for rung, p := range pools {
		for _, c := range sortByID(p.clips) {
			if taken(used, c) || seen[c.ID()] {
				continue
			}
			seen[c.ID()] = true
			cands = append(cands, rungClip{Clip: c, rung: rung, region: regionOf(c.Country)})
		}
	}
	if len(cands) == 0 {
		return "", nil // nothing eligible → caller uses the bumper card
	}

	// Deterministic candidate order, then a seeded shuffle so selection varies by
	// window but reproduces exactly for a given seed (§19).
	rng.Shuffle(len(cands), func(i, j int) { cands[i], cands[j] = cands[j], cands[i] })
	// Stable sorting after the seeded shuffle preserves varied tie-breaking while making the
	// ladder and durable rotation authoritative (see ladderLess).
	sort.SliceStable(cands, func(i, j int) bool { return ladderLess(cands[i], cands[j], w, policy) })
	// At least one in-season commercial when one is rested: the first that fits is placed first, so
	// nothing can crowd it out, and takes a seeded position once the break is filled (decision 35).
	lead := ""
	if share.in == 0 {
		for i, c := range cands {
			if inSeason(c.Clip, w) && !resting(c.Clip, w, policy) && c.DurationMs <= budget {
				cands = append([]rungClip{c}, slices.Delete(cands, i, i+1)...)
				lead = c.ID()
				break
			}
		}
	}

	var out []Clip
	var totalMs int64
	loosest := -1
	lastCat := ""
	segment := false // a break holds one station segment (#2065)
	// Two passes per BAND (one rung, rested or resting — see ladderBand): first honor category
	// variety strictly; the fallback pass allows a repeat only if the pod would otherwise be short
	// of clips. Variety wins WITHIN a band; we don't descend the ladder just to avoid a repeat
	// (§10 — the ladder is the duration fallback, variety is a placement rule).
	//
	// ⚠ V45a: `c.Category` is the DERIVED primary product leaf, so "no two back-to-back with the
	// same Category" reads as "don't stack two clips of the same product family" — two beer ads are a
	// repeat, a beer and a car ad are not. That is the right granularity and the rule is unchanged; a
	// clip with no product tag ("" Category) is exempt from the repeat check, as an untagged one was.
	place := func(band []rungClip, allowRepeat bool) {
		for _, c := range band {
			if len(out) >= podMax {
				return
			}
			// ⚠ Keyed by HASH (Clip.ID()), not Path — `used` is seeded by hash-keyed pinned/excluded
			// and reserved by hash everywhere (§10 V45a). Path keying here meant a pinned/excluded
			// clip was never recognised as used, so it could be double-placed or ignore its exclusion.
			if used[c.ID()] || contains(out, c.ID()) || (segment && isStationSegment(c.Clip)) {
				continue
			}
			if !allowRepeat && c.Category != "" && c.Category == lastCat {
				continue // would repeat a category back-to-back
			}
			if totalMs+c.DurationMs > budget {
				continue // does not fit; caller's fallback card is the bounded never-dead-air answer
			}
			if !share.admits(c.Clip, w) {
				continue // the break already holds its half of in-season clips
			}
			share.add(c.Clip, w)
			out = append(out, c.Clip)
			totalMs += c.DurationMs
			lastCat = c.Category
			segment = segment || isStationSegment(c.Clip)
			if c.rung > loosest {
				loosest = c.rung
			}
		}
	}
	// cands is sorted by ladderLess, so each band is one contiguous run.
	for start := 0; start < len(cands) && len(out) < podMax; {
		end := start + 1
		for end < len(cands) && ladderBand(cands[end], w, policy) == ladderBand(cands[start], w, policy) {
			end++
		}
		place(cands[start:end], false)
		if len(out) < podMax {
			place(cands[start:end], true) // top up, accepting a category repeat rather than a shorter pod
		}
		start = end
	}
	if len(out) == 0 {
		return "", nil
	}
	return pools[loosest].level, placeLead(out, lead, rng)
}

// placeLead moves the guaranteed in-season commercial from the front of out to a seeded position
// among the commercials, one that keeps category variety (decision 35).
func placeLead(out []Clip, lead string, rng *rand.Rand) []Clip {
	if lead == "" || len(out) < 2 || out[0].ID() != lead {
		return out
	}
	c, rest := out[0], slices.Clone(out[1:])
	clash := func(other Clip) bool { return c.Category != "" && other.Category == c.Category }
	var at []int
	for i := 0; i <= len(rest); i++ {
		if (i == 0 || !clash(rest[i-1])) && (i == len(rest) || !clash(rest[i])) {
			at = append(at, i)
		}
	}
	if len(at) == 0 {
		return out
	}
	return slices.Insert(rest, at[rng.Intn(len(at))], c)
}

// rungClip is a ladder candidate: the clip and the tightest rung (index into the pools) that
// admits it.
type rungClip struct {
	Clip
	rung int
	// region is the clip's country in the installation's preference order: 0 home.
	region int
}

// ladderBand is the placement band a candidate belongs to: its rung, with every clip resting
// inside its cooldown moved below all rested ones. Bands are what ladderLess orders.
func ladderBand(c rungClip, w Window, policy Policy) int {
	resting := 0
	if tier, _ := rotationRank(c.Clip, w, policy); tier == rotationRecent {
		resting = 1
	}
	return resting<<8 | c.rung // resting dominates; the rung (0-2) orders within it
}

// ladderLess orders ladder candidates: by band (rested before resting, then tightest rung
// first), then the home country's clips before each fallback country's in preference order
// (#2035), then by durable rotation (new, then least recently played).
//
// ⚠ Region orders WITHIN a band, never makes one: the variety pass runs per band, and a band per
// country would stack two home beer ads before it reached a fallback car ad. Region also yields to
// era: a fallback country's clip from the channel's own era outranks a home clip from a widened
// one, since "US first, then GB when US supply for an era runs thin" is per era.
func ladderLess(a, b rungClip, w Window, policy Policy) bool {
	if ab, bb := ladderBand(a, w, policy), ladderBand(b, w, policy); ab != bb {
		return ab < bb
	}
	if a.region != b.region {
		return a.region < b.region
	}
	return rotationLess(a.Clip, b.Clip, w, policy)
}

const (
	rotationNever = iota
	rotationReady
	rotationRecent
)

// rotationRank is the domain-owned anti-repeat ordering (§10 V58). The returned timestamp is
// meaningful only for aired clips; lower tiers and older times rank first.
func rotationRank(c Clip, w Window, policy Policy) (int, time.Time) {
	exposure := w.Exposures[c.ID()]
	// Versions of one ad rest together (#2033): a viewer who saw the 30 s spot an hour ago has
	// seen the 15 s cut-down too.
	for _, v := range c.Variants {
		if ve := w.Exposures[v]; ve.PlayCount > 0 {
			exposure.PlayCount += ve.PlayCount
			if ve.LastPlayedAt.After(exposure.LastPlayedAt) {
				exposure.LastPlayedAt = ve.LastPlayedAt
			}
		}
	}
	if exposure.PlayCount <= 0 || exposure.LastPlayedAt.IsZero() {
		return rotationNever, time.Time{}
	}
	if policy.Cooldown <= 0 || w.SnapshotAt.IsZero() ||
		!exposure.LastPlayedAt.After(w.SnapshotAt.Add(-policy.Cooldown)) {
		return rotationReady, exposure.LastPlayedAt
	}
	return rotationRecent, exposure.LastPlayedAt
}

func rotationLess(a, b Clip, w Window, policy Policy) bool {
	at, aa := rotationRank(a, w, policy)
	bt, ba := rotationRank(b, w, policy)
	if at != bt {
		return at < bt
	}
	if !aa.Equal(ba) {
		return aa.Before(ba)
	}
	// The caller establishes a deterministic seeded order before this stable comparison.
	return false
}

// contains reports whether a clip id (its HASH), or another version of its ad, is already in the
// pod-in-progress. Variant links are stored both ways, so the placed clip's list is enough.
func contains(out []Clip, id string) bool {
	for _, c := range out {
		if c.ID() == id || slices.Contains(c.Variants, id) {
			return true
		}
	}
	return false
}

// --- pure filters (deterministic, no rng) ---

// EraRange is the year window a channel draws its breaks from — §10's "a year range", which the
// wire has always carried and the domain, until V51f, threw half of away.
//
// ⚠ **The zero value means ANY, and the ambiguity that costs is resolved one layer up, not here.**
// `SelectionForChannel` owns the three states the operator sees (unset = inherit the channel's
// programming era, `{0,0}` = explicitly any, a set range = that window); by the time a range
// reaches the ladder the inheritance has already been applied, so the only question left is
// "which years". Keeping the third state OUT of this type is what stops every filter below from
// having to know about channel scope.
type EraRange struct {
	From int
	To   int
}

// Any reports whether the range constrains nothing.
func (r EraRange) Any() bool { return r.From <= 0 && r.To <= 0 }

// Contains reports whether a clip's year falls in the window. A zero bound is unbounded on that
// end, so {1990, 0} is "1990 onwards" and {0, 1999} is "up to 1999".
//
// ⚠ An untagged clip (year 0) is IN no range but in "any" — the same shape as the audience rule.
// A clip whose era Loomarr could not ground must not silently satisfy an era the operator set.
func (r EraRange) Contains(year int) bool {
	if r.Any() {
		return true
	}
	if year <= 0 {
		return false
	}
	if r.From > 0 && year < r.From {
		return false
	}
	if r.To > 0 && year > r.To {
		return false
	}
	return true
}

// Widened is the ladder's second rung: a decade either side of the range.
//
// ⚠ **It grows the range by ten years at each end rather than snapping to the containing decade,
// and the difference matters for ranges.** Decade-bucket snapping (1994 → 1990–1999) cannot
// generalise: a range that already spans 1990–1999 snaps to itself, so the widened rung would be
// identical to `exact` and the ladder would silently lose a rung exactly when the operator asked
// for a decade. Growing by ten years at each end is strictly wider than `exact` for EVERY range,
// which is the property a fallback rung has to have to be one.
func (r EraRange) Widened() EraRange {
	if r.Any() {
		return r
	}
	w := r
	if w.From > 0 {
		w.From -= 10
	}
	if w.To > 0 {
		w.To += 10
	}
	return w
}

func (w Window) eraWindows() []EraRange {
	if len(w.EraWindows) > 0 {
		return NormalizeEraWindows(w.EraWindows)
	}
	return []EraRange{w.Era}
}

func widenEraWindows(windows []EraRange) []EraRange {
	out := make([]EraRange, len(windows))
	for i, r := range windows {
		out[i] = r.Widened()
	}
	return NormalizeEraWindows(out)
}

func filterEraWindows(clips []Clip, windows []EraRange) []Clip {
	for _, r := range windows {
		if r.Any() {
			return clips
		}
	}
	out := make([]Clip, 0, len(clips))
	for _, c := range clips {
		for _, r := range windows {
			if r.Contains(c.Era) {
				out = append(out, c)
				break
			}
		}
	}
	return out
}

// NormalizeEraWindows sorts and merges overlapping or adjacent windows while
// retaining disjoint gaps. An explicit any window canonicalizes to just any. The merge is
// schedule.NormalizeRanges, the one that normalizes a channel's stored date windows, so a
// zero To (open-ended) absorbs every later window here exactly as it does there.
func NormalizeEraWindows(in []EraRange) []EraRange {
	if len(in) == 0 {
		return nil
	}
	ranges := make([]schedule.Range, len(in))
	for i, r := range in {
		if r.Any() {
			return []EraRange{{}}
		}
		ranges[i] = schedule.Range{From: r.From, To: r.To}
	}
	merged := schedule.NormalizeRanges(ranges)
	out := make([]EraRange, len(merged))
	for i, r := range merged {
		out[i] = EraRange{From: r.From, To: r.To}
	}
	return out
}

// audienceCeiling is what a channel of one audience may air (#3838 decision 8): the channel's own
// audience is a ceiling, never a match. A kids channel airs kids and family only (#1449); family
// and general channels air kids, family and general; a late-night channel airs everything. A
// clip with no grounded audience is in no ceiling.
func audienceCeiling(channel Audience) []Audience {
	switch channel {
	case Kids:
		return []Audience{Kids, Family}
	case Family, General:
		return []Audience{Kids, Family, General}
	case LateNight:
		return []Audience{Kids, Family, General, LateNight}
	default:
		return nil
	}
}

// filterAudience keeps the clips inside the channel's audience ceiling. A channel that expressed
// no audience airs every clip whose audience is grounded.
func filterAudience(clips []Clip, aud Audience) []Clip {
	out := make([]Clip, 0, len(clips))
	if aud == "" {
		for _, c := range clips {
			if c.Audience != "" {
				out = append(out, c)
			}
		}
		return out
	}
	allowed := audienceCeiling(aud)
	for _, c := range clips {
		if !slices.Contains(allowed, c.Audience) {
			continue
		}
		// Strict for kids only (#3838 decision 7): a lone model guess does not put a clip in front
		// of children. Every other ceiling takes the guess.
		if aud == Kids && c.AudienceGuess {
			continue
		}
		out = append(out, c)
	}
	return out
}

// filterRoles keeps only playable clips matching the selected descriptive roles (§10 per-channel
// "roles to include"). Empty selection accepts every playable Placement whose role, era and
// audience are known (FactsComplete). Applied catalog-wide BEFORE pickBumper/candidatePools, so a
// "bumpers only" selection empties the commercial pool and a "no bumpers" one leaves
// pickBumper nothing — the role choice shapes the whole pod, not just the middle.
//
// Political and news clips are opt-in (#2037): the empty selection leaves them out, and only a
// selection that names them lets them in. A station's own news is the exception. It is station
// material like the station's idents, so it airs wherever station idents do, but only from a
// station in `local`, the channel's resolved radius. The check is here rather than left to
// GeographicallyEligible, which admits every clip on an installation with no location.
func filterRoles(clips []Clip, roles []string, local map[string]bool) []Clip {
	allow := map[Role]bool{}
	if len(roles) == 0 {
		for _, r := range defaultRoles {
			allow[r] = true
		}
	}
	for _, r := range roles {
		allow[Role(r)] = true
	}
	out := make([]Clip, 0, len(clips))
	for _, c := range clips {
		// A clip missing a fact fits no channel (#3838 decision 6), and this is the one catalog-wide
		// filter assembly, coverage and fit all share, so the gate holds for each of them.
		if c.EffectivePlacement() == PlacementNotPlayable || !FactsComplete(c) {
			continue
		}
		switch {
		case allow[c.Role]:
			out = append(out, c)
		case c.Role == News && c.GeographicScope == GeographicLocal && local[c.Station] && allow[StationID]:
			out = append(out, c)
		}
	}
	return out
}

// defaultRoles is what a channel that chose no roles airs: every role except the opt-in ones.
var defaultRoles = []Role{Commercial, Bumper, StationID, PSA, Trailer, Interstitial}

// DefaultRoles is defaultRoles as wire strings, for a page that shows the default ticked.
func DefaultRoles() []string {
	out := make([]string, len(defaultRoles))
	for i, r := range defaultRoles {
		out[i] = string(r)
	}
	return out
}

// filterCategories keeps only clips whose TAXONOMY TAG SET intersects the selected categories (§10
// per-channel "categories", V45a). Empty selection = any category.
//
// ⚠ **Rollup-set intersection, not a single-string equality** (the flat-`category` model this
// replaces). A clip's `Tags` is the full leaf+rollup expansion, so selecting `food` matches a clip
// tagged `cereal` (cereal rolls up to food) — the question the flat string could NOT answer and the
// whole reason the taxonomy exists. A clip with NO tags is dropped under a non-empty selection: an
// untagged clip can't be known to match, and the selection is an explicit narrowing (unchanged from
// the flat model's polarity). This is deterministic set membership — eligibility, not fuzzy ranking
// (§10: structured filters stay deterministic; theme ranking never decides eligibility).
func filterCategories(clips []Clip, cats []string) []Clip {
	if len(cats) == 0 {
		return clips
	}
	allow := make(map[string]bool, len(cats))
	for _, c := range cats {
		allow[c] = true
	}
	out := make([]Clip, 0, len(clips))
	for _, c := range clips {
		for _, tag := range c.Tags {
			if allow[tag] {
				out = append(out, c)
				break
			}
		}
	}
	return out
}

// qualityEligible reports whether a clip clears the opt-in minimum-quality floor (V17c).
//
// ⚠ Two things are deliberately permissive here, and both are the difference between a floor
// and a catalog-emptier:
//
//   - A floor of 0 (the default) admits everything, so an install that sets nothing behaves
//     exactly as it did before this existed. `TestFloorOffIsByteIdentical` pins that.
//   - A clip whose quality is UNKNOWN is admitted. Audio-only clips have no height, and every
//     clip scanned before `00014_clips_quality` has an empty label until the next sync
//     re-probes it. Excluding unknowns would make setting a floor look like the catalog
//     vanished, which is the opposite of the diagnosis an operator needs.
func qualityEligible(c Clip, policy Policy) bool {
	if policy.MinQualityHeight <= 0 {
		return true
	}
	h := heightFromQuality(c.Quality)
	if h == 0 {
		return true // unknown quality is not evidence of LOW quality
	}
	return h >= policy.MinQualityHeight
}

// heightFromQuality maps a stored quality LABEL back to the pixel height it represents.
//
// ⚠ The label is what `clips.quality` holds — `QualityFromHeight` bucketed the probed height
// at scan time and the original number was never stored. So this is the inverse of that
// bucketing, and it returns each bucket's LOWER bound: "480p" covers heights 460-699, and
// treating it as 460 means a floor of 480 admits it. Rounding to the nominal 480 instead would
// exclude a 470-line clip that the label already called 480p, which is not a distinction an
// operator setting "480" is trying to draw.
func heightFromQuality(label string) int {
	switch label {
	case "4K":
		return 2000
	case "1080p":
		return 1000
	case "720p":
		return 700
	case "480p":
		return 460
	case "360p":
		return 340
	case "240p":
		return 1 // a real, known-low height: any floor above 0 excludes it
	default:
		return 0 // "" and anything unrecognised — treated as unknown, see qualityEligible
	}
}

func durationEligible(c Clip, policy Policy) bool {
	if policy.MinClipMs > 0 && c.DurationMs < policy.MinClipMs {
		return false
	}
	if policy.MaxClipMs > 0 && c.DurationMs > policy.MaxClipMs {
		return false
	}
	return true
}

// sortByID returns a deterministically-ordered copy (so a random pick can't leak
// catalog input order — §19 determinism).
func sortByID(clips []Clip) []Clip {
	out := make([]Clip, len(clips))
	copy(out, clips)
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}
