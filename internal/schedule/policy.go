package schedule

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
)

// ChannelPolicy is the per-channel programming contract (programming-design.md §2):
// the suggester EXTRACTS it from intent; this package ENFORCES it deterministically.
// It is the *sparse* form — every field optional, the zero value meaning "use the
// built-in default" — so it round-trips small and an LLM only emits what it means.
// Enforcement never reads this directly; it goes through Resolved(), which fills
// every default exactly once. Stored as policy_json on the channel row.
// The struct groups its fields by OWNER via two anonymous embeds — encoding/json
// flattens them, so policy_json stays a flat object (scope/audience/…, no nesting)
// and every p.Scope / p.Filler access keeps working via field promotion. Ownership
// is enforced in ONE place, the merge methods (MergeFromProposal / MergeFromOperator
// / WithApplied), so no writer hand-restores fields (§8.2).
type ChannelPolicy struct {
	// ProposalPolicy — EXTRACTED by the suggester (§8) and refreshed by a refine/
	// re-curation, EXCEPT any field the operator has pinned (see OperatorSet).
	ProposalPolicy
	// OperatorPolicy — never LLM-emitted; edited only on the channel page and always
	// preserved across a re-approval.
	OperatorPolicy
	// Applied records the relaxation-ladder steps taken at the last reconcile (§7).
	// It is NOT extracted — enforcement writes it, the API surfaces it. Recomputed
	// from scratch each reconcile, so it un-relaxes automatically when the pool recovers.
	// Reconcile-owned: rejected on write, force-preserved by MergeFromOperator.
	Applied []AppliedRelaxation `json:"applied,omitempty"`
	// OperatorSet lists the proposal-owned field paths the operator has explicitly set
	// ("scope.era", "audience.ceiling", "ordering", …). MergeFromProposal skips any field
	// named here, so an operator edit is STICKY — a later refine/re-curation cannot silently
	// revert it (§8.2). Bookkeeping only; rides policy_json, so it needs no migration.
	OperatorSet []string `json:"operatorSet,omitempty"`
}

// ProposalPolicy is the suggester-extracted half of ChannelPolicy (embedded, flattened).
type ProposalPolicy struct {
	Scope      ScopePolicy      `json:"scope,omitempty"`      // WHAT is allowed (§2)
	Audience   AudiencePolicy   `json:"audience,omitempty"`   // WHO it's for — safety-critical (§4)
	Separation SeparationPolicy `json:"separation,omitempty"` // HOW OFTEN things recur (§3)
	Ordering   OrderingMode     `json:"ordering,omitempty"`   // "" = inherit Channel.Strategy (§5)
	Seasonal   SeasonalPolicy   `json:"seasonal,omitempty"`   // WHEN in the year (§6)
	// Rules is the ordered set of curation rules (§6.5): wall-clock-conditional
	// programming (weekend marathons, holiday blocks, dayparts). Empty = today's
	// single-deck behavior. Each rule carries a Source (llm|operator): the suggester
	// seeds llm rules, the editor authors operator rules, and MergeFromProposal replaces
	// only the llm rules — so refine-authored and hand-authored rules COMPOSE (§8.2).
	// Every rule VALUE is grounded/clamped, evaluation is deterministic.
	Rules []SchedulingRule `json:"rules,omitempty"`
}

// OperatorPolicy is the channel-page-edited half of ChannelPolicy (embedded, flattened).
type OperatorPolicy struct {
	// Filler is the channel's per-channel commercial-break selection (§10): which
	// filler the channel draws from (theme) plus specific clips to always/never use.
	// A pointer so an omitted selection serializes nothing and means "any" (the whole
	// catalog — the additive default). Edited on the channel page; seeded from Scope.Era
	// at channel creation.
	Filler *FillerSelection `json:"filler,omitempty"`
	// Window is the rolling-window horizon a channel materializes (§6.5): the scheduler
	// emits ~Window of runtime rather than the whole run, advancing across boundaries.
	// 0 = inherit the global default (sched.window_hours, 24h); WindowFull = the whole
	// run (no truncation). A per-rule Window overrides this for that rule's slice.
	Window Duration `json:"window,omitempty"`
	// AutoCurate is the per-channel self-updating opt-in (§8.2). Nil (the default) = OFF:
	// the channel is frozen at its last-approved lineup and re-curation skips it. When set,
	// the scheduled `channel-recurate` job re-evaluates the channel's intent against the
	// current library and applies changes UNATTENDED — in-library matches added directly,
	// net-new acquisitions requested through the approval gate (never a raw wanted write),
	// bounded by the quality bar + title cap (global defaults, optionally overridden here).
	// Rides policy_json (no schema change), so it round-trips like every other policy field.
	AutoCurate *AutoCurate `json:"autoCurate,omitempty"`
	// Playout overrides which backend streams THIS channel (§9.1). Nil (the default) =
	// inherit the `playout.backend` registry setting. Rides policy_json like everything
	// above it, so there is no schema change and no migration.
	//
	// The nil-means-inherit shape is what makes the promise true rather than aspirational:
	// "changing the default affects new channels only — the ones already on the other
	// backend keep playing exactly as they are". A channel that never opted in has no
	// stored value to change, and one that DID has a value the global cannot overwrite.
	// A fleet-wide flip is therefore not expressible by accident.
	Playout *PlayoutPolicy `json:"playout,omitempty"`
}

// PlayoutPolicy is a channel's own answer to "who streams this" (§9.1). Deliberately a
// struct rather than a bare string: the encoder/transport knobs are global today, but a
// per-channel override for them is a plausible next step, and widening a struct is a
// smaller change than replacing a string field that has already round-tripped through
// operators' policy_json.
type PlayoutPolicy struct {
	// Backend is "internal" | "tunarr". Empty = inherit the global (same meaning as a
	// nil *PlayoutPolicy; both are tolerated so a hand-edited policy cannot mean
	// something surprising).
	Backend string `json:"backend,omitempty"`
}

// PlayoutBackendInternal is the `playout.backend` enum value meaning "Loomarr streams it".
const PlayoutBackendInternal = "internal"

// PlaysInternally resolves the nil-means-inherit precedence §15 defines: a channel's own
// `policy.playout.backend` wins when set, otherwise the global `playout.backend`.
//
// ⚠ ONE COPY, DELIBERATELY. This rule decides which subsystem answers for a channel, and it is
// now consulted from at least three places — the tuner/XMLTV filter, the Live TV wiring, and the
// now/next router. A second copy that drifted would not fail loudly; it would make one surface
// answer for a backend that is not streaming the channel, which is exactly the class of bug that
// put a channel in the guide that would not play and had the card contradict the grid. Callers
// pass the resolved global rather than reading settings here, because this package must not
// depend on the settings registry.
func PlaysInternally(policy ChannelPolicy, globalBackend string) bool {
	if p := policy.Playout; p != nil && p.Backend != "" {
		return p.Backend == PlayoutBackendInternal
	}
	return strings.TrimSpace(globalBackend) == PlayoutBackendInternal
}

// AutoCurate is a channel's self-updating configuration (§8.2). Its mere presence is the
// opt-in; the two optional overrides let a channel be stricter or looser than the global
// recurate defaults. A zero-value (non-nil) AutoCurate means "opted in, use the global
// defaults for both thresholds".
type AutoCurate struct {
	// MinScorePct overrides recurate.min_score_pct (the 0–100 quality bar a not-in-library
	// title must clear before auto-curate REQUESTS it). 0 = inherit the global default.
	MinScorePct int `json:"minScorePct,omitempty"`
	// MaxTitles overrides recurate.max_titles (the cap on how large this channel may grow).
	// 0 = inherit the global default.
	MaxTitles int `json:"maxTitles,omitempty"`
}

// FillerSelection is a channel's per-channel filler choice (§10). Every field is
// optional; an empty/absent field means "any", so the zero selection is the whole
// catalog (the prior global-pool behavior). The theme fields (Era/Audience/Categories/
// Kinds) narrow which clips are eligible; Pinned/Excluded are per-clip overrides on top.
// Enum values mirror the filler package's wire strings deliberately — the policy model
// validates its own closed sets and imports nothing (see the other enums here).
type FillerSelection struct {
	Era        *Range   `json:"era,omitempty"`        // year window; nil = any. Seeded from Scope.Era at create.
	Audience   string   `json:"audience,omitempty"`   // "" = any; else kids|family|general|late_night
	Categories []string `json:"categories,omitempty"` // empty = any; else a subset of the closed category set
	Kinds      []string `json:"kinds,omitempty"`      // empty = the default kinds; else the chosen subset
	Pinned     []string `json:"pinned,omitempty"`     // TunarrProgramIDs always included (a top-priority pool)
	Excluded   []string `json:"excluded,omitempty"`   // TunarrProgramIDs never used (wins over Pinned)
}

// The closed sets FillerSelection validates against (mirrors internal/filler wire
// values; kept local so the policy model stays dependency-free).
var (
	fillerKinds = map[string]bool{
		"commercial": true, "bumper": true, "station_id": true,
		"psa": true, "trailer": true, "interstitial": true,
	}
	fillerAudiences = map[string]bool{
		"kids": true, "family": true, "general": true, "late_night": true,
	}
	// ⚠ `fillerCategories` was DELETED (§10 V45a). It was the THIRD hardcoded copy of the flat
	// category enum (the tagger's knownCategories and the LLM prompt were the others, both also gone),
	// and the taxonomy replaced all three with an operator-editable graph. A selection's Categories are
	// now taxonomy SLUGS — an open, operator-editable set this pure domain package cannot know
	// (validating against a fixed list would drift AND reject operator-added taxa). So they are OPAQUE
	// here, exactly like Pinned/Excluded clip ids: not validated in `schedule`, matched at assembly
	// (a stale slug simply matches nothing in filterCategories). The API layer, which can reach the
	// live taxonomy, is where an unknown slug is rejected on write.
)

// validate rejects a selection with an unknown non-empty enum value or an inverted era
// range. Empty fields are always valid (they mean "any"). Pinned/Excluded are opaque
// clip ids — not validated here (a stale id simply matches nothing at assembly).
func (f *FillerSelection) validate() error {
	if f == nil {
		return nil
	}
	if f.Audience != "" && !fillerAudiences[f.Audience] {
		return fmt.Errorf("filler: unknown audience %q", f.Audience)
	}
	for _, k := range f.Kinds {
		if !fillerKinds[k] {
			return fmt.Errorf("filler: unknown kind %q", k)
		}
	}
	// ⚠ Categories are NOT validated here (§10 V45a). They are taxonomy slugs — an open, operator-
	// editable set — so they are opaque to this pure domain package, exactly like Pinned/Excluded clip
	// ids above: a stale or unknown slug simply matches nothing at assembly (filterCategories). The API
	// layer, which can read the live taxonomy graph, rejects an unknown slug on write.
	if f.Era != nil && f.Era.From > 0 && f.Era.To > 0 && f.Era.From > f.Era.To {
		return fmt.Errorf("filler: era range %d–%d is inverted", f.Era.From, f.Era.To)
	}
	return nil
}

// ScopePolicy is the "what is allowed on this channel" filter (§2). Ids are
// resolved provisioning keys, never names.
type ScopePolicy struct {
	Series      []provision.Key `json:"series,omitempty"`      // restrict to these series (resolved ids)
	Collections []string        `json:"collections,omitempty"` // media-server collections
	Seasons     *Range          `json:"seasons,omitempty"`     // per-series season window
	Era         *Range          `json:"era,omitempty"`         // by first-air/release year
	Genres      GenreFilter     `json:"genres,omitempty"`      // include/exclude by genre name
	RuntimeMax  int             `json:"runtimeMax,omitempty"`  // seconds; 0 = unbounded ("nothing over an hour")
}

// GenreFilter is an include/exclude pair over genre names (§2). Include empty =
// any genre allowed; Exclude always subtracts.
type GenreFilter struct {
	Include []string `json:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
}

// Range is an inclusive [From, To] window; 0 means unbounded on that end (§2).
// Used for both season ranges and eras.
type Range struct {
	From int `json:"from,omitempty"`
	To   int `json:"to,omitempty"`
}

// Contains reports whether v falls within the range, treating a 0 bound as
// unbounded on that end. A nil range contains everything (no constraint).
func (r *Range) Contains(v int) bool {
	if r == nil {
		return true
	}
	if r.From > 0 && v < r.From {
		return false
	}
	if r.To > 0 && v > r.To {
		return false
	}
	return true
}

// AudiencePolicy is the fail-closed content-rating gate (§4) — the one heuristic
// where an error is a *harm*, not an aesthetic bug.
type AudiencePolicy struct {
	Ceiling Rating        `json:"ceiling,omitempty"` // closed enum ladder; "" = no ceiling (adult/general)
	Unrated UnratedPolicy `json:"unrated,omitempty"` // "" resolves by ceiling (kids ⇒ exclude)
}

// SeparationPolicy governs how often the same content may recur (§3). Durations
// serialize as Go-duration strings ("168h") to match the §2 schema.
type SeparationPolicy struct {
	EpisodeNoRepeat Duration `json:"episodeNoRepeat,omitempty"` // same episode once per window
	MovieNoRepeat   Duration `json:"movieNoRepeat,omitempty"`
	SeriesMinGap    Duration `json:"seriesMinGap,omitempty"` // same series on MIXED channels
	BlockMax        int      `json:"blockMax,omitempty"`     // max consecutive slots from one series
}

// SeasonalPolicy controls holiday-aware programming (§6).
type SeasonalPolicy struct {
	Mode      SeasonalMode `json:"mode,omitempty"`      // off | auto | exclusive; "" = auto
	Holidays  []string     `json:"holidays,omitempty"`  // subset of the built-in calendar ids
	OffSeason OffSeason    `json:"offSeason,omitempty"` // exclusive-mode fallback; "" = loop
}

// AppliedRelaxation records one step the relaxation ladder took (§7), surfaced in
// the UI ("policy relaxed: repeat window 7d → 3d"). Kind is a stable slug; From/To
// are human-readable values for display.
type AppliedRelaxation struct {
	Kind string `json:"kind"` // "episodeNoRepeat" | "seriesMinGap" | "era" | ...
	From string `json:"from"`
	To   string `json:"to"`
}

// --- closed enums (Validate rejects unknown non-empty values) -------------------

// OrderingMode is how the eligible pool is laid onto the cycle (§5).
type OrderingMode string

const (
	// OrderInherit ("") defers to the channel's Strategy — a policy-less channel
	// keeps its existing sequential/shuffle behavior (the locked non-breaking default).
	OrderInherit     OrderingMode = ""
	OrderSequential  OrderingMode = "sequential"
	OrderShuffle     OrderingMode = "shuffle"
	OrderSyndication OrderingMode = "syndication" // deck-deal (default for a policy that omits ordering intent)
)

func (o OrderingMode) valid() bool {
	switch o {
	case OrderInherit, OrderSequential, OrderShuffle, OrderSyndication:
		return true
	}
	return false
}

// UnratedPolicy decides what happens to an item with missing/unmappable rating (§4).
type UnratedPolicy string

const (
	UnratedDefault UnratedPolicy = ""        // resolves by ceiling: kids ⇒ exclude, else allow
	UnratedExclude UnratedPolicy = "exclude" // fail-closed
	UnratedAllow   UnratedPolicy = "allow"
)

func (u UnratedPolicy) valid() bool {
	switch u {
	case UnratedDefault, UnratedExclude, UnratedAllow:
		return true
	}
	return false
}

// SeasonalMode is the holiday behavior (§6).
type SeasonalMode string

const (
	SeasonalDefault   SeasonalMode = ""          // resolves to auto
	SeasonalOff       SeasonalMode = "off"       // no detection, no bench
	SeasonalAuto      SeasonalMode = "auto"      // boost in-window, bench out-of-window
	SeasonalExclusive SeasonalMode = "exclusive" // the channel IS the holiday
)

func (m SeasonalMode) valid() bool {
	switch m {
	case SeasonalDefault, SeasonalOff, SeasonalAuto, SeasonalExclusive:
		return true
	}
	return false
}

// OffSeason is what an exclusive-mode channel does out of its holiday window (§6).
type OffSeason string

const (
	OffSeasonDefault OffSeason = ""     // resolves to loop
	OffSeasonLoop    OffSeason = "loop" // loop the scope without the seasonal filter
	OffSeasonDark    OffSeason = "dark" // go dark
)

func (o OffSeason) valid() bool {
	switch o {
	case OffSeasonDefault, OffSeasonLoop, OffSeasonDark:
		return true
	}
	return false
}

// --- audience rating ladder (§4) ------------------------------------------------

// Rating is a content rating from either the TV or film system (§4). Comparison
// is by ladderRank; an unmappable/empty value is treated as *unrated* (no rank).
type Rating string

// ladderRank maps both rating systems onto one ordered ladder (§4):
// TV-Y < TV-Y7 < TV-G/G < TV-PG/PG < TV-14/PG-13 < TV-MA/R/NC-17.
// A rating absent from this map is unrated (handled fail-closed).
var ladderRank = map[Rating]int{
	"TV-Y":  0,
	"TV-Y7": 1,
	"TV-G":  2, "G": 2,
	"TV-PG": 3, "PG": 3,
	"TV-14": 4, "PG-13": 4,
	"TV-MA": 5, "R": 5, "NC-17": 5,
}

// kidsCeilingRank is the highest ladder rank still considered a "kids" ceiling
// (TV-PG). At or below it, the default unrated policy is exclude (§4).
const kidsCeilingRank = 3

// KidsCeilingRank exposes the kids-ceiling boundary (TV-PG) so callers can keep an
// auto-raise from crossing kids→adult — a kids channel must never be forced above
// TV-PG just because a mis-rated pick slipped in (a TV-MA pick is dropped, not admitted).
const KidsCeilingRank = kidsCeilingRank

// NormalizeRating maps a raw media-server OfficialRating string into a ladder
// Rating, for callers stamping the rating onto a lineup entry at create time. An
// unmappable value normalizes to "" (unrated) — enforcement handles that
// fail-closed under a kids ceiling.
func NormalizeRating(raw string) Rating { return normalizeRating(raw) }

// normalizeRating maps a raw media-server OfficialRating string into a ladder
// Rating. It upper-cases, trims, and normalizes the common Emby/Jellyfin variants
// ("TV_Y7", "tv-14", "Rated R" → …); anything not on the ladder returns "" (unrated).
func normalizeRating(raw string) Rating {
	s := strings.ToUpper(strings.TrimSpace(raw))
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, " ", "")
	s = strings.TrimPrefix(s, "RATED")
	if _, ok := ladderRank[Rating(s)]; ok {
		return Rating(s)
	}
	return "" // unmappable → unrated (fail-closed at the enforcement site)
}

// mapped reports whether r is a known ladder rating (not unrated).
func (r Rating) mapped() bool { _, ok := ladderRank[r]; return ok }

// Rank returns r's position on the audience ladder (TV-Y=0 … TV-MA/R=5) and whether
// r is a known ladder rating. Exposed so the suggester can guarantee its audience
// ceiling admits the titles it actually grounded — a ceiling stricter than a picked
// title would silently empty the channel at enforcement (programming-design §4).
func (r Rating) Rank() (int, bool) { n, ok := ladderRank[r]; return n, ok }

// atOrBelow reports whether r is at or below the ceiling on the ladder. Both must
// be mapped; an unrated item is never "at or below" — it's decided by UnratedPolicy.
func (r Rating) atOrBelow(ceiling Rating) bool {
	rr, rok := ladderRank[r]
	cc, cok := ladderRank[ceiling]
	return rok && cok && rr <= cc
}

// --- Duration newtype (JSON as "168h", not nanoseconds) -------------------------

// Duration wraps time.Duration so it (un)marshals as a Go-duration STRING ("168h")
// — matching the programming-design §2 schema and staying human/LLM-authorable.
// stdlib time.Duration would marshal as an integer nanosecond count.
type Duration time.Duration

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	// Accept a string ("168h"); tolerate a bare number as nanoseconds for robustness.
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		parsed, err := time.ParseDuration(s)
		if err != nil {
			return fmt.Errorf("policy duration %q: %w", s, err)
		}
		*d = Duration(parsed)
		return nil
	}
	var n int64
	if err := json.Unmarshal(b, &n); err != nil {
		return fmt.Errorf("policy duration: want a duration string like \"168h\", got %s", b)
	}
	*d = Duration(n)
	return nil
}

// Std returns the underlying time.Duration.
func (d Duration) Std() time.Duration { return time.Duration(d) }

// --- built-in defaults (design §15; the two-tier `channel-policy > built-in`) ---
// The registry-default middle tier (config-design.md) is deferred until the
// settings registry lands; these constants are the v1 default source of truth.
const (
	defaultEpisodeNoRepeat = 168 * time.Hour // 7d
	defaultMovieNoRepeat   = 720 * time.Hour // 30d
	defaultSeriesMinGap    = 2 * time.Hour
	defaultBlockMax        = 2
	defaultOrdering        = OrderSyndication
	defaultSeasonalMode    = SeasonalAuto
	defaultOffSeason       = OffSeasonLoop
)

// ResolvedPolicy is the fully-populated form enforcement consumes: every default
// filled, no zero-means-default ambiguity, so slotting/filter code never repeats
// "if unset use default". Produced once per reconcile by ChannelPolicy.Resolved().
type ResolvedPolicy struct {
	Scope    ScopePolicy
	Ceiling  Rating
	Unrated  UnratedPolicy // resolved to exclude|allow (never "")
	Sep      ResolvedSeparation
	Ordering OrderingMode // resolved to a concrete mode (never OrderInherit)
	Seasonal ResolvedSeasonal
	// LastAired is when each key last aired ON THIS CHANNEL (§3.1) — the recency signal
	// placement biases on. Nil/empty ⇒ no history, and ordering is exactly what it was before
	// the signal existed.
	//
	// NOT a policy field: it is observed state, not something an operator or the LLM authors,
	// which is why it has no ChannelPolicy counterpart and no §9 extensibility entry. It rides
	// here because it is an INPUT to placement and this is what placement receives.
	LastAired map[provision.Key]time.Time
}

// ResolvedSeparation is the separation policy with every window/limit filled.
type ResolvedSeparation struct {
	EpisodeNoRepeat time.Duration
	MovieNoRepeat   time.Duration
	SeriesMinGap    time.Duration
	BlockMax        int // 0 = unbounded (single-series auto-relax)
}

// ResolvedSeasonal is the seasonal policy with mode/off-season concrete.
type ResolvedSeasonal struct {
	Mode      SeasonalMode
	Holidays  []string
	OffSeason OffSeason
}

// Resolved fills every default and applies the single-series auto-relax (§2).
//
//   - strategy: the channel's Strategy — an omitted policy Ordering inherits it
//     (a policy-less channel keeps its existing behavior, the locked decision).
//   - singleSeries: true when the eligible entry set is exactly one series and no
//     movies; separation there is EPISODE spacing, so SeriesMinGap/BlockMax relax
//     (rule-based, not LLM judgment).
func (p ChannelPolicy) Resolved(strategy Strategy, singleSeries bool) ResolvedPolicy {
	sep := ResolvedSeparation{
		EpisodeNoRepeat: orDur(p.Separation.EpisodeNoRepeat, defaultEpisodeNoRepeat),
		MovieNoRepeat:   orDur(p.Separation.MovieNoRepeat, defaultMovieNoRepeat),
		SeriesMinGap:    orDur(p.Separation.SeriesMinGap, defaultSeriesMinGap),
		BlockMax:        orInt(p.Separation.BlockMax, defaultBlockMax),
	}
	if singleSeries {
		// Separation is episode spacing here, not series spacing — relax both.
		sep.SeriesMinGap = 0
		sep.BlockMax = 0 // unbounded
	}

	ordering := p.Ordering
	if ordering == OrderInherit {
		ordering = orderingFromStrategy(strategy)
	}

	mode := p.Seasonal.Mode
	if mode == SeasonalDefault {
		mode = defaultSeasonalMode
	}
	off := p.Seasonal.OffSeason
	if off == OffSeasonDefault {
		off = defaultOffSeason
	}

	return ResolvedPolicy{
		Scope:    p.Scope,
		Ceiling:  p.Audience.Ceiling,
		Unrated:  resolveUnrated(p.Audience),
		Sep:      sep,
		Ordering: ordering,
		Seasonal: ResolvedSeasonal{Mode: mode, Holidays: p.Seasonal.Holidays, OffSeason: off},
	}
}

// resolveUnrated fills the unrated policy: explicit value wins; otherwise a kids
// ceiling (TV-Y..TV-PG) defaults to exclude (fail-closed) and any other ceiling
// (including none) defaults to allow (§4).
func resolveUnrated(a AudiencePolicy) UnratedPolicy {
	if a.Unrated != UnratedDefault {
		return a.Unrated
	}
	if a.Ceiling != "" && ladderRank[a.Ceiling] <= kidsCeilingRank {
		return UnratedExclude
	}
	return UnratedAllow
}

// orderingFromStrategy maps the channel's Strategy onto an OrderingMode for the
// inherit case. TimeSlot has no separate ordering yet → sequential (its block
// mapping happens at push time, matching lineup.go's TimeSlot handling).
func orderingFromStrategy(s Strategy) OrderingMode {
	switch s {
	case Shuffle:
		return OrderShuffle
	case Sequential, TimeSlot:
		return OrderSequential
	default:
		return defaultOrdering
	}
}

func orDur(d Duration, def time.Duration) time.Duration {
	if d == 0 {
		return def
	}
	return d.Std()
}

func orInt(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

// Validate rejects a policy with an unknown non-empty enum value or an unmappable
// audience ceiling (grounding: the suggester must never emit a ceiling outside the
// closed ladder). Empty/omitted fields are always valid (they resolve to defaults).
func (p ChannelPolicy) Validate() error {
	if !p.Ordering.valid() {
		return fmt.Errorf("channel policy: unknown ordering %q", p.Ordering)
	}
	if !p.Audience.Unrated.valid() {
		return fmt.Errorf("channel policy: unknown unrated policy %q", p.Audience.Unrated)
	}
	if p.Audience.Ceiling != "" && !p.Audience.Ceiling.mapped() {
		return fmt.Errorf("channel policy: ceiling %q is not on the rating ladder", p.Audience.Ceiling)
	}
	if !p.Seasonal.Mode.valid() {
		return fmt.Errorf("channel policy: unknown seasonal mode %q", p.Seasonal.Mode)
	}
	if !p.Seasonal.OffSeason.valid() {
		return fmt.Errorf("channel policy: unknown offSeason %q", p.Seasonal.OffSeason)
	}
	if err := p.Filler.validate(); err != nil {
		return fmt.Errorf("channel policy: %w", err)
	}
	if p.AutoCurate != nil {
		if p.AutoCurate.MinScorePct < 0 || p.AutoCurate.MinScorePct > 100 {
			return fmt.Errorf("channel policy: autoCurate.minScorePct %d out of range (0–100)", p.AutoCurate.MinScorePct)
		}
		if p.AutoCurate.MaxTitles < 0 {
			return fmt.Errorf("channel policy: autoCurate.maxTitles %d must not be negative", p.AutoCurate.MaxTitles)
		}
	}
	return nil
}
