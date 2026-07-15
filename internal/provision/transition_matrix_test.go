package provision

import (
	"math/rand"
	"reflect"
	"testing"
	"time"
)

// allStates and allEventKinds are the complete enumerations the state machine is
// defined over (state.go const block, transition.go EventKind const block).
var allStates = []State{Wanted, Requested, Downloading, Available, Unavailable}

var allEventKinds = []EventKind{
	RequestAccepted, SubmitFailed, Grabbed, LibraryConfirmed, DeadlineExceeded,
}

// progressRank encodes the acquisition lifecycle order (§4):
// wanted → requested → downloading → {available|unavailable}. Both terminals sit
// at the same, highest rank — they are absorbing, and neither is "further" than
// the other. This is used to assert the machine never regresses.
func progressRank(s State) int {
	switch s {
	case Wanted:
		return 0
	case Requested:
		return 1
	case Downloading:
		return 2
	case Available, Unavailable:
		return 3
	}
	return -1
}

// fieldsEqual reports whether two records agree on every domain field Apply may
// touch. UpdatedAt is deliberately excluded — it is the "something changed"
// signal, checked separately.
func fieldsEqual(a, b Record) bool {
	return a.State == b.State &&
		a.LibraryID == b.LibraryID &&
		a.Deadline.Equal(b.Deadline) &&
		a.LastError == b.LastError &&
		a.Attempts == b.Attempts &&
		a.RequestedAt.Equal(b.RequestedAt)
}

// TestApplyMatrix enumerates every (state × event-kind) cell and asserts each
// result is legal:
//   - no panic (the func returns);
//   - terminal states are absorbing: no field changes and nothing is emitted;
//   - an emission occurs iff the State actually changed into an emitting state;
//   - a non-terminal, non-changing cell is a silent no-op (no UpdatedAt bump).
//
// The event is populated generously (LibraryID, a live Deadline, an Err) so that
// every kind has the inputs it might act on; `now` is chosen strictly past the
// record's deadline so DeadlineExceeded is eligible to fire.
func TestApplyMatrix(t *testing.T) {
	const origLib = "orig-lib"
	baseDeadline := t0.Add(24 * time.Hour)
	now := baseDeadline.Add(time.Hour) // strictly past the deadline
	newDeadline := now.Add(6 * time.Hour)

	for _, from := range allStates {
		for _, kind := range allEventKinds {
			from, kind := from, kind
			t.Run(string(from)+"_"+string(kind), func(t *testing.T) {
				in := Record{
					Key:       testKey,
					Title:     testTitl,
					State:     from,
					LibraryID: origLib,
					Deadline:  baseDeadline,
					UpdatedAt: t0,
				}
				ev := Event{
					Kind:      kind,
					LibraryID: "new-lib",
					Deadline:  newDeadline,
					Err:       "detail",
				}

				out, emitted := Apply(in, ev, now) // must not panic

				// Terminal states are absorbing.
				if from.Terminal() {
					if !fieldsEqual(out, in) {
						t.Fatalf("terminal %s + %s changed fields: %+v -> %+v", from, kind, in, out)
					}
					if !out.UpdatedAt.Equal(in.UpdatedAt) {
						t.Errorf("terminal %s + %s bumped UpdatedAt", from, kind)
					}
					if len(emitted) != 0 {
						t.Errorf("terminal %s + %s emitted %d events, want 0", from, kind, len(emitted))
					}
					return
				}

				changed := !fieldsEqual(out, in)

				// Emission happens iff the State changed into an emitting state.
				stateChanged := out.State != in.State
				wantEmit := stateChanged && out.State.Emits()
				if wantEmit {
					if len(emitted) != 1 {
						t.Errorf("%s + %s -> %s should emit exactly one event, got %d", from, kind, out.State, len(emitted))
					} else {
						if emitted[0].State != out.State {
							t.Errorf("emitted state %s, want %s", emitted[0].State, out.State)
						}
						if emitted[0].Key != in.Key {
							t.Errorf("emitted key %q, want %q", emitted[0].Key, in.Key)
						}
						if !reflect.DeepEqual(emitted[0].Title, in.Title) {
							t.Errorf("emitted title %+v, want %+v", emitted[0].Title, in.Title)
						}
					}
				} else if len(emitted) != 0 {
					t.Errorf("%s + %s (stateChanged=%v emits=%v) emitted %d events, want 0",
						from, kind, stateChanged, out.State.Emits(), len(emitted))
				}

				// UpdatedAt is bumped iff a field changed.
				if changed {
					if !out.UpdatedAt.Equal(now) {
						t.Errorf("%s + %s changed fields but UpdatedAt=%v, want now=%v", from, kind, out.UpdatedAt, now)
					}
				} else {
					if !out.UpdatedAt.Equal(in.UpdatedAt) {
						t.Errorf("%s + %s no field change but UpdatedAt bumped to %v", from, kind, out.UpdatedAt)
					}
					if len(emitted) != 0 {
						t.Errorf("%s + %s no field change but emitted %d events", from, kind, len(emitted))
					}
				}

				// The machine must never regress along the lifecycle.
				if progressRank(out.State) < progressRank(in.State) {
					t.Errorf("%s + %s regressed to %s", from, kind, out.State)
				}
			})
		}
	}
}

// randomEvent builds an arbitrary Event whose fields are randomized so a duplicate
// application under the same rng draw is exact. Deadlines are drawn around `base`
// so that `now` (also drawn around base) can land either side of them.
func randomEvent(rng *rand.Rand, base time.Time) Event {
	kind := allEventKinds[rng.Intn(len(allEventKinds))]
	ev := Event{Kind: kind}
	// Roughly half the time the deadline is zero (caller left it unset).
	if rng.Intn(2) == 0 {
		ev.Deadline = base.Add(time.Duration(rng.Intn(72)-36) * time.Hour)
	}
	if rng.Intn(3) != 0 {
		ev.LibraryID = "lib-" + string(rune('a'+rng.Intn(6)))
	}
	if rng.Intn(3) != 0 {
		ev.Err = "err-" + string(rune('a'+rng.Intn(6)))
	}
	return ev
}

func randomState(rng *rand.Rand) State {
	return allStates[rng.Intn(len(allStates))]
}

func randomRecord(rng *rand.Rand, base time.Time) Record {
	r := Record{
		Key:       testKey,
		Title:     testTitl,
		State:     randomState(rng),
		UpdatedAt: base,
	}
	if rng.Intn(2) == 0 {
		r.LibraryID = "lib-" + string(rune('a'+rng.Intn(6)))
	}
	if rng.Intn(2) == 0 {
		r.Deadline = base.Add(time.Duration(rng.Intn(72)-36) * time.Hour)
	}
	if rng.Intn(2) == 0 {
		r.RequestedAt = base.Add(time.Duration(rng.Intn(48)) * time.Hour)
	}
	r.Attempts = rng.Intn(4)
	if rng.Intn(2) == 0 {
		r.LastError = "prev-err"
	}
	return r
}

// TestApplyProperties is a seeded, plain-Go property loop (no framework, no new
// deps — CLAUDE.md testing rules). Over many random Record × Event × now draws it
// asserts the invariants Apply promises (transition.go doc):
//
//   - Absorbing terminal set: once State.Terminal(), no further event changes any
//     field or emits.
//   - Idempotency: applying the identical event twice never moves the lifecycle
//     again (State/LibraryID/Deadline stable, no re-emit). The sole exception is
//     SubmitFailed-while-wanted, which is a retry counter by design.
//   - UpdatedAt bumps iff a domain field changed.
//   - Monotone progress: State never regresses along the lifecycle rank.
//   - Never panics.
//
// The seed is fixed so any failure reproduces deterministically.
func TestApplyProperties(t *testing.T) {
	const seed = 0x10035
	base := time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)

	const iterations = 20000
	rng := rand.New(rand.NewSource(seed))

	for i := 0; i < iterations; i++ {
		rec := randomRecord(rng, base)
		ev := randomEvent(rng, base)
		now := base.Add(time.Duration(rng.Intn(96)-48) * time.Hour)

		out, emitted := Apply(rec, ev, now) // never panics

		// Absorbing terminal set.
		if rec.State.Terminal() {
			if !fieldsEqual(out, rec) || !out.UpdatedAt.Equal(rec.UpdatedAt) {
				t.Fatalf("iter %d seed %#x: terminal %s changed by %s: %+v -> %+v",
					i, seed, rec.State, ev.Kind, rec, out)
			}
			if len(emitted) != 0 {
				t.Fatalf("iter %d seed %#x: terminal %s emitted on %s", i, seed, rec.State, ev.Kind)
			}
		}

		// Monotone progress: never regress.
		if progressRank(out.State) < progressRank(rec.State) {
			t.Fatalf("iter %d seed %#x: %s + %s regressed to %s",
				i, seed, rec.State, ev.Kind, out.State)
		}

		fieldChanged := !fieldsEqual(out, rec)

		// UpdatedAt bumps iff a field changed.
		if fieldChanged {
			if !out.UpdatedAt.Equal(now) {
				t.Fatalf("iter %d seed %#x: field changed on %s but UpdatedAt=%v, want %v",
					i, seed, ev.Kind, out.UpdatedAt, now)
			}
		} else if !out.UpdatedAt.Equal(rec.UpdatedAt) {
			t.Fatalf("iter %d seed %#x: no field change on %s but UpdatedAt bumped %v -> %v",
				i, seed, ev.Kind, rec.UpdatedAt, out.UpdatedAt)
		}

		// Emission is consistent with an emitting state change.
		if len(emitted) != 0 {
			if out.State == rec.State {
				t.Fatalf("iter %d seed %#x: emitted without a state change (%s)", i, seed, out.State)
			}
			if !out.State.Emits() {
				t.Fatalf("iter %d seed %#x: emitted from non-emitting state %s", i, seed, out.State)
			}
		}

		// Idempotency: re-applying the identical event to the first result never
		// moves the lifecycle again — State/LibraryID/Deadline are stable and it
		// never re-emits (the availability feed fires once per settle).
		//
		// The single, deliberate exception is SubmitFailed while wanted: it is a
		// retry counter (transition.go), so a second application legitimately bumps
		// Attempts (and thus UpdatedAt). That is not a lifecycle move — State stays
		// wanted — so we exclude only the counter fields from the strict no-op check
		// while still asserting the lifecycle-identity fields hold.
		out2, emitted2 := Apply(out, ev, now)
		if out2.State != out.State || out2.LibraryID != out.LibraryID ||
			!out2.Deadline.Equal(out.Deadline) || !out2.RequestedAt.Equal(out.RequestedAt) {
			t.Fatalf("iter %d seed %#x: %s not idempotent on lifecycle fields: %+v -> %+v",
				i, seed, ev.Kind, out, out2)
		}
		if len(emitted2) != 0 {
			t.Fatalf("iter %d seed %#x: idempotent re-apply of %s emitted %d events",
				i, seed, ev.Kind, len(emitted2))
		}

		submitRetry := ev.Kind == SubmitFailed && out.State == Wanted
		if submitRetry {
			// The counter must advance and UpdatedAt must track it.
			if out2.Attempts != out.Attempts+1 {
				t.Fatalf("iter %d seed %#x: SubmitFailed retry did not bump Attempts %d -> %d",
					i, seed, out.Attempts, out2.Attempts)
			}
			if !out2.UpdatedAt.Equal(now) {
				t.Fatalf("iter %d seed %#x: SubmitFailed retry changed Attempts but UpdatedAt=%v, want %v",
					i, seed, out2.UpdatedAt, now)
			}
		} else {
			// Every other event is a strict field no-op on the second application.
			if !fieldsEqual(out2, out) {
				t.Fatalf("iter %d seed %#x: %s not idempotent: %+v -> %+v",
					i, seed, ev.Kind, out, out2)
			}
			if !out2.UpdatedAt.Equal(out.UpdatedAt) {
				t.Fatalf("iter %d seed %#x: idempotent re-apply of %s bumped UpdatedAt %v -> %v",
					i, seed, ev.Kind, out.UpdatedAt, out2.UpdatedAt)
			}
		}
	}
}
