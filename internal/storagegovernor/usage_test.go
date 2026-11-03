package storagegovernor_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/storagegovernor"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{now: time.Unix(1_700_000_000, 0)} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

// walkMeter counts usage walks and can hold one open to prove no lock is held across it.
type walkMeter struct {
	byPath  map[string]storagegovernor.Measurement
	managed int64
	walks   atomic.Int64
	started chan struct{}
	release chan struct{}
}

func (m *walkMeter) Measure(_ context.Context, path string) (storagegovernor.Measurement, error) {
	return m.byPath[path], nil
}

func (m *walkMeter) ManagedBytes(context.Context, string, storagegovernor.Domain) (int64, error) {
	m.walks.Add(1)
	if m.started != nil {
		m.started <- struct{}{}
		<-m.release
	}
	return m.managed, nil
}

func roomyMeasurement(id string) storagegovernor.Measurement {
	return storagegovernor.Measurement{ID: id, TotalBytes: 128 * storagegovernor.GiB, FreeBytes: 100 * storagegovernor.GiB}
}

func TestReservationsDoNotWalkTheManagedTree(t *testing.T) {
	t.Parallel()
	const root = "/filler"
	m := &walkMeter{byPath: map[string]storagegovernor.Measurement{root: roomyMeasurement("disk-a")}}
	governor := storagegovernor.New(m, fillerPolicy(10*storagegovernor.GiB))
	for range 5 {
		lease, decision := governor.Reserve(context.Background(), storagegovernor.Request{Path: root, EstimatedBytes: 1 << 20})
		if lease == nil || !decision.Allowed {
			t.Fatalf("reserve = %+v", decision)
		}
		if got := lease.Revalidate(context.Background(), 1); !got.Allowed {
			t.Fatalf("revalidate = %+v", got)
		}
		lease.Release()
		governor.Snapshot(context.Background(), root)
	}
	if walks := m.walks.Load(); walks != 1 {
		t.Fatalf("usage walks = %d, want exactly the one initial walk", walks)
	}
}

func TestNoLockIsHeldAcrossAUsageWalk(t *testing.T) {
	t.Parallel()
	const other, slow = "/other", "/slow"
	m := &walkMeter{byPath: map[string]storagegovernor.Measurement{other: roomyMeasurement("disk-b"), slow: roomyMeasurement("disk-a")}}
	governor := storagegovernor.New(m, fillerPolicy(10*storagegovernor.GiB))
	if lease, decision := governor.Reserve(context.Background(), storagegovernor.Request{Path: other, EstimatedBytes: 1}); lease == nil {
		t.Fatalf("warm-up reserve = %+v", decision)
	}
	m.started, m.release = make(chan struct{}), make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		governor.Snapshot(context.Background(), slow) // first sight of disk-a: walks while blocked
	}()
	<-m.started
	proceeded := make(chan struct{})
	go func() {
		defer close(proceeded)
		if lease, _ := governor.Reserve(context.Background(), storagegovernor.Request{Path: other, EstimatedBytes: 1}); lease != nil {
			lease.Release()
		}
	}()
	select {
	case <-proceeded:
	case <-time.After(5 * time.Second):
		t.Fatal("a reservation stalled behind another filesystem's usage walk")
	}
	close(m.release)
	<-done
}

func TestRunningTotalMatchesAFreshWalkAfterCommitAndRelease(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	roots := []storagegovernor.ManagedRoot{{Path: root, Domain: storagegovernor.DomainFiller}}
	governor := newFilesystemGovernor(t, roots, fillerPolicy(storagegovernor.GiB))
	fresh := func() int64 {
		d := newFilesystemGovernor(t, roots, fillerPolicy(storagegovernor.GiB)).Snapshot(context.Background(), root)
		if !d.Allowed {
			t.Fatalf("fresh snapshot = %+v", d)
		}
		return d.Snapshot.ManagedBytes
	}
	managed := func() int64 { return governor.Snapshot(context.Background(), root).Snapshot.ManagedBytes }
	if managed() != 0 {
		t.Fatalf("empty tree managed = %d", managed())
	}

	committed, decision := governor.Reserve(context.Background(), storagegovernor.Request{Path: root, EstimatedBytes: 1000})
	if committed == nil {
		t.Fatalf("reserve = %+v", decision)
	}
	if err := os.WriteFile(filepath.Join(root, "committed.part"), make([]byte, 600), 0o600); err != nil {
		t.Fatal(err)
	}
	if d := committed.Revalidate(context.Background(), 600); !d.Allowed {
		t.Fatalf("commit = %+v", d)
	}
	if got, want := managed(), fresh(); got != want || got != 600 {
		t.Fatalf("after reserve->commit managed = %d, fresh walk = %d, want 600", got, want)
	}

	released, decision := governor.Reserve(context.Background(), storagegovernor.Request{Path: root, EstimatedBytes: 1000})
	if released == nil {
		t.Fatalf("reserve = %+v", decision)
	}
	released.Release()
	if got, want := managed(), fresh(); got != want || got != 600 {
		t.Fatalf("after reserve->release managed = %d, fresh walk = %d, want 600", got, want)
	}

	committed.Release() // written bytes stay on disk as abandoned staging
	if got, want := managed(), fresh(); got != want {
		t.Fatalf("after commit->release managed = %d, fresh walk = %d", got, want)
	}
}

func TestBackgroundRewalkCorrectsOutOfBandChange(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	governor := newFilesystemGovernor(t, []storagegovernor.ManagedRoot{{Path: root, Domain: storagegovernor.DomainFiller}}, fillerPolicy(storagegovernor.GiB))
	clock := newFakeClock()
	storagegovernor.SetClock(governor, clock.Now)
	managed := func() int64 { return governor.Snapshot(context.Background(), root).Snapshot.ManagedBytes }
	if managed() != 0 {
		t.Fatal("empty tree should measure 0")
	}
	if err := os.WriteFile(filepath.Join(root, "outside.mp4"), make([]byte, 500), 0o600); err != nil {
		t.Fatal(err)
	}

	clock.Advance(storagegovernor.UsageRefreshInterval - time.Second)
	managed()
	storagegovernor.WaitForRefresh(governor)
	if got := managed(); got != 0 {
		t.Fatalf("managed = %d before the interval elapsed, want the running total (0)", got)
	}

	clock.Advance(2 * time.Second)
	managed() // triggers the background re-walk
	storagegovernor.WaitForRefresh(governor)
	if got := managed(); got != 500 {
		t.Fatalf("managed = %d after the re-walk, want 500", got)
	}
}
