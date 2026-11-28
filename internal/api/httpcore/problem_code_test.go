package httpcore

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
)

// Only a coded refusal carries `code`: another 409, and huma's own refusals, have none, so a client
// that retries a stale save cannot mistake them for one.
func TestOnlyACodedProblemCarriesACode(t *testing.T) {
	for name, tc := range map[string]struct {
		problem huma.StatusError
		want    string
	}{
		"stale save":     {CodedConflict(ProblemChannelChanged, "Channel changed", "Refresh it."), "channel_changed"},
		"other conflict": {Conflict("Channel already exists", "Pick another number."), ""},
		"huma's own":     {huma.NewError(http.StatusUnprocessableEntity, "validation failed"), ""},
	} {
		raw, err := json.Marshal(tc.problem)
		if err != nil {
			t.Fatal(err)
		}
		var body struct {
			Status int    `json:"status"`
			Code   string `json:"code"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		if body.Code != tc.want || body.Status != tc.problem.GetStatus() {
			t.Errorf("%s: %s, want code %q", name, raw, tc.want)
		}
		if tc.want == "" && strings.Contains(string(raw), `"code"`) {
			t.Errorf("%s: an uncoded problem serialises a code: %s", name, raw)
		}
	}
}

func TestProblemDetailIsPublishedAsErrorModel(t *testing.T) {
	if got := SchemaName(reflect.TypeFor[*ProblemDetail](), "Error"); got != "ErrorModel" {
		t.Fatalf("schema name = %q", got)
	}
	field, _ := reflect.TypeFor[ProblemDetail]().FieldByName("Code")
	if got := field.Tag.Get("enum"); got != string(ProblemChannelChanged) {
		t.Fatalf("code enum = %q, want every ProblemCode constant", got)
	}
}
