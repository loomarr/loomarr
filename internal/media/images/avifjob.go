package images

import (
	"context"
	"log/slog"
)

// AVIF is worker-rendered but remains background-only: its CPU cost must not sit on request
// latency or fan out with a cold browser grid.
const avifBatch = 25

type AVIFStore interface {
	Store
	ListMissingFormat(ctx context.Context, recipe string, f Format, limit int) ([]Image, error)
}

type AVIFResult struct {
	Considered int
	Images     int
	Renditions int
	Failed     int
}

type AVIFJob struct {
	svc   *Service
	store AVIFStore
	log   *slog.Logger
}

func NewAVIFJob(svc *Service, st AVIFStore, log *slog.Logger) *AVIFJob {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &AVIFJob{svc: svc, store: st, log: log}
}

func (j *AVIFJob) Run(ctx context.Context) (AVIFResult, error) {
	work, err := j.store.ListMissingFormat(ctx, renditionRecipe, FormatAVIF, avifBatch)
	if err != nil {
		return AVIFResult{}, err
	}
	out := AVIFResult{Considered: len(work)}
	for _, img := range work {
		n, renderErr := j.encodeLadder(ctx, img)
		out.Renditions += n
		if renderErr != nil {
			out.Failed++
			j.log.Warn("avif encode failed", "hash", img.Hash, "err", renderErr)
		} else if n > 0 {
			out.Images++
		}
		if ctx.Err() != nil {
			break
		}
	}
	return out, nil
}

func (j *AVIFJob) encodeLadder(ctx context.Context, rec Image) (int, error) {
	if rec.Animated {
		return 0, nil
	}
	widths := rec.Role.Widths()
	if _, err := j.svc.encodeRenditions(ctx, rec, FormatAVIF, widths); err != nil {
		return 0, err
	}
	return len(widths), nil
}
