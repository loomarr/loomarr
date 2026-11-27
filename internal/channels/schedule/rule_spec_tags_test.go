package schedule

import (
	"reflect"
	"strconv"
	"testing"
	"time"
)

// The OpenAPI schema is generated from struct tags, and a client trusts it: a bound it states must
// be the bound the validator enforces, one for one, or the spec promises what the server refuses
// (or the reverse). Each tag is probed at its edge and one past it against WhenPredicate.validate.
func TestRuleSpecTagsAreTheValidatorsBounds(t *testing.T) {
	field := func(name string) reflect.StructField {
		f, ok := reflect.TypeOf(WhenPredicate{}).FieldByName(name)
		if !ok {
			t.Fatalf("WhenPredicate has no field %s", name)
		}
		return f
	}
	bound := func(f reflect.StructField, tag string) int {
		raw, ok := f.Tag.Lookup(tag)
		if !ok {
			t.Fatalf("%s carries no %s tag", f.Name, tag)
		}
		n, err := strconv.Atoi(raw)
		if err != nil {
			t.Fatalf("%s %s = %q", f.Name, tag, raw)
		}
		return n
	}

	days := field("Days")
	lo, hi := bound(days, "minimum"), bound(days, "maximum")
	if bound(days, "maxItems") != 7 || days.Tag.Get("uniqueItems") != "true" {
		t.Errorf("Days tags = %q, want maxItems 7 and uniqueItems", days.Tag)
	}
	for _, c := range []struct {
		day  int
		want bool
	}{{lo - 1, false}, {lo, true}, {hi, true}, {hi + 1, false}} {
		got := WhenPredicate{Days: []time.Weekday{time.Weekday(c.day)}}.validate() == nil
		if got != c.want {
			t.Errorf("weekday %d: validator accepts = %v, the spec's %d..%d says %v", c.day, got, lo, hi, c.want)
		}
	}

	from := field("HourFrom")
	fromLo, fromHi := bound(from, "minimum"), bound(from, "maximum")
	for _, c := range []struct {
		hour int
		want bool
	}{{fromLo - 1, false}, {fromLo, true}, {fromHi, true}, {fromHi + 1, false}} {
		// hourTo 24 never equals an in-range hourFrom, so only hourFrom's own range is probed.
		got := WhenPredicate{HourFrom: c.hour, HourTo: 24}.validate() == nil
		if got != c.want {
			t.Errorf("hourFrom %d: validator accepts = %v, the spec's %d..%d says %v", c.hour, got, fromLo, fromHi, c.want)
		}
	}
	to := field("HourTo")
	toLo, toHi := bound(to, "minimum"), bound(to, "maximum")
	for _, c := range []struct {
		hour int
		want bool
	}{{toLo - 1, false}, {toLo, true}, {toHi, true}, {toHi + 1, false}} {
		got := WhenPredicate{HourFrom: 1, HourTo: c.hour}.validate() == nil
		if got != c.want {
			t.Errorf("hourTo %d: validator accepts = %v, the spec's %d..%d says %v", c.hour, got, toLo, toHi, c.want)
		}
	}

	roles, ok := reflect.TypeOf(FillerSelection{}).FieldByName("Roles")
	if !ok || roles.Tag.Get("uniqueItems") != "true" || bound(roles, "maxItems") != len(fillerRoles) {
		t.Errorf("FillerSelection.Roles tags = %q, want uniqueItems and maxItems %d", roles.Tag, len(fillerRoles))
	}
}
