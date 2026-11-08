package storagegovernor

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Reason is the server-owned explanation for refusing managed work.
type Reason string

const (
	ReasonLibraryLimit        Reason = "library_limit"
	ReasonHostReserve         Reason = "host_reserve"
	ReasonEstimateUnknown     Reason = "estimate_unknown"
	ReasonCapacityUnavailable Reason = "capacity_unavailable"
)

// Mode distinguishes unattended work from a manual import whose size and
// consequence the operator explicitly confirmed.
type Mode uint8

const (
	Automatic Mode = iota
	ConfirmedManual
)

// Measurement is one live filesystem observation. ID must be stable for paths
// on the same real filesystem/device; path spelling is not an identity.
type Measurement struct {
	ID         string
	TotalBytes int64
	FreeBytes  int64
}

// Meter is the governor's internal filesystem seam. Production measures the
// real host; tests provide deterministic adapters. ManagedBytes must include
// abandoned staging because restart never recreates in-memory reservations.
type Meter interface {
	Measure(ctx context.Context, path string) (Measurement, error)
	ManagedBytes(ctx context.Context, filesystemID string, domain Domain) (int64, error)
}

// Request describes one managed write before queue or worker mutation.
type Request struct {
	Path           string
	Domain         Domain
	EstimatedBytes int64
	Mode           Mode
}

// Snapshot is the complete server-owned capacity projection for one domain on
// one real filesystem. ReservedBytes is domain-local; FilesystemReservedBytes
// includes every domain competing for the hard host reserve.
type Snapshot struct {
	Domain                  Domain
	FilesystemID            string
	TotalBytes              int64
	FreeBytes               int64
	ManagedBytes            int64
	ReservedBytes           int64
	FilesystemReservedBytes int64
	SoftBudgetBytes         int64
	SoftLimitEnabled        bool
	HardReserveBytes        int64
	AvailableBytes          int64
	Reason                  Reason
}

// Decision reports whether work may proceed and carries the same snapshot the
// UI/readiness projections consume. Err is diagnostic context and is never the
// public reason vocabulary.
type Decision struct {
	Allowed  bool
	Snapshot Snapshot
	Err      error
}

// CapacityState is the server-facing presentation state for one snapshot. It deliberately lives
// beside the capacity arithmetic so clients never invent their own warning threshold.
type CapacityState string

const (
	CapacityHealthy     CapacityState = "healthy"
	CapacityApproaching CapacityState = "approaching"
	CapacityPaused      CapacityState = "paused"
	CapacityUnknown     CapacityState = "unknown"
)

// State classifies a snapshot without hiding its exact typed pause reason. Approaching means less
// than 1 GiB is left, or less than five percent of a configured soft budget, whichever is larger
// (#2737): a big allowance warns before its last gigabyte, a small one at its last gigabyte.
func State(snapshot Snapshot) CapacityState {
	switch snapshot.Reason {
	case ReasonCapacityUnavailable, ReasonEstimateUnknown:
		return CapacityUnknown
	case ReasonHostReserve, ReasonLibraryLimit:
		return CapacityPaused
	case "":
		threshold := GiB
		if snapshot.SoftLimitEnabled && snapshot.SoftBudgetBytes > 0 {
			threshold = max(threshold, snapshot.SoftBudgetBytes/20)
		}
		if threshold > 0 && snapshot.AvailableBytes > 0 && snapshot.AvailableBytes <= threshold {
			return CapacityApproaching
		}
		return CapacityHealthy
	default:
		return CapacityUnknown
	}
}

// Governor atomically accounts for every in-process reservation sharing a real
// filesystem. Its mutex covers the cheap capacity measurement, the in-memory
// managed-bytes totals and the reservations, so two callers cannot both observe
// the same unspent bytes and reserve them. It is never held across a walk of the
// managed tree: each total is walked once on first use, adjusted as leases report
// written bytes, and re-walked in the background every usageRefreshInterval.
type Governor struct {
	meter  Meter
	policy func(Domain) Policy

	mu           sync.Mutex
	nextLeaseID  uint64
	reservations map[uint64]reservation
	usage        map[usageKey]*usage
	now          func() time.Time
	refreshes    sync.WaitGroup
}

var errManagedInvalid = errors.New("managed storage measurement is invalid")

type reservation struct {
	filesystemID string
	path         string
	domain       Domain
	estimated    int64
	remaining    int64
	written      int64
}

// New constructs a governor. Policy is read on every operation so a saved
// allowance hot-applies to new work without invalidating existing leases.
func New(meter Meter, policy func(Domain) Policy) *Governor {
	if policy == nil {
		policy = func(domain Domain) Policy {
			return Policy{AutomaticBudget: domain == DomainFiller}
		}
	}
	return &Governor{meter: meter, policy: policy, reservations: make(map[uint64]reservation), usage: make(map[usageKey]*usage), now: time.Now}
}

// Snapshot reports current automatic filler capacity without reserving it.
func (g *Governor) Snapshot(ctx context.Context, path string) Decision {
	return g.DomainSnapshot(ctx, path, DomainFiller)
}

// DomainSnapshot reports current automatic capacity for one domain.
func (g *Governor) DomainSnapshot(ctx context.Context, path string, domain Domain) Decision {
	domain = normalizeDomain(domain)
	if !validDomain(domain) {
		return Decision{Snapshot: Snapshot{Domain: domain, Reason: ReasonCapacityUnavailable}, Err: errors.New("storage domain is unknown")}
	}
	if err := g.prepareUsage(ctx, path, domain); err != nil {
		return Decision{Snapshot: Snapshot{Domain: domain, Reason: ReasonCapacityUnavailable}, Err: err}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.snapshotLocked(ctx, path, domain, Automatic, 0)
}

// Reserve atomically grants estimated work or returns one typed pause reason.
func (g *Governor) Reserve(ctx context.Context, request Request) (*Lease, Decision) {
	if request.EstimatedBytes <= 0 {
		decision := Decision{Snapshot: Snapshot{Domain: normalizeDomain(request.Domain), Reason: ReasonEstimateUnknown}}
		return nil, decision
	}
	request.Domain = normalizeDomain(request.Domain)
	if !validDomain(request.Domain) {
		decision := Decision{Snapshot: Snapshot{Domain: request.Domain, Reason: ReasonCapacityUnavailable}, Err: errors.New("storage domain is unknown")}
		return nil, decision
	}
	if err := g.prepareUsage(ctx, request.Path, request.Domain); err != nil {
		return nil, Decision{Snapshot: Snapshot{Domain: request.Domain, Reason: ReasonCapacityUnavailable}, Err: err}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	decision := g.snapshotLocked(ctx, request.Path, request.Domain, request.Mode, 0)
	if !decision.Allowed {
		return nil, decision
	}
	reason := limitingReason(decision.Snapshot, request.EstimatedBytes, request.Mode)
	if reason != "" {
		decision.Allowed = false
		decision.Snapshot.Reason = reason
		return nil, decision
	}

	g.nextLeaseID++
	id := g.nextLeaseID
	g.reservations[id] = reservation{
		filesystemID: decision.Snapshot.FilesystemID, path: request.Path, domain: request.Domain,
		estimated: request.EstimatedBytes, remaining: request.EstimatedBytes,
	}
	decision.Snapshot.ReservedBytes += request.EstimatedBytes
	decision.Snapshot.FilesystemReservedBytes += request.EstimatedBytes
	decision.Snapshot.AvailableBytes -= request.EstimatedBytes
	return &Lease{governor: g, id: id, path: request.Path, domain: request.Domain}, decision
}

// Lease is an exclusive claim on estimated bytes. Revalidate before execution,
// report monotonic progress as bytes materialize, and release on every terminal
// path. Release is idempotent.
type Lease struct {
	governor *Governor
	id       uint64
	path     string
	domain   Domain
}

// Revalidate re-measures capacity and converts written bytes into real managed
// usage while retaining only the outstanding reservation. Reporting more bytes
// than the estimate fails closed: the estimate was not safe enough to write.
func (l *Lease) Revalidate(ctx context.Context, writtenBytes int64) Decision {
	if l == nil || l.governor == nil {
		return Decision{Snapshot: Snapshot{Reason: ReasonCapacityUnavailable}, Err: errors.New("storage lease is unavailable")}
	}
	g := l.governor
	if err := g.prepareUsage(ctx, l.path, l.domain); err != nil {
		return Decision{Snapshot: Snapshot{Domain: l.domain, Reason: ReasonCapacityUnavailable}, Err: err}
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	current, ok := g.reservations[l.id]
	if !ok {
		return Decision{Snapshot: Snapshot{Reason: ReasonCapacityUnavailable}, Err: errors.New("storage lease was released")}
	}
	if writtenBytes < current.written || writtenBytes > current.estimated {
		return Decision{Snapshot: Snapshot{Domain: current.domain, Reason: ReasonEstimateUnknown}, Err: errors.New("written bytes exceed the storage estimate")}
	}

	// Soft limits govern admission, not already-reserved work. Revalidation still applies the
	// live hard host reserve, which no domain setting or manual confirmation may waive.
	decision := g.snapshotLocked(ctx, current.path, current.domain, ConfirmedManual, l.id)
	if !decision.Allowed {
		return decision
	}
	remaining := current.estimated - writtenBytes
	reason := limitingReason(decision.Snapshot, remaining, ConfirmedManual)
	if reason != "" {
		decision.Allowed = false
		decision.Snapshot.Reason = reason
		return decision
	}

	g.addUsageLocked(usageKey{filesystemID: current.filesystemID, domain: current.domain}, writtenBytes-current.written)
	current.written = writtenBytes
	current.remaining = remaining
	g.reservations[l.id] = current
	decision.Snapshot.ReservedBytes += remaining
	decision.Snapshot.FilesystemReservedBytes += remaining
	decision.Snapshot.AvailableBytes -= remaining
	return decision
}

// Release returns all outstanding bytes. It is safe to call more than once.
func (l *Lease) Release() {
	if l == nil || l.governor == nil {
		return
	}
	l.governor.mu.Lock()
	delete(l.governor.reservations, l.id)
	l.governor.mu.Unlock()
}

func (g *Governor) snapshotLocked(ctx context.Context, path string, domain Domain, mode Mode, excludedLease uint64) Decision {
	if err := ctx.Err(); err != nil {
		return Decision{Snapshot: Snapshot{Domain: domain, Reason: ReasonCapacityUnavailable}, Err: err}
	}
	if g.meter == nil || path == "" {
		return Decision{Snapshot: Snapshot{Domain: domain, Reason: ReasonCapacityUnavailable}, Err: errors.New("storage meter or destination is unavailable")}
	}
	measurement, err := g.meter.Measure(ctx, path)
	if err != nil {
		return Decision{Snapshot: Snapshot{Domain: domain, Reason: ReasonCapacityUnavailable}, Err: err}
	}
	if measurement.ID == "" || measurement.TotalBytes <= 0 || measurement.FreeBytes < 0 || measurement.FreeBytes > measurement.TotalBytes {
		return Decision{Snapshot: Snapshot{Domain: domain, Reason: ReasonCapacityUnavailable}, Err: errors.New("filesystem measurement is invalid")}
	}
	entry := g.usage[usageKey{filesystemID: measurement.ID, domain: domain}]
	if entry == nil || !entry.loaded {
		return Decision{Snapshot: Snapshot{Domain: domain, FilesystemID: measurement.ID, Reason: ReasonCapacityUnavailable}, Err: errManagedInvalid}
	}
	managed := entry.bytes
	filesystemReserved, domainReserved := g.reservedLocked(measurement.ID, domain, excludedLease)
	softBudget, softLimit := g.policy(domain).budget(measurement.TotalBytes)
	snapshot := Snapshot{
		Domain: domain, FilesystemID: measurement.ID, TotalBytes: measurement.TotalBytes,
		FreeBytes: measurement.FreeBytes, ManagedBytes: managed, ReservedBytes: domainReserved,
		FilesystemReservedBytes: filesystemReserved, SoftBudgetBytes: softBudget,
		SoftLimitEnabled: softLimit, HardReserveBytes: HardReserveBytes(measurement.TotalBytes),
	}
	hostAvailable := subtractFloor(measurement.FreeBytes, snapshot.HardReserveBytes, filesystemReserved)
	softAvailable := subtractFloor(snapshot.SoftBudgetBytes, managed, domainReserved)
	snapshot.AvailableBytes = hostAvailable
	if mode != ConfirmedManual && softLimit && softAvailable < snapshot.AvailableBytes {
		snapshot.AvailableBytes = softAvailable
	}
	if hostAvailable == 0 {
		snapshot.Reason = ReasonHostReserve
	} else if mode != ConfirmedManual && softLimit && softAvailable == 0 {
		snapshot.Reason = ReasonLibraryLimit
	}
	return Decision{Allowed: snapshot.Reason == "", Snapshot: snapshot}
}

func (g *Governor) reservedLocked(filesystemID string, domain Domain, excludedLease uint64) (int64, int64) {
	var filesystemTotal, domainTotal int64
	for id, reservation := range g.reservations {
		if id == excludedLease || reservation.filesystemID != filesystemID {
			continue
		}
		filesystemTotal = saturatingAdd(filesystemTotal, reservation.remaining)
		if reservation.domain == domain {
			domainTotal = saturatingAdd(domainTotal, reservation.remaining)
		}
	}
	return filesystemTotal, domainTotal
}

func limitingReason(snapshot Snapshot, bytes int64, mode Mode) Reason {
	hostAvailable := subtractFloor(snapshot.FreeBytes, snapshot.HardReserveBytes, snapshot.FilesystemReservedBytes)
	if bytes > hostAvailable {
		return ReasonHostReserve
	}
	if mode != ConfirmedManual && snapshot.SoftLimitEnabled {
		softAvailable := subtractFloor(snapshot.SoftBudgetBytes, snapshot.ManagedBytes, snapshot.ReservedBytes)
		if bytes > softAvailable {
			return ReasonLibraryLimit
		}
	}
	return ""
}

func subtractFloor(value int64, subtractors ...int64) int64 {
	for _, subtractor := range subtractors {
		if subtractor >= value {
			return 0
		}
		value -= subtractor
	}
	return value
}

func saturatingAdd(left, right int64) int64 {
	if right <= 0 {
		return left
	}
	maximum := int64(^uint64(0) >> 1)
	if left > maximum-right {
		return maximum
	}
	return left + right
}

func saturatingMultiply(left, right int64) int64 {
	if left <= 0 || right <= 0 {
		return 0
	}
	maximum := int64(^uint64(0) >> 1)
	if left > maximum/right {
		return maximum
	}
	return left * right
}

// Extend raises this lease's estimate to newEstimate so a writer that outgrew its forecast is
// re-estimated from what it has actually written instead of being discarded. It is admission for
// the additional bytes only: the live hard host reserve (and any bytes other leases hold) still
// applies, so an extension can be refused when the disk genuinely cannot take the rest.
func (l *Lease) Extend(ctx context.Context, newEstimate int64) Decision {
	if l == nil || l.governor == nil {
		return Decision{Snapshot: Snapshot{Reason: ReasonCapacityUnavailable}, Err: errors.New("storage lease is unavailable")}
	}
	g := l.governor
	if err := g.prepareUsage(ctx, l.path, l.domain); err != nil {
		return Decision{Snapshot: Snapshot{Domain: l.domain, Reason: ReasonCapacityUnavailable}, Err: err}
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	current, ok := g.reservations[l.id]
	if !ok {
		return Decision{Snapshot: Snapshot{Reason: ReasonCapacityUnavailable}, Err: errors.New("storage lease was released")}
	}
	if newEstimate <= current.estimated {
		return Decision{Allowed: true, Snapshot: Snapshot{Domain: current.domain}}
	}
	decision := g.snapshotLocked(ctx, current.path, current.domain, ConfirmedManual, l.id)
	if !decision.Allowed {
		return decision
	}
	if reason := limitingReason(decision.Snapshot, newEstimate-current.written, ConfirmedManual); reason != "" {
		decision.Allowed = false
		decision.Snapshot.Reason = reason
		return decision
	}
	current.estimated = newEstimate
	current.remaining = newEstimate - current.written
	g.reservations[l.id] = current
	return decision
}

// Estimated reports the bytes this lease currently forecasts it will write in total.
func (l *Lease) Estimated() int64 {
	if l == nil || l.governor == nil {
		return 0
	}
	l.governor.mu.Lock()
	defer l.governor.mu.Unlock()
	return l.governor.reservations[l.id].estimated
}
