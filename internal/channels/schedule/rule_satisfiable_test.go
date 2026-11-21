package schedule

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/holidayvocab"
)

// weekHours is the set of the 168 hours of a week (day*24+hour, Sunday first) that satisfy the
// weekly part of a predicate, computed from the documented meaning of each field and nothing in
// the package under test: it is the independent answer the validator and the lowering are held to.
func weekHours(weekend, weekday bool, days []time.Weekday, from, to int) map[int]bool {
	out := map[int]bool{}
	for d := range 7 {
		inDays := len(days) == 0
		for _, x := range days {
			if int(x) == d {
				inDays = true
			}
		}
		isWeekend := d == 0 || d == 6
		if !inDays || (weekend && !isWeekend) || (weekday && isWeekend) {
			continue
		}
		for h := range 24 {
			var inHours bool
			switch {
			case from == 0 && to == 0:
				inHours = true
			case from <= to:
				inHours = h >= from && h < to
			default:
				inHours = h >= from || h < to
			}
			if inHours {
				out[d*24+h] = true
			}
		}
	}
	return out
}

func intersect(a, b map[int]bool) map[int]bool {
	out := map[int]bool{}
	for h := range a {
		if b[h] {
			out[h] = true
		}
	}
	return out
}

func sameHours(a, b map[int]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for h := range a {
		if !b[h] {
			return false
		}
	}
	return true
}

func hoursOf(w WhenPredicate) map[int]bool {
	return weekHours(w.Weekend, w.Weekday, w.Days, w.HourFrom, w.HourTo)
}

// A rule that matches no hour of the week validates and then never fires, so the channel quietly
// never airs what it was scheduled for. Every combination of the weekly fields is held to the
// independent answer: it validates exactly when some hour of the week satisfies it.
func TestRuleMustMatchSomeHourOfTheWeek(t *testing.T) {
	dayLists := map[string][]time.Weekday{
		"no days":      nil,
		"Sunday":       {time.Sunday},
		"Monday":       {time.Monday},
		"Saturday":     {time.Saturday},
		"Sat and Sun":  {time.Saturday, time.Sunday},
		"Mon to Fri":   {1, 2, 3, 4, 5},
		"every day":    {0, 1, 2, 3, 4, 5, 6},
		"Tue and Thur": {2, 4},
	}
	windows := map[string][2]int{
		"any hour":       {0, 0},
		"morning":        {6, 10},
		"late night":     {23, 2},
		"to the day end": {20, 24},
		"to midnight":    {5, 0},
	}
	checked := 0
	for _, weekend := range []bool{false, true} {
		for _, weekday := range []bool{false, true} {
			for dn, days := range dayLists {
				for wn, win := range windows {
					when := WhenPredicate{Weekend: weekend, Weekday: weekday, Days: days, HourFrom: win[0], HourTo: win[1]}
					name := fmt.Sprintf("weekend=%v weekday=%v %s %s", weekend, weekday, dn, wn)
					t.Run(name, func(t *testing.T) {
						want := len(hoursOf(when)) > 0
						err := rulePolicy(when).Validate()
						if want && err != nil {
							t.Fatalf("Validate = %v, want nil: it matches %d hours of the week", err, len(hoursOf(when)))
						}
						if !want && err == nil {
							t.Fatal("Validate = nil for a rule that matches no hour of the week")
						}
						// The same verdict through the vocabulary-aware write path.
						if verr := rulePolicy(when).ValidateWithHolidays(holidayvocab.Default()); (verr == nil) != want {
							t.Fatalf("ValidateWithHolidays = %v, want ok=%v", verr, want)
						}
						// And the engine agrees: a rule Validate accepts fires at some hour.
						fires := false
						for h := range 168 {
							at := time.Date(2024, time.January, 7, 0, 0, 0, 0, time.UTC).Add(time.Duration(h) * time.Hour)
							fires = fires || when.Matches(at, Calendar{})
						}
						if fires != want {
							t.Fatalf("engine fires=%v but the independent count says %v", fires, want)
						}
					})
					checked++
				}
			}
		}
	}
	if checked != 2*2*len(dayLists)*len(windows) {
		t.Fatalf("checked %d combinations", checked)
	}
}

// A holiday or a date range is placed in the year, not the week, so it can neither rescue a
// contradictory weekly part nor make a sound one refused.
func TestRuleSatisfiabilityIgnoresHolidayAndDateRange(t *testing.T) {
	from := time.Date(2026, 12, 24, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 12, 26, 0, 0, 0, 0, time.UTC)
	for name, tc := range map[string]struct {
		when WhenPredicate
		ok   bool
	}{
		"holiday alone":                         {WhenPredicate{Holiday: "christmas"}, true},
		"holiday with weekend":                  {WhenPredicate{Holiday: "christmas", Weekend: true}, true},
		"holiday cannot rescue weekend+weekday": {WhenPredicate{Holiday: "christmas", Weekend: true, Weekday: true}, false},
		"date range alone":                      {WhenPredicate{DateFrom: &from, DateTo: &to}, true},
		"date range cannot rescue weekend+Mon":  {WhenPredicate{DateFrom: &from, DateTo: &to, Weekend: true, Days: []time.Weekday{time.Monday}}, false},
	} {
		t.Run(name, func(t *testing.T) {
			err := rulePolicy(tc.when).ValidateWithHolidays(holidayvocab.Default())
			if (err == nil) != tc.ok {
				t.Fatalf("ValidateWithHolidays = %v, want ok=%v", err, tc.ok)
			}
		})
	}
}

// The error names the rule and says what is wrong in the rule's own words, and carries the rule so
// a caller can say so too.
func TestRuleErrorNamesTheRule(t *testing.T) {
	p := ChannelPolicy{Rules: []SchedulingRule{
		{ID: "r1", Label: "Fine"},
		{ID: "r2", Label: "Cartoons", When: WhenPredicate{Weekend: true, Weekday: true}},
	}}
	err := p.Validate()
	var re *RuleError
	if !errors.As(err, &re) {
		t.Fatalf("Validate = %v, want a *RuleError", err)
	}
	if re.Index != 2 || re.ID != "r2" || re.Label != "Cartoons" {
		t.Errorf("RuleError = %+v, want rule 2 r2 Cartoons", re)
	}
	if !strings.HasPrefix(err.Error(), "channel policy: rule 2: ") {
		t.Errorf("Error() = %q, want it to keep its channel policy: rule 2: prefix", err.Error())
	}
	words := re.Describe()
	for _, want := range []string{"Rule 2", "Cartoons", "never"} {
		if !strings.Contains(words, want) {
			t.Errorf("Describe() = %q, want it to mention %q", words, want)
		}
	}
}

// Policies saved before the check: a rule already stored is not held to it by an edit that leaves
// the rule alone, and an edit that touches it is. Every kind of stored rule the older writers could
// leave is in the table, and the rule is compared as stored (JSON), not by pointer identity.
func TestEditOfAStoredPolicyIsNotRefusedForRulesItLeavesAlone(t *testing.T) {
	deadWeekend := SchedulingRule{ID: "r1", Label: "Weekend weekday", Source: RuleSourceLLM, When: WhenPredicate{Weekend: true, Weekday: true}}
	deadDay := SchedulingRule{ID: "r2", Label: "Sunday and Monday", Source: RuleSourceOperator, When: WhenPredicate{Weekend: true, Days: []time.Weekday{time.Monday}}}
	deadHour := SchedulingRule{ID: "r3", Label: "Single hour", Source: RuleSourceOperator, When: WhenPredicate{HourFrom: 5, HourTo: 5}}
	deadHoliday := SchedulingRule{ID: "r4", Label: "Gone holiday", Source: RuleSourceOperator, When: WhenPredicate{Holiday: "not-a-holiday"}}
	good := SchedulingRule{ID: "r5", Label: "Mornings", When: WhenPredicate{HourFrom: 6, HourTo: 10}}
	stored := ChannelPolicy{Rules: []SchedulingRule{deadWeekend, deadDay, deadHour, deadHoliday, good}}
	vocab := holidayvocab.Default()

	if err := stored.ValidateWithHolidays(vocab); err == nil {
		t.Fatal("setup: the stored policy should not pass a fresh write")
	}

	roundTrip := func(r SchedulingRule) SchedulingRule { // as the editor sends it back
		var out SchedulingRule
		raw, _ := json.Marshal(r)
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	t.Run("an unrelated edit passes", func(t *testing.T) {
		next := stored
		next.Rules = []SchedulingRule{roundTrip(deadWeekend), roundTrip(deadDay), roundTrip(deadHour), roundTrip(deadHoliday), roundTrip(good)}
		next.Audience.Ceiling = "TV-PG"
		if err := next.ValidateEditOf(stored, vocab); err != nil {
			t.Fatalf("ValidateEditOf = %v, want nil: the edit leaves every stored rule alone", err)
		}
	})
	t.Run("a reordered list passes", func(t *testing.T) {
		next := ChannelPolicy{Rules: []SchedulingRule{good, deadHour, deadWeekend}}
		if err := next.ValidateEditOf(stored, vocab); err != nil {
			t.Fatalf("ValidateEditOf = %v, want nil", err)
		}
	})
	t.Run("removing a dead rule passes", func(t *testing.T) {
		next := ChannelPolicy{Rules: []SchedulingRule{good}}
		if err := next.ValidateEditOf(stored, vocab); err != nil {
			t.Fatalf("ValidateEditOf = %v", err)
		}
	})
	t.Run("changing a dead rule without fixing it is refused and named", func(t *testing.T) {
		touched := deadWeekend
		touched.Label = "Renamed"
		next := ChannelPolicy{Rules: []SchedulingRule{good, touched}}
		err := next.ValidateEditOf(stored, vocab)
		var re *RuleError
		if !errors.As(err, &re) || re.Index != 2 || re.Label != "Renamed" {
			t.Fatalf("ValidateEditOf = %v, want rule 2 (Renamed) named", err)
		}
	})
	t.Run("fixing a dead rule passes", func(t *testing.T) {
		fixed := deadWeekend
		fixed.When = WhenPredicate{Weekend: true}
		if err := (ChannelPolicy{Rules: []SchedulingRule{fixed}}).ValidateEditOf(stored, vocab); err != nil {
			t.Fatalf("ValidateEditOf = %v", err)
		}
	})
	t.Run("a new dead rule beside stored ones is refused", func(t *testing.T) {
		added := SchedulingRule{ID: "r9", Label: "New", When: WhenPredicate{Weekday: true, Days: []time.Weekday{time.Sunday}}}
		next := ChannelPolicy{Rules: []SchedulingRule{deadWeekend, added}}
		if err := next.ValidateEditOf(stored, vocab); err == nil {
			t.Fatal("a new rule that never fires was accepted because a stored one did")
		}
	})
	t.Run("the same dead rule under a new id is a new rule", func(t *testing.T) {
		clone := deadWeekend
		clone.ID = "r77"
		if err := (ChannelPolicy{Rules: []SchedulingRule{clone}}).ValidateEditOf(stored, vocab); err == nil {
			t.Fatal("a copy of a stored dead rule was treated as unchanged")
		}
	})
	t.Run("everything else is still checked", func(t *testing.T) {
		next := ChannelPolicy{Rules: []SchedulingRule{deadWeekend}, Audience: AudiencePolicy{Ceiling: "not-a-rating"}}
		if err := next.ValidateEditOf(stored, vocab); err == nil {
			t.Fatal("an off-ladder ceiling passed because the rules were left alone")
		}
	})
	t.Run("with nothing stored it is a fresh write", func(t *testing.T) {
		if err := stored.ValidateEditOf(ChannelPolicy{}, vocab); err == nil {
			t.Fatal("ValidateEditOf against an empty stored policy accepted dead rules")
		}
	})
}
