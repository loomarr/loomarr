package schedule

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
)

// A fixed reference clock: 2026-07-25 is a Saturday, 2026-07-23 a Thursday. Using UTC
// so weekday/hour assertions are container-clock deterministic (§6.5 rules evaluate the
// wall-clock `now` the reconcile passes).
var (
	satNoon = time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC) // Saturday 12:00
	thu9am  = time.Date(2026, 7, 23, 9, 0, 0, 0, time.UTC)  // Thursday 09:00
	thu1am  = time.Date(2026, 7, 23, 1, 0, 0, 0, time.UTC)  // Thursday 01:00 (late night)
	oct15   = time.Date(2026, 10, 15, 20, 0, 0, 0, time.UTC)
	jul4    = time.Date(2026, 7, 4, 20, 0, 0, 0, time.UTC)
)

func TestWhenPredicate_Matches(t *testing.T) {
	dref := func(y int, m time.Month, d int) *time.Time {
		v := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
		return &v
	}
	cases := []struct {
		name string
		w    WhenPredicate
		now  time.Time
		want bool
	}{
		{"zero predicate always matches", WhenPredicate{}, thu9am, true},
		{"zero predicate matches even a zero clock", WhenPredicate{}, time.Time{}, true},
		{"non-zero predicate never matches zero clock", WhenPredicate{Weekend: true}, time.Time{}, false},

		{"weekend on saturday", WhenPredicate{Weekend: true}, satNoon, true},
		{"weekend on thursday", WhenPredicate{Weekend: true}, thu9am, false},
		{"weekday on thursday", WhenPredicate{Weekday: true}, thu9am, true},
		{"weekday on saturday", WhenPredicate{Weekday: true}, satNoon, false},

		{"days list hit", WhenPredicate{Days: []time.Weekday{time.Thursday}}, thu9am, true},
		{"days list miss", WhenPredicate{Days: []time.Weekday{time.Monday, time.Friday}}, thu9am, false},

		{"hour window [8,10) contains 9", WhenPredicate{HourFrom: 8, HourTo: 10}, thu9am, true},
		{"hour window [8,10) excludes 12", WhenPredicate{HourFrom: 8, HourTo: 10}, satNoon, false},
		{"wrapping late-night [23,2) contains 1", WhenPredicate{HourFrom: 23, HourTo: 2}, thu1am, true},
		{"wrapping late-night [23,2) excludes 9", WhenPredicate{HourFrom: 23, HourTo: 2}, thu9am, false},

		{"holiday halloween in october", WhenPredicate{Holiday: "halloween"}, oct15, true},
		{"holiday halloween in july", WhenPredicate{Holiday: "halloween"}, jul4, false},
		{"unknown holiday never matches", WhenPredicate{Holiday: "arbor-day"}, oct15, false},

		{"date range contains", WhenPredicate{DateFrom: dref(2026, 7, 1), DateTo: dref(2026, 7, 31)}, thu9am, true},
		{"date range before start", WhenPredicate{DateFrom: dref(2026, 8, 1)}, thu9am, false},
		{"date range after end", WhenPredicate{DateTo: dref(2026, 6, 30)}, thu9am, false},

		// AND semantics: all set sub-predicates must hold.
		{"weekend AND hour both hold", WhenPredicate{Weekend: true, HourFrom: 10, HourTo: 14}, satNoon, true},
		{"weekend holds but hour fails", WhenPredicate{Weekend: true, HourFrom: 18, HourTo: 22}, satNoon, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.w.Matches(c.now); got != c.want {
				t.Errorf("Matches(%v) = %v, want %v", c.now, got, c.want)
			}
		})
	}
}

// Seasonality is the archetypal time-conditional rule (§6/§6.5): the rule engine's
// holiday When-predicate and the seasonal engine MUST share one calendar, so a
// When{Holiday:"christmas"} rule is active exactly when the seasonal engine considers
// christmas in-window. This locks that invariant across the whole year — if a future edit
// diverges the two (e.g. changes a window in one place only), this fails. It's the Phase-2
// "seasonal-as-a-rule" guarantee: one calendar, two consumers, provably in agreement.
func TestHolidayRuleAndSeasonalShareOneCalendar(t *testing.T) {
	// Walk every day of a full year; for each builtin holiday, the rule predicate
	// (inHoliday) and the seasonal engine (holidayActive) must agree.
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for _, h := range builtinCalendar {
		for d := 0; d < 366; d++ {
			now := base.AddDate(0, 0, d)
			ruleSide := inHoliday(h.id, now)
			seasonalSide := holidayActive(h, now)
			if ruleSide != seasonalSide {
				t.Fatalf("calendar divergence for %q on %s: rule=%v seasonal=%v",
					h.id, now.Format("2006-01-02"), ruleSide, seasonalSide)
			}
		}
	}
}

// A holiday When-rule and SeasonalPolicy resolve the SAME active-holiday set from the same
// clock — the composition point in ComputeDesiredAt (rule scope then seasonal bench) relies
// on this so a "December holiday rule" and seasonal `auto` never contradict each other.
func TestHolidayRuleMatchesActiveHolidays(t *testing.T) {
	dec20 := time.Date(2026, time.December, 20, 12, 0, 0, 0, time.UTC)
	jul14 := time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC)

	// In December, the christmas rule matches AND christmas is in the seasonal active set.
	if !inHoliday("christmas", dec20) {
		t.Error("christmas rule should match Dec 20")
	}
	active := activeHolidays(dec20, []string{"christmas"})
	if len(active) != 1 || active[0].id != "christmas" {
		t.Errorf("seasonal should have christmas active on Dec 20, got %v", active)
	}
	// In July, neither fires.
	if inHoliday("christmas", jul14) {
		t.Error("christmas rule must not match Jul 14")
	}
	if len(activeHolidays(jul14, []string{"christmas"})) != 0 {
		t.Error("seasonal should have no christmas active on Jul 14")
	}
}

func TestPickRule_HighestPriorityThenListOrder(t *testing.T) {
	base := SchedulingRule{ID: "base"} // zero When → always matches
	weekend := SchedulingRule{ID: "weekend", Priority: 10, When: WhenPredicate{Weekend: true}}
	holiday := SchedulingRule{ID: "holiday", Priority: 20, When: WhenPredicate{Holiday: "halloween"}}

	t.Run("no rules → no match", func(t *testing.T) {
		if _, ok := pickRule(nil, satNoon); ok {
			t.Fatal("expected no match on empty rules")
		}
	})
	t.Run("only base matches on a weekday", func(t *testing.T) {
		r, ok := pickRule([]SchedulingRule{base, weekend}, thu9am)
		if !ok || r.ID != "base" {
			t.Fatalf("got %q ok=%v, want base", r.ID, ok)
		}
	})
	t.Run("higher-priority weekend beats base on saturday", func(t *testing.T) {
		r, ok := pickRule([]SchedulingRule{base, weekend}, satNoon)
		if !ok || r.ID != "weekend" {
			t.Fatalf("got %q, want weekend (priority 10 > 0)", r.ID)
		}
	})
	t.Run("holiday beats weekend when both match", func(t *testing.T) {
		// oct15 is a Thursday, so use a holiday that also lands on a weekend. Halloween
		// window covers all of October; pick a Saturday in it.
		octSat := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC) // Saturday in Halloween window
		r, ok := pickRule([]SchedulingRule{base, weekend, holiday}, octSat)
		if !ok || r.ID != "holiday" {
			t.Fatalf("got %q, want holiday (priority 20 > 10)", r.ID)
		}
	})
	t.Run("ties break by list order (first wins)", func(t *testing.T) {
		a := SchedulingRule{ID: "a", Priority: 5, When: WhenPredicate{Weekend: true}}
		b := SchedulingRule{ID: "b", Priority: 5, When: WhenPredicate{Weekend: true}}
		r, _ := pickRule([]SchedulingRule{a, b}, satNoon)
		if r.ID != "a" {
			t.Fatalf("got %q, want a (equal priority → first in list)", r.ID)
		}
	})
}

func TestOrderingAt_RuleThenPolicyThenStrategy(t *testing.T) {
	weekendShuffle := SchedulingRule{ID: "w", Priority: 10, When: WhenPredicate{Weekend: true},
		How: RuleOrdering{Ordering: OrderShuffle}}
	for _, tc := range []struct {
		name     string
		policy   ChannelPolicy
		strategy Strategy
		at       time.Time
		want     OrderingMode
	}{
		{"strategy when nothing overrides", ChannelPolicy{}, Shuffle, thu9am, OrderShuffle},
		{"a channel with no strategy deals a syndication deck", ChannelPolicy{}, "", thu9am, OrderSyndication},
		{"time-slot strategy airs in order", ChannelPolicy{}, TimeSlot, thu9am, OrderSequential},
		{"policy beats strategy", ChannelPolicy{Ordering: OrderSyndication}, Shuffle, thu9am, OrderSyndication},
		{"active rule beats policy", ChannelPolicy{Ordering: OrderSequential, Rules: []SchedulingRule{weekendShuffle}}, Sequential, satNoon, OrderShuffle},
		{"inactive rule leaves the policy", ChannelPolicy{Ordering: OrderSequential, Rules: []SchedulingRule{weekendShuffle}}, Sequential, thu9am, OrderSequential},
	} {
		if got := OrderingAt(tc.policy, tc.strategy, tc.at); got != tc.want {
			t.Errorf("%s: OrderingAt = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// Shuffle and syndication both deal a seeded deck (slotting.go), so only sequential airs a
// show's episodes in order.
func TestOrderingMode_AirsInOrder(t *testing.T) {
	for mode, want := range map[OrderingMode]bool{OrderSequential: true, OrderShuffle: false, OrderSyndication: false} {
		if got := mode.AirsInOrder(); got != want {
			t.Errorf("%q.AirsInOrder() = %v, want %v", mode, got, want)
		}
	}
}

// ActiveRuleAt is the exported attribution window onto pickRule (§8.1): it must name the
// SAME rule pickRule selects, and report base-policy (matched:false) when nothing matches.
func TestActiveRuleAt_Attribution(t *testing.T) {
	weekend := SchedulingRule{ID: "w", Label: "Weekend TNG marathon", Priority: 10, When: WhenPredicate{Weekend: true}}
	holiday := SchedulingRule{ID: "h", Label: "Spooky Season", Priority: 20, When: WhenPredicate{Holiday: "halloween"}}
	rules := []SchedulingRule{weekend, holiday}

	t.Run("no rule matches → base policy, matched false", func(t *testing.T) {
		a := ActiveRuleAt(rules, thu9am) // Thursday, not a weekend or holiday
		if a.Matched || a.Label != "Base policy" || a.ID != "" {
			t.Fatalf("got %+v, want unmatched base policy", a)
		}
	})
	t.Run("weekend matches → attributed to the weekend rule with its label", func(t *testing.T) {
		a := ActiveRuleAt(rules, satNoon)
		if !a.Matched || a.ID != "w" || a.Label != "Weekend TNG marathon" || a.Priority != 10 {
			t.Fatalf("got %+v, want the weekend rule", a)
		}
	})
	t.Run("highest priority wins the attribution", func(t *testing.T) {
		octSat := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC) // Saturday in the Halloween window
		a := ActiveRuleAt(rules, octSat)
		if a.ID != "h" || a.Priority != 20 {
			t.Fatalf("got %+v, want the higher-priority holiday rule", a)
		}
	})
	t.Run("nil rules → base policy", func(t *testing.T) {
		if a := ActiveRuleAt(nil, satNoon); a.Matched {
			t.Fatalf("nil rules should never match, got %+v", a)
		}
	})
}

// Describe uses an explicit Label when set, else synthesizes one from the predicate + how,
// so an unnamed/legacy rule still reads sensibly in the preview.
func TestSchedulingRule_Describe(t *testing.T) {
	cases := []struct {
		name string
		rule SchedulingRule
		want string
	}{
		{"explicit label wins", SchedulingRule{Label: "My Rule", When: WhenPredicate{Weekend: true}}, "My Rule"},
		{"synthesized weekend + marathon", SchedulingRule{
			When: WhenPredicate{Weekend: true},
			How:  RuleOrdering{Ordering: OrderSequential, NoBreaks: true},
		}, "Weekends — Marathon"},
		{"synthesized holiday", SchedulingRule{When: WhenPredicate{Holiday: "christmas"}}, "Christmas"},
		{"synthesized hours", SchedulingRule{When: WhenPredicate{HourFrom: 6, HourTo: 10}}, "06:00–10:00"},
		{"zero rule → Always", SchedulingRule{}, "Always"},
		{"how only", SchedulingRule{How: RuleOrdering{Ordering: OrderSyndication}}, "Always — Syndication"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.rule.Describe(); got != tc.want {
				t.Errorf("Describe() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveWindow_Inheritance(t *testing.T) {
	const day = 24 * time.Hour
	cases := []struct {
		name            string
		ruleW, channelW Duration
		defaultW        time.Duration
		want            time.Duration
	}{
		{"rule wins", Duration(6 * time.Hour), Duration(12 * time.Hour), day, 6 * time.Hour},
		{"channel wins when rule inherits", 0, Duration(12 * time.Hour), day, 12 * time.Hour},
		{"global default when both inherit", 0, 0, day, day},
		{"rule WindowFull → unbounded (0)", WindowFull, Duration(12 * time.Hour), day, 0},
		{"channel WindowFull → unbounded (0)", 0, WindowFull, day, 0},
		{"all inherit, no default → unbounded", 0, 0, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolveWindow(c.ruleW, c.channelW, c.defaultW); got != c.want {
				t.Errorf("resolveWindow(%v,%v,%v) = %v, want %v", c.ruleW, c.channelW, c.defaultW, got, c.want)
			}
		})
	}
}

func TestWindowIndex_ConstantWithinAdvancesAcross(t *testing.T) {
	const win = 24 * time.Hour
	base := time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC)

	// Two moments in the SAME 24h window → identical index (idempotent seed → no re-push).
	a := base.Add(1 * time.Hour)
	b := base.Add(23 * time.Hour)
	if windowIndex(a, win, nil) != windowIndex(b, win, nil) {
		t.Errorf("indices differ within a window: %d vs %d", windowIndex(a, win, nil), windowIndex(b, win, nil))
	}
	// A moment in the NEXT window → index advances by exactly 1.
	next := base.Add(25 * time.Hour)
	if windowIndex(next, win, nil) != windowIndex(a, win, nil)+1 {
		t.Errorf("next window index = %d, want %d", windowIndex(next, win, nil), windowIndex(a, win, nil)+1)
	}
	// Zero window or zero clock → 0 (un-windowed behavior preserved).
	if windowIndex(a, 0, nil) != 0 {
		t.Error("zero window should yield index 0")
	}
	if windowIndex(time.Time{}, win, nil) != 0 {
		t.Error("zero clock should yield index 0")
	}
}

// #1675: the window grid is laid on the guide's wall clock. On a UTC grid a daily window turned at
// 00:00 UTC, which is 20:00 in New York in summer: primetime. It must turn at local midnight, and
// across a DST change the local day is 23 h or 25 h long while the index still advances by one.
func TestWindowGrid_TurnsAtLocalMidnightAcrossDST(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("zone database unavailable: %v", err)
	}
	const day = 24 * time.Hour
	primetime := time.Date(2026, time.July, 23, 20, 30, 0, 0, ny) // 00:30 UTC the next day
	if got, want := WindowStart(primetime, day, ny), time.Date(2026, time.July, 23, 0, 0, 0, 0, ny); !got.Equal(want) {
		t.Errorf("window containing 20:30 New York opened %v, want local midnight %v", got, want)
	}
	if got, want := NextWindowStart(primetime, day, ny), time.Date(2026, time.July, 24, 0, 0, 0, 0, ny); !got.Equal(want) {
		t.Errorf("window containing 20:30 New York closes %v, want the next local midnight %v", got, want)
	}

	for _, tc := range []struct {
		name string
		day  time.Time // local midnight opening the DST day
		long time.Duration
	}{
		{"spring forward", time.Date(2026, time.March, 8, 0, 0, 0, 0, ny), 23 * time.Hour},
		{"fall back", time.Date(2026, time.November, 1, 0, 0, 0, 0, ny), 25 * time.Hour},
	} {
		noon := tc.day.Add(12 * time.Hour)
		opened, closes := WindowStart(noon, day, ny), NextWindowStart(noon, day, ny)
		if !opened.Equal(tc.day) || closes.Sub(opened) != tc.long {
			t.Errorf("%s: window %v → %v (%v), want %v lasting %v", tc.name, opened, closes, closes.Sub(opened), tc.day, tc.long)
		}
		if windowIndex(closes, day, ny) != windowIndex(noon, day, ny)+1 {
			t.Errorf("%s: index did not advance by exactly one across the day", tc.name)
		}
	}
}

func TestWindowSlice(t *testing.T) {
	prog := func(id string, ms int64) Slot { return Slot{Kind: SlotProgram, Key: provision.Key(id), DurationMs: ms} }
	hour := int64(60 * 60 * 1000)
	// 4×1h films — total 4h. With a 2h window this is BIGGER than the window (rotates).
	deck := []Slot{prog("a", hour), prog("b", hour), prog("c", hour), prog("d", hour)}
	keys := func(ss []Slot) string {
		out := ""
		for _, s := range ss {
			out += string(s.Key)
		}
		return out
	}

	t.Run("index 0 keeps the head slice meeting the window budget", func(t *testing.T) {
		got := windowSlice(deck, 2*time.Hour, 0) // first two hours meet the budget
		if keys(got) != "ab" {
			t.Fatalf("slice = %q, want ab", keys(got))
		}
	})
	t.Run("next window TILES the catalog (continues where it left off, not a slide)", func(t *testing.T) {
		// Window holds 2 films; stride = 2, so window 1 airs films [c,d], window 2 wraps to [a,b].
		if got := keys(windowSlice(deck, 2*time.Hour, 1)); got != "cd" {
			t.Fatalf("window 1 = %q, want cd (tiles, not slides to bc)", got)
		}
		if got := keys(windowSlice(deck, 2*time.Hour, 2)); got != "ab" {
			t.Fatalf("window 2 = %q, want ab (wrapped back to the head)", got)
		}
	})
	t.Run("coverage: every film airs across a full cycle (no starved tail)", func(t *testing.T) {
		seen := map[string]bool{}
		for i := int64(0); i < 2; i++ { // ceil(4h/2h) = 2 windows cover the catalog
			for _, s := range windowSlice(deck, 2*time.Hour, i) {
				seen[string(s.Key)] = true
			}
		}
		for _, want := range []string{"a", "b", "c", "d"} {
			if !seen[want] {
				t.Fatalf("film %q never aired across a full cycle (starvation)", want)
			}
		}
	})
	t.Run("idempotent within a window: same index → identical slice", func(t *testing.T) {
		// Two independent calls at the same index must agree (idempotency / no thrash). Build
		// a fresh deck for the second call so it's genuinely recomputed, not the same slice ref.
		deck2 := append([]Slot(nil), deck...)
		if keys(windowSlice(deck, 2*time.Hour, 3)) != keys(windowSlice(deck2, 2*time.Hour, 3)) {
			t.Fatal("same index must yield the identical slice (idempotency / no thrash)")
		}
	})
	t.Run("catalog fits in one window → whole deck every window (nothing to rotate)", func(t *testing.T) {
		if got := windowSlice(deck, 10*time.Hour, 0); len(got) != 4 {
			t.Fatalf("kept %d, want 4", len(got))
		}
		if got := windowSlice(deck, 10*time.Hour, 5); len(got) != 4 { // any index, still whole
			t.Fatalf("kept %d at index 5, want 4 (small pool never rotates)", len(got))
		}
	})
	t.Run("always keeps at least one program even if it exceeds the window", func(t *testing.T) {
		got := windowSlice([]Slot{prog("x", 3*hour)}, 1*time.Hour, 0) // single 3h program, 1h window
		if len(got) != 1 {
			t.Fatalf("kept %d, want 1 (never dark)", len(got))
		}
	})
	t.Run("zero window keeps the whole deck (unbounded / WindowFull path)", func(t *testing.T) {
		if got := windowSlice(deck, 0, 7); len(got) != 4 {
			t.Fatalf("kept %d, want 4 (unbounded, no rotation)", len(got))
		}
	})
	t.Run("empty deck stays empty", func(t *testing.T) {
		if got := windowSlice(nil, time.Hour, 0); len(got) != 0 {
			t.Fatalf("kept %d, want 0", len(got))
		}
	})
	t.Run("zero-duration deck (unknown runtimes) plays whole, never a meaningless prefix", func(t *testing.T) {
		zero := []Slot{prog("a", 0), prog("b", 0), prog("c", 0)}
		if got := windowSlice(zero, 2*time.Hour, 1); len(got) != 3 {
			t.Fatalf("kept %d, want 3 (0-runtime deck airs whole)", len(got))
		}
	})
}
