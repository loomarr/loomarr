package tunarr

import (
	"context"
	"time"

	"github.com/loomarr/loomarr/internal/setup"
)

// Adapter is the single seam for Loomarr's optional Tunarr playout backend (§6/§9,
// Refs #1564). It composes the existing Programmer port (channel + lineup + filler-list)
// with the Tunarr-shaped concerns that previously had no port of their own — guide
// reads, media-source wiring, local filler-source annotation, and Live TV URL
// selection — each reached before this PR via a type-assertion, a parallel ad hoc
// private interface, or a concrete *Tunarr parameter. *Tunarr is the only production
// implementation; app wires the interface everywhere above it, never the concrete
// client.
type Adapter interface {
	Programmer

	// Guide reads Tunarr's generated schedule (see Guide's doc comment on *Tunarr).
	Guide(ctx context.Context, from, to time.Time) (map[string][]GuideEntry, error)

	// Media-source wiring (previously reached via setup.MediaSourceProgrammer, satisfied
	// by a type-assertion on the concrete client in buildchannels.go).
	EnsureEmbySource(ctx context.Context, flavor, embyURL, token, userID string) (string, error)
	ConnectLibraries(ctx context.Context, sourceID string) (int, error)
	MediaLibrariesReady(ctx context.Context) (bool, error)

	// Local filler-source annotation (previously the private tunarrFillerClient
	// interface in app/filler.go, narrowed off a concrete *Tunarr parameter).
	EnsureLocalFillerSource(ctx context.Context, dir string) (EnsureLocalSourceResult, error)
	ListLocalFillerClipsAll(ctx context.Context) ([]LocalClip, error)

	// LiveTVURLs returns Tunarr's own Live TV URL pair, derived from the live-configured
	// base URL (§6) — byte-for-byte what setup.TunarrURLsFrom(configured base URL) gave
	// the deleted setup.LiveTVURLsFor free function on its non-internal branch. Callers
	// still decide internal-vs-Tunarr themselves; that branch collapses onto the adapter
	// in a later PR.
	LiveTVURLs() setup.LiveTVURLs
}

var _ Adapter = (*Tunarr)(nil)
