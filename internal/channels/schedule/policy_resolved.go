package schedule

import (
	"time"

	"github.com/loomarr/loomarr/internal/platform/rating"
)

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
	defaultHolidayStrength = HolidayStrengthFeatured
)

// ResolvedPolicy is the fully-populated form enforcement consumes: every default
// filled, no zero-means-default ambiguity, so slotting/filter code never repeats
// "if unset use default". Produced once per reconcile by ChannelPolicy.Resolved().
type ResolvedPolicy struct {
	Scope    ScopePolicy
	Ceiling  rating.Rating
	Unrated  UnratedPolicy // resolved to exclude|allow (never "")
	Sep      ResolvedSeparation
	Ordering OrderingMode // resolved to a concrete mode (never OrderInherit)
	Seasonal ResolvedSeasonal
	// LastAired is when each unit (library item id) last aired ON THIS CHANNEL (§3.1) — the
	// recency signal placement biases on. Nil/empty ⇒ no history, and ordering is exactly what
	// it was before the signal existed.
	//
	// NOT a policy field: it is observed state, not something an operator or the LLM authors,
	// which is why it has no ChannelPolicy counterpart and no §9 extensibility entry. It rides
	// here because it is an INPUT to placement and this is what placement receives.
	LastAired map[string]time.Time
	// Calendar is the household's holiday calendar (#2030), which judges and dates the seasonal
	// pass and the holiday-episode matchers. Household state, not policy — it rides here for the
	// same reason LastAired does.
	Calendar Calendar
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
	Strength  HolidayStrength // never "": light | featured | heavy
	Evenings  bool            // featured holiday episodes air in the evening band
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
	strength := p.Seasonal.Strength
	if strength == HolidayStrengthDefault {
		strength = defaultHolidayStrength
	}

	return ResolvedPolicy{
		Scope:    p.Scope,
		Ceiling:  p.Audience.Ceiling,
		Unrated:  resolveUnrated(p.Audience),
		Sep:      sep,
		Ordering: ordering,
		Seasonal: ResolvedSeasonal{
			Mode: mode, Holidays: p.Seasonal.Holidays, OffSeason: off,
			Strength: strength, Evenings: p.Seasonal.Placement != HolidayPlacementAnytime,
		},
	}
}

// resolveUnrated fills the unrated policy: explicit value wins; otherwise a kids
// ceiling (TV-Y..TV-PG) defaults to exclude (fail-closed) and any other ceiling
// (including none) defaults to allow (§4).
func resolveUnrated(a AudiencePolicy) UnratedPolicy {
	if a.Unrated != UnratedDefault {
		return a.Unrated
	}
	if rank, _ := a.Ceiling.Rank(); a.Ceiling != "" && rank <= rating.KidsCeilingRank {
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
