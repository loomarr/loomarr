package scan

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/inventory"
	"github.com/loomarr/loomarr/internal/store"
)

// headerBatch is how many read results are saved per write.
const headerBatch = 50

// readHeaders reads the header of each queued file, wanted items first, headerConcurrency at a
// time. Reads wait while live playout runs, since each is a disk seek a viewer's stream would
// otherwise queue behind (decision 11); the kill switch stops them between files.
func (im *Importer) readHeaders(ctx context.Context, queue []pending) error {
	if len(queue) == 0 || im.Header == nil {
		return nil
	}
	im.prioritize(ctx, queue)
	im.setProgress(func(p *Progress) { p.Stage, p.HeadersAll, p.HeadersDone = StageReading, len(queue), 0 })

	work := make(chan pending)
	var (
		mu    sync.Mutex
		batch []store.LibraryFile
		first error
	)
	flush := func(force bool) {
		if len(batch) == 0 || (!force && len(batch) < headerBatch) {
			return
		}
		if err := im.Store.SaveLibraryFiles(ctx, batch); err != nil && first == nil {
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
					batch = append(batch, row)
					flush(false)
				}
				mu.Unlock()
				im.setProgress(func(p *Progress) { p.HeadersDone++ })
			}
		}()
	}
	for _, job := range queue {
		if err := im.waitForHeadroom(ctx); err != nil {
			break
		}
		if im.Enabled != nil && !im.Enabled() {
			break
		}
		work <- job
	}
	close(work)
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	flush(true)
	if first != nil {
		return first
	}
	return ctx.Err()
}

// readOne reads one file's header and writes the file's item with it. A file that cannot be read
// is recorded as failed and not tried again until its bytes change.
func (im *Importer) readOne(ctx context.Context, job pending) (store.LibraryFile, error) {
	row := job.row
	h, err := im.Header(ctx, job.file.Path)
	if err != nil {
		if ctx.Err() != nil {
			return row, ctx.Err()
		}
		im.log().Debug("library scan: header not read", "err", err)
		row.Header = store.LibraryHeaderFailed
		return row, nil
	}
	facts := h.SourceFacts()
	snapshot := job.snapshot
	snapshot.Sources = []inventory.SourceSnapshot{sourceSnapshot(job.file, &facts, im.now())}
	if err := im.apply(ctx, snapshot); err != nil {
		if !errors.Is(err, inventory.ErrIdentityConflict) {
			return row, err
		}
		row.Problem = ProblemIdentityConflict
	}
	row.Header = store.LibraryHeaderRead
	return row, nil
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
	isWanted := func(p pending) bool {
		for _, id := range p.snapshot.ExternalIDs {
			if wanted[id] {
				return true
			}
		}
		// An episode is wanted when its series is.
		for _, rel := range p.snapshot.Observation.Facts.Relationships {
			if ns, value, ok := strings.Cut(rel.ExternalID, ":"); ok && wanted[inventory.ExternalID{Namespace: ns, Value: value}] {
				return true
			}
		}
		return false
	}
	sort.SliceStable(queue, func(i, j int) bool { return isWanted(queue[i]) && !isWanted(queue[j]) })
}
