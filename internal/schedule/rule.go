package schedule

import (
	"fmt"
	"strings"
	"time"
)

// Curation rules (programming-design §6.5): wall-clock-conditional programming. A
// channel carries an ordered []SchedulingRule; at reconcile the pure lineup builder
// evaluates `now` → picks the highest-priority matching rule → its WHAT narrows the
// eligible pool and its HOW drives ordering. §5 ordering and §6 seasonality both
// compose into this model. Everything here is DETERMINISTIC and I/O-free — the same
// (rules, now) always resolves to the same active rule, so reconcile stays idempotent.
//
// The LLM only proposes rule VALUES (from a closed preset vocabulary, grounded +
// clamped in the suggester); this package evaluates them. Nothing here trusts the
// model — a rule's WHAT can only NARROW the channel's own scope, never widen it, and
// a rule can never raise the audience ceiling (§4 is enforced upstream, before rules).

// SchedulingRule is one (WHEN, WHAT, HOW) curation rule (§6.5). All fields optional;
// a zero rule is an always-match base rule that changes nothing.
type SchedulingRule struct {
	// ID is a stable identifier for the rule (for the editor + preview attribution).
	ID string `json:"id,omitempty"`
	// Source is the rule's provenance (§8.2): RuleSourceLLM (the suggester's groundRules
	// seeded it) or RuleSourceOperator (hand-authored in the rules editor). A refine/
	// re-curation (ChannelPolicy.MergeFromProposal) replaces the LLM rules and preserves
	// the operator's, so refine-authored and hand-authored rules COMPOSE. Empty is treated
	// as "not LLM" (preserved) for back-compat — a rule authored before provenance existed
	// was operator-locked-after-seed, so preserving it keeps the old behavior.
	Source RuleSource `json:"source,omitempty"`
	// Label is the human-readable name the editor/LLM assigns ("Weekend TNG marathon").
	// Attribution in the cycle preview (§8.1) shows it so which rule is active at a moment
	// is legible. Empty ⇒ Describe() synthesizes one from the predicate/how (below), so a
	// rule authored before this field still reads sensibly. Purely cosmetic — the engine
	// never keys behavior on it.
	Label string `json:"label,omitempty"`
	// Priority orders overlapping matches — higher wins; ties break by list order.
	Priority int `json:"priority,omitempty"`
	// When is the time predicate; a zero predicate matches always (the base rule).
	When WhenPredicate `json:"when,omitempty"`
	// What narrows the eligible pool (reuses ScopePolicy). Nil = inherit channel scope
	// (no extra narrowing). It can only intersect the channel's own scope, never widen.
	What *ScopePolicy `json:"what,omitempty"`
	// How overrides ordering/separation for this rule's window (the "marathon vs
	// syndication" texture). Zero = inherit the channel's resolved ordering.
	How RuleOrdering `json:"how,omitempty"`
	// Window overrides the channel's rolling-window horizon for this rule (§6.5).
	// 0 = inherit the channel/global default; a marathon sets it to WindowFull.
	Window Duration `json:"window,omitempty"`
}

// RuleSource is a curation rule's provenance (§8.2) — who authored it, which decides
// whether a refresh proposal may replace it.
type RuleSource string

const (
	// RuleSourceLLM marks a rule the suggester seeded (groundRules). A refine/re-curation
	// replaces these with the fresh proposal's rules.
	RuleSourceLLM RuleSource = "llm"
	// RuleSourceOperator marks a hand-authored rule from the channel's rules editor. A
	// refine never touches these — operator authorship is sticky (§8.2).
	RuleSourceOperator RuleSource = "operator"
)

// isLLM reports whether a refresh proposal may replace this rule. Only an explicitly
// LLM-sourced rule is replaceable; operator rules AND unknown-provenance ("") rules are
// preserved (fail safe — never silently drop authored work).
func (s RuleSource) isLLM() bool { return s == RuleSourceLLM }

// WindowFull is the sentinel that disables truncation for a rule/channel — the whole
// run materializes (a binge/marathon). Distinct from 0, which means "inherit default".
const WindowFull Duration = -1

// RuleOrdering is a rule's HOW: the ordering mode plus the texture overrides that make
// a "marathon" (sequential, long blocks, no breaks) distinct from "syndication".
type RuleOrdering struct {
	// Ordering is the mode; "" (OrderInherit) keeps the channel's resolved ordering.
	Ordering OrderingMode `json:"ordering,omitempty"`
	// Separation overrides the channel separation for this rule (blockMax/gap). Zero
	// fields inherit. A marathon sets BlockMax=0 (unbounded) so a series can binge.
	Separation SeparationPolicy `json:"separation,omitempty"`
	// NoBreaks suppresses commercial-break interleaving for this rule (a marathon runs
	// uninterrupted). Nil-safe: false = inherit the channel's break density.
	NoBreaks bool `json:"noBreaks,omitempty"`
}

// WhenPredicate is a deterministic, composable time predicate (§6.5). Sub-predicates
// AND together; a fully-zero predicate matches every moment (the base rule). Evaluated
// against the container wall-clock `now` at reconcile time.
type WhenPredicate struct {
	// Weekend / Weekday match Sat–Sun / Mon–Fri respectively. Both false = no day-kind
	// constraint; both true is contradictory and matches nothing (caller should not set).
	Weekend bool `json:"weekend,omitempty"`
	Weekday bool `json:"weekday,omitempty"`
	// Days restricts to specific weekdays (0=Sunday…6=Saturday). Empty = any day.
	Days []time.Weekday `json:"days,omitempty"`
	// HourFrom/HourTo is an inclusive-start, exclusive-end hour window [from, to) in
	// 24h wall-clock; it WRAPS when from > to (late-night, e.g. 23→2). Both 0 = any hour.
	HourFrom int `json:"hourFrom,omitempty"`
	HourTo   int `json:"hourTo,omitempty"`
	// Holiday matches when `now` falls in the named holiday's window (reuses the §6
	// builtinCalendar ids: "christmas", "halloween", …). "" = no holiday constraint.
	Holiday string `json:"holiday,omitempty"`
	// DateFrom/DateTo bound an explicit date range (inclusive). Nil = unbounded on that
	// end. For a one-off ("this weekend's marathon") without a recurring predicate.
	DateFrom *time.Time `json:"dateFrom,omitempty"`
	DateTo   *time.Time `json:"dateTo,omitempty"`
}

// isZero reports whether the predicate constrains nothing — an always-match base rule.
func (w WhenPredicate) isZero() bool {
	return !w.Weekend && !w.Weekday && len(w.Days) == 0 &&
		w.HourFrom == 0 && w.HourTo == 0 && w.Holiday == "" &&
		w.DateFrom == nil && w.DateTo == nil
}

// Matches reports whether the predicate holds at `now` (container wall-clock). All set
// sub-predicates must hold (AND). A zero `now` never matches a non-zero predicate — the
// policy-free path (no clock) has no wall-clock rules. A zero predicate matches always.
func (w WhenPredicate) Matches(now time.Time) bool {
	if w.isZero() {
		return true
	}
	if now.IsZero() {
		return false // no clock ⇒ time predicates can't be evaluated
	}
	if w.Weekend && !isWeekend(now.Weekday()) {
		return false
	}
	if w.Weekday && isWeekend(now.Weekday()) {
		return false
	}
	if len(w.Days) > 0 && !containsWeekday(w.Days, now.Weekday()) {
		return false
	}
	if (w.HourFrom != 0 || w.HourTo != 0) && !hourInWindow(now.Hour(), w.HourFrom, w.HourTo) {
		return false
	}
	if w.Holiday != "" && !inHoliday(w.Holiday, now) {
		return false
	}
	if w.DateFrom != nil && now.Before(*w.DateFrom) {
		return false
	}
	if w.DateTo != nil && now.After(*w.DateTo) {
		return false
	}
	return true
}

func isWeekend(d time.Weekday) bool { return d == time.Saturday || d == time.Sunday }

func containsWeekday(days []time.Weekday, d time.Weekday) bool {
	for _, x := range days {
		if x == d {
			return true
		}
	}
	return false
}

// hourInWindow reports whether hour h is in [from, to), wrapping when from > to so a
// late-night window like 23→2 covers 23,0,1. from==to (both non-zero) is a single hour.
func hourInWindow(h, from, to int) bool {
	if from <= to {
		return h >= from && h < to
	}
	// Wrapping window (e.g. 23→2): match if at/after start OR before end.
	return h >= from || h < to
}

// inHoliday reports whether `now` is inside the named builtin holiday's window (§6).
// Reuses builtinCalendar so a holiday rule and seasonal detection share one calendar.
func inHoliday(id string, now time.Time) bool {
	for _, hol := range builtinCalendar {
		if hol.id != id {
			continue
		}
		start, end := hol.window(now.Year())
		if !now.Before(start) && !now.After(end) {
			return true
		}
		// A window straddling the year boundary (e.g. New Year) also checks last year's.
		ps, pe := hol.window(now.Year() - 1)
		return !now.Before(ps) && !now.After(pe)
	}
	return false
}

// applyRuleHow overlays the active rule's ordering/separation onto the already-resolved
// policy (§6.5). A zero How inherits the channel's resolution (no change). Set fields
// override: a marathon's OrderSequential + BlockMax:0/SeriesMinGap:0 turns the rule's
// slice into an uninterrupted binge without disturbing the channel's base ordering.
func applyRuleHow(rp ResolvedPolicy, how RuleOrdering) ResolvedPolicy {
	if how.Ordering != OrderInherit {
		rp.Ordering = how.Ordering
	}
	if how.Separation.BlockMax != 0 {
		rp.Sep.BlockMax = how.Separation.BlockMax
	}
	if how.Separation.SeriesMinGap != 0 {
		rp.Sep.SeriesMinGap = how.Separation.SeriesMinGap.Std()
	}
	if how.Separation.EpisodeNoRepeat != 0 {
		rp.Sep.EpisodeNoRepeat = how.Separation.EpisodeNoRepeat.Std()
	}
	if how.Separation.MovieNoRepeat != 0 {
		rp.Sep.MovieNoRepeat = how.Separation.MovieNoRepeat.Std()
	}
	return rp
}

// applyRuleScope intersects the eligible entries with the active rule's WHAT scope
// (§6.5). It can only NARROW — every entry it keeps already passed the channel's own
// scope + audience filters, so a rule cannot admit content the channel excluded, and
// (crucially) it cannot bypass §4 audience safety. A nil scope keeps everything (the
// rule inherits the channel scope). Reuses the same scope predicates as filterEntries.
func applyRuleScopeWithTrace(entries []LineupEntry, what *ScopePolicy, trace *scheduleTraceBuilder) []LineupEntry {
	if what == nil {
		return entries
	}
	seriesAllow := seriesAllowSet(what.Series)
	out := make([]LineupEntry, 0, len(entries))
	for _, e := range entries {
		if seriesAllow != nil && e.Key.IsSeries() {
			if _, ok := seriesAllow[e.Key]; !ok {
				trace.add(hardFilterFact(e, OutcomeExcluded, ReasonOutOfRuleScope))
				continue
			}
		}
		if e.Year > 0 && !what.Era.Contains(e.Year) {
			trace.add(hardFilterFact(e, OutcomeExcluded, ReasonOutOfRuleScope))
			continue
		}
		if !genreOK(e.Genres, what.Genres) {
			trace.add(hardFilterFact(e, OutcomeExcluded, ReasonOutOfRuleScope))
			continue
		}
		if what.RuntimeMax > 0 && e.RuntimeSec > 0 && e.RuntimeSec > what.RuntimeMax {
			trace.add(hardFilterFact(e, OutcomeExcluded, ReasonOutOfRuleScope))
			continue
		}
		out = append(out, e)
	}
	return out
}

// pickRule returns the highest-priority rule whose When matches `now`, breaking ties by
// list order (first wins). Returns (zeroRule, false) when no rule matches — the caller
// falls through to the channel's base whole-policy behavior (§6.5). Deterministic: the
// same (rules, now) always yields the same active rule.
func pickRule(rules []SchedulingRule, now time.Time) (SchedulingRule, bool) {
	best := -1
	for i, r := range rules {
		if !r.When.Matches(now) {
			continue
		}
		if best == -1 || r.Priority > rules[best].Priority {
			best = i
		}
	}
	if best == -1 {
		return SchedulingRule{}, false
	}
	return rules[best], true
}

// ActiveRuleAttribution names which rule is active at a wall-clock — the cycle-preview
// attribution (§8.1). Matched=false means no rule matched and the channel is on its base
// whole-policy behavior (Label "Base policy", ID "", Priority 0). Derived from the SAME
// pickRule the engine uses, so the preview can never disagree with what reconcile picks.
type ActiveRuleAttribution struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Priority int    `json:"priority"`
	Matched  bool   `json:"matched"`
}

// ActiveRuleAt reports the rule active at `now` for attribution (§8.1). It is the exported
// window onto pickRule: the API's cycle preview calls it to label "what airs at this moment"
// with the exact rule reconcile would select — one code path, so preview == reality.
func ActiveRuleAt(rules []SchedulingRule, now time.Time) ActiveRuleAttribution {
	r, ok := pickRule(rules, now)
	if !ok {
		return ActiveRuleAttribution{Label: "Base policy", Matched: false}
	}
	return ActiveRuleAttribution{ID: r.ID, Label: r.Describe(), Priority: r.Priority, Matched: true}
}

// Describe returns the rule's display label: its explicit Label if set, else a synthesized
// one from the WHEN predicate + HOW ordering (so a rule authored before the Label field, or
// one the LLM left unnamed, still reads sensibly in the preview). Deterministic + I/O-free.
func (r SchedulingRule) Describe() string {
	if strings.TrimSpace(r.Label) != "" {
		return r.Label
	}
	when := r.When.describe()
	how := r.How.describe()
	switch {
	case when != "" && how != "":
		return when + " — " + how
	case when != "":
		return when
	case how != "":
		return how
	default:
		return "Rule"
	}
}

// describe renders a WhenPredicate as a short human phrase for a synthesized rule label.
func (w WhenPredicate) describe() string {
	if w.isZero() {
		return "Always"
	}
	parts := []string{}
	if w.Holiday != "" {
		parts = append(parts, capitalize(w.Holiday))
	}
	switch {
	case w.Weekend:
		parts = append(parts, "Weekends")
	case w.Weekday:
		parts = append(parts, "Weekdays")
	}
	if w.HourFrom != 0 || w.HourTo != 0 {
		parts = append(parts, fmt.Sprintf("%02d:00–%02d:00", w.HourFrom, w.HourTo))
	}
	if len(parts) == 0 {
		return "Scheduled"
	}
	return strings.Join(parts, " ")
}

// capitalize upper-cases the first byte of an ASCII holiday id ("christmas" → "Christmas").
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// describe renders a RuleOrdering as a short human phrase for a synthesized rule label.
func (o RuleOrdering) describe() string {
	switch o.Ordering {
	case OrderSequential:
		if o.NoBreaks {
			return "Marathon"
		}
		return "In order"
	case OrderShuffle:
		return "Shuffle"
	case OrderSyndication:
		return "Syndication"
	default:
		if o.NoBreaks {
			return "No breaks"
		}
		return ""
	}
}
