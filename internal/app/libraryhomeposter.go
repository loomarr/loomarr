package app

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/loomarr/loomarr/internal/media/images"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
	"github.com/loomarr/loomarr/internal/playout"
)

// A home video has no catalog artwork, so its poster is a frame of its main file, about a tenth of
// the way in (#4476, library.md "A home video's picture"). The title having no poster is the whole
// record of the work left: the scanner owns the poster role and clears it whenever it writes a kept
// title, which a changed file and a taken-back answer both do.

const (
	// homePosterEvery is how often the kept titles are checked, and homePosterBatch the most
	// frames one check extracts, one at a time.
	homePosterEvery = 2 * time.Minute
	homePosterBatch = 10
	// A title whose frame fails waits homePosterRetry, doubling to homePosterRetryMax.
	homePosterRetry    = 5 * time.Minute
	homePosterRetryMax = 24 * time.Hour
)

type homePosterStore interface {
	HomeVideosWithoutPoster(ctx context.Context) ([]librarystore.LibraryHomeVideoFrame, error)
}

type homePosterFailure struct {
	count int
	until time.Time
}

type homePosters struct {
	store  homePosterStore
	images *images.Service
	still  playout.SourceStillExtractor
	// busy reports whether a viewer is fetching a channel; nil is never.
	busy func() bool
	log  *slog.Logger
	now  func() time.Time
	// failed is keyed by the title and its file, so another file is tried at once. It is held in
	// memory: a restart tries each failed title again.
	failed map[librarystore.LibraryHomeVideoFrame]homePosterFailure
}

// Run checks every homePosterEvery until ctx ends.
func (p *homePosters) Run(ctx context.Context) {
	tick := time.NewTicker(homePosterEvery)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		if err := p.pass(ctx); err != nil && ctx.Err() == nil {
			p.log.Warn("home video posters: not checked", "err", err)
		}
	}
}

// pass extracts the frames that are due, at most homePosterBatch of them.
func (p *homePosters) pass(ctx context.Context) error {
	wanted, err := p.store.HomeVideosWithoutPoster(ctx)
	if err != nil {
		return err
	}
	tried := 0
	failed := make(map[librarystore.LibraryHomeVideoFrame]homePosterFailure)
	for _, w := range wanted {
		f, before := p.failed[w]
		if tried < homePosterBatch && ctx.Err() == nil && !p.now().Before(f.until) && (p.busy == nil || !p.busy()) {
			tried++
			err := p.extract(ctx, w)
			if err == nil {
				continue
			}
			f.count++
			wait := min(homePosterRetry<<min(f.count-1, 16), homePosterRetryMax)
			f.until, before = p.now().Add(wait), true
			p.log.Warn("home video posters: no frame", "item", w.ItemID, "retry_in", wait, "err", err)
		}
		if before {
			failed[w] = f
		}
	}
	// Only the titles still wanted are remembered.
	p.failed = failed
	return nil
}

// extract stores the frame as the title's poster.
func (p *homePosters) extract(ctx context.Context, w librarystore.LibraryHomeVideoFrame) error {
	frame, err := p.still(ctx, playout.StillSource{Input: w.Path, HDR: w.HDR},
		time.Duration(w.DurationMs)*time.Millisecond/10)
	if err != nil {
		return err
	}
	if len(frame) == 0 {
		return fmt.Errorf("ffmpeg wrote no frame")
	}
	// The answer can be taken back, or the file replaced, while ffmpeg runs.
	now, err := p.store.HomeVideosWithoutPoster(ctx)
	if err != nil {
		return err
	}
	for _, still := range now {
		if still != w {
			continue
		}
		// Extracted, not upload: losing the image directory loses nothing, the video is still there.
		_, err := p.images.Ingest(ctx, bytes.NewReader(frame), images.IngestRequest{
			Role: images.RolePoster, Visibility: images.VisibilityMember, Origin: images.OriginExtracted,
			OwnerKind: librarystore.LibraryItemImageOwner, OwnerID: w.ItemID,
		})
		return err
	}
	return nil
}
