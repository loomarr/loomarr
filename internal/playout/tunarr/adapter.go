package tunarr

import (
	"context"
	"time"
)

// Adapter is the single seam for Loomarr's optional Tunarr playout backend (§6/§9,
// Refs #1564). It composes the existing Programmer port (channel + lineup + filler-list)
// with the Tunarr-shaped concerns that are not part of Programmer — guide
// reads, media-source wiring, local filler-source annotation, and Live TV URL
// selection — so callers need no type-assertion or concrete *Tunarr parameter. *Tunarr is the only production
// implementation; app wires the interface everywhere above it, never the concrete
// client.
type Adapter interface {
	Programmer

	// Guide reads Tunarr's generated schedule (see Guide's doc comment on *Tunarr).
	Guide(ctx context.Context, from, to time.Time) (map[string][]GuideEntry, error)

	// Media-source wiring; the Adapter satisfies livetvpublish.MediaSourceProgrammer.
	EnsureEmbySource(ctx context.Context, flavor, embyURL, token, userID string) (string, error)
	ConnectLibraries(ctx context.Context, sourceID string) (int, error)
	MediaLibrariesReady(ctx context.Context) (bool, error)

	// Local filler-source annotation.
	EnsureLocalFillerSource(ctx context.Context, dir string) (EnsureLocalSourceResult, error)
	ListLocalFillerClipsAll(ctx context.Context) ([]LocalClip, error)

	// LiveTVURLs returns Tunarr's own Live TV URL pair, derived from the live-configured
	// base URL (§6). Callers decide internal-vs-Tunarr themselves.
	LiveTVURLs() LiveTVURLs
}

var _ Adapter = (*Tunarr)(nil)
