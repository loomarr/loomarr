package storetest

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Querier runs a query; *sql.DB and the store's Handle both are one.
type Querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// CheckQueryPlans asks SQLite, through db (any migrated database), how it plans every statement
// recorded, and fails on a full scan of a table that excused does not name. excused is keyed
// "VERB table" (SELECT, INSERT, UPDATE, DELETE), each with why the statement reads it whole.
//
// On a full run (no -run filter) it also fails when fewer than minStatements were recorded — a
// recorder that saw nothing proves nothing — and when an excuse is no longer needed, so the list
// only shrinks.
func CheckQueryPlans(t testing.TB, db Querier, statements map[string][]any, excused map[string]string, minStatements int) {
	t.Helper()
	ctx := context.Background()
	tables, err := sqliteTables(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	fullRun := flag.Lookup("test.run").Value.String() == ""
	if fullRun && len(statements) < minStatements {
		t.Fatalf("the recorder saw %d statements, want at least %d; the plan check proves nothing", len(statements), minStatements)
	}

	scans := map[string][]string{}
	for query, args := range statements {
		verb := statementVerb(query)
		if verb == "" {
			continue
		}
		plan, err := ExplainSQLite(ctx, db, query, args)
		if err != nil {
			t.Errorf("explain %q: %v", oneLine(query), err)
			continue
		}
		for _, table := range ScannedTables(query, plan, tables) {
			key := verb + " " + table
			scans[key] = append(scans[key], oneLine(query))
		}
	}

	var unexcused []string
	for key, queries := range scans {
		if _, ok := excused[key]; !ok {
			unexcused = append(unexcused, fmt.Sprintf("%s\n    %s", key, strings.Join(queries, "\n    ")))
		}
	}
	sort.Strings(unexcused)
	for _, u := range unexcused {
		t.Errorf("full table scan; give the lookup an index, or excuse it with a reason:\n%s", u)
	}
	if fullRun {
		for key := range excused {
			if _, ok := scans[key]; !ok {
				t.Errorf("%q is excused but no statement scans it any more; delete the excuse", key)
			}
		}
	}
}

// ExplainSQLite plans query with the arguments it ran with. A prepared statement was recorded
// before it had any, and plans the same with each marker bound to NULL.
func ExplainSQLite(ctx context.Context, db Querier, query string, args []any) ([]string, error) {
	if len(args) == 0 {
		args = make([]any, strings.Count(query, "?"))
	}
	rows, err := db.QueryContext(ctx, "EXPLAIN QUERY PLAN "+query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var plan []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			return nil, err
		}
		plan = append(plan, detail)
	}
	return plan, rows.Err()
}

var (
	planScan      = regexp.MustCompile(`^SCAN (\w+)`)
	planDerived   = regexp.MustCompile(`^(?:CO-ROUTINE|MATERIALIZE) (\w+)`)
	tableAndAlias = regexp.MustCompile(`(?i)\b(?:FROM|JOIN|UPDATE|INTO)\s+(\w+)(?:\s+(?:AS\s+)?(\w+))?`)
)

// ScannedTables names the real tables a plan reads whole. SQLite names a table by its alias when
// it has one, and a subquery or CTE by its own name, so aliases resolve through the query text
// and anything the plan materialises itself is not a table.
func ScannedTables(query string, plan []string, tables map[string]bool) []string {
	alias := map[string]string{}
	for _, m := range tableAndAlias.FindAllStringSubmatch(query, -1) {
		alias[m[1]] = m[1]
		if m[2] != "" {
			alias[m[2]] = m[1]
		}
	}
	derived := map[string]bool{}
	for _, detail := range plan {
		if m := planDerived.FindStringSubmatch(detail); m != nil {
			derived[m[1]] = true
		}
	}
	var out []string
	for _, detail := range plan {
		m := planScan.FindStringSubmatch(detail)
		if m == nil || strings.Contains(detail, " INDEX ") || derived[m[1]] {
			continue
		}
		if table := alias[m[1]]; tables[table] {
			out = append(out, table)
		}
	}
	return out
}

// statementVerb is the statement's kind, or "" for one with no plan worth checking (PRAGMA, DDL,
// transaction control, EXPLAIN).
func statementVerb(query string) string {
	fields := strings.Fields(strings.ToUpper(query))
	if len(fields) == 0 {
		return ""
	}
	switch fields[0] {
	case "SELECT", "WITH":
		return "SELECT"
	case "UPDATE", "DELETE", "INSERT":
		return fields[0]
	}
	return ""
}

func sqliteTables(ctx context.Context, db Querier) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table'`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	tables := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables[name] = true
	}
	return tables, rows.Err()
}

func oneLine(query string) string {
	q := strings.Join(strings.Fields(query), " ")
	if len(q) > 240 {
		q = q[:240] + "…"
	}
	return q
}
