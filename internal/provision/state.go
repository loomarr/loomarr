package provision

import "time"

// State is a provisioning lifecycle state (§4).
type State string

const (
	Wanted      State = "wanted"      // submission not yet accepted downstream
	Requested   State = "requested"   // accepted by Seerr/*arr; awaiting a release
	Downloading State = "downloading" // a release was grabbed; in flight
	Available   State = "available"   // present in the library, schedulable (emits)
	Unavailable State = "unavailable" // gave up: deadline / unfindable (emits)
)

// Terminal reports whether s is an end state. Terminal states are monotonic for
// the acquisition lifecycle: no provisioning event regresses them (invariant 1).
func (s State) Terminal() bool { return s == Available || s == Unavailable }

// Emits reports whether entering s produces a domain event for the scheduler.
// Only available/unavailable do (invariant 2).
func (s State) Emits() bool { return s == Available || s == Unavailable }

// Record is the persisted provisioning state for one Title (§3). The domain
// operates on it as a value; the store (Phase 3) owns persistence and the epoch
// timestamp encoding (§3 note).
type Record struct {
	Key         Key
	Title       Title
	State       State
	LibraryID   string // media-server item id once present
	RequestedAt time.Time
	Deadline    time.Time
	Attempts    int
	LastError   string
	UpdatedAt   time.Time

	// Download progress (§18.1 arr-queue-poll). Display-only, poll-updated via the store's
	// targeted UpdateTitleProgress — NOT written by the state-machine Upsert, so a reconcile
	// or scan pass never clobbers the latest progress. Zero on the Seerr path / once available.
	Progress       float64 // 0..1 completion fraction
	ETAText        string  // the arr's human time-left string
	DownloadStatus string  // the arr's queue status ("downloading"/"warning"/"stalled"/…)
}
