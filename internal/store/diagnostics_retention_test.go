package store

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/diagnostics"
)

// These tests run against the REAL SQLite store with its production MaxOpenConns(1). #1411 was a
// contention bug: one connection, held for the length of a long statement, starved everything
// else (River's leader election, the health probe). A fake that never refuses would pass with
// or without the fix (§ fakes-that-never-refuse), so nothing here is faked.

func openRetentionStore(t *testing.T) Store {
	t.Helper()
	st, err := Open(context.Background(), "sqlite://"+filepath.Join(t.TempDir(), "loomarr.db"), true)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// seedDiagnosticEvents appends n events, one per second ending at `newest`, in writer-sized
// batches (each its own transaction, as the recorder does).
func seedDiagnosticEvents(t *testing.T, st Store, prefix string, n int, newest time.Time) {
	t.Helper()
	const batch = 1000
	for start := 0; start < n; start += batch {
		records := make([]diagnostics.Record, 0, batch)
		for i := start; i < min(start+batch, n); i++ {
			at := newest.Add(-time.Duration(i) * time.Second)
			records = append(records, diagnostics.Record{
				ID: fmt.Sprintf("%s-%08d", prefix, i), OccurredAt: at.UnixMilli(), ReceivedAt: at.UnixMilli(),
				Level: diagnostics.LevelInfo, Source: diagnostics.SourceServer, Subsystem: "seed",
				Event: "seed.event", AttributesJSON: `{}`, SizeBytes: 100,
			})
		}
		if err := st.AppendDiagnosticEvents(context.Background(), records); err != nil {
			t.Fatalf("seed events: %v", err)
		}
	}
}

// probeConnection stands in for the elector/health probe: a trivial query on the shared pool
// with the short budget River and the health check give themselves. It records how many probes
// completed while `work` ran and how many blew their budget.
func probeConnection(t *testing.T, st Store, budget time.Duration, work func()) (completed, timedOut int64) {
	t.Helper()
	pool := PoolOf(st)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			ctx, cancel := context.WithTimeout(context.Background(), budget)
			var one int
			if err := pool.QueryRowContext(ctx, `SELECT 1`).Scan(&one); err != nil {
				atomic.AddInt64(&timedOut, 1)
			} else {
				atomic.AddInt64(&completed, 1)
			}
			cancel()
			time.Sleep(time.Millisecond)
		}
	}()
	work()
	close(stop)
	wg.Wait()
	return atomic.LoadInt64(&completed), atomic.LoadInt64(&timedOut)
}

// The fallback purge used to delete every expired row in ONE transaction, holding the only
// connection for the whole delete. On the household install (2.7M rows, 11 indexes) that is
// minutes. It must now yield the connection between bounded batches.
func TestPurgeDiagnosticsYieldsTheConnectionBetweenBatches(t *testing.T) {
	st := openRetentionStore(t)
	now := time.Now()
	seedDiagnosticEvents(t, st, "old", 20_000, now.Add(-72*time.Hour))
	seedDiagnosticEvents(t, st, "new", 10, now) // survivors

	var result diagnostics.PurgeResult
	completed, timedOut := probeConnection(t, st, 2*time.Second, func() {
		var err error
		result, err = st.PurgeDiagnostics(context.Background(), now.Add(-24*time.Hour), 0)
		if err != nil {
			t.Errorf("purge: %v", err)
		}
	})
	if result.Events != 20_000 {
		t.Fatalf("purged %d events, want 20000", result.Events)
	}
	if timedOut != 0 {
		t.Fatalf("%d probes starved behind the purge", timedOut)
	}
	// One giant transaction lets the probe through zero times until it commits.
	if completed < 3 {
		t.Fatalf("only %d probes got the connection during the purge; it held it in one transaction", completed)
	}
	left, err := st.ListDiagnosticEvents(context.Background(), 100)
	if err != nil || len(left) != 10 {
		t.Fatalf("survivors = %d, err %v; want 10", len(left), err)
	}
}

// Deleting by candidate page is what the production ProcessManager does. Its page query must be
// index-driven: a UNION ALL subquery ordered outside forced a scan+sort of the whole events table
// for every 256-row page, which is how one nightly pass outlived its deadline.
func TestRetentionCandidatePageIsIndexDriven(t *testing.T) {
	st := openRetentionStore(t)
	seedDiagnosticEvents(t, st, "old", 2_000, time.Now().Add(-72*time.Hour))
	s := st.(*sqlStore)

	query, args := diagnosticRetentionCandidatesQuery(time.Now().Add(-24*time.Hour).UnixMilli(), 256)
	rows, err := s.db.QueryContext(context.Background(), `EXPLAIN QUERY PLAN `+s.ph(query), args...)
	if err != nil {
		t.Fatalf("explain: %v", err)
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
	if !strings.Contains(joined, "idx_diagnostic_events_time") {
		t.Fatalf("events branch does not use the time index:\n%s", joined)
	}
	if strings.Contains(joined, "SCAN diagnostic_events\n") || strings.HasSuffix(joined, "SCAN diagnostic_events") {
		t.Fatalf("events branch scans the whole table:\n%s", joined)
	}
}

// End to end through the production coordinator: the purge completes over a large table, in
// bounded batches, while a short-budget probe keeps getting the connection.
func TestProcessManagerPurgeOverLargeTableDoesNotStarveTheConnection(t *testing.T) {
	st := openRetentionStore(t)
	now := time.Now()
	seedDiagnosticEvents(t, st, "old", 20_000, now.Add(-72*time.Hour))
	seedDiagnosticEvents(t, st, "new", 10, now)

	manager := diagnostics.NewProcessManager(st, nil, diagnostics.ProcessOptions{})
	var result diagnostics.PurgeResult
	completed, timedOut := probeConnection(t, st, 2*time.Second, func() {
		var err error
		result, err = manager.Purge(context.Background(), now.Add(-24*time.Hour), 0)
		if err != nil {
			t.Errorf("purge: %v", err)
		}
	})
	if result.Events != 20_000 {
		t.Fatalf("purged %d events, want 20000", result.Events)
	}
	if timedOut != 0 || completed < 3 {
		t.Fatalf("probe completed=%d timedOut=%d; the purge monopolised the connection", completed, timedOut)
	}
}

// A backup must not sit on the app's single connection for the length of a 1.8 GB VACUUM INTO:
// the health probe and River queue behind it. Occupy that connection and the backup must still
// complete, which is only possible if it reads on a handle of its own.
func TestWriteBackupDoesNotNeedTheSharedConnection(t *testing.T) {
	st := openRetentionStore(t)
	seedDiagnosticEvents(t, st, "bk", 100, time.Now())

	held, err := PoolOf(st).Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	backup, err := BackupWriter(st).WriteBackup(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("backup queued behind the shared connection: %v", err)
	}
	if backup.Bytes == 0 {
		t.Fatal("empty backup")
	}
}
