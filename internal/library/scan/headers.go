package scan

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/loomarr/loomarr/internal/inventory"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// headerBatch is how many read results are saved per write.
const headerBatch = 50

// readHeaders reads the header of each queued file, wanted items first, headerConcurrency at a
// time. Reads wait while live playout runs, since each is a disk seek a viewer's stream would
// otherwise queue behind (decision 11); the kill switch stops them between files.
//
// One stage runs at a time, and it waits for playback without the pass lock, so no pass waits for
// a viewer: a pass that finds a stage running hands its files to it and returns. Each read's write
// takes the pass lock and is dropped when a later import wrote the file after the read was queued.
func (im *Importer) readHeaders(ctx context.Context, queue []pending) error {
	if len(queue) == 0 || im.Header == nil {
		return nil
	}
	if !im.headers.TryLock() {
		im.mu.Lock()
		if im.reading {
			im.handoff = append(im.handoff, queue...)
			im.progress.HeadersAll += len(queue)
			im.mu.Unlock()
			return nil
		}
		im.mu.Unlock()
		im.headers.Lock() // the running stage takes no more files: wait for it to end
	}
	defer im.headers.Unlock()
	im.prioritize(ctx, queue)
	im.setProgress(func(p *Progress) {
		p.Stage, p.HeadersAll, p.HeadersDone = StageReading, len(queue), 0
		im.reading, im.touched = true, map[string]int64{}
	})
	defer im.setProgress(func(p *Progress) {
		im.reading, im.handoff, im.touched = false, nil, nil
		p.Waiting = false
		if p.Stage == StageReading {
			p.Stage = StageIdle
		}
	})

	// staged is every file this stage has queued. A file handed over again is read again only when
	// the import that handed it wrote it, which makes the stage's earlier read of it stale.
	staged := map[string]bool{}
	for _, job := range queue {
		staged[job.file.Path] = true
	}
	next := func() (pending, bool) {
		for len(queue) == 0 {
			im.mu.Lock()
			more := im.handoff
			im.handoff = nil
			if len(more) == 0 {
				im.reading = false
				im.mu.Unlock()
				return pending{}, false
			}
			for _, job := range more {
				if staged[job.file.Path] && im.touched[job.file.Path] != job.gen {
					im.progress.HeadersAll--
					continue
				}
				staged[job.file.Path] = true
				queue = append(queue, job)
			}
			im.mu.Unlock()
			im.prioritize(ctx, queue)
		}
		job := queue[0]
		queue = queue[1:]
		return job, true
	}

	work := make(chan pending)
	var (
		mu    sync.Mutex
		batch []pending
		first error
	)
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
				row, err := im.readOne(ctx, job)
				mu.Lock()
				switch {
				case err != nil && first == nil:
					first = err
				case err == nil:
					job.row = row
					batch = append(batch, job)
					flush(false)
				}
				mu.Unlock()
				im.setProgress(func(p *Progress) { p.HeadersDone++ })
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
		work <- job
	}
	close(work)
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	flush(true)
	if errors.Is(first, errShareFault) {
		// The files read so far are saved; the rest stay pending for the next pass.
		im.log().Warn("library scan: header reads stopped", "err", first)
		return ctx.Err()
	}
	if first != nil {
		return first
	}
	return ctx.Err()
}

// readOne reads one file's header and writes the file's item with it. A file whose header does not
// parse is recorded as failed and not tried again until its bytes change. A file the share could
// not give back (gone, an I/O error, a timeout) stays pending and ends the stage: the share, not
// the file, is the problem, and the next pass tries again.
func (im *Importer) readOne(ctx context.Context, job pending) (store.LibraryFile, error) {
	row := job.row
	if im.stale(job) {
		return row, nil // checked again before the write, which a later import may still beat
	}
	h, err := im.Header(ctx, job.file.Path)
	if err != nil {
		if ctx.Err() != nil {
			return row, ctx.Err()
		}
		if shareFault(err) {
			return row, fmt.Errorf("%w: %w", errShareFault, err)
		}
		im.log().Debug("library scan: header not read", "err", err)
		row.Header = store.LibraryHeaderFailed
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
	snapshot := job.b.snapshot(ctx, job.file, job.id, now)
	snapshot.Sources = []inventory.SourceSnapshot{sourceSnapshot(job.file, &facts, im.reader(), now)}
	itemID, problem, err := im.applyFile(ctx, snapshot)
	if err != nil {
		return row, err
	}
	if problem != "" {
		row.Problem, row.Identity = problem, ""
	} else {
		row.ItemID = itemID
	}
	row.Header = store.LibraryHeaderRead
	return row, nil
}

// saveRead saves the rows the stage read, leaving out any a later import wrote since.
func (im *Importer) saveRead(ctx context.Context, read []pending) error {
	im.run.Lock()
	defer im.run.Unlock()
	rows := make([]store.LibraryFile, 0, len(read))
	for _, job := range read {
		if !im.stale(job) {
			rows = append(rows, job.row)
		}
	}
	if len(rows) == 0 {
		return nil
	}
	return im.Store.SaveLibraryFiles(ctx, rows)
}

// touch notes the files an import wrote while a header stage runs.
func (im *Importer) touch(rows []store.LibraryFile, gen int64) {
	im.mu.Lock()
	defer im.mu.Unlock()
	if im.touched == nil {
		return
	}
	for _, r := range rows {
		im.touched[r.Path] = gen
	}
}

// stale reports whether an import later than the one that queued job wrote its file.
func (im *Importer) stale(job pending) bool {
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.touched[job.file.Path] > job.gen
}

// errShareFault ends a header stage whose share stopped answering.
var errShareFault = errors.New("library scan: the share stopped answering")

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
		im.setProgress(func(p *Progress) { p.Waiting = true })
		t := time.NewTimer(poll)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
	im.setProgress(func(p *Progress) { p.Waiting = false })
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
