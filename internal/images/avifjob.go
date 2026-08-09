package images

import (
	"context"
	"log/slog"
	"os"
)

// The AVIF job (§22, §18.1 — V52 phase 3b): the one rendition Loomarr never produces on a request.
//
// ⚠ The reason is CONCURRENCY, not latency, and avif.go carries the measurements that establish
// it: ~86ms against WebP's ~67ms, about 1.3×. What a request cannot survive is that each encode
// forks a natively-multithreaded ffmpeg, so a cold grid of fifty posters would fork fifty at once.
// This job does the same work at a rate the box chooses.

// avifBatch bounds one pass. A poster ladder is five rungs at roughly 86ms each, so twenty-five
// images is on the order of ten seconds of CPU — long enough to drain a backlog steadily, short
// enough that a pass never overlaps the next hourly tick.
const avifBatch = 25

// AVIFStore is the persistence the AVIF job needs on top of the serve path's.
type AVIFStore interface {
	Store
	// ListMissingFormat is the work list: images that already have a rendition but none in this
	// format. ⚠ The "already have one" half keeps the job off images whose bytes have not been
	// processed at all — those belong to the fetch job, and encoding AVIF for something with no
	// WebP would invert the priority the lazy/job split exists to create.
	ListMissingFormat(ctx context.Context, f Format, limit int) ([]Image, error)
}

// AVIFResult is one pass's tally.
type AVIFResult struct {
	Considered int
	Images     int
	Renditions int
	Failed     int
}

// AVIFJob encodes the AVIF ladder for images that lack it.
type AVIFJob struct {
	svc   *Service
	store AVIFStore
	enc   AVIFEncoder
	log   *slog.Logger
}

// NewAVIFJob builds the job. A nil encoder makes every pass a no-op, which is the correct
// behaviour for a build whose ffmpeg carries no AV1 encoder — the renditions simply never appear
// and clients keep taking WebP.
func NewAVIFJob(svc *Service, st AVIFStore, enc AVIFEncoder, log *slog.Logger) *AVIFJob {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &AVIFJob{svc: svc, store: st, enc: enc, log: log}
}

// Run encodes one batch.
func (j *AVIFJob) Run(ctx context.Context) (AVIFResult, error) {
	// ⚠ Both gates are read per run rather than at construction. `images.formats` hot-applies, so
	// an operator dropping `avif` to save CPU on a struggling NAS expects the next tick to stop
	// encoding — not the next restart.
	if j.enc == nil || !j.svc.Produces(FormatAVIF) {
		return AVIFResult{}, nil
	}

	work, err := j.store.ListMissingFormat(ctx, FormatAVIF, avifBatch)
	if err != nil {
		return AVIFResult{}, err
	}

	out := AVIFResult{Considered: len(work)}
	for _, img := range work {
		n, err := j.encodeLadder(ctx, img)
		out.Renditions += n
		switch {
		case err != nil:
			out.Failed++
			// One image failing must not stop the batch: a single unreadable original would
			// otherwise stall AVIF coverage for the whole catalog behind it, and the work list is
			// ordered, so the same image would be first again on every subsequent pass.
			j.log.Warn("avif encode failed", "hash", img.Hash, "err", err)
		case n > 0:
			out.Images++
		}
		if ctx.Err() != nil {
			// A cancelled context means shutdown. Stop rather than churning through the rest;
			// whatever is unencoded is picked up next pass by construction.
			break
		}
	}
	return out, nil
}

// encodeLadder produces every rung of one image's ladder, returning how many it wrote.
func (j *AVIFJob) encodeLadder(ctx context.Context, rec Image) (int, error) {
	origPath, err := j.svc.blob.OriginalPath(rec.Hash, extForMIME(rec.MIME))
	if err != nil {
		return 0, err
	}
	if _, ok := j.svc.blob.Stat(origPath); !ok {
		// The row exists and the bytes do not — a restore onto an empty image directory. Not this
		// job's problem to fix and not an error worth a red Tasks row: rehydrate owns it.
		return 0, nil
	}
	data, err := os.ReadFile(origPath) //nolint:gosec // path built from a validated hash
	if err != nil {
		return 0, err
	}
	decoded, _, err := Decode(data)
	if err != nil {
		return 0, err
	}

	widths := rec.Role.Widths()
	if rec.Animated {
		// An animation has one rendition and skips the ladder, matching the serve path. Encoding
		// an animated original as a still AVIF would also silently drop the motion, which is worse
		// than not offering the format at all.
		return 0, nil
	}

	// ⚠ One ResizeLadder call for the whole ladder, not Resize per rung. The rungs are produced by
	// stepping DOWN through each other — resampling from full resolution to each target
	// independently re-walks the halving chain every time, which a benchmark caught costing
	// 231ms against 100ms for the same output.
	rungs := ResizeLadder(decoded, widths)

	written := 0
	for _, w := range widths {
		img, ok := rungs[w]
		if !ok {
			continue
		}
		dst, err := j.svc.blob.DerivativePath(rec.Hash, w, FormatAVIF)
		if err != nil {
			return written, err
		}
		if _, exists := j.svc.blob.Stat(dst); exists {
			// Present on disk but evidently not recorded, or recorded for another rung. Record it
			// and move on rather than re-forking ffmpeg for bytes that already exist.
			if err := j.recordRendition(ctx, rec.Hash, w, dst); err != nil {
				return written, err
			}
			continue
		}
		if err := j.enc(ctx, img, dst); err != nil {
			return written, err
		}
		if err := j.recordRendition(ctx, rec.Hash, w, dst); err != nil {
			return written, err
		}
		written++
	}
	return written, nil
}

// recordRendition writes the derivative row from what is ON DISK.
//
// ⚠ The size comes from Stat rather than from the encoder, because ffmpeg can exit 0 having
// written nothing — a failure `filler.GenerateArtwork` already learned to catch by measuring
// rather than by checking existence. A zero-byte derivative recorded as present renders as a
// BROKEN image, which is strictly worse than an absent one: absent has a designed fallback.
func (j *AVIFJob) recordRendition(ctx context.Context, hash string, width int, dst string) error {
	size, ok := j.svc.blob.Stat(dst)
	if !ok {
		_ = j.svc.blob.Remove(dst)
		return nil
	}
	return j.store.PutDerivative(ctx, Derivative{
		ImageHash: hash, Format: FormatAVIF, Width: width,
		Bytes: size, Path: dst, CreatedAt: j.svc.now(),
	})
}
