package schedule

import (
	"strconv"
	"strings"

	"github.com/mantonx/loomarr/internal/provision"
)

// Preset lowering (programming-design §6.6): the closed authoring vocabulary. The LLM/UI
// compose rules from tokens; these pure functions lower each token to concrete
// SchedulingRule fields. An unknown token returns ok=false so the caller drops it (a rule
// that loses all its parts degrades to the base policy — the §8 fail-safe). No token here
// emits a raw WhenPredicate the model could have hand-authored: the model picks a name, the
// table supplies the predicate.
//
// The suggester's groundPolicy calls these and then applies the grounding CLAMPS (series
// intersection, stricter-only audience ceiling, window bound) — those need the channel's
// grounded picks/ceiling, which live in the suggester, so they are not in this pure table.

// LowerWhen maps a WHEN token to a (predicate, defaultPriority, ok). Composite tokens like
// "weekend-mornings" AND their parts. Case-insensitive. "holiday:<id>" carries the id.
func LowerWhen(token string) (WhenPredicate, int, bool) {
	t := strings.ToLower(strings.TrimSpace(token))

	// holiday:<id> — the §6 calendar. Validate the id against the builtin calendar.
	if id, ok := strings.CutPrefix(t, "holiday:"); ok {
		id = strings.TrimSpace(id)
		if !knownHoliday(id) {
			return WhenPredicate{}, 0, false
		}
		return WhenPredicate{Holiday: id}, 60, true
	}

	// Composite "a-b" (e.g. weekend-mornings) = AND of two base tokens, priority = max+5.
	if base, sub, found := strings.Cut(t, "-"); found {
		if w1, p1, ok1 := lowerBaseWhen(base); ok1 {
			if w2, p2, ok2 := lowerBaseWhen(sub); ok2 {
				return andPredicate(w1, w2), maxInt(p1, p2) + 5, true
			}
		}
		// fall through: "early-morning"/"late-night"/"graveyard" are single base tokens.
	}
	return lowerBaseWhen(t)
}

// lowerBaseWhen handles the atomic (non-composite, non-holiday) WHEN tokens.
func lowerBaseWhen(t string) (WhenPredicate, int, bool) {
	switch t {
	case "weekend":
		return WhenPredicate{Weekend: true}, 20, true
	case "weekday":
		return WhenPredicate{Weekday: true}, 20, true
	case "mornings", "early morning", "early-morning", "morning":
		return WhenPredicate{HourFrom: 6, HourTo: 10}, 30, true
	case "daytime":
		return WhenPredicate{HourFrom: 10, HourTo: 17}, 30, true
	case "primetime", "prime-time", "prime time":
		return WhenPredicate{HourFrom: 20, HourTo: 23}, 40, true
	case "late-night", "late night", "latenight":
		return WhenPredicate{HourFrom: 23, HourTo: 2}, 40, true // wraps
	case "overnight", "graveyard":
		return WhenPredicate{HourFrom: 2, HourTo: 6}, 40, true
	default:
		return WhenPredicate{}, 0, false
	}
}

// andPredicate combines two sub-predicates (used for composite WHEN tokens). Only the
// day-kind + hour-window fields compose here (the base tokens set exactly those).
func andPredicate(a, b WhenPredicate) WhenPredicate {
	out := a
	if b.Weekend {
		out.Weekend = true
	}
	if b.Weekday {
		out.Weekday = true
	}
	if b.HourFrom != 0 || b.HourTo != 0 {
		out.HourFrom, out.HourTo = b.HourFrom, b.HourTo
	}
	return out
}

// LowerWhat maps a WHAT token to a *ScopePolicy that only NARROWS. Returns (nil, true) for
// "all" (the base — no narrowing) and (scope, true) for a recognized narrowing token.
// (scope, true) with a needsCeiling hint for kids/family so the caller applies the
// stricter-only audience clamp. Unknown → (nil, false), dropped by the caller.
//
// A `series:<key>` scope is returned as-is; groundPolicy intersects it with grounded picks.
func LowerWhat(token string) (scope *ScopePolicy, kidsCeiling Rating, ok bool) {
	t := strings.ToLower(strings.TrimSpace(token))
	switch {
	case t == "all", t == "":
		return nil, "", true // no narrowing (the base)
	case t == "holiday-matched":
		// Seasonal keyword filter is applied by the seasonal engine, not scope; a rule
		// signals it by carrying a nil scope + relying on the channel seasonal mode. v1:
		// treat as "no extra scope" here; the holiday WHEN already gates the timing.
		return nil, "", true
	case t == "kids":
		return &ScopePolicy{Genres: GenreFilter{Include: kidSafeGenres}}, NormalizeRating("TV-Y7"), true
	case t == "family":
		return &ScopePolicy{Genres: GenreFilter{Include: familyGenres}}, NormalizeRating("TV-PG"), true
	case strings.HasPrefix(t, "series:"):
		key := strings.TrimSpace(token[len("series:"):]) // preserve original case for the key
		if key == "" {
			return nil, "", false
		}
		return &ScopePolicy{Series: []provision.Key{provision.Key(key)}}, "", true
	case strings.HasPrefix(t, "genre-not:"):
		g := strings.TrimSpace(token[len("genre-not:"):])
		if g == "" {
			return nil, "", false
		}
		return &ScopePolicy{Genres: GenreFilter{Exclude: []string{g}}}, "", true
	case strings.HasPrefix(t, "genre:"):
		g := strings.TrimSpace(token[len("genre:"):])
		if g == "" {
			return nil, "", false
		}
		return &ScopePolicy{Genres: GenreFilter{Include: []string{g}}}, "", true
	case strings.HasPrefix(t, "era:"):
		if r, ok := parseEraToken(t[len("era:"):]); ok {
			return &ScopePolicy{Era: r}, "", true
		}
		return nil, "", false
	default:
		return nil, "", false
	}
}

// LowerHow maps a HOW token to a RuleOrdering + a per-rule Window (0 = inherit). Unknown →
// (zero, 0, false), dropped by the caller (the rule then inherits the channel ordering).
func LowerHow(token string) (RuleOrdering, Duration, bool) {
	switch strings.ToLower(strings.TrimSpace(token)) {
	case "syndication":
		return RuleOrdering{Ordering: OrderSyndication}, 0, true
	case "shuffle":
		return RuleOrdering{Ordering: OrderShuffle}, 0, true
	case "marathon":
		// sequential + unbounded block (binge one show) + no breaks + don't truncate a binge.
		// BlockMax -1, NOT 0: applyRuleHow overlays a Separation field only when it's != 0
		// (0 means "inherit the channel's blockMax"), while the separation engine reads any
		// value <= 0 as UNBOUNDED. -1 is the value that is both non-zero (so it overlays) and
		// <= 0 (so it disables the block cap) — letting one series binge without interruption.
		return RuleOrdering{
			Ordering:   OrderSequential,
			Separation: SeparationPolicy{BlockMax: -1},
			NoBreaks:   true,
		}, WindowFull, true
	case "feature", "sequential":
		return RuleOrdering{Ordering: OrderSequential}, 0, true
	default:
		return RuleOrdering{}, 0, false
	}
}

// kidSafeGenres / familyGenres are the include-sets for the kids/family WHAT tokens. These
// are display genre names matched case-insensitively at enforcement (§4); an unknown name
// simply never matches (harmless). The audience CEILING (returned alongside) is the real
// safety gate — genres are a soft preference, the ceiling is the hard floor.
var (
	kidSafeGenres = []string{"Animation", "Family", "Kids"}
	familyGenres  = []string{"Family", "Animation", "Adventure", "Comedy"}
)

// knownHoliday reports whether id is a builtin-calendar holiday (§6), so an unknown
// "holiday:xyz" token is dropped rather than lowered to a never-matching predicate.
func knownHoliday(id string) bool {
	for _, h := range builtinCalendar {
		if h.id == id {
			return true
		}
	}
	return false
}

// parseEraToken parses "1990-1999" / "1990-" / "-1999" into a *Range. Both bounds optional
// but at least one required. Returns (nil, false) on garbage.
func parseEraToken(s string) (*Range, bool) {
	from, to, found := strings.Cut(strings.TrimSpace(s), "-")
	if !found {
		return nil, false
	}
	r := &Range{}
	if from = strings.TrimSpace(from); from != "" {
		n, err := strconv.Atoi(from)
		if err != nil || n <= 0 {
			return nil, false
		}
		r.From = n
	}
	if to = strings.TrimSpace(to); to != "" {
		n, err := strconv.Atoi(to)
		if err != nil || n <= 0 {
			return nil, false
		}
		r.To = n
	}
	if r.From == 0 && r.To == 0 {
		return nil, false
	}
	return r, true
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
