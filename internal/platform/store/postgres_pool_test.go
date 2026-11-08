//go:build integration

package store

import (
	"context"
	"sync"
	"testing"
)

// A Postgres store has a bounded pool and keeps its connections between bursts, rather than
// database/sql's default of no ceiling and two idle connections (#2446).
func TestPostgresPool_IsBoundedAndKeepsItsConnectionsWarm(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, startPostgres(t), true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	db := PoolOf(st)
	if got := db.Stats().MaxOpenConnections; got != postgresMaxOpenConns {
		t.Fatalf("MaxOpenConnections = %d, want %d", got, postgresMaxOpenConns)
	}

	// A burst of concurrent reads, each holding its connection until all have one.
	const burst = 8
	var ready, release sync.WaitGroup
	ready.Add(burst)
	release.Add(1)
	errs := make(chan error, burst)
	for range burst {
		go func() {
			conn, err := db.Conn(ctx)
			ready.Done()
			if err != nil {
				errs <- err
				return
			}
			release.Wait()
			errs <- conn.Close()
		}()
	}
	ready.Wait()
	release.Done()
	for range burst {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if s := db.Stats(); s.Idle < burst || s.MaxIdleClosed != 0 {
		t.Fatalf("after a burst of %d: idle %d, closed for idle limit %d; want all kept", burst, s.Idle, s.MaxIdleClosed)
	}
}
