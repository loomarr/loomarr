package schedule

import (
	"reflect"
	"strconv"
	"testing"
)

// A rule's series scope is "no larger than a lineup", so it carries the lineup ceiling, declared
// once as MaxLineupEntries (a struct tag cannot spell a constant).
func TestSeriesScopeCarriesTheLineupCeiling(t *testing.T) {
	f, ok := reflect.TypeFor[ScopePolicy]().FieldByName("Series")
	if !ok {
		t.Fatal("ScopePolicy has no Series")
	}
	if got, want := f.Tag.Get("maxItems"), strconv.Itoa(MaxLineupEntries); got != want {
		t.Fatalf("ScopePolicy.Series maxItems = %q, want %q", got, want)
	}
}
