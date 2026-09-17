package storagegovernor_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/loomarr/loomarr/internal/storagegovernor"
)

type meter struct {
	mu         sync.Mutex
	byPath     map[string]storagegovernor.Measurement
	managed    map[string]map[storagegovernor.Domain]int64
	measureErr error
	managedErr error
}

func (m *meter) Measure(_ context.Context, path string) (storagegovernor.Measurement, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.measureErr != nil {
		return storagegovernor.Measurement{}, m.measureErr
	}
	return m.byPath[path], nil
}

func (m *meter) ManagedBytes(_ context.Context, filesystemID string, domain storagegovernor.Domain) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.managedErr != nil {
		return 0, m.managedErr
	}
	return m.managed[filesystemID][domain], nil
}

func (m *meter) update(path string, measurement storagegovernor.Measurement, managed int64) {
	m.mu.Lock()
	m.byPath[path] = measurement
	if m.managed[measurement.ID] == nil {
		m.managed[measurement.ID] = map[storagegovernor.Domain]int64{}
	}
	m.managed[measurement.ID][storagegovernor.DomainFiller] = managed
	m.mu.Unlock()
}

func fillerPolicy(bytes int64) func(storagegovernor.Domain) storagegovernor.Policy {
	return func(domain storagegovernor.Domain) storagegovernor.Policy {
		if domain == storagegovernor.DomainFiller {
			return storagegovernor.Policy{SoftBudgetBytes: bytes, AutomaticBudget: bytes <= 0}
		}
		return storagegovernor.Policy{}
	}
}

func TestRepresentativeVolumePolicy(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		total       int64
		wantBudget  int64
		wantReserve int64
	}{
		{name: "16 GiB appliance", total: 16 * storagegovernor.GiB, wantBudget: 16 * storagegovernor.GiB / 10, wantReserve: 2 * storagegovernor.GiB},
		{name: "32 GiB appliance", total: 32 * storagegovernor.GiB, wantBudget: 32 * storagegovernor.GiB / 10, wantReserve: 32 * storagegovernor.GiB / 10},
		{name: "64 GiB appliance", total: 64 * storagegovernor.GiB, wantBudget: 64 * storagegovernor.GiB / 10, wantReserve: 64 * storagegovernor.GiB / 10},
		{name: "128 GiB appliance", total: 128 * storagegovernor.GiB, wantBudget: 128 * storagegovernor.GiB / 10, wantReserve: 128 * storagegovernor.GiB / 10},
		{name: "4 TiB NAS", total: 4 * 1024 * storagegovernor.GiB, wantBudget: 20 * storagegovernor.GiB, wantReserve: 20 * storagegovernor.GiB},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := storagegovernor.AutomaticBudgetBytes(tc.total); got != tc.wantBudget {
				t.Fatalf("automatic budget = %d, want %d", got, tc.wantBudget)
			}
			if got := storagegovernor.HardReserveBytes(tc.total); got != tc.wantReserve {
				t.Fatalf("hard reserve = %d, want %d", got, tc.wantReserve)
			}
		})
	}
}

func TestCapacityStateOwnsTheApproachingThreshold(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		snapshot storagegovernor.Snapshot
		want     storagegovernor.CapacityState
	}{
		{name: "healthy", snapshot: storagegovernor.Snapshot{
			SoftLimitEnabled: true, SoftBudgetBytes: 10 * storagegovernor.GiB, AvailableBytes: 2 * storagegovernor.GiB,
		}, want: storagegovernor.CapacityHealthy},
		{name: "one GiB warning cap", snapshot: storagegovernor.Snapshot{
			SoftLimitEnabled: true, SoftBudgetBytes: 20 * storagegovernor.GiB, AvailableBytes: storagegovernor.GiB,
		}, want: storagegovernor.CapacityApproaching},
		{name: "small appliance percentage", snapshot: storagegovernor.Snapshot{
			SoftLimitEnabled: true, SoftBudgetBytes: 2 * storagegovernor.GiB, AvailableBytes: 400 << 20,
		}, want: storagegovernor.CapacityApproaching},
		{name: "paused", snapshot: storagegovernor.Snapshot{Reason: storagegovernor.ReasonHostReserve}, want: storagegovernor.CapacityPaused},
		{name: "unknown", snapshot: storagegovernor.Snapshot{Reason: storagegovernor.ReasonCapacityUnavailable}, want: storagegovernor.CapacityUnknown},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := storagegovernor.State(tc.snapshot); got != tc.want {
				t.Fatalf("State() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestReserveUsesTheSmallerHostAndLibraryAllowance(t *testing.T) {
	t.Parallel()
	const root = "/filler"
	m := &meter{
		byPath:  map[string]storagegovernor.Measurement{root: {ID: "disk-a", TotalBytes: 64 * storagegovernor.GiB, FreeBytes: 40 * storagegovernor.GiB}},
		managed: map[string]map[storagegovernor.Domain]int64{"disk-a": {storagegovernor.DomainFiller: 6 * storagegovernor.GiB}},
	}
	governor := storagegovernor.New(m, nil)
	if lease, decision := governor.Reserve(context.Background(), storagegovernor.Request{Path: root, EstimatedBytes: storagegovernor.GiB}); lease != nil || decision.Snapshot.Reason != storagegovernor.ReasonLibraryLimit {
		t.Fatalf("reserve = lease %v decision %+v, want library limit", lease, decision)
	}

	m.update(root, storagegovernor.Measurement{ID: "disk-a", TotalBytes: 64 * storagegovernor.GiB, FreeBytes: 6 * storagegovernor.GiB}, 0)
	if lease, decision := governor.Reserve(context.Background(), storagegovernor.Request{Path: root, EstimatedBytes: storagegovernor.GiB}); lease != nil || decision.Snapshot.Reason != storagegovernor.ReasonHostReserve {
		t.Fatalf("reserve = lease %v decision %+v, want host reserve", lease, decision)
	}
}

func TestConfirmedManualMayCrossSoftLimitButNotHostReserve(t *testing.T) {
	t.Parallel()
	const root = "/filler"
	m := &meter{
		byPath:  map[string]storagegovernor.Measurement{root: {ID: "disk-a", TotalBytes: 32 * storagegovernor.GiB, FreeBytes: 10 * storagegovernor.GiB}},
		managed: map[string]map[storagegovernor.Domain]int64{"disk-a": {storagegovernor.DomainFiller: 4 * storagegovernor.GiB}},
	}
	governor := storagegovernor.New(m, nil)
	lease, decision := governor.Reserve(context.Background(), storagegovernor.Request{
		Path: root, EstimatedBytes: storagegovernor.GiB, Mode: storagegovernor.ConfirmedManual,
	})
	if lease == nil || !decision.Allowed {
		t.Fatalf("confirmed manual reserve = %+v, want allowed", decision)
	}
	lease.Release()

	m.update(root, storagegovernor.Measurement{ID: "disk-a", TotalBytes: 32 * storagegovernor.GiB, FreeBytes: 4 * storagegovernor.GiB}, 4*storagegovernor.GiB)
	if lease, decision = governor.Reserve(context.Background(), storagegovernor.Request{
		Path: root, EstimatedBytes: storagegovernor.GiB, Mode: storagegovernor.ConfirmedManual,
	}); lease != nil || decision.Snapshot.Reason != storagegovernor.ReasonHostReserve {
		t.Fatalf("confirmed manual host reserve = lease %v decision %+v", lease, decision)
	}
}

func TestConcurrentPathsOnOneFilesystemCannotDoubleSpend(t *testing.T) {
	t.Parallel()
	m := &meter{
		byPath: map[string]storagegovernor.Measurement{
			"/filler-a": {ID: "disk-a", TotalBytes: 128 * storagegovernor.GiB, FreeBytes: 80 * storagegovernor.GiB},
			"/filler-b": {ID: "disk-a", TotalBytes: 128 * storagegovernor.GiB, FreeBytes: 80 * storagegovernor.GiB},
		},
		managed: map[string]map[storagegovernor.Domain]int64{"disk-a": {}},
	}
	governor := storagegovernor.New(m, fillerPolicy(10*storagegovernor.GiB))
	start := make(chan struct{})
	var granted atomic.Int32
	var leasesMu sync.Mutex
	var leases []*storagegovernor.Lease
	var wg sync.WaitGroup
	for _, path := range []string{"/filler-a", "/filler-b"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			lease, _ := governor.Reserve(context.Background(), storagegovernor.Request{Path: path, EstimatedBytes: 6 * storagegovernor.GiB})
			if lease != nil {
				granted.Add(1)
				leasesMu.Lock()
				leases = append(leases, lease)
				leasesMu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()
	if got := granted.Load(); got != 1 {
		t.Fatalf("granted reservations = %d, want exactly one", got)
	}
	for _, lease := range leases {
		lease.Release()
	}
}

func TestDomainsShareHostReservationsWithoutSharingSoftUsage(t *testing.T) {
	t.Parallel()
	m := &meter{
		byPath: map[string]storagegovernor.Measurement{
			"/filler":   {ID: "disk-a", TotalBytes: 128 * storagegovernor.GiB, FreeBytes: 30 * storagegovernor.GiB},
			"/prepared": {ID: "disk-a", TotalBytes: 128 * storagegovernor.GiB, FreeBytes: 30 * storagegovernor.GiB},
		},
		managed: map[string]map[storagegovernor.Domain]int64{
			"disk-a": {storagegovernor.DomainFiller: storagegovernor.GiB, storagegovernor.DomainPrepared: 9 * storagegovernor.GiB},
		},
	}
	governor := storagegovernor.New(m, fillerPolicy(10*storagegovernor.GiB))
	prepared, decision := governor.Reserve(context.Background(), storagegovernor.Request{
		Path: "/prepared", Domain: storagegovernor.DomainPrepared, EstimatedBytes: 10 * storagegovernor.GiB,
	})
	if prepared == nil || !decision.Allowed {
		t.Fatalf("prepared reserve = %+v", decision)
	}
	defer prepared.Release()

	if decision = governor.DomainSnapshot(context.Background(), "/filler", storagegovernor.DomainFiller); decision.Snapshot.ManagedBytes != storagegovernor.GiB {
		t.Fatalf("filler managed bytes = %d, want prepared usage excluded", decision.Snapshot.ManagedBytes)
	}
	if filler, blocked := governor.Reserve(context.Background(), storagegovernor.Request{
		Path: "/filler", Domain: storagegovernor.DomainFiller, EstimatedBytes: 8 * storagegovernor.GiB,
	}); filler != nil || blocked.Snapshot.Reason != storagegovernor.ReasonHostReserve {
		t.Fatalf("filler reserve = lease %v decision %+v, want shared host reserve", filler, blocked)
	}
}

func TestDifferentFilesystemsHaveIndependentReservations(t *testing.T) {
	t.Parallel()
	m := &meter{
		byPath: map[string]storagegovernor.Measurement{
			"/one": {ID: "disk-a", TotalBytes: 64 * storagegovernor.GiB, FreeBytes: 40 * storagegovernor.GiB},
			"/two": {ID: "disk-b", TotalBytes: 64 * storagegovernor.GiB, FreeBytes: 40 * storagegovernor.GiB},
		},
		managed: map[string]map[storagegovernor.Domain]int64{"disk-a": {}, "disk-b": {}},
	}
	governor := storagegovernor.New(m, fillerPolicy(8*storagegovernor.GiB))
	for _, path := range []string{"/one", "/two"} {
		lease, decision := governor.Reserve(context.Background(), storagegovernor.Request{Path: path, EstimatedBytes: 6 * storagegovernor.GiB})
		if lease == nil || !decision.Allowed {
			t.Fatalf("reserve %s = %+v, want allowed", path, decision)
		}
		defer lease.Release()
	}
}

func TestUnknownEstimateAndCapacityFailClosed(t *testing.T) {
	t.Parallel()
	m := &meter{
		byPath:  map[string]storagegovernor.Measurement{"/filler": {ID: "disk-a", TotalBytes: 64 * storagegovernor.GiB, FreeBytes: 40 * storagegovernor.GiB}},
		managed: map[string]map[storagegovernor.Domain]int64{"disk-a": {}},
	}
	governor := storagegovernor.New(m, nil)
	if lease, decision := governor.Reserve(context.Background(), storagegovernor.Request{Path: "/filler"}); lease != nil || decision.Snapshot.Reason != storagegovernor.ReasonEstimateUnknown {
		t.Fatalf("unknown estimate = lease %v decision %+v", lease, decision)
	}
	m.measureErr = errors.New("stat failed")
	if lease, decision := governor.Reserve(context.Background(), storagegovernor.Request{Path: "/filler", EstimatedBytes: 1}); lease != nil || decision.Snapshot.Reason != storagegovernor.ReasonCapacityUnavailable || decision.Err == nil {
		t.Fatalf("stat failure = lease %v decision %+v", lease, decision)
	}
}

func TestUnknownDomainFailsClosed(t *testing.T) {
	t.Parallel()
	m := &meter{
		byPath:  map[string]storagegovernor.Measurement{"/filler": {ID: "disk-a", TotalBytes: 64 * storagegovernor.GiB, FreeBytes: 40 * storagegovernor.GiB}},
		managed: map[string]map[storagegovernor.Domain]int64{"disk-a": {}},
	}
	governor := storagegovernor.New(m, nil)
	lease, decision := governor.Reserve(context.Background(), storagegovernor.Request{
		Path: "/filler", Domain: "typo", EstimatedBytes: 1,
	})
	if lease != nil || decision.Snapshot.Reason != storagegovernor.ReasonCapacityUnavailable || decision.Err == nil {
		t.Fatalf("unknown domain = lease %v decision %+v", lease, decision)
	}
}

func TestRevalidateTracksOutstandingBytesAndReleaseIsIdempotent(t *testing.T) {
	t.Parallel()
	const root = "/filler"
	m := &meter{
		byPath:  map[string]storagegovernor.Measurement{root: {ID: "disk-a", TotalBytes: 128 * storagegovernor.GiB, FreeBytes: 80 * storagegovernor.GiB}},
		managed: map[string]map[storagegovernor.Domain]int64{"disk-a": {}},
	}
	governor := storagegovernor.New(m, fillerPolicy(10*storagegovernor.GiB))
	lease, decision := governor.Reserve(context.Background(), storagegovernor.Request{Path: root, EstimatedBytes: 4 * storagegovernor.GiB})
	if lease == nil || !decision.Allowed {
		t.Fatalf("reserve = %+v", decision)
	}
	m.update(root, storagegovernor.Measurement{ID: "disk-a", TotalBytes: 128 * storagegovernor.GiB, FreeBytes: 78 * storagegovernor.GiB}, 2*storagegovernor.GiB)
	decision = lease.Revalidate(context.Background(), 2*storagegovernor.GiB)
	if !decision.Allowed || decision.Snapshot.ReservedBytes != 2*storagegovernor.GiB || decision.Snapshot.FilesystemReservedBytes != 2*storagegovernor.GiB {
		t.Fatalf("progress decision = %+v, want 2 GiB outstanding", decision)
	}
	if decision = lease.Revalidate(context.Background(), 5*storagegovernor.GiB); decision.Snapshot.Reason != storagegovernor.ReasonEstimateUnknown {
		t.Fatalf("estimate overrun = %+v, want estimate_unknown", decision)
	}
	lease.Release()
	lease.Release()
	if snapshot := governor.Snapshot(context.Background(), root); snapshot.Snapshot.ReservedBytes != 0 {
		t.Fatalf("reserved after release = %d", snapshot.Snapshot.ReservedBytes)
	}
}

func TestLoweredSoftBudgetPausesNewWorkWithoutRevokingAReservation(t *testing.T) {
	t.Parallel()
	const root = "/filler"
	m := &meter{
		byPath:  map[string]storagegovernor.Measurement{root: {ID: "disk-a", TotalBytes: 128 * storagegovernor.GiB, FreeBytes: 80 * storagegovernor.GiB}},
		managed: map[string]map[storagegovernor.Domain]int64{"disk-a": {}},
	}
	budget := int64(10 * storagegovernor.GiB)
	governor := storagegovernor.New(m, func(domain storagegovernor.Domain) storagegovernor.Policy {
		if domain == storagegovernor.DomainFiller {
			return storagegovernor.Policy{SoftBudgetBytes: budget}
		}
		return storagegovernor.Policy{}
	})
	lease, decision := governor.Reserve(context.Background(), storagegovernor.Request{Path: root, EstimatedBytes: 4 * storagegovernor.GiB})
	if lease == nil || !decision.Allowed {
		t.Fatalf("reserve = %+v", decision)
	}
	budget = storagegovernor.GiB
	if decision = lease.Revalidate(context.Background(), 0); !decision.Allowed {
		t.Fatalf("existing reservation was revoked by a soft limit: %+v", decision)
	}
	if second, next := governor.Reserve(context.Background(), storagegovernor.Request{Path: root, EstimatedBytes: 1}); second != nil || next.Snapshot.Reason != storagegovernor.ReasonLibraryLimit {
		t.Fatalf("new reservation = lease %v decision %+v, want library limit", second, next)
	}
	lease.Release()
}
