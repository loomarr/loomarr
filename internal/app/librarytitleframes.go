package app

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/loomarr/loomarr/internal/media/images"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
	"github.com/loomarr/loomarr/internal/playout/hls"
)

// A title with no catalog artwork gets a frame of its main file, about a tenth of the way in
// (#4476, library.md "A home video's picture" and "A frame on a Needs you card"): a kept title as
// its poster, and a title waiting in Needs you as its card's frame. The title having no poster, or
// no frame, is the whole record of the work left: the scanner owns the poster role, and writing a
// kept title keeps the frame only while the title is kept and plays the file the frame came from;
// the sweep after the scanner saves file rows does the same for a Needs you frame.

const (
	// titleFrameEvery is how often the titles are checked, and titleFrameBatch the most frames one
	// check extracts, one at a time, kept titles' posters first.
	titleFrameEvery = 2 * time.Minute
	titleFrameBatch = 10
	// A title whose frame fails waits titleFrameRetry, doubling to titleFrameRetryMax.
	titleFrameRetry    = 5 * time.Minute
	titleFrameRetryMax = 24 * time.Hour
)

type titleFrameStore interface {
	HomeVideosWithoutPoster(ctx context.Context) ([]librarystore.LibraryFrameFile, error)
	PutHomeVideoPoster(ctx context.Context, frame librarystore.LibraryFrameFile, imageHash string) (bool, error)
	LibraryNeedsWithoutFrame(ctx context.Context) ([]librarystore.LibraryFrameFile, error)
	PutLibraryNeedFrame(ctx context.Context, frame librarystore.LibraryFrameFile, imageHash string) (bool, error)
}

// titleFrameFile is the title, the file and the kind of frame a failure belongs to.
type titleFrameFile struct {
	itemID, path, revision string
	need                   bool
}

type titleFrameFailure struct {
	count int
	until time.Time
}

// titleFrameWanted is a frame to take: a kept title's poster, or a Needs you card's frame.
type titleFrameWanted struct {
	librarystore.LibraryFrameFile
	need bool
}

type titleFrames struct {
	store  titleFrameStore
	images *images.Service
	still  hls.SourceStillExtractor
	// busy reports whether a viewer is fetching a channel; nil is never.
	busy func() bool
	log  *slog.Logger
	now  func() time.Time
	// failed is keyed by the title and its file, so another file is tried at once. It is held in
	// memory: a restart tries each failed title again.
	failed map[titleFrameFile]titleFrameFailure
}

// Run checks every titleFrameEvery until ctx ends.
func (p *titleFrames) Run(ctx context.Context) {
	tick := time.NewTicker(titleFrameEvery)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		if err := p.pass(ctx); err != nil && ctx.Err() == nil {
			p.log.Warn("library frames: not checked", "err", err)
		}
	}
}

// wanted is every frame due a look: the kept titles' posters, then the Needs you frames.
func (p *titleFrames) wanted(ctx context.Context) ([]titleFrameWanted, error) {
	kept, err := p.store.HomeVideosWithoutPoster(ctx)
	if err != nil {
		return nil, err
	}
	needs, err := p.store.LibraryNeedsWithoutFrame(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]titleFrameWanted, 0, len(kept)+len(needs))
	for _, f := range kept {
		out = append(out, titleFrameWanted{LibraryFrameFile: f})
	}
	for _, f := range needs {
		out = append(out, titleFrameWanted{LibraryFrameFile: f, need: true})
	}
	return out, nil
}

// pass extracts the frames that are due, at most titleFrameBatch of them.
func (p *titleFrames) pass(ctx context.Context) error {
	wanted, err := p.wanted(ctx)
	if err != nil {
		return err
	}
	tried := 0
	failed := make(map[titleFrameFile]titleFrameFailure)
	for _, w := range wanted {
		key := titleFrameFile{w.ItemID, w.Path, w.Revision, w.need}
		f, before := p.failed[key]
		if tried < titleFrameBatch && ctx.Err() == nil && !p.now().Before(f.until) && (p.busy == nil || !p.busy()) {
			tried++
			err := p.extract(ctx, w)
			if err == nil {
				continue
			}
			f.count++
			wait := min(titleFrameRetry<<min(f.count-1, 16), titleFrameRetryMax)
			f.until, before = p.now().Add(wait), true
			p.log.Warn("library frames: no frame", "item", w.ItemID, "needs_you", w.need, "retry_in", wait, "err", err)
		}
		if before {
			failed[key] = f
		}
	}
	// Only the frames still wanted are remembered.
	p.failed = failed
	return nil
}

// extract stores the frame as the kept title's poster, or as the waiting title's frame.
func (p *titleFrames) extract(ctx context.Context, w titleFrameWanted) error {
	frame, err := p.still(ctx, hls.StillSource{Input: w.Path, HDR: w.HDR},
		time.Duration(w.DurationMs)*time.Millisecond/10)
	if err != nil {
		return err
	}
	if len(frame) == 0 {
		return fmt.Errorf("ffmpeg wrote no frame")
	}
	// A poster takes the poster ladder; a card's frame is drawn 16:9, so it takes a still's.
	role := images.RolePoster
	if w.need {
		role = images.RoleThumb
	}
	// Extracted, not upload: losing the image directory loses nothing, the video is still there.
	rec, err := p.images.Ingest(ctx, bytes.NewReader(frame), images.IngestRequest{
		Role: role, Visibility: images.VisibilityMember, Origin: images.OriginExtracted,
	})
	if err != nil {
		return err
	}
	// The answer can be given or taken back, the file replaced or another frame written while
	// ffmpeg and the image service run, so the store checks and writes the ref as one step. An
	// image it refuses has no owner, and the image sweep takes it.
	if w.need {
		_, err = p.store.PutLibraryNeedFrame(ctx, w.LibraryFrameFile, rec.Hash)
	} else {
		_, err = p.store.PutHomeVideoPoster(ctx, w.LibraryFrameFile, rec.Hash)
	}
	return err
}
