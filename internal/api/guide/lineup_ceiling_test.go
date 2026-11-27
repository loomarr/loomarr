package guide

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/loomarr/loomarr/internal/channels/schedule"
)

// Both draft previews lower what a save would, so their lineup fields carry the save's ceiling,
// which is declared once as schedule.MaxLineupEntries (struct tags cannot spell a constant).
func TestDraftPreviewLineupsCarryTheSharedCeiling(t *testing.T) {
	want := strconv.Itoa(schedule.MaxLineupEntries)
	for name, typ := range map[string]reflect.Type{
		"programming/preview": reflect.TypeFor[previewProgrammingInput](),
		"programming/changes": reflect.TypeFor[previewProgrammingChangesInput](),
	} {
		body, ok := typ.FieldByName("Body")
		if !ok {
			t.Fatalf("%s has no Body", name)
		}
		f, ok := body.Type.FieldByName("Lineup")
		if !ok {
			t.Fatalf("%s has no Lineup", name)
		}
		if got := f.Tag.Get("maxItems"); got != want {
			t.Errorf("%s: lineup maxItems = %q, want %q", name, got, want)
		}
	}
}
