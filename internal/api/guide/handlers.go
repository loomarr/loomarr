// Package guide serves what is on and what would be on: the time-grid guide and its highlights,
// each channel's now, next, upcoming and timeline, the programming and cycle previews, the filler
// pod and coverage previews, the programming vocabulary and the holiday calendar.
package guide

import (
	"context"
	"log/slog"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/media/images"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
	"github.com/loomarr/loomarr/internal/playout"
)

// Store is the slice of the aggregate store these routes read.
type Store interface {
	store.ChannelStore
	store.ClipStore
	librarystore.LibraryArtReader
}

// PlayoutGuide resolves the programmes airing on a channel across a window, with absolute
// wall-clock times (§9.1 V6b).
type PlayoutGuide interface {
	// BroadcastsBetween returns the programmes airing on a channel across [from, to).
	BroadcastsBetween(ctx context.Context, channelID string, from, to time.Time) ([]playout.Broadcast, error)
	// BroadcastsWithPending is the same walk plus pending acquisitions as nominal-width
	// placeholders, for Loomarr's own time grid (V13b).
	BroadcastsWithPending(ctx context.Context, channelID string, from, to time.Time) ([]playout.Broadcast, error)
}

// ImagePaths builds the same-origin path of an image rendition.
type ImagePaths interface {
	PathFor(hash string, width int, f images.Format) string
}

// Deps is what the guide routes need from the rest of the server. Every field is optional: a nil
// service leaves its routes answering 501 or reading empty, as the API's nil-semantics describe.
type Deps struct {
	Log   *slog.Logger
	Store Store
	// LiveSettings reads the guide's timezone and retention.
	LiveSettings settings.Reader
	// Now is the request clock; nil is time.Now.
	Now func() time.Time
	// Channels previews what a channel would air and supplies the holiday calendar.
	Channels ChannelService
	// Pods previews the commercial pools and measures their coverage.
	Pods PodPreviewer
	// Guide reads now and next from whichever backend streams each channel.
	Guide GuideReader
	// PlayoutGuide resolves the programmes on internal playout's timeline.
	PlayoutGuide PlayoutGuide
	// TimelineThumbs resolves a TMDB preview image per programme; nil leaves the strip imageless.
	TimelineThumbs TimelineThumbResolver
	// Images builds rendition paths; nil leaves logos as stored.
	Images ImagePaths
	// ImageRecords resolves image hashes to their records in one deduplicated pass.
	ImageRecords func(ctx context.Context, hashes []string) map[string]*httpcore.ImageDTO
	// LibraryImage resolves one image hash to its record, or nil when it has no bytes.
	LibraryImage func(ctx context.Context, hash string) *httpcore.ImageDTO
}

// Handlers serves the guide routes.
type Handlers struct {
	log            *slog.Logger
	store          Store
	liveSettings   settings.Reader
	now            func() time.Time
	channels       ChannelService
	pods           PodPreviewer
	guide          GuideReader
	playoutGuide   PlayoutGuide
	timelineThumbs TimelineThumbResolver
	images         ImagePaths
	imageRecords   func(ctx context.Context, hashes []string) map[string]*httpcore.ImageDTO
	libraryImage   func(ctx context.Context, hash string) *httpcore.ImageDTO
}

// New returns the guide handlers over d.
func New(d Deps) *Handlers {
	return &Handlers{
		log: d.Log, store: d.Store, liveSettings: d.LiveSettings, now: d.Now, channels: d.Channels,
		pods: d.Pods, guide: d.Guide, playoutGuide: d.PlayoutGuide, timelineThumbs: d.TimelineThumbs,
		images: d.Images, imageRecords: d.ImageRecords, libraryImage: d.LibraryImage,
	}
}

// Register mounts every route this package serves.
func (s *Handlers) Register(api huma.API) {
	s.registerGuide(api)
	s.registerProgramming(api)
	s.registerHolidays(api)
	s.registerChannelViews(api)
}

// clock is the request clock: the pinned one in tests, time.Now otherwise.
func (s *Handlers) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// holidayCalendar is the engine's holiday calendar, or the default (the US, UTC) without one.
func (s *Handlers) holidayCalendar() schedule.Calendar {
	if s.channels == nil {
		return schedule.Calendar{}
	}
	return s.channels.Calendar()
}

// imageDTOsByHash resolves image hashes in one pass, or nothing without an image service.
func (s *Handlers) imageDTOsByHash(ctx context.Context, hashes []string) map[string]*httpcore.ImageDTO {
	if s.imageRecords == nil || len(hashes) == 0 {
		return nil
	}
	return s.imageRecords(ctx, hashes)
}
