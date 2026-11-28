package split

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"

	"github.com/loomarr/loomarr/internal/filler/structure"
)

// declaredStrings returns the values of every constant of one structure type, read from the syntax
// tree so that a new constant cannot be added without appearing here.
func declaredStrings(t *testing.T, typeName string) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "../structure/types.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var values []string
	ast.Inspect(file, func(node ast.Node) bool {
		spec, ok := node.(*ast.ValueSpec)
		if !ok {
			return true
		}
		if ident, ok := spec.Type.(*ast.Ident); !ok || ident.Name != typeName {
			return true
		}
		for _, value := range spec.Values {
			literal, err := strconv.Unquote(value.(*ast.BasicLit).Value)
			if err != nil {
				t.Fatal(err)
			}
			values = append(values, literal)
		}
		return true
	})
	return values
}

func TestEveryStructureUnitHasAStatedKindProjection(t *testing.T) {
	want := map[structure.Unit]Kind{
		structure.UnitStandalone:       KindSingleUnit,
		structure.UnitProgrammeExcerpt: KindSingleUnit,
		structure.UnitCompilation:      KindCompilationBreak,
		structure.UnitProgrammeSpots:   KindProgrammeSpots,
		structure.UnitUnusable:         "",
		structure.UnitUnclear:          "",
	}
	units := declaredStrings(t, "Unit")
	if len(units) != len(want) {
		t.Fatalf("structure declares units %q, the table covers %d", units, len(want))
	}
	for _, value := range units {
		kind, stated := want[structure.Unit(value)]
		got, err := projectedStructureKind(structure.Unit(value))
		if !stated || got != kind || (err == nil) != (kind != "") {
			t.Errorf("unit %q projects to %q (err %v), stated %q", value, got, err, kind)
		}
	}
}

func TestEveryStructureDispositionHasAStatedProjection(t *testing.T) {
	want := map[structure.Disposition]Disposition{
		structure.DispositionFillerCandidate: DispositionKeep,
		structure.DispositionNonFiller:       DispositionDiscard,
		structure.DispositionUnresolved:      "",
	}
	dispositions := declaredStrings(t, "Disposition")
	if len(dispositions) != len(want) {
		t.Fatalf("structure declares dispositions %q, the table covers %d", dispositions, len(want))
	}
	for _, value := range dispositions {
		disposition, stated := want[structure.Disposition(value)]
		got, err := projectedStructureDisposition(structure.Disposition(value))
		if !stated || got != disposition || (err == nil) != (disposition != "") {
			t.Errorf("disposition %q projects to %q (err %v), stated %q", value, got, err, disposition)
		}
	}
}
