// Package upkeep is the standing job that lets filler go of what it no longer needs, so storage
// never fills with work that already failed (#3855, step 1 of #3838). Each pass:
//
//  1. empties quarantine batches that have waited a day;
//  2. lets go of every clip Loomarr fetched and rejected at least a week ago: the clip leaves the
//     catalog for good, its pipeline row keeps the reason, and its files go to quarantine;
//  3. removes leftover download staging whose run has ended.
//
// A clip added by hand (no acquisition) or pinned by a channel is never let go.
package upkeep

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/loomarr/loomarr/internal/filler/acquisition"
	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/filler/identity"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/media/tools"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// RejectRest is how long a rejected clip Loomarr fetched keeps its files (decision 12 of #3838).
const RejectRest = 7 * 24 * time.Hour

// Store is what a pass reads and writes.
type Store interface {
	// ListClipPipelines reads pipeline rows; a pass asks for the rejected ones.
	ListClipPipelines(ctx context.Context, f pipelinestate.PipelineFilter) ([]pipelinestate.ClipPipeline, error)
	// RetireRejectedClip reaps a fetched, unpinned clip rejected no later than rejectedBy and marks
	// its row's files deleted; false when it no longer qualifies.
	RetireRejectedClip(ctx context.Context, hash string, rejectedBy, at time.Time) (bool, error)
	// GetClip reads one clip, removed and reaped ones included.
	GetClip(ctx context.Context, hash string) (clipstore.Clip, bool, error)
	// ListClipPaths is every clip's path, removed ones included, so an asset another clip's sidecar
	// names is never deleted with this one.
	ListClipPaths(ctx context.Context) ([]string, error)
}

// Bin is the quarantine (quarantine.Bin).
type Bin interface {
	Put(files []string) map[string]error
	Empty() (int, error)
}

// Cleaner removes leftover download staging (fetch.AcquisitionCleaner).
type Cleaner interface {
	Clean(ctx context.Context) (acquisition.StorageCleanupResult, error)
}

// Job is one upkeep pass.
type Job struct {
	store   Store
	bin     Bin
	cleaner Cleaner
	clipDir string
	now     func() time.Time
	log     *slog.Logger
	// disposed remembers rows whose files are already in quarantine, so a pass does not read their
	// sidecars again. It is per process: after a restart each is checked once more.
	disposed map[string]bool
}

// Result counts one pass.
type Result struct {
	Emptied      int   // quarantine batches deleted
	LetGo        int   // rejected clips taken off the catalog this pass
	HeldBack     int   // fetched rejects past their rest that stayed: pinned, or the clip changed within the week
	Quarantined  int   // clips whose files went to quarantine this pass
	Leftovers    int   // leftover staging folders removed
	LeftoverByte int64 // bytes those folders held
}

// New builds the upkeep job.
func New(store Store, bin Bin, cleaner Cleaner, clipDir string, now func() time.Time, log *slog.Logger) *Job {
	return &Job{store: store, bin: bin, cleaner: cleaner, clipDir: clipDir, now: now, log: log, disposed: map[string]bool{}}
}

// Run is one pass. A step that fails is logged and the next still runs, so one stuck file never
// stops the rest; the pass returns the first error.
func (j *Job) Run(ctx context.Context) (Result, error) {
	var res Result
	var errs []error
	n, err := j.bin.Empty()
	res.Emptied = n
	if err != nil {
		j.warn("upkeep: some quarantined files could not be deleted", "err", err)
		errs = append(errs, err)
	}
	if err := j.letGoOfRejects(ctx, &res); err != nil {
		errs = append(errs, err)
	}
	if j.cleaner != nil {
		cleaned, err := j.cleaner.Clean(ctx)
		res.Leftovers, res.LeftoverByte = cleaned.RemovedItems, cleaned.RemovedBytes
		if err != nil {
			j.warn("upkeep: leftover download staging could not be cleaned", "err", err)
			errs = append(errs, err)
		}
	}
	// held_back is what to read when rejects pile up and none go: a clip row something keeps
	// rewriting is never "unchanged for a week".
	if j.log != nil && (res.Emptied > 0 || res.LetGo > 0 || res.HeldBack > 0 || res.Quarantined > 0 || res.Leftovers > 0) {
		j.log.Info("upkeep: filler let go of what it no longer needs", "rejects", res.LetGo, "held_back", res.HeldBack,
			"quarantined", res.Quarantined, "emptied", res.Emptied, "leftovers", res.Leftovers,
			"leftover_bytes", res.LeftoverByte)
	}
	return res, errors.Join(errs...)
}

// letGoOfRejects retires every fetched reject past its rest, then moves the files of every retired
// reject still on disk into quarantine. The retire commits before any file moves, so a crash
// between the two leaves bytes on disk for the next pass, never a rejected clip back in play.
func (j *Job) letGoOfRejects(ctx context.Context, res *Result) error {
	rows, err := j.store.ListClipPipelines(ctx, pipelinestate.PipelineFilter{
		Dispositions: []pipelinestate.Disposition{pipelinestate.DispositionRejected},
	})
	if err != nil {
		return err
	}
	now := j.now().UTC()
	rejectedBy := now.Add(-RejectRest)
	var referenced map[string]int
	for _, row := range rows {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if row.AcquisitionID == "" || j.disposed[row.ClipHash] {
			continue
		}
		if row.FilesDeletedAt.IsZero() {
			if row.UpdatedAt.After(rejectedBy) {
				continue
			}
			retired, err := j.store.RetireRejectedClip(ctx, row.ClipHash, rejectedBy, now)
			if err != nil {
				return err
			}
			if !retired {
				res.HeldBack++
				continue
			}
			res.LetGo++
		}
		if referenced == nil {
			paths, err := j.store.ListClipPaths(ctx)
			if err != nil {
				return err
			}
			referenced = identity.ReferencedAssets(j.clipDir, paths)
		}
		if j.quarantine(ctx, row.ClipHash, referenced) {
			j.disposed[row.ClipHash] = true
			res.Quarantined++
		}
	}
	return nil
}

// quarantine moves a retired clip's own files aside and reports whether all of them went. The
// sidecar goes last: it names the master and evidence copy, so while any of those is left the
// next pass can still find them.
func (j *Job) quarantine(ctx context.Context, hash string, referenced map[string]int) bool {
	clip, found, err := j.store.GetClip(ctx, hash)
	if err != nil {
		j.warn("upkeep: a rejected clip could not be read; the next pass tries again", "clip", hash, "err", err)
		return false
	}
	if !found {
		j.warn("upkeep: a rejected clip has no catalog row, so its files cannot be found; the next pass looks again", "clip", hash)
		return false
	}
	full, err := cliplibrary.ClipPath(j.clipDir, clip.Path, filepath.Ext(clip.Path))
	if err != nil {
		j.warn("upkeep: a rejected clip has no usable path", "clip", hash, "err", err)
		return false
	}
	// The sidecar names the master and the evidence copy, so a clip's files are only set aside when
	// it can be read: moving the sidecar of a clip whose sidecar could not be parsed would leave
	// those copies where nothing points. No sidecar at all, or a valid one with nothing of Loomarr's
	// in it, names nothing and is ordinary.
	tags, state := asset.ReadSidecarTagsState(full)
	if state == asset.SidecarInvalid {
		j.warn("upkeep: a rejected clip's sidecar could not be read, so its files are left where they are; the next pass tries again",
			"clip", hash, "sidecar", tools.SidecarPathFor(full))
		return false
	}
	files, err := identity.OwnFiles(j.clipDir, clip.Path, tags, referenced)
	if err != nil {
		j.warn("upkeep: a rejected clip's files could not be listed; the next pass tries again", "clip", hash, "err", err)
		return false
	}
	sidecar := tools.SidecarPathFor(full)
	var media []string
	for _, f := range files {
		if f != sidecar {
			media = append(media, f)
		}
	}
	if j.report(hash, j.bin.Put(media)) {
		return false
	}
	return !j.report(hash, j.bin.Put([]string{sidecar}))
}

func (j *Job) report(hash string, failed map[string]error) bool {
	for f, err := range failed {
		j.warn("upkeep: a rejected clip's file could not be moved to quarantine; the next pass tries again",
			"clip", hash, "file", f, "err", err)
	}
	return len(failed) > 0
}

func (j *Job) warn(msg string, args ...any) {
	if j.log != nil {
		j.log.Warn(msg, args...)
	}
}
