package provision

import (
	"testing"
	"time"
)

var (
	t0       = time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	reqTTL   = t0.Add(48 * time.Hour)
	dlTTL    = t0.Add(12 * time.Hour)
	testKey  = Key("movie:tmdb:1111867")
	testTitl = Title{MediaType: Movie, TMDBID: 1111867, Name: "In Flames"}
)

func rec(state State, deadline time.Time) Record {
	return Record{Key: testKey, Title: testTitl, State: state, Deadline: deadline}
}

// Happy path through the §4 diagram: wanted→requested→downloading→available.
func TestHappyPath(t *testing.T) {
	r := rec(Wanted, time.Time{})

	r, ev := Apply(r, Event{Kind: RequestAccepted, Deadline: reqTTL}, t0)
	if r.State != Requested {
		t.Fatalf("after RequestAccepted: %s, want requested", r.State)
	}
	if len(ev) != 0 {
		t.Errorf("requested must not emit (invariant 2), got %d events", len(ev))
	}

	r, ev = Apply(r, Event{Kind: Grabbed, Deadline: dlTTL}, t0.Add(time.Hour))
	if r.State != Downloading {
		t.Fatalf("after Grabbed: %s, want downloading", r.State)
	}
	if !r.Deadline.Equal(dlTTL) {
		t.Errorf("Grabbed must reset deadline to downloading TTL (invariant 5), got %v", r.Deadline)
	}
	if len(ev) != 0 {
		t.Errorf("downloading must not emit, got %d", len(ev))
	}

	r, ev = Apply(r, Event{Kind: LibraryConfirmed, LibraryID: "2879419"}, t0.Add(2*time.Hour))
	if r.State != Available {
		t.Fatalf("after LibraryConfirmed: %s, want available", r.State)
	}
	if r.LibraryID != "2879419" {
		t.Errorf("LibraryID = %q, want 2879419", r.LibraryID)
	}
	if len(ev) != 1 || ev[0].State != Available || ev[0].Key != testKey {
		t.Errorf("available must emit one DomainEvent for the scheduler (invariant 2), got %+v", ev)
	}
}

// Invariant 1: terminal states are frozen against provisioning events.
func TestTerminalMonotonicity(t *testing.T) {
	for _, term := range []State{Available, Unavailable} {
		r := rec(term, reqTTL)
		r.LibraryID = "orig"
		for _, k := range []EventKind{Grabbed, RequestAccepted, LibraryConfirmed, DeadlineExceeded} {
			out, ev := Apply(r, Event{Kind: k, LibraryID: "changed", Deadline: dlTTL}, reqTTL.Add(time.Hour))
			if out.State != term {
				t.Errorf("%s + %s regressed to %s (invariant 1)", term, k, out.State)
			}
			if len(ev) != 0 {
				t.Errorf("%s + %s emitted an event; terminal must be silent", term, k)
			}
		}
	}
}

// Invariant 3: a duplicate event that changes nothing is a no-op (no UpdatedAt bump, no emit).
func TestIdempotentDuplicate(t *testing.T) {
	r := rec(Downloading, dlTTL)
	r.UpdatedAt = t0
	out, ev := Apply(r, Event{Kind: Grabbed, Deadline: dlTTL}, t0.Add(time.Hour))
	if len(ev) != 0 {
		t.Errorf("duplicate Grabbed emitted %d events, want 0", len(ev))
	}
	if !out.UpdatedAt.Equal(t0) {
		t.Errorf("no-op bumped UpdatedAt to %v (invariant 3)", out.UpdatedAt)
	}
}

// Invariant 4: library confirmation from any in-flight state wins (source of truth).
func TestLibraryConfirmFromAnyInflight(t *testing.T) {
	for _, from := range []State{Wanted, Requested, Downloading} {
		out, ev := Apply(rec(from, reqTTL), Event{Kind: LibraryConfirmed, LibraryID: "x"}, t0)
		if out.State != Available {
			t.Errorf("LibraryConfirmed from %s → %s, want available (invariant 4)", from, out.State)
		}
		if len(ev) != 1 {
			t.Errorf("LibraryConfirmed from %s should emit once, got %d", from, len(ev))
		}
	}
}

// Invariant 5: deadline is only honored past its time; a not-yet-due check is a no-op.
func TestDeadlineDiscipline(t *testing.T) {
	// Not yet past deadline → no give-up.
	r := rec(Requested, reqTTL)
	out, ev := Apply(r, Event{Kind: DeadlineExceeded, Err: "ttl"}, reqTTL.Add(-time.Minute))
	if out.State != Requested || len(ev) != 0 {
		t.Errorf("premature DeadlineExceeded changed state to %s (invariant 5)", out.State)
	}
	// Past deadline → unavailable + emit.
	out, ev = Apply(r, Event{Kind: DeadlineExceeded, Err: "ttl"}, reqTTL.Add(time.Minute))
	if out.State != Unavailable {
		t.Fatalf("expired DeadlineExceeded → %s, want unavailable", out.State)
	}
	if len(ev) != 1 || ev[0].State != Unavailable {
		t.Errorf("unavailable must emit for the scheduler, got %+v", ev)
	}
}

// SubmitFailed holds the record at wanted and records the error for reconcile retry.
func TestSubmitFailedHoldsWanted(t *testing.T) {
	out, ev := Apply(rec(Wanted, time.Time{}), Event{Kind: SubmitFailed, Err: "seerr 500"}, t0)
	if out.State != Wanted {
		t.Errorf("SubmitFailed → %s, want wanted", out.State)
	}
	if out.Attempts != 1 || out.LastError != "seerr 500" {
		t.Errorf("SubmitFailed should record attempt+error, got attempts=%d err=%q", out.Attempts, out.LastError)
	}
	if len(ev) != 0 {
		t.Errorf("wanted must not emit, got %d", len(ev))
	}
}

// Illegal transitions leave the record unchanged (no crash, no spurious emit).
func TestIllegalTransitionIsNoop(t *testing.T) {
	// Grabbed while still wanted is not a legal edge in §4.
	out, ev := Apply(rec(Wanted, time.Time{}), Event{Kind: Grabbed, Deadline: dlTTL}, t0)
	if out.State != Wanted || len(ev) != 0 {
		t.Errorf("illegal Grabbed-from-wanted changed state to %s / emitted %d", out.State, len(ev))
	}
}
