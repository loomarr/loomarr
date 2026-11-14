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
// rate baseline and done, since then, etaSamples or etaShare of its total, whichever comes first:
// before that the rate is noise. The count is what makes a rate steady, so a large step (forty
// thousand files read a few a second) tells its time left within a minute or two, not after its
// first 2%; the share lets a small step, done before it reaches the count, tell it too.
const (
	etaAfter   = 30 * time.Second
	etaSamples = 20
	etaShare   = 0.02
)

// etaWindow is how far back the rate is measured once a step has run a while: the baseline slides
// up behind the step, staying between one and two windows old, so the time left follows the rate
// the step goes at now. A first import's reading that crawls for its first minutes and then runs
// would otherwise tell "50 hours left" long after it has sped up.
const etaWindow = 2 * time.Minute

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
	// rates are each step's last measured rate, done a second, kept when the step ends, begins
	// again or pauses: a step that starts over (a second folder added, a new reading stage, going
	// again after a pause) tells its time left by it until it has measured its own, instead of
	// "working out" again. Rebase clears it, as the work ahead goes at another rate.
	rates [len(steps)]float64
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
	// markAt and markDone are the checkpoint the baseline slides up to: taken each etaWindow, the
	// baseline moving to the one before.
	markAt   time.Time
	markDone int
}

// rebase takes the rate baseline, and the first checkpoint, at now.
func (s *stepState) rebase(now time.Time) {
	s.since, s.sinceDone = now, s.done
	s.markAt, s.markDone = now, s.done
}

// slide takes a checkpoint each etaWindow, moving the baseline up to the one before, so the rate is
// what the step has done over the last one to two windows.
func (s *stepState) slide(now time.Time) {
	if now.Sub(s.markAt) < etaWindow {
		return
	}
	if s.markAt.After(s.since) {
		s.since, s.sinceDone = s.markAt, s.markDone
	}
	s.markAt, s.markDone = now, s.done
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
		now := t.now()
		t.keepRate(step, now)
		t.state[step.index()] = stepState{active: true, total: max(total, 0)}
		t.state[step.index()].rebase(now)
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
			if !s.paused {
				s.slide(t.now())
			}
		}
	})
}

// Rebase restarts a running step's rate baseline at what is done now. The work ahead goes at a
// different rate from the work behind, as when an import turns from listing folders to writing
// what it found, so the time left is worked out from here.
func (t *Tracker) Rebase(step Step) {
	t.update(false, func() {
		if s := &t.state[step.index()]; s.active {
			s.rebase(t.now())
			t.rates[step.index()] = 0
		}
	})
}

// keepRate keeps step's measured rate (rates) before its baseline is lost; one not measured yet
// leaves the last kept. Called under the lock.
func (t *Tracker) keepRate(step Step, now time.Time) {
	if r, ok := t.state[step.index()].rate(now); ok {
		t.rates[step.index()] = r
	}
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
		if paused {
			t.keepRate(step, t.now())
		}
		s.paused = paused
		if !paused {
			s.rebase(t.now())
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
		t.keepRate(step, t.now())
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
		} else if out.ETA, out.HasETA = s.eta(now); !out.HasETA && t.rates[i] > 0 && s.total > 0 {
			// Not measured yet: the step's last rate tells it meanwhile.
			out.ETA, out.HasETA = time.Duration(float64(max(s.total-s.done, 0))/t.rates[i]*float64(time.Second)), true
		}
		out.Quiet = steps[i] == StepWalking && !t.asked &&
			!t.state[StepReading.index()].active && !t.state[StepLookingUp.index()].active
		return out
	}
	return ProgressSnapshot{}
}

// rate is what the step has done a second since its rate baseline, once it has run etaAfter and
// done something.
func (s stepState) rate(now time.Time) (float64, bool) {
	elapsed, since := now.Sub(s.since), s.done-s.sinceDone
	if !s.active || s.paused || elapsed < etaAfter || since <= 0 {
		return 0, false
	}
	return float64(since) / elapsed.Seconds(), true
}

// eta is remaining × (elapsed ÷ done) since the rate baseline, once the step has run etaAfter
// and done etaSamples or etaShare of its total since then.
func (s stepState) eta(now time.Time) (time.Duration, bool) {
	elapsed, since := now.Sub(s.since), s.done-s.sinceDone
	if elapsed < etaAfter || since <= 0 || s.total <= 0 ||
		(since < etaSamples && float64(since) < etaShare*float64(s.total)) {
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
