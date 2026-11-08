package schemadoc

import (
	"slices"
	"strings"
	"testing"
)

// Parity names each way the engines can drift, maps types by what each stores, and holds its
// known gaps to shrinking.
func TestParityNamesEachDriftAndHoldsKnownGapsToShrinking(t *testing.T) {
	saved := KnownParityGaps
	t.Cleanup(func() { KnownParityGaps = saved })
	KnownParityGaps = map[string]string{
		"column jobs.done": "listed, still a gap",
		"column jobs.gone": "listed, since fixed",
	}
	lite := []LiveTable{
		{Name: "jobs", Columns: []LiveColumn{
			{Name: "id", Type: "TEXT", NotNull: true, PrimaryKey: true},
			{Name: "count", Type: "INTEGER", NotNull: true},                           // 64-bit in SQLite
			{Name: "size", Type: "BIGINT", NotNull: true},                             // matches bigint
			{Name: "flag", Type: "BOOLEAN", NotNull: true, Checked: true, Flag: true}, // declared a flag
			{Name: "done", Type: "INTEGER", NotNull: true},                            // a listed gap
			{Name: "open", Type: "BOOLEAN", NotNull: true},                            // BOOLEAN that takes a 2
			{Name: "seen", Type: "INTEGER", NotNull: true, Checked: true, Flag: true}, // a flag typed INTEGER
			{Name: "ratio", Type: "REAL"},                                             // 8 bytes in SQLite
			{Name: "doc", Type: "TEXT"},                                               // jsonb, but not declared JSON
			{Name: "body", Type: "TEXT", Checked: true, JSON: true},                   // declared JSON: jsonb
			{Name: "spec", Type: "TEXT", Checked: true, JSON: true},                   // declared JSON, left as text
			{Name: "status", Type: "TEXT", Checked: true},                             // a state column checked on one side only
			{Name: "user_id", Type: "TEXT", References: "users"},                      // SQLite names the parent table alone
			{Name: "lite_only", Type: "TEXT"},
		}, ForeignKeys: [][]string{{"user_id"}}, Indexes: [][]string{{"id"}, {"user_id"}}},
		{Name: "river_job"}, // a library's table: never compared
		{Name: "only_lite"},
	}
	pg := []LiveTable{
		{Name: "jobs", Columns: []LiveColumn{
			{Name: "id", Type: "text", NotNull: true, PrimaryKey: true},
			{Name: "count", Type: "integer", NotNull: true},
			{Name: "size", Type: "bigint", NotNull: true},
			{Name: "flag", Type: "boolean", NotNull: true},
			{Name: "done", Type: "boolean", NotNull: true},
			{Name: "open", Type: "boolean", NotNull: true},
			{Name: "seen", Type: "boolean", NotNull: true},
			{Name: "ratio", Type: "real"},
			{Name: "doc", Type: "jsonb"},
			{Name: "body", Type: "jsonb"},
			{Name: "spec", Type: "text"},
			{Name: "status", Type: "text"},
			{Name: "user_id", Type: "text", References: "users.id"},
			{Name: "pg_only", Type: "text"},
		}, ForeignKeys: [][]string{{"user_id"}}, Indexes: [][]string{{"id"}}},
		{Name: "only_pg"},
	}
	want := []string{
		"column jobs.count: type INTEGER in SQLite, integer in Postgres",
		"column jobs.doc: type TEXT in SQLite, jsonb in Postgres",
		"column jobs.gone is fixed: remove it from KnownParityGaps (was: listed, since fixed)",
		"column jobs.lite_only: only in SQLite",
		"column jobs.open: type BOOLEAN in SQLite, boolean in Postgres",
		"column jobs.pg_only: only in Postgres",
		"column jobs.ratio: type REAL in SQLite, real in Postgres",
		"column jobs.seen: type INTEGER in SQLite, boolean in Postgres",
		"column jobs.spec: type TEXT in SQLite, text in Postgres",
		"column jobs.status: CHECKed true in SQLite, false in Postgres",
		"index jobs(user_id): only in SQLite",
		"table only_lite: only in SQLite",
		"table only_pg: only in Postgres",
	}
	if got := Parity(lite, pg); !slices.Equal(got, want) {
		t.Fatalf("Parity =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}
