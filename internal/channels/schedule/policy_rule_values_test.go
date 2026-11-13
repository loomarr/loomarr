package schedule

import (
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/holidayvocab"
)

// A rule whose values name no weekday, hour or holiday validates and then never fires, so the
// channel quietly never airs what its owner scheduled. #3780 bounded how many days and roles there
// can be; the values themselves are closed sets too.
func rulePolicy(when WhenPredicate) ChannelPolicy {
	return ChannelPolicy{Rules: []SchedulingRule{{Label: "first"}, {Label: "second", When: when}}}
}

func TestRuleWhenValuesAreClosedSets(t *testing.T) {
	day := func(y int, m time.Month, d int) *time.Time {
		v := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
		return &v
	}
	valid := map[string]WhenPredicate{
		"zero predicate":               {},
		"Sunday":                       {Days: []time.Weekday{time.Sunday}},
		"Saturday":                     {Days: []time.Weekday{time.Saturday}},
		"every day of the week":        {Days: []time.Weekday{0, 1, 2, 3, 4, 5, 6}},
		"some days, any order":         {Days: []time.Weekday{5, 1, 3}},
		"weekend flag":                 {Weekend: true},
		"morning window":               {HourFrom: 6, HourTo: 10},
		"wrapping late night":          {HourFrom: 23, HourTo: 2},
		"wrapping to midnight":         {HourFrom: 22, HourTo: 0},
		"from midnight":                {HourFrom: 0, HourTo: 5},
		"all day, midnight to 24":      {HourFrom: 0, HourTo: 24},
		"last hour of the day":         {HourFrom: 23, HourTo: 24},
		"first hour of the day":        {HourFrom: 0, HourTo: 1},
		"one-day range":                {DateFrom: day(2026, 12, 24), DateTo: day(2026, 12, 24)},
		"range":                        {DateFrom: day(2026, 12, 24), DateTo: day(2026, 12, 26)},
		"open-ended from":              {DateFrom: day(2026, 12, 24)},
		"open-ended to":                {DateTo: day(2026, 12, 24)},
		"all fields at their extremes": {Days: []time.Weekday{0, 6}, HourFrom: 23, HourTo: 24, DateFrom: day(2026, 1, 1), DateTo: day(2026, 1, 1)},
	}
	for name, when := range valid {
		t.Run("valid/"+name, func(t *testing.T) {
			if err := rulePolicy(when).Validate(); err != nil {
				t.Fatalf("Validate = %v, want nil", err)
			}
		})
	}

	invalid := map[string]struct {
		when WhenPredicate
		want string
	}{
		"weekday 7 (one past Saturday)":  {WhenPredicate{Days: []time.Weekday{7}}, "days"},
		"weekday 9":                      {WhenPredicate{Days: []time.Weekday{9}}, "days"},
		"weekday -1 (one before Sunday)": {WhenPredicate{Days: []time.Weekday{-1}}, "days"},
		"valid then out of range":        {WhenPredicate{Days: []time.Weekday{1, 7}}, "days"},
		"the same day twice":             {WhenPredicate{Days: []time.Weekday{1, 1}}, "days"},
		"a repeat past the first pair":   {WhenPredicate{Days: []time.Weekday{0, 6, 3, 0}}, "days"},
		"eight entries":                  {WhenPredicate{Days: []time.Weekday{0, 1, 2, 3, 4, 5, 6, 0}}, "days"},
		"hourFrom 24 (one past 23)":      {WhenPredicate{HourFrom: 24, HourTo: 5}, "hourFrom"},
		"hourFrom 25":                    {WhenPredicate{HourFrom: 25, HourTo: 30}, "hourFrom"},
		"hourFrom -1":                    {WhenPredicate{HourFrom: -1, HourTo: 5}, "hourFrom"},
		"hourTo 25 (one past 24)":        {WhenPredicate{HourFrom: 1, HourTo: 25}, "hourTo"},
		"hourTo 30":                      {WhenPredicate{HourFrom: 20, HourTo: 30}, "hourTo"},
		"hourTo -1":                      {WhenPredicate{HourFrom: 5, HourTo: -1}, "hourTo"},
		"hourTo alone out of range":      {WhenPredicate{HourTo: 25}, "hourTo"},
		"hourFrom alone out of range":    {WhenPredicate{HourFrom: 24}, "hourFrom"},
		"an empty window, 5 to 5":        {WhenPredicate{HourFrom: 5, HourTo: 5}, "hour"},
		"an empty window, 23 to 23":      {WhenPredicate{HourFrom: 23, HourTo: 23}, "hour"},
		"a range that ends first":        {WhenPredicate{DateFrom: day(2026, 12, 26), DateTo: day(2026, 12, 24)}, "date"},
		"a range that ends a nanosecond first": {WhenPredicate{
			DateFrom: func() *time.Time { v := time.Date(2026, 12, 24, 0, 0, 0, 1, time.UTC); return &v }(),
			DateTo:   day(2026, 12, 24)}, "date"},
	}
	for name, tc := range invalid {
		t.Run("invalid/"+name, func(t *testing.T) {
			err := rulePolicy(tc.when).Validate()
			if err == nil {
				t.Fatal("Validate = nil, want an error")
			}
			// The second rule is the bad one; the message says which, and what.
			if !strings.Contains(err.Error(), "rule 2") || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate = %q, want it to name rule 2 and %q", err, tc.want)
			}
		})
	}
}

// The reason the values are refused: each of those rules matches no hour of any week.
func TestInvalidRuleValuesNeverMatch(t *testing.T) {
	start := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC) // a Monday
	for name, when := range map[string]WhenPredicate{
		"weekday 9":  {Days: []time.Weekday{9}},
		"weekday -1": {Days: []time.Weekday{-1}},
		"hour 25":    {HourFrom: 25, HourTo: 30},
		"5 to 5":     {HourFrom: 5, HourTo: 5},
	} {
		matches := 0
		for h := range 7 * 24 {
			if when.Matches(start.Add(time.Duration(h)*time.Hour), Calendar{}) {
				matches++
			}
		}
		if matches != 0 {
			t.Errorf("%s matches %d hours of a week; the rule is not what the validator says it is", name, matches)
		}
	}
}

// Every rule is checked, not just the first, and a rule's own predicate is the one named.
func TestRuleValuesAreCheckedForEveryRule(t *testing.T) {
	p := ChannelPolicy{Rules: []SchedulingRule{
		{Label: "ok", When: WhenPredicate{Days: []time.Weekday{1}}},
		{Label: "ok too", When: WhenPredicate{HourFrom: 6, HourTo: 10}},
		{Label: "bad", When: WhenPredicate{Days: []time.Weekday{8}}},
	}}
	err := p.Validate()
	if err == nil || !strings.Contains(err.Error(), "rule 3") {
		t.Fatalf("Validate = %v, want rule 3 named", err)
	}
}

func TestFillerRolesAreUnique(t *testing.T) {
	roles := func(r ...string) ChannelPolicy { return ChannelPolicy{Filler: &FillerSelection{Roles: r}} }
	for name, p := range map[string]ChannelPolicy{
		"none":          roles(),
		"one":           roles("commercial"),
		"all eight":     roles("commercial", "bumper", "station_id", "psa", "trailer", "interstitial", "political", "news"),
		"a subset":      roles("news", "commercial"),
		"no filler set": {},
	} {
		if err := p.Validate(); err != nil {
			t.Errorf("%s: Validate = %v, want nil", name, err)
		}
	}
	for name, p := range map[string]ChannelPolicy{
		"the same role eight times":    roles("commercial", "commercial", "commercial", "commercial", "commercial", "commercial", "commercial", "commercial"),
		"twice":                        roles("commercial", "commercial"),
		"twice, not adjacent":          roles("commercial", "news", "commercial"),
		"a distinct run then a repeat": roles("commercial", "bumper", "station_id", "psa", "trailer", "interstitial", "political", "news", "psa"),
	} {
		err := p.Validate()
		if err == nil || !strings.Contains(err.Error(), "role") {
			t.Errorf("%s: Validate = %v, want a duplicate-role error", name, err)
		}
	}
}

// A holiday id is one the household's holiday vocabulary defines, exactly: the engine compares ids
// as written, so a differently-cased or invented one is a rule that never fires. The vocabulary is
// operator-editable, so it is asked, not hard-coded, and the plain Validate (which has none) leaves
// the holiday alone.
func TestRuleHolidaysMustBeDefined(t *testing.T) {
	vocab := holidayvocab.Default()
	var known string
	for _, d := range vocab.Definitions() {
		if known == "" {
			known = d.ID
		}
		if err := rulePolicy(WhenPredicate{Holiday: d.ID}).ValidateWithHolidays(vocab); err != nil {
			t.Errorf("defined holiday %q: %v", d.ID, err)
		}
	}
	if known == "" {
		t.Fatal("the default vocabulary defines no holiday")
	}
	for _, id := range []string{"xyz", "christmass", strings.ToUpper(known), " " + known, known + " ", "holiday:" + known} {
		err := rulePolicy(WhenPredicate{Holiday: id}).ValidateWithHolidays(vocab)
		if err == nil || !strings.Contains(err.Error(), "rule 2") || !strings.Contains(err.Error(), "holiday") {
			t.Errorf("holiday %q: ValidateWithHolidays = %v, want rule 2's unknown holiday", id, err)
		}
	}
	// No holiday constraint is always fine, and the other checks still run.
	if err := rulePolicy(WhenPredicate{}).ValidateWithHolidays(vocab); err != nil {
		t.Errorf("no holiday: %v", err)
	}
	if err := rulePolicy(WhenPredicate{Days: []time.Weekday{9}}).ValidateWithHolidays(vocab); err == nil {
		t.Error("ValidateWithHolidays skipped the value checks Validate makes")
	}
}
