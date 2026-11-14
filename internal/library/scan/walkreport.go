package scan

import (
	"context"
	"os"
	"strings"
	"sync"
)

// walkReport tells the Tracker how one root goes through step 1 of an import: the folder it is on,
// named from the root's own folder name down ("TV / Night Ferry / Season 2"), and how many of the
// root's top-level entries are finished. An entry is finished in two halves, once the walk has
// listed it and once the import has written its files, so the count moves through both: over a
// share either can take minutes. The walk runs on its own goroutine, which a walk given up on
// outlives, so it is locked, and close ends it.
type walkReport struct {
	tracker *Tracker
	// name is the root's folder name; entries is how many top-level entries the import counted.
	name    string
	entries int

	mu sync.Mutex
	// walked and written are how many entries the walk has listed and the import has written.
	walked, written int
	// reported is how many entries the Tracker was told are finished: half of walked and written.
	reported int
	closed   bool
}

// listing names the folder the walk lists, by its path relative to the root.
func (r *walkReport) listing(rel string) {
	if r == nil {
		return
	}
	current := r.name
	if rel != "." {
		current += " / " + strings.ReplaceAll(rel, "/", " / ")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.closed {
		r.tracker.Current(StepWalking, current)
	}
}

// relDir is the folder of a path relative to the root, "." for a file in the root itself.
func relDir(rel string) string {
	if i := strings.LastIndexByte(rel, '/'); i >= 0 {
		return rel[:i]
	}
	return "."
}

// counted is the root's top-level entries as the walk lists them. The count made before the walk
// can come back empty (a listing late past readableTimeout) or be stale (an entry added or removed
// since), so step 1's total takes the walk's count: a root counted as nothing would otherwise read
// full while every one of its files is still to come.
func (r *walkReport) counted(n int) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || n == r.entries {
		return
	}
	r.tracker.Grow(StepWalking, n-r.entries)
	r.entries = n
}

// listed reports n of the root's top-level entries listed by the walk.
func (r *walkReport) listed(n int) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.walked = max(r.walked, min(n, r.entries))
	r.tell()
}

// writing begins the import's writes: the walk is over, and what is left goes at the writes' rate,
// so the time left is worked out from here.
func (r *walkReport) writing() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.closed {
		r.walked = r.entries
		r.tell()
		r.tracker.Rebase(StepWalking)
	}
}

// wrote reports the files of n of the root's top-level entries written.
func (r *walkReport) wrote(n int) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.written = max(r.written, min(n, r.entries))
	r.tell()
}

// tell reports to the Tracker the entries finished since it was last told: half of those listed
// and those written, never more than were counted. Called with mu held.
func (r *walkReport) tell() {
	if r.closed {
		return
	}
	if n := (r.walked + r.written) / 2; n > r.reported {
		r.tracker.Advance(StepWalking, n-r.reported)
		r.reported = n
	}
}

// close finishes every counted entry, however the root's pass ended, and stops hearing it.
func (r *walkReport) close() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.walked, r.written = r.entries, r.entries
	r.tell()
	r.closed = true
}

// topEntries counts root's top-level entries within readableTimeout; 0 when it cannot be listed
// in time. A mount whose server went away can block a listing for good, so one that is late is
// left to end on its own.
func topEntries(ctx context.Context, root string) int {
	ctx, cancel := context.WithTimeout(ctx, readableTimeout)
	defer cancel()
	n := make(chan int, 1)
	go func() {
		entries, _ := os.ReadDir(root)
		n <- len(entries)
	}()
	select {
	case c := <-n:
		return c
	case <-ctx.Done():
		return 0
	}
}
