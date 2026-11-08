package store

import (
	"maps"
	"slices"
	"testing"
)

func TestCheckedColumnsReadsColumnAndTableConstraints(t *testing.T) {
	ddl := `CREATE TABLE jobs (
	id INTEGER PRIMARY KEY,
	status TEXT NOT NULL CHECK (status IN ('queued', 'done')),
	kind TEXT NOT NULL,
	note TEXT,
	attempts INTEGER NOT NULL DEFAULT 0,
	CONSTRAINT kind_known CHECK(kind IN ('a', 'b') AND (attempts >= 0))
)`
	got := slices.Sorted(maps.Keys(checkedColumns(ddl)))
	for _, want := range []string{"attempts", "kind", "status"} {
		if !slices.Contains(got, want) {
			t.Errorf("checkedColumns = %v, lacks %q", got, want)
		}
	}
	if slices.Contains(got, "note") {
		t.Errorf("checkedColumns = %v, names note, which no CHECK mentions", got)
	}
	if len(checkedColumns(`CREATE TABLE plain (id INTEGER)`)) != 0 {
		t.Error("a table without CHECK constraints has checked columns")
	}
}
