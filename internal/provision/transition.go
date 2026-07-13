package provision

import "time"

// EventKind is a provisioning input the state machine reacts to (§4). These are
// domain events, distinct from the raw webhooks (Phase 6 maps webhooks → these).
type EventKind string

const (
	// RequestAccepted: Seerr/*arr accepted the request (201/409 — §6). wanted→requested.
	RequestAccepted EventKind = "request_accepted"
	// SubmitFailed: downstream submission failed; hold as wanted for reconcile retry.
	SubmitFailed EventKind = "submit_failed"
	// Grabbed: a release was grabbed (webhook Grab). requested→downloading; resets deadline.
	Grabbed EventKind = "grabbed"
	// LibraryConfirmed: the library reports the item present (§4 invariant 4). →available.
	LibraryConfirmed EventKind = "library_confirmed"
	// DeadlineExceeded: past the record's deadline; give up. →unavailable.
	DeadlineExceeded EventKind = "deadline_exceeded"
)

// Event is a single input to Apply. Fields are populated as the kind needs:
// LibraryConfirmed carries LibraryID; Grabbed/RequestAccepted carry the new
// Deadline the caller computed from the relevant TTL (§6, invariant 5).
type Event struct {
	Kind      EventKind
	LibraryID string    // LibraryConfirmed
	Deadline  time.Time // RequestAccepted (request TTL), Grabbed (shorter downloading TTL)
	Err       string    // SubmitFailed / DeadlineExceeded detail
}

// DomainEvent is emitted to the scheduler when a record reaches a terminal state
// (invariant 2). It is the internal availability feed (§2).
type DomainEvent struct {
	Key   Key
	State State // available | unavailable
	Title Title
}

// Apply is the pure state transition (§4). Given the current record, an event,
// and the current time, it returns the next record and any domain events to
// emit. It never mutates its input and performs no I/O.
//
// Invariants enforced here:
//  1. Terminal monotonicity: available/unavailable ignore all further events.
//  3. Idempotent: a duplicate event that would not change state is a no-op.
//  5. Deadline discipline: Grabbed resets the deadline (to the caller's shorter
//     downloading TTL); DeadlineExceeded is only honored while still in flight.
//
// Illegal transitions (e.g. Grabbed while wanted) leave the record unchanged and
// emit nothing — the reconciler treats a no-op as "nothing to do", never a crash.
func Apply(rec Record, ev Event, now time.Time) (Record, []DomainEvent) {
	// Invariant 1: terminal states are frozen against provisioning events.
	if rec.State.Terminal() {
		return rec, nil
	}

	next := rec // copy; never mutate the caller's record

	switch ev.Kind {
	case RequestAccepted:
		// wanted → requested (or a no-op refresh while already requested).
		if rec.State == Wanted || rec.State == Requested {
			next.State = Requested
			if !ev.Deadline.IsZero() {
				next.Deadline = ev.Deadline
			}
			if rec.RequestedAt.IsZero() {
				next.RequestedAt = now
			}
		}

	case SubmitFailed:
		// Stay/enter wanted for a later reconcile retry; record the error.
		if rec.State == Wanted {
			next.State = Wanted
			next.LastError = ev.Err
			next.Attempts = rec.Attempts + 1
		}

	case Grabbed:
		// requested → downloading; reset the deadline to the shorter TTL (inv. 5).
		// A duplicate Grab while already downloading just refreshes the deadline.
		if rec.State == Requested || rec.State == Downloading {
			next.State = Downloading
			if !ev.Deadline.IsZero() {
				next.Deadline = ev.Deadline
			}
		}

	case LibraryConfirmed:
		// Library is the source of truth (inv. 4): any in-flight state → available.
		next.State = Available
		next.LibraryID = ev.LibraryID

	case DeadlineExceeded:
		// Only give up if still in flight and actually past the deadline.
		if !now.Before(rec.Deadline) {
			next.State = Unavailable
			next.LastError = ev.Err
		}
	}

	if next.State == rec.State && next.LibraryID == rec.LibraryID &&
		next.Deadline.Equal(rec.Deadline) && next.LastError == rec.LastError &&
		next.Attempts == rec.Attempts {
		// No effective change — idempotent no-op (invariant 3). Don't bump UpdatedAt.
		return rec, nil
	}

	next.UpdatedAt = now

	var emitted []DomainEvent
	if next.State != rec.State && next.State.Emits() {
		emitted = []DomainEvent{{Key: next.Key, State: next.State, Title: next.Title}}
	}
	return next, emitted
}
