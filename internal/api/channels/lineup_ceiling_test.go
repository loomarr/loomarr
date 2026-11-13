package channels

import (
	"fmt"
	"reflect"
	"strconv"
	"testing"

	"github.com/loomarr/loomarr/internal/channels/schedule"
)

// maxItemsOf is the `maxItems` tag of the named field of the struct type t (following one level of
// anonymous Body struct when the field is not on t itself).
func maxItemsOf(t *testing.T, typ reflect.Type, field string) string {
	t.Helper()
	if f, ok := typ.FieldByName(field); ok {
		return f.Tag.Get("maxItems")
	}
	if body, ok := typ.FieldByName("Body"); ok {
		return maxItemsOf(t, body.Type, field)
	}
	t.Fatalf("%s has no field %s", typ, field)
	return ""
}

// Struct tags cannot spell a constant, so the lineup ceiling is declared once, as
// schedule.MaxLineupEntries, and every field that bounds a lineup is held to it here.
func TestLineupFieldsCarryTheSharedCeiling(t *testing.T) {
	want := strconv.Itoa(schedule.MaxLineupEntries)
	for name, typ := range map[string]reflect.Type{
		"PATCH /v1/channels/{id}":   reflect.TypeFor[updateChannelInput](),
		"manual channel submission": reflect.TypeFor[submitManualChannelInput](),
	} {
		if got := maxItemsOf(t, typ, "Lineup"); got != want {
			t.Errorf("%s: lineup maxItems = %q, want %q (schedule.MaxLineupEntries)", name, got, want)
		}
	}
}

// The check behind the tag, for a lineup that reaches the lowering without the schema (a caller
// inside the process): the same ceiling, from the same constant, refused with a message that says
// the number.
func TestLineupEditPastTheCeilingIsRefused(t *testing.T) {
	dto := func(n int) []LineupEntryDTO {
		out := make([]LineupEntryDTO, n)
		for i := range out {
			out[i] = LineupEntryDTO{Key: fmt.Sprintf("movie:tmdb:%d", 90000000+i), Name: fmt.Sprintf("Film %d", i)}
		}
		return out
	}
	for name, tc := range map[string]struct {
		n  int
		ok bool
	}{
		"empty":                 {0, true},
		"one":                   {1, true},
		"one under the ceiling": {schedule.MaxLineupEntries - 1, true},
		"exactly the ceiling":   {schedule.MaxLineupEntries, true},
		"one over the ceiling":  {schedule.MaxLineupEntries + 1, false},
		"far over the ceiling":  {schedule.MaxLineupEntries * 3, false},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LineupEntriesFromDTOs(dto(tc.n))
			if (err == nil) != tc.ok {
				t.Fatalf("LineupEntriesFromDTOs(%d) = %v, want ok=%v", tc.n, err, tc.ok)
			}
			if _, merr := mergeLineupEdit(nil, dto(tc.n)); (merr == nil) != tc.ok {
				t.Fatalf("mergeLineupEdit(%d) = %v, want ok=%v", tc.n, merr, tc.ok)
			}
		})
	}
}
