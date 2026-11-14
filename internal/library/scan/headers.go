package scan

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/loomarr/loomarr/internal/library/inventory"
	"github.com/loomarr/loomarr/internal/media/header"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// headerBatch is how many read results are saved per write.
const headerBatch = 50

// headerStall is how long one header read may take before the stage gives up on it. A read is one
// seek or one ffprobe run, seconds at the worst; a mount whose server went away can block it for
// good.
const headerStall = 2 * time.Minute

// headerFaultLimit is how many files in a row the share may fail to give back before the stage
// stops reading its folder. One unreadable sector is that file's problem; a run of them with no
// read between is the share's. A file gone between the walk and its read, from a folder still
// there, is neither: an arr renamed it, and the next walk finds it by its new name.
const headerFaultLimit = 2 * headerConcurrency

// readHeaders reads the header of each queued file, wanted items first, headerConcurrency at a
// time. Reads wait while live playout runs, since each is a disk seek a viewer's stream would
// otherwise queue behind (decision 11); the kill switch stops them between files.
//
// One stage takes files at a time, and it waits for playback without the pass lock, so no other
// pass waits for a viewer: a pass that finds a stage taking files hands its files to it and
// returns, and one that finds none becomes the stage. Each read's write takes the pass lock and is
// dropped when a later import wrote the file after the read was queued.
//
// A share fault stops reading one folder, never the stage: a read given up on sets its folder's
// files aside, and the other folders read on. Once nothing else is left, the stage waits up to
// the stall for the folder's read to return, then takes them up again. complete reports that the
// stage read every file it was given, or handed them to the stage reading.
func (im *Importer) readHeaders(ctx context.Context, queue []pending) (complete bool, err error) {
	if len(queue) == 0 || im.Header == nil {
		return true, nil
	}
	im.mu.Lock()
	if im.reading {
		// A file the stage already holds is handed over again only when the import handing it wrote
		// it, which makes the stage's earlier read of it stale. Every pass queues each file still
		// pending, so without this a long stage would count, and hold, the library once a pass.
		kept := 0
		for _, job := range queue {
			if im.staged[job.file.Path] && im.touched[job.file.Path] != job.gen {
				continue
			}
			im.staged[job.file.Path] = true
			im.handoff = append(im.handoff, job)
			kept++
		}
		// Counted under the pass lock, so the stage's Begin is never after a hand-off's Grow.
		im.Tracker.Grow(StepReading, kept)
		im.mu.Unlock()
		return true, nil
	}
	im.reading = true
	im.staged = make(map[string]bool, len(queue))
	for _, job := range queue {
		im.staged[job.file.Path] = true
	}
	im.Tracker.Begin(StepReading, len(queue))
	im.mu.Unlock()
	// released is set when this stage stops taking files, after which the next pass may become a
	// stage while this one's last reads finish.
	released := false
	defer func() {
		im.mu.Lock()
		if !released {
			// Ended early (a share fault, the kill switch, an error): the files handed to it stay
			// pending, and the next walk over them queues them again.
			im.reading, im.handoff, im.staged = false, nil, nil
		}
		ended := !im.reading
		im.mu.Unlock()
		if ended {
			im.Tracker.End(StepReading)
			if im.AfterReading != nil {
				im.AfterReading()
			}
		}
	}()
	im.prioritize(ctx, queue)

	work := make(chan pending)
	var (
		mu    sync.Mutex
		batch []pending
		first error
		// faults counts the files in a row the share could not give back.
		faults int
		// faulted are the folders whose share gave back headerFaultLimit files in a row with no
		// read between: the stage reads no more of them.
		faulted = map[string]bool{}
		// aside are files set aside under a folder with a read given up on, and retried the ones
		// given up on themselves once already: a second time, they are left.
		aside   []pending
		retried = map[string]bool{}
		// left counts the files the stage leaves pending in a folder it stopped reading, for the
		// next walk to queue again. One file the share could not give back is that file's own.
		left int
		// inflight counts the files handed to readers and not done; idle signals one done.
		inflight int
		rounds   int
	)
	idle := sync.NewCond(&mu)
	next := func() (pending, bool) {
		for len(queue) == 0 {
			im.mu.Lock()
			more := im.handoff
			im.handoff = nil
			im.mu.Unlock()
			if len(more) > 0 {
				queue = append(queue, more...)
				im.prioritize(ctx, queue)
				continue
			}
			// A reader still holding a file can yet set it aside.
			mu.Lock()
			for inflight > 0 {
				idle.Wait()
			}
			set := aside
			aside = nil
			mu.Unlock()
			if len(set) > 0 && rounds < headerRounds && im.awaitRoots(ctx, set) {
				rounds++
				queue = set
				im.Tracker.Grow(StepReading, len(set))
				continue
			}
			im.mu.Lock()
			if len(im.handoff) > 0 {
				im.mu.Unlock()
				mu.Lock()
				aside = append(aside, set...)
				mu.Unlock()
				continue
			}
			im.reading, im.staged, released = false, nil, true
			im.mu.Unlock()
			mu.Lock()
			left += len(set)
			mu.Unlock()
			return pending{}, false
		}
		job := queue[0]
		queue = queue[1:]
		return job, true
	}
	flush := func(force bool) {
		if len(batch) == 0 || (!force && len(batch) < headerBatch) {
			return
		}
		if err := im.saveRead(ctx, batch); err != nil && first == nil {
			first = err
		}
		batch = batch[:0]
	}
	var wg sync.WaitGroup
	for range headerConcurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range work {
				im.Tracker.Current(StepReading, filepath.Base(job.file.Path))
				row, err := im.readOne(ctx, job)
				mu.Lock()
				switch {
				case errors.Is(err, errFileGone):
					// Renamed or deleted since the walk: the next walk says which.
				case errors.Is(err, errShareFault):
					// Given up on: its folder's files wait until the read returns.
					if retried[job.file.Path] {
						left++
					} else {
						retried[job.file.Path] = true
						aside = append(aside, job)
					}
				case errors.Is(err, errFileFault):
					// The file stays pending, and the stage goes on to the next.
					faults++
					if faults >= headerFaultLimit && !faulted[job.root()] {
						faulted[job.root()] = true
						im.log().Warn("library scan: header reads stopped for a folder", "root", job.root(),
							"err", fmt.Errorf("%w: %d files in a row, the last: %w", errShareFault, faults, err))
					}
				case err != nil:
					if first == nil {
						first = err
					}
				default:
					faults = 0
					job.row = row
					batch = append(batch, job)
					flush(false)
				}
				inflight--
				idle.Broadcast()
				mu.Unlock()
				im.Tracker.Advance(StepReading, 1)
			}
		}()
	}
	failed := func() bool {
		mu.Lock()
		defer mu.Unlock()
		return first != nil
	}
	for {
		job, ok := next()
		if !ok {
			break
		}
		if err := im.waitForHeadroom(ctx); err != nil {
			break
		}
		if (im.Enabled != nil && !im.Enabled()) || failed() {
			break
		}
		mu.Lock()
		switch {
		case faulted[job.root()]:
			left++
			im.Tracker.Grow(StepReading, -1)
		case im.stalled(job.root()):
			// A read under this root was given up on and has still not returned: the file waits
			// rather than blocking another reader on the same mount.
			aside = append(aside, job)
			im.Tracker.Grow(StepReading, -1)
		default:
			inflight++
			mu.Unlock()
			work <- job
			continue
		}
		mu.Unlock()
	}
	close(work)
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	flush(true)
	if first != nil {
		return false, first
	}
	return released && left == 0 && len(aside) == 0 && ctx.Err() == nil, ctx.Err()
}

// headerRounds is how many times a stage takes up again the files it set aside under a folder
// whose read was given up on.
const headerRounds = 3

// awaitRoots waits, up to the stall, for every read given up on under the folders of jobs to
// return. It reports false when one is still out, or ctx ends first.
func (im *Importer) awaitRoots(ctx context.Context, jobs []pending) bool {
	timer := time.NewTimer(orDuration(im.HeaderStall, headerStall))
	defer timer.Stop()
	for _, job := range jobs {
		im.mu.Lock()
		done, ok := im.stalledWalks[job.root()]
		im.mu.Unlock()
		if !ok {
			continue
		}
		select {
		case <-done:
		case <-ctx.Done():
			return false
		case <-timer.C:
			return false
		}
	}
	return true
}

// readOne reads one file's header and writes the file's item with it. A file whose header does not
// parse is recorded as failed and not tried again until its bytes change. A file the share could
// not give back (an I/O error, a timeout, gone with its folder) stays pending for the next pass and
// returns errFileFault: the stage goes on, and stops reading its folder only after
// headerFaultLimit of them in a row. One gone from a folder still there returns errFileGone. A read
// that does not return within the stall returns errShareFault, which sets its folder's files aside.
func (im *Importer) readOne(ctx context.Context, job pending) (librarystore.LibraryFile, error) {
	row := job.row
	if im.stale(job) {
		return row, nil // checked again before the write, which a later import may still beat
	}
	h, err := im.readHeader(ctx, job)
	if err != nil {
		if ctx.Err() != nil {
			return row, ctx.Err()
		}
		if errors.Is(err, errShareFault) {
			return row, err
		}
		if errors.Is(err, fs.ErrNotExist) && folderAnswers(filepath.Dir(job.file.Path)) {
			return row, fmt.Errorf("%w: %w", errFileGone, err)
		}
		if shareFault(err) {
			return row, fmt.Errorf("%w: %w", errFileFault, err)
		}
		im.log().Info("library scan: header not read", "path", job.file.Path, "err", err)
		row.Header = librarystore.LibraryHeaderFailed
		return row, nil
	}
	im.run.Lock()
	defer im.run.Unlock()
	if im.stale(job) {
		// A later import wrote the file: its own read, if it needs one, is queued.
		return row, nil
	}
	facts := h.SourceFacts()
	now := im.now()
	// The item's observation keeps the pass's time, not the moment this worker finished: several
	// workers read at once, and the item takes its name from its newest present origin, so a
	// per-read time would let whichever copy's header was read last (an extra copy included) name
	// the title. Origins of one pass tie, and the tie breaks by key, so the pick is stable.
	seen := job.row.SeenAt
	if seen.IsZero() {
		seen = now
	}
	snapshot, waiting := job.b.snapshot(ctx, job.file, job.id, seen)
	snapshot.Sources = []inventory.SourceSnapshot{sourceSnapshot(job.file, &facts, im.reader(), now)}
	itemID, problem, err := im.applyFile(ctx, snapshot)
	if err != nil {
		return row, err
	}
	switch {
	case problem != "":
		row.Problem, row.Identity = problem, ""
	case waiting:
		// As the walk saves it: a record still to come is written on a later pass (#3565).
		row.ItemID, row.Identity = itemID, ""
	default:
		row.ItemID = itemID
	}
	row.Header = librarystore.LibraryHeaderRead
	return row, nil
}

// saveRead saves the rows the stage read, leaving out any a later import wrote since.
func (im *Importer) saveRead(ctx context.Context, read []pending) error {
	im.run.Lock()
	defer im.run.Unlock()
	rows := make([]librarystore.LibraryFile, 0, len(read))
	for _, job := range read {
		if !im.stale(job) {
			rows = append(rows, job.row)
		}
	}
	if len(rows) == 0 {
		return nil
	}
	if err := im.Store.SaveLibraryFiles(ctx, rows); err != nil {
		return err
	}
	// The files read are what makes their titles ready.
	im.titlesChanged()
	return nil
}

// touch notes the files an import wrote. It is kept for every import, not only while a stage runs:
// a pass's reads are queued before it reaches the stage, and an import between the two still
// makes them stale.
func (im *Importer) touch(rows []librarystore.LibraryFile, gen int64) {
	im.mu.Lock()
	defer im.mu.Unlock()
	if im.touched == nil {
		im.touched = map[string]int64{}
	}
	for _, r := range rows {
		im.touched[r.Path] = gen
	}
}

// Reading reports whether a header stage is taking files.
func (im *Importer) Reading() bool {
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.reading
}

// stale reports whether an import later than the one that queued job wrote its file, or forgot its
// root.
func (im *Importer) stale(job pending) bool {
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.touched[job.file.Path] > job.gen || im.forgotten[job.root()] > job.gen
}

// errShareFault ends a header stage whose share stopped answering.
var errShareFault = errors.New("library scan: the share stopped answering")

// errFileFault is one file the share could not give back. The file stays pending.
var errFileFault = errors.New("library scan: the share could not give the file back")

// errFileGone is a file gone from a folder the share still lists: an arr renamed or deleted it
// since the walk. It says nothing of the share.
var errFileGone = errors.New("library scan: the file is gone from its folder")

// folderAnswers reports whether the share lists dir with something in it. A share that went away
// lists its folders empty, or not at all.
func folderAnswers(dir string) bool {
	f, err := os.Open(dir)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	names, err := f.Readdirnames(1)
	return err == nil && len(names) > 0
}

// readHeader reads job's header, giving up once the read has taken the stall. A read blocked on a
// dead mount cannot be cancelled, so one given up on is left to end on its own, and its root reads
// as stalled until it does: nothing more under it is read or walked meanwhile.
func (im *Importer) readHeader(ctx context.Context, job pending) (header.Header, error) {
	type result struct {
		h   header.Header
		err error
	}
	out := make(chan result, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		h, err := im.Header(ctx, job.file.Path)
		out <- result{h, err}
	}()
	stall := time.NewTimer(orDuration(im.HeaderStall, headerStall))
	defer stall.Stop()
	select {
	case r := <-out:
		return r.h, r.err
	case <-ctx.Done():
		return header.Header{}, ctx.Err()
	case <-stall.C:
		im.markStalled(job.root(), done)
		return header.Header{}, fmt.Errorf("%w: no answer reading %s", errShareFault, job.file.Path)
	}
}

// root is the media folder the file was found under; "" when it is not known.
func (job pending) root() string {
	if job.b == nil {
		return ""
	}
	return job.b.root
}

// shareFault reports whether a header read failed because of the share rather than the file.
func shareFault(err error) bool {
	var timeout interface{ Timeout() bool }
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.EIO) || errors.Is(err, syscall.ESTALE) ||
		errors.Is(err, syscall.EHOSTDOWN) || errors.Is(err, os.ErrDeadlineExceeded) ||
		(errors.As(err, &timeout) && timeout.Timeout())
}

// waitForHeadroom returns once no playout session is live.
func (im *Importer) waitForHeadroom(ctx context.Context) error {
	if im.Busy == nil {
		return ctx.Err()
	}
	poll := im.Poll
	if poll <= 0 {
		poll = 5 * time.Second
	}
	for im.Busy() {
		im.Tracker.Pause(StepReading, true)
		t := time.NewTimer(poll)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
	im.Tracker.Pause(StepReading, false)
	return ctx.Err()
}

// prioritize moves the files of items a channel or proposal asked for to the front.
func (im *Importer) prioritize(ctx context.Context, queue []pending) {
	if im.Wanted == nil {
		return
	}
	wanted := map[inventory.ExternalID]bool{}
	for _, id := range im.Wanted(ctx) {
		wanted[id] = true
	}
	if len(wanted) == 0 {
		return
	}
	// A file is wanted when its item is, and an episode or extra when its series or parent is.
	isWanted := func(p pending) bool {
		for _, id := range p.id.cheapIDs() {
			if wanted[id] {
				return true
			}
		}
		return false
	}
	sort.SliceStable(queue, func(i, j int) bool { return isWanted(queue[i]) && !isWanted(queue[j]) })
}
