package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/store/storetest"
)

// TestScannedTablesResolvesAliasesAndSkipsSubqueries: SQLite names an aliased table by its alias
// and a subquery by its own name; only real tables read whole count.
func TestScannedTablesResolvesAliasesAndSkipsSubqueries(t *testing.T) {
	tables := map[string]bool{"proposal_jobs": true, "proposal_job_attempts": true, "diagnostic_events": true}
	for _, tc := range []struct {
		name  string
		query string
		plan  []string
		want  []string
	}{
		{"alias", `SELECT a.status FROM proposal_job_attempts a JOIN proposal_jobs j ON j.id = a.job_id`,
			[]string{"SCAN a", "SEARCH j USING INTEGER PRIMARY KEY (rowid=?)"}, []string{"proposal_job_attempts"}},
		{"index scan", `SELECT id FROM proposal_jobs ORDER BY id`,
			[]string{"SCAN proposal_jobs USING COVERING INDEX idx"}, nil},
		{"subquery", `SELECT kind FROM (SELECT 'event' AS kind FROM diagnostic_events LIMIT 1) e`,
			[]string{"CO-ROUTINE e", "SCAN diagnostic_events USING INDEX idx_diagnostic_events_time", "SCAN e"}, nil},
		{"not a table", `SELECT 1`, []string{"SCAN CONSTANT ROW"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := storetest.ScannedTables(tc.query, tc.plan, tables); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ScannedTables = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestQueryPlanCheckSeesAMissingIndex: the check has teeth. Without the suggestion list's index,
// SQLite plans ListSuggestionJobs as a full scan, and the check reports it.
func TestQueryPlanCheckSeesAMissingIndex(t *testing.T) {
	ctx := context.Background()
	db := newSQLiteStore(t).(*sqlStore).db
	query := `SELECT id FROM proposal_jobs WHERE kind = 'suggest' ORDER BY updated_at DESC, id DESC LIMIT ?`
	tables := map[string]bool{"proposal_jobs": true}

	plan, err := storetest.ExplainSQLite(ctx, db, query, []any{10})
	if err != nil {
		t.Fatal(err)
	}
	if got := storetest.ScannedTables(query, plan, tables); got != nil {
		t.Fatalf("with its index the suggestion list scans %v: %v", got, plan)
	}
	if _, err := db.ExecContext(ctx, `DROP INDEX idx_proposal_jobs_kind_updated`); err != nil {
		t.Fatal(err)
	}
	plan, err = storetest.ExplainSQLite(ctx, db, query, []any{10})
	if err != nil {
		t.Fatal(err)
	}
	if got := storetest.ScannedTables(query, plan, tables); !reflect.DeepEqual(got, []string{"proposal_jobs"}) {
		t.Fatalf("without its index the check saw %v, want the scan of proposal_jobs: %v", got, plan)
	}
}
