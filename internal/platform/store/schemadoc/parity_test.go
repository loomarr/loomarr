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
			{Name: "count", Type: "INTEGER", NotNull: true},               // 64-bit in SQLite
			{Name: "size", Type: "BIGINT", NotNull: true},                 // matches bigint
			{Name: "flag", Type: "INTEGER", NotNull: true, Checked: true}, // a CHECKed 0/1 flag: boolean
			{Name: "done", Type: "INTEGER", NotNull: true},                // unchecked flag: a listed gap
			{Name: "ratio", Type: "REAL"},                                 // 8 bytes in SQLite
			{Name: "doc", Type: "TEXT"},                                   // jsonb would rewrite it
			{Name: "status", Type: "TEXT", Checked: true},                 // a state column checked on one side only
			{Name: "user_id", Type: "TEXT", References: "users"},          // SQLite names the parent table alone
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
			{Name: "ratio", Type: "real"},
			{Name: "doc", Type: "jsonb"},
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
		"column jobs.pg_only: only in Postgres",
		"column jobs.ratio: type REAL in SQLite, real in Postgres",
		"column jobs.status: CHECKed true in SQLite, false in Postgres",
		"index jobs(user_id): only in SQLite",
		"table only_lite: only in SQLite",
		"table only_pg: only in Postgres",
	}
	if got := Parity(lite, pg); !slices.Equal(got, want) {
		t.Fatalf("Parity =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}
