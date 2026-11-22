package images

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/activitystore"
)

// The image garbage collector (§22, §18.1 — V52 phase 3b). Four duties, deliberately in one job
// because they are all "walk the store and reconcile it with the disk", and three of them are
// cheap enough that splitting them would cost more in Tasks-page rows than it buys.
//
// ⚠ **They are not equally important, and the ordering here reflects that.** Eviction is the one
// that sounds like the point and is the LEAST significant: at the documented envelope (≤50
// channels, ≤10k clips) the budget is a backstop rather than a routine event, and every derivative
// is regenerable by definition, so a wrong eviction costs one re-encode and never data. The TMDB
// TTL is the one that genuinely must not be skipped, because it is a compliance ceiling rather
// than a tuning knob.

// gcBatch bounds each read of a sweep's work list. The GC is daily and its passes are idempotent,
// so an orphan backlog larger than this drains over successive days; the expiry sweep reads batch
// after batch until nothing due is left (enforceTTL).
const gcBatch = 500

// GCStore is the persistence the collector needs on top of the serve path's.
type GCStore interface {
	Store
	// ListExpiredBefore pages through remote images fetched before cutoff in (fetch time, hash)
	// order, starting after the cursor.
	ListExpiredBefore(ctx context.Context, cutoff time.Time, after ExpiryCursor, limit int) ([]Image, error)
	ListOrphans(ctx context.Context, limit int) ([]Image, error)
	ListUnrecoverable(ctx context.Context, limit int) ([]Image, error)
	ListColdestDerivatives(ctx context.Context, limit int) ([]Derivative, error)
	TotalDerivativeBytes(ctx context.Context) (int64, error)
	DeleteDerivative(ctx context.Context, hash, recipe string, f Format, width int) error
	DeleteDerivatives(ctx context.Context, hash string) error
	DeleteImage(ctx context.Context, hash string) error
}

// ExpiryCursor is a position in the expiry sweep's order: the fetch time and hash of the last image
// read. The zero cursor is the start.
type ExpiryCursor struct {
	FetchedAt time.Time
	Hash      string
}

// GCResult is one pass's tally. Every field is surfaced in the job's log line; MissingUnrecoverable
// is the one an operator is meant to act on.
type GCResult struct {
	OrphansDeleted int
	Expired        int
	Evicted        int
	EvictedBytes   int64
	// MissingUnrecoverable counts rows whose bytes are gone and cannot be got back — operator
	// uploads, after losing /data/images. ⚠ Counted rather than repaired because there is nothing
	// to repair: §22 accepts this as the cost of not putting image bytes in the database, and the
	// obligation that comes with accepting it is to make the loss VISIBLE rather than let it
	// render as a broken image, which reads as a bug.
	MissingUnrecoverable int
}

// Notifier receives the GC's one operator-facing finding. Satisfied by the activity recorder at
// wiring; nil is fine and means the count lands only in the log.
type Notifier interface {
	Warn(ctx context.Context, kind, subjectID, text string)
}

// GC collects unreferenced images, expired remote artwork and over-budget derivatives.
type GC struct {
	svc    *Service
	store  GCStore
	log    *slog.Logger
	notify Notifier

	// remoteTTL is the application-owned TMDB compliance ceiling.
	remoteTTL func() time.Duration
	// budgetMB is `images.cache_budget_mb`.
	budgetMB func() int
}

// NewGC builds the collector.
func NewGC(svc *Service, st GCStore, remoteTTL func() time.Duration, budgetMB func() int, notify Notifier, log *slog.Logger) *GC {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &GC{svc: svc, store: st, log: log, notify: notify, remoteTTL: remoteTTL, budgetMB: budgetMB}
}

// Run performs one pass.
//
// ⚠ Each duty's failure is logged and the pass continues. They are independent reconciliations,
// and letting a failed orphan delete skip the TTL sweep would turn a transient disk error into a
// compliance lapse. A pass whose context ends (the job's timeout, or shutdown) is the exception:
// it stops between images, skips the duties it has not reached, and returns an error with what it
// got done, so the job is recorded as cut short rather than as a clean pass.
func (g *GC) Run(ctx context.Context) (GCResult, error) {
	var out GCResult
	// ⚠ **Orphans before the TTL, and the order is load-bearing rather than stylistic.** Both
	// sweeps can select the same row — an expired image nothing references any more — and running
	// the TTL first purges its bytes and puts it back on the FETCH queue moments before the orphan
	// sweep deletes it. The outcome is the same, but between the two steps the row is a download
	// instruction for an image no surface will ever show, and if the delete fails for any reason
	// that instruction is what survives. Collecting first means the TTL only ever looks at images
	// something is still using.
	duties := []struct {
		name string
		run  func(context.Context, *GCResult) error
	}{
		{"orphan sweep", g.deleteOrphans},
		{"TTL sweep", g.enforceTTL},
		{"missing-file check", g.warnUnrecoverable},
		{"cache eviction", g.evict},
	}
	for _, d := range duties {
		err := d.run(ctx, &out)
		if ctxErr := ctx.Err(); ctxErr != nil {
			g.log.Warn("image gc pass cut short", "during", d.name, "expired", out.Expired, "err", ctxErr)
			return out, fmt.Errorf("images: gc pass cut short during the %s after %d expired: %w", d.name, out.Expired, ctxErr)
		}
		if err != nil {
			g.log.Warn("image "+d.name+" failed", "err", err)
		}
	}

	g.log.Info("image gc pass",
		"orphans_deleted", out.OrphansDeleted, "expired", out.Expired,
		"evicted", out.Evicted, "evicted_bytes", out.EvictedBytes,
		"missing_unrecoverable", out.MissingUnrecoverable)
	return out, nil
}

// enforceTTL implements the six-month TMDB ceiling (§22).
//
// ⚠ **The bytes are DELETED here and re-fetched afterwards, not refreshed in place.** The ceiling
// is a term of TMDB's API licence rather than a cache heuristic, so the only implementation that
// actually honours it is one where no cached byte outlives the TTL — which means the delete cannot
// be conditional on a successful re-download. Clearing `origin_fetched_at` puts the row straight
// back on the fetch job's queue, behind what is already waiting, and that job takes a batch every
// minute, so the operator-visible cost is a placeholder until its turn on the anniversary of an
// import.
//
// The rejected alternative is worth naming because it is the one that looks obviously nicer:
// re-fetching in place and deleting only on failure leaves the compliance question entirely inside
// an error branch — TMDB being unreachable for a day would silently keep serving expired bytes,
// and the ceiling would be enforced by nothing.
func (g *GC) enforceTTL(ctx context.Context, out *GCResult) error {
	ttl := g.remoteTTL()
	if ttl <= 0 {
		return nil
	}
	cutoff := g.svc.now().Add(-ttl)
	// Every image due is purged in this run, a batch of gcBatch at a time: thousands fetched on one
	// day all expire on one day, and a batch a day would leave the last of them months past the
	// ceiling. One image is purged at a time (its files, then its row), so the run's load on the
	// disk and the database is one delete and one write after another, never a burst. The reads
	// page by a cursor in (fetch time, hash) order, so an image whose purge failed is passed over
	// for this run and tried tomorrow, never read again or allowed to hold back the ones behind
	// it. A short page is the end of what is due.
	var after ExpiryCursor
	for {
		expired, err := g.store.ListExpiredBefore(ctx, cutoff, after, gcBatch)
		if err != nil {
			return err
		}
		for _, img := range expired {
			if err := ctx.Err(); err != nil {
				return err
			}
			after = ExpiryCursor{FetchedAt: img.OriginFetchedAt, Hash: img.Hash}
			if err := g.purgeBytes(ctx, img); err != nil {
				g.log.Warn("expired image purge failed", "hash", img.Hash, "err", err)
				continue
			}
			// ⚠ Zero is the fetch job's "never fetched" sentinel, so this is a requeue rather than a
			// flag: the row rejoins ListAwaitingFetch by construction and no second queue exists to
			// keep in step with this one. It joins at the back, behind what is already waiting.
			now := g.svc.now()
			img.OriginFetchedAt = time.Time{}
			img.FetchQueuedAt = now
			img.UpdatedAt = now
			if err := g.store.PutImage(ctx, img); err != nil {
				return err
			}
			out.Expired++
		}
		if len(expired) < gcBatch {
			return nil
		}
	}
}

// deleteOrphans removes images nothing references any more.
func (g *GC) deleteOrphans(ctx context.Context, out *GCResult) error {
	orphans, err := g.store.ListOrphans(ctx, gcBatch)
	if err != nil {
		return err
	}
	for _, img := range orphans {
		if err := g.purgeBytes(ctx, img); err != nil {
			g.log.Warn("orphan image purge failed", "hash", img.Hash, "err", err)
			continue
		}
		// ⚠ Files first, row second. The row is the only record of where the files are — deleting
		// it first would strand the bytes on disk with nothing that could ever find them again,
		// which is how a content-addressed store silently fills a volume.
		if err := g.store.DeleteImage(ctx, img.Hash); err != nil {
			return err
		}
		out.OrphansDeleted++
	}
	return nil
}

// warnUnrecoverable counts rows whose bytes are gone for good and tells the operator once.
func (g *GC) warnUnrecoverable(ctx context.Context, out *GCResult) error {
	rows, err := g.store.ListUnrecoverable(ctx, gcBatch)
	if err != nil {
		return err
	}
	for _, img := range rows {
		path, err := g.svc.blob.OriginalPath(img.Hash, extForMIME(img.MIME))
		if err != nil {
			continue
		}
		if _, ok := g.svc.blob.Stat(path); !ok {
			out.MissingUnrecoverable++
		}
	}
	if out.MissingUnrecoverable > 0 && g.notify != nil {
		// One notification for the whole sweep, not one per image. An operator who restored a
		// database onto an empty image directory has hundreds of these, and hundreds of rows in
		// the activity feed would bury the single fact they need: uploads did not survive, and
		// /data is what to back up.
		g.notify.Warn(ctx, activitystore.ActivityKindSystem, "", pluralUnrecoverable(out.MissingUnrecoverable))
	}
	return nil
}

// evict drops the coldest derivatives until the cache is under budget.
//
// ⚠ **Image-level LRU, ordered by the parent row's last_used_at.** Per-derivative LRU is the
// obvious refinement and is rejected on cost: `image_derivatives` has no last_used_at, and adding
// one would make every image request a row WRITE — a fifty-poster grid becomes fifty writes per
// page load against SQLite with a single writer. Image-level ordering costs one write per image
// request, which the serve path already pays through TouchImage.
func (g *GC) evict(ctx context.Context, out *GCResult) error {
	budget := int64(g.budgetMB()) << 20
	if budget <= 0 {
		return nil
	}
	total, err := g.store.TotalDerivativeBytes(ctx)
	if err != nil {
		return err
	}
	if total <= budget {
		return nil
	}

	coldest, err := g.store.ListColdestDerivatives(ctx, gcBatch)
	if err != nil {
		return err
	}
	for _, d := range coldest {
		if total <= budget {
			break
		}
		if err := g.svc.blob.Remove(d.Path); err != nil {
			g.log.Warn("evicting a derivative failed", "path", d.Path, "err", err)
			continue
		}
		if err := g.store.DeleteDerivative(ctx, d.ImageHash, d.Recipe, d.Format, d.Width); err != nil {
			return err
		}
		total -= d.Bytes
		out.Evicted++
		out.EvictedBytes += d.Bytes
	}
	if total > budget {
		// Said out loud rather than left to be inferred from an unchanged disk usage: one pass
		// evicts at most gcBatch rungs, so a store far over budget converges over several days.
		g.log.Info("image cache still over budget after a full batch",
			"bytes", total, "budget", budget, "evicted_this_pass", out.Evicted)
	}
	return nil
}

// purgeBytes removes an image's original and every derivative from disk, and its derivative rows.
//
// Shared by the TTL sweep and the orphan sweep because they want exactly the same thing done to
// the files; only what happens to the ROW afterwards differs (requeued vs deleted).
func (g *GC) purgeBytes(ctx context.Context, img Image) error {
	if orig, err := g.svc.blob.OriginalPath(img.Hash, extForMIME(img.MIME)); err == nil {
		if err := g.svc.blob.Remove(orig); err != nil {
			return err
		}
	}
	if err := g.svc.blob.RemoveAllFor(img.Hash); err != nil {
		return err
	}
	return g.store.DeleteDerivatives(ctx, img.Hash)
}

// pluralUnrecoverable writes the operator-facing warning.
//
// ⚠ It names the REMEDY, not the symptom. "3 images are missing" tells someone nothing they can
// act on; the actionable fact is that the application backup is a database backup and /data is the
// volume to back up, which is the sentence §22 requires to appear in the help docs and here.
func pluralUnrecoverable(n int) string {
	noun := "images"
	if n == 1 {
		noun = "image"
	}
	return strconv.Itoa(n) + " uploaded " + noun + " can no longer be shown: the files are missing " +
		"from the image folder and there is nowhere to get them back from. Loomarr's backup covers " +
		"the database only — back up the /data volume to protect uploads."
}
