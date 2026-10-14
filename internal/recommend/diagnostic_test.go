package recommend_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/recommend"
)

func TestDiagnoseOutputClassifiesStructureWithoutRetainingContent(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want recommend.StructuralDiagnostics
	}{
		{
			name: "valid abstention",
			raw:  `{"concepts":[]}`,
			want: recommend.StructuralDiagnostics{RootJSONValid: true, RequiredFieldsValid: true, Abstained: true},
		},
		{
			name: "unknown and effectful fields",
			raw:  `{"concepts":[{"name":"private marker","intent":{"description":"secret","mood":"hidden"},"evidenceIds":["signal:secret"],"approve":true}]}`,
			want: recommend.StructuralDiagnostics{RootJSONValid: true, RequiredFieldsValid: true, UnknownFieldCount: 2, EffectfulFieldCount: 1, ConceptCount: 1},
		},
		{
			name: "missing required field",
			raw:  `{"concepts":[{"name":"private marker","intent":{},"evidenceIds":[]}]}`,
			want: recommend.StructuralDiagnostics{RootJSONValid: true, ConceptCount: 1},
		},
		{
			name: "truncated object",
			raw:  `{"concepts":[{"name":"private marker"`,
			want: recommend.StructuralDiagnostics{Truncated: true},
		},
		{
			name: "non object root",
			raw:  `["private marker"]`,
			want: recommend.StructuralDiagnostics{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := recommend.DiagnoseOutput([]byte(test.raw))
			if got != test.want {
				t.Fatalf("diagnostics = %+v, want %+v", got, test.want)
			}
			blob, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			for _, forbidden := range []string{"private marker", "secret", "hidden", "signal:secret", "approve", "mood"} {
				if strings.Contains(string(blob), forbidden) {
					t.Fatalf("diagnostic artifact retained %q: %s", forbidden, blob)
				}
			}
		})
	}
}
