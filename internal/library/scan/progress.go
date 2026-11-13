package scan

import (
	"sync"
	"time"
)

// Step is one of the three steps a library import shows the maintainer (#2151): looking through
// the folders, reading each new file, and looking up each new title's details.
type Step string

const (
	// StepWalking is the walk: done counts the top-level entries of the walked roots finished.
	StepWalking Step = "walking"
	// StepReading is the header reads: done counts the files read.
	StepReading Step = "reading"
	// StepLookingUp is the Enricher's fetches of titles new to the catalog cache.
	StepLookingUp Step = "lookingUp"
)

// steps is every step in the order the maintainer sees them; a step's number is its place here
// plus one.
var steps = [...]Step{StepWalking, StepReading, StepLookingUp}

// An estimate of the time left in a step is given only once the step has run etaAfter since its
// rate baseline and done at least etaShare of its total: before that the rate is noise.
const (
	etaAfter = 30 * time.Second
	etaShare = 0.02
)

// ProgressSnapshot is what the maintainer sees of an import at one moment: the earliest step still
// running, how far it is, and the time it has left.
type ProgressSnapshot struct {
	// Step is the earliest step running; "" when none is.
	Step Step
	// Number is Step's place, 1 to 3; 0 when none runs.
	Number      int
	Done, Total int
	// Current is what the step is on now: a folder, a file or a title; "" when unknown.
	Current string
	// ETA is the time the step has left, valid only when HasETA.
	ETA    time.Duration
	HasETA bool
	// Paused is true while the step waits for someone to stop watching.
	Paused bool
	// Quiet is an import nobody asked for with nothing to read or look up: the hourly pass
	// finding nothing new, shown as one small line.
	Quiet bool
}

// Tracker is the one record of an import's progress. The Importer and the Enricher report into it
// as each step begins, grows, advances, pauses and ends; the status endpoint and the live frames
// read it through Snapshot. Every method is safe on a nil Tracker, which records nothing.
type Tracker struct {
	// Now is the clock rate baselines are taken on; nil is time.Now.
	Now func() time.Time
	// OnChange hears every change, after it is made and outside the Tracker's lock, so it may call
	// Snapshot. urgent is true when the step shown or its pause may have changed: a step began or
	// ended, or a pause began or ended. It must not block.
	OnChange func(urgent bool)

	mu    sync.Mutex
	state [len(steps)]stepState
	// asked is whether the running pass was asked for; pending is an ask the next pass takes.
	asked, pending bool
}

// stepState is one step's counts.
type stepState struct {
	active      bool
	done, total int
	current     string
	paused      bool
	// since and sinceDone are the rate baseline: when it was taken and done then.
	since     time.Time
	sinceDone int
}

// NewTracker returns a Tracker on the clock now (time.Now when nil) that tells onChange of each
// change (nil tells nothing).
func NewTracker(now func() time.Time, onChange func(urgent bool)) *Tracker {
	return &Tracker{Now: now, OnChange: onChange}
}

// Ask marks the next pass as asked for: Scan now, Refresh everything, a folder used or added, or
// the first import. An asked pass is never quiet.
func (t *Tracker) Ask() {
	t.update(false, func() { t.pending = true })
}

// Begin starts step with total to do, from nothing done. Beginning the walk starts a pass, which
// takes a pending ask.
func (t *Tracker) Begin(step Step, total int) {
	t.update(true, func() {
		if step == StepWalking {
			t.asked, t.pending = t.pending, false
		}
		t.state[step.index()] = stepState{active: true, total: max(total, 0), since: t.now(), sinceDone: 0}
	})
}

// Grow adds n to a running step's total; n may be negative, as when a queued file is dropped.
func (t *Tracker) Grow(step Step, n int) {
	t.update(false, func() {
		if s := &t.state[step.index()]; s.active {
			s.total = max(s.total+n, 0)
		}
	})
}

// Due raises a running step's total so that at least left remain: more became due while it ran.
func (t *Tracker) Due(step Step, left int) {
	t.update(false, func() {
		if s := &t.state[step.index()]; s.active && s.done+left > s.total {
			s.total = s.done + left
		}
	})
}

// Advance counts n more done in a running step.
func (t *Tracker) Advance(step Step, n int) {
	t.update(false, func() {
		if s := &t.state[step.index()]; s.active {
			s.done += n
		}
	})
}

// Rebase restarts a running step's rate baseline at what is done now. The work ahead goes at a
// different rate from the work behind, as when an import turns from listing folders to writing
// what it found, so the time left is worked out from here.
func (t *Tracker) Rebase(step Step) {
	t.update(false, func() {
		if s := &t.state[step.index()]; s.active {
			s.since, s.sinceDone = t.now(), s.done
		}
	})
}

// Current names what a running step is on now.
func (t *Tracker) Current(step Step, current string) {
	t.update(false, func() {
		if s := &t.state[step.index()]; s.active {
			s.current = current
		}
	})
}

// Pause marks a running step waiting, or going again. Going again restarts the rate baseline, so
// the wait never counts against the rate.
func (t *Tracker) Pause(step Step, paused bool) {
	if t == nil {
		return
	}
	t.mu.Lock()
	s := &t.state[step.index()]
	changed := s.active && s.paused != paused
	if changed {
		s.paused = paused
		if !paused {
			s.since, s.sinceDone = t.now(), s.done
		}
	}
	t.mu.Unlock()
	if changed {
		t.notify(true)
	}
}

// End ends step. Ending the walk ends the pass's ask.
func (t *Tracker) End(step Step) {
	t.update(true, func() {
		t.state[step.index()] = stepState{}
		if step == StepWalking {
			t.asked = false
		}
	})
}

// Active reports whether step is running.
func (t *Tracker) Active(step Step) bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.state[step.index()].active
}

// Snapshot is the import as it stands at now.
func (t *Tracker) Snapshot(now time.Time) ProgressSnapshot {
	if t == nil {
		return ProgressSnapshot{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for i, s := range t.state {
		if !s.active {
			continue
		}
		out := ProgressSnapshot{
			Step: steps[i], Number: i + 1, Done: min(s.done, s.total), Total: s.total,
			Current: s.current, Paused: s.paused,
		}
		if s.paused {
			out.Current = ""
		} else {
			out.ETA, out.HasETA = s.eta(now)
		}
		out.Quiet = steps[i] == StepWalking && !t.asked &&
			!t.state[StepReading.index()].active && !t.state[StepLookingUp.index()].active
		return out
	}
	return ProgressSnapshot{}
}

// eta is remaining × (elapsed ÷ done) since the rate baseline, once the step has run etaAfter
// and done etaShare of its total.
func (s stepState) eta(now time.Time) (time.Duration, bool) {
	elapsed, since := now.Sub(s.since), s.done-s.sinceDone
	if elapsed < etaAfter || since <= 0 || s.total <= 0 || float64(s.done) < etaShare*float64(s.total) {
		return 0, false
	}
	left := max(s.total-s.done, 0)
	return time.Duration(float64(elapsed) / float64(since) * float64(left)), true
}

// update makes change under the lock, then tells OnChange.
func (t *Tracker) update(urgent bool, change func()) {
	if t == nil {
		return
	}
	t.mu.Lock()
	change()
	t.mu.Unlock()
	t.notify(urgent)
}

func (t *Tracker) notify(urgent bool) {
	if t.OnChange != nil {
		t.OnChange(urgent)
	}
}

func (t *Tracker) now() time.Time {
	if t.Now != nil {
		return t.Now()
	}
	return time.Now()
}

func (s Step) index() int {
	for i, step := range steps {
		if step == s {
			return i
		}
	}
	panic("scan: unknown step " + string(s))
}
