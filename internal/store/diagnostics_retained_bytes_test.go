package store

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/diagnostics"
)

// #1398: DiagnosticRetainedBytes used to SUM(size_bytes) over a 2.7M-row table on every
// housekeeping run. It is now a maintained running total. These tests run on the real SQLite store
// and compare the total against a ground-truth SUM after every kind of write.

// truthRetainedBytes is the full-scan ground truth the running total must always equal.
func truthRetainedBytes(t *testing.T, st Store) int64 {
	t.Helper()
	var sum int64
	if err := PoolOf(st).QueryRowContext(context.Background(), `SELECT
		COALESCE((SELECT SUM(size_bytes) FROM diagnostic_events), 0) +
		COALESCE((SELECT SUM(size_bytes) FROM diagnostic_process_runs), 0)`).Scan(&sum); err != nil {
		t.Fatal(err)
	}
	return sum
}

func requireRetainedBytes(t *testing.T, st Store, step string) int64 {
	t.Helper()
	got, err := st.DiagnosticRetainedBytes(context.Background())
	if err != nil {
		t.Fatalf("%s: %v", step, err)
	}
	if want := truthRetainedBytes(t, st); got != want {
		t.Fatalf("%s: DiagnosticRetainedBytes = %d, ground-truth SUM = %d", step, got, want)
	}
	return got
}

func seedProcessRun(t *testing.T, st Store, id string, size int64, status diagnostics.ProcessStatus, endedAt int64) {
	t.Helper()
	err := st.UpsertDiagnosticProcessRun(context.Background(), diagnostics.ProcessRun{
		ID: id, Purpose: "test", StartedAt: 1000, EndedAt: endedAt, Status: status, UpdatedAt: 1000, SizeBytes: size,
	})
	if err != nil {
		t.Fatalf("upsert run %s: %v", id, err)
	}
}

func TestDiagnosticRetainedBytesIsNotAFullScan(t *testing.T) {
	st := openRetentionStore(t)
	rows, err := PoolOf(st).QueryContext(context.Background(), "EXPLAIN QUERY PLAN "+diagnosticRetainedBytesQuery)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var plan []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	joined := strings.Join(plan, "\n")
	for _, table := range []string{"diagnostic_events", "diagnostic_process_runs"} {
		if strings.Contains(joined, table) {
			t.Fatalf("retained-bytes query touches %s (full scan per housekeeping run):\n%s", table, joined)
		}
	}
}

func TestDiagnosticRetainedBytesTracksEveryWrite(t *testing.T) {
	st := openRetentionStore(t)
	ctx := context.Background()
	now := time.Now()
	requireRetainedBytes(t, st, "empty")

	seedDiagnosticEvents(t, st, "old", 3_000, now.Add(-72*time.Hour)) // 100 B each
	seedDiagnosticEvents(t, st, "new", 50, now)
	if got := requireRetainedBytes(t, st, "after append"); got != 3_050*100 {
		t.Fatalf("after append = %d, want %d", got, 3_050*100)
	}

	seedProcessRun(t, st, "run-a", 700, diagnostics.ProcessSucceeded, 2000)
	seedProcessRun(t, st, "run-b", 300, diagnostics.ProcessRunning, 0)
	requireRetainedBytes(t, st, "after run insert")
	seedProcessRun(t, st, "run-a", 900, diagnostics.ProcessSucceeded, 2000) // upsert GROWS the row
	if got := requireRetainedBytes(t, st, "after run upsert"); got != 3_050*100+900+300 {
		t.Fatalf("after run upsert = %d", got)
	}

	// A batch that fails midway rolls back and must not leak bytes into the total.
	bad := []diagnostics.Record{
		{ID: "dup-1", OccurredAt: 1, ReceivedAt: 1, Level: diagnostics.LevelInfo, Source: diagnostics.SourceServer, Event: "e", SizeBytes: 5000},
		{ID: "dup-1", OccurredAt: 1, ReceivedAt: 1, Level: diagnostics.LevelInfo, Source: diagnostics.SourceServer, Event: "e", SizeBytes: 5000},
	}
	if err := st.AppendDiagnosticEvents(ctx, bad); err == nil {
		t.Fatal("duplicate id in one batch must fail")
	}
	requireRetainedBytes(t, st, "after failed batch")

	if n, err := st.DeleteDiagnosticEvents(ctx, []string{"old-00000000", "old-00000001"}); err != nil || n != 2 {
		t.Fatalf("delete events n=%d err=%v", n, err)
	}
	requireRetainedBytes(t, st, "after event delete")
	if ok, err := st.DeleteDiagnosticProcessRun(ctx, "run-a"); err != nil || !ok {
		t.Fatalf("delete run ok=%v err=%v", ok, err)
	}
	if ok, err := st.DeleteDiagnosticProcessRun(ctx, "run-b"); err != nil || ok {
		t.Fatalf("a running run must not be deletable (ok=%v err=%v)", ok, err)
	}
	requireRetainedBytes(t, st, "after run delete")

	res, err := st.PurgeDiagnostics(ctx, now.Add(-24*time.Hour), 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Events != 2_998 {
		t.Fatalf("purged events = %d, want 2998", res.Events)
	}
	if got := requireRetainedBytes(t, st, "after age purge"); res.RetainedBytes != got {
		t.Fatalf("PurgeResult.RetainedBytes = %d, table says %d", res.RetainedBytes, got)
	}
}

func TestDiagnosticRetainedBytesSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "loomarr.db")
	open := func() Store {
		st, err := Open(context.Background(), "sqlite://"+path, true)
		if err != nil {
			t.Fatal(err)
		}
		return st
	}
	st := open()
	seedDiagnosticEvents(t, st, "e", 1_500, time.Now())
	seedProcessRun(t, st, "run", 250, diagnostics.ProcessFailed, 5)
	before := requireRetainedBytes(t, st, "before restart")
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st = open()
	defer func() { _ = st.Close() }()
	if after := requireRetainedBytes(t, st, "after restart"); after != before || before != 1_500*100+250 {
		t.Fatalf("restart: before=%d after=%d", before, after)
	}
}

func TestPurgeDiagnosticsConvergesUnderTheByteBudget(t *testing.T) {
	st := openRetentionStore(t)
	now := time.Now()
	seedDiagnosticEvents(t, st, "e", 20_000, now) // 2,000,000 B, all inside the age window
	seedProcessRun(t, st, "done", 40_000, diagnostics.ProcessSucceeded, 10)
	const budget = 500_000
	res, err := st.PurgeDiagnostics(context.Background(), now.Add(-24*time.Hour), budget)
	if err != nil {
		t.Fatal(err)
	}
	got := requireRetainedBytes(t, st, "after budget purge")
	if got > budget || got < budget-1_000 {
		t.Fatalf("retained = %d, want just under the %d budget", got, budget)
	}
	if res.RetainedBytes != got {
		t.Fatalf("PurgeResult.RetainedBytes = %d, table says %d", res.RetainedBytes, got)
	}
	// The newest evidence survives; the budget deletes oldest first.
	newest, err := st.ListDiagnosticEvents(context.Background(), 1)
	if err != nil || len(newest) != 1 || newest[0].ID != "e-00000000" {
		t.Fatalf("newest event was deleted: %v %v", newest, err)
	}
}

func explainPlan(t *testing.T, st Store, query string, args ...any) string {
	t.Helper()
	rows, err := PoolOf(st).QueryContext(context.Background(), "EXPLAIN QUERY PLAN "+query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var plan []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	return strings.Join(plan, "\n")
}

func sqliteIndexes(t *testing.T, st Store) map[string]string {
	t.Helper()
	rows, err := PoolOf(st).QueryContext(context.Background(),
		`SELECT name, COALESCE(sql, '') FROM sqlite_master WHERE type = 'index' AND tbl_name = 'diagnostic_events'`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var name, ddl string
		if err := rows.Scan(&name, &ddl); err != nil {
			t.Fatal(err)
		}
		out[name] = ddl
	}
	return out
}

// Migration 00121 seeds the running total from the rows that already exist (the household table)
// and drops the indexes no query needs.
func TestDiagnosticRetainedBytesMigrationSeedsTheTotalAndTrimsIndexes(t *testing.T) {
	ctx := context.Background()
	s, err := openSQLite(ctx, filepath.Join(t.TempDir(), "diag.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	provider, err := newMigrationProvider(s.db, s.dialect, "migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 120); err != nil {
		t.Fatalf("migrate through 120: %v", err)
	}
	for i, size := range []int{120, 130, 250} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO diagnostic_events (id, occurred_at, received_at, level, source, event, size_bytes)
			VALUES (?, ?, ?, 'info', 'server', 'e', ?)`, fmt.Sprintf("e%d", i), i+1, i+1, size); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO diagnostic_process_runs (id, purpose, started_at, status, updated_at, size_bytes)
		VALUES ('r1', 'p', 1, 'succeeded', 1, 77)`); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 121); err != nil {
		t.Fatalf("apply 00121: %v", err)
	}
	if got, err := s.DiagnosticRetainedBytes(ctx); err != nil || got != 120+130+250+77 {
		t.Fatalf("seeded total = %d, %v; want %d", got, err, 120+130+250+77)
	}
	indexes := sqliteIndexes(t, s)
	for _, gone := range []string{"idx_diagnostic_events_source_time", "idx_diagnostic_events_instance"} {
		if _, ok := indexes[gone]; ok {
			t.Errorf("%s should have been dropped", gone)
		}
	}
	for _, partial := range []string{"request", "playback", "channel", "schedule_block", "job", "process"} {
		if ddl := indexes["idx_diagnostic_events_"+partial]; !strings.Contains(ddl, "<> ''") {
			t.Errorf("idx_diagnostic_events_%s should be partial, got %q", partial, ddl)
		}
	}
	for _, kept := range []string{"idx_diagnostic_events_time", "idx_diagnostic_events_level_time", "idx_diagnostic_events_subsystem_time"} {
		if _, ok := indexes[kept]; !ok {
			t.Errorf("%s must survive", kept)
		}
	}
}

// Every filter the API exposes must still walk an index (not scan the retained window)
// where the value is selective; the partial correlation indexes must actually be chosen.
func TestQueryDiagnosticEventsCorrelationFiltersUseTheirIndexes(t *testing.T) {
	st := openRetentionStore(t)
	seedDiagnosticEvents(t, st, "e", 2_000, time.Now())
	for name, filter := range map[string]diagnostics.EventStoreQuery{
		"request_id":          {RequestID: "req-1"},
		"playback_session_id": {PlaybackSessionID: "ps-1"},
		"channel_id":          {ChannelID: "ch-1"},
		"schedule_block_id":   {ScheduleBlockID: "sb-1"},
		"job_id":              {JobID: "job-1"},
		"process_run_id":      {ProcessRunID: "pr-1"},
	} {
		filter.From, filter.To, filter.Limit = 0, time.Now().Add(time.Hour).UnixMilli(), 50
		sqlText, args := diagnosticEventQuery(filter)
		plan := explainPlan(t, st, sqlText, args...)
		if !strings.Contains(plan, "USING INDEX idx_diagnostic_events_") || strings.Contains(plan, "idx_diagnostic_events_time") {
			t.Errorf("%s filter does not use its correlation index:\n%s", name, plan)
		}
	}
}
