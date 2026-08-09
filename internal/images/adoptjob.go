package images

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"
)

// The artwork-adoption job (§22, V52 phase 6): pull already-rendered files on disk into the image
// service, so they gain content addressing, a width ladder, modern formats and honest caching.
//
// ⚠ **A JOB rather than an inline call at render time, and the reason is not laziness.** The
// producer of these files (`internal/filler`'s ffmpeg pass) works on a clip whose row has not
// necessarily been written yet, and wiring an ingest into it would make a domain package depend on
// this one. A job that reads "what is on disk but not yet adopted" needs neither: it is idempotent,
// survives a crash mid-batch, and — the part that matters most — it adopts EXISTING artwork and
// newly-rendered artwork through exactly the same path. There is no separate backfill to write, and
// therefore no second implementation to drift.
//
// ⚠ **This package still knows nothing about clips.** The store slice below speaks in OWNER ids and
// file paths; the composition root maps them onto the clips table, which is the same translation
// `imageadapter.go` already performs for the rest of this service. A `Clip` type in here would be
// the leak that makes `internal/images` un-reusable for the next producer of artwork.

// PendingArtwork is one owner's un-adopted artwork pair, as the store reports it.
type PendingArtwork struct {
	// OwnerID identifies the thing the artwork belongs to — a clip's content hash today.
	OwnerID string
	// StillPath / AnimPath are ABSOLUTE paths to the rendered files. Either may be empty: the two
	// renders fail independently, so an owner can legitimately have a still and no animation.
	StillPath string
	AnimPath  string
}

// ArtworkAdoptStore is the narrow slice the job needs.
type ArtworkAdoptStore interface {
	ListPendingArtwork(ctx context.Context, limit int) ([]PendingArtwork, error)
	SetAdoptedArtwork(ctx context.Context, ownerID, stillHash, animHash string, at time.Time) error
}

const (
	// adoptBatch bounds one run. Adoption re-encodes nothing on ingest, but it does hash and copy
	// bytes, and a first run on a mature catalog would otherwise walk thousands of files in one go.
	adoptBatch = 200
	// ownerKindClip is the Ref owner kind recorded for adopted clip artwork — what keeps the GC
	// from collecting a still that a clip in the catalog is still pointing at.
	ownerKindClip = "clip"
)

// AdoptJob adopts rendered artwork into the image service.
type AdoptJob struct {
	svc   *Service
	store ArtworkAdoptStore
	now   func() time.Time
	log   *slog.Logger
}

func NewAdoptJob(svc *Service, st ArtworkAdoptStore, now func() time.Time, log *slog.Logger) *AdoptJob {
	if now == nil {
		now = time.Now
	}
	return &AdoptJob{svc: svc, store: st, now: now, log: log}
}

// AdoptResult is what one run did, for the log line.
type AdoptResult struct {
	Owners  int
	Adopted int
	Missing int
	Failed  int
}

// Run adopts one batch.
//
// ⚠ **A missing FILE is not a failure, and the distinction is the point of the two counters.**
// `thumbnail`/`preview` record what was rendered once; the artwork cache lives under FILLER_DIR and
// is explicitly regenerable, so an operator who cleared it has rows pointing at files that are
// legitimately gone. Counting that as an error would make a normal state look broken and bury the
// genuine failures underneath it — the same reasoning §22 applies to a missing AVIF.
func (j *AdoptJob) Run(ctx context.Context) (AdoptResult, error) {
	var res AdoptResult
	if j.svc == nil || j.store == nil {
		return res, nil
	}
	pending, err := j.store.ListPendingArtwork(ctx, adoptBatch)
	if err != nil {
		return res, fmt.Errorf("list pending artwork: %w", err)
	}
	res.Owners = len(pending)

	for _, p := range pending {
		still, sMissing, sErr := j.adoptOne(ctx, p.OwnerID, p.StillPath)
		anim, aMissing, aErr := j.adoptOne(ctx, p.OwnerID, p.AnimPath)
		res.Missing += sMissing + aMissing
		if sErr != nil || aErr != nil {
			res.Failed++
			if j.log != nil {
				j.log.Warn("adopt artwork", "owner", p.OwnerID, "still_err", sErr, "anim_err", aErr)
			}
		}
		// ⚠ Recorded even on a PARTIAL success — one hash present, the other empty. Skipping the
		// write until both succeed would re-list this owner every run forever when only one of the
		// two renders ever existed, which is the ordinary state for a clip whose animation failed.
		if still == "" && anim == "" {
			continue
		}
		if err := j.store.SetAdoptedArtwork(ctx, p.OwnerID, still, anim, j.now()); err != nil {
			res.Failed++
			if j.log != nil {
				j.log.Warn("record adopted artwork", "owner", p.OwnerID, "err", err)
			}
			continue
		}
		res.Adopted++
	}
	return res, nil
}

// adoptOne ingests one file, reporting "missing" separately from "failed".
// ⚠ No `animated` parameter: the hover loop's motion is derived from the BYTES during ingest, not
// asserted by the caller. A flag here would be a second source of truth for something the decoder
// already knows, and the two would disagree the first time a still was rendered as an animated WebP.
func (j *AdoptJob) adoptOne(ctx context.Context, ownerID, path string) (hash string, missing int, err error) {
	if path == "" {
		return "", 0, nil
	}
	f, oErr := os.Open(path) //nolint:gosec // path comes from our own render, not from a request
	if oErr != nil {
		if os.IsNotExist(oErr) {
			return "", 1, nil
		}
		return "", 0, oErr
	}
	defer func() { _ = f.Close() }()

	// ⚠ Visibility MEMBER, not public. A clip still is catalog content behind a session, unlike a
	// channel icon which Tunarr fetches machine-to-machine with no credentials (§22: visibility is
	// a property of the image, not the route). Getting this backwards would publish the filler
	// catalog to anyone who could guess a hash.
	//
	// ⚠ Origin EXTRACTED, not upload: these are re-derivable from the clip by the same ffmpeg pass,
	// which is what makes them recoverable after losing /data/images. Labelling them `upload` would
	// tell the GC they are unrecoverable and make it warn about images it could simply rebuild.
	rec, iErr := j.svc.Ingest(ctx, f, IngestRequest{
		Role:       RoleThumb,
		Visibility: VisibilityMember,
		Origin:     OriginExtracted,
		OwnerKind:  ownerKindClip,
		OwnerID:    ownerID,
	})
	if iErr != nil {
		return "", 0, iErr
	}
	return rec.Hash, 0, nil
}
