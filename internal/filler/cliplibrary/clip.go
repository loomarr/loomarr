// Package cliplibrary is the on-disk clip library: the clip folder's layout, the one intake route
// every clip takes into it, the folder and library scans that find clips, the names derived for
// them, and the thumbnail and preview artwork rendered from them. It also holds the clip shapes
// those scans produce and the catalog round-trips (RawClip, StoreClip). The filler root's Syncer
// reconciles the catalog against it.
package cliplibrary

import (
	"context"
	"time"

	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/pod"
)

// RawClip is one clip as discovered by a scan, before catalog metadata is merged.
type RawClip struct {
	// ID is the identity: the clip's sparse content hash (§10 V38c).
	//
	// ⚠ Separate from Path since V38c. Identity used to BE the path, which broke the moment many
	// watched folders were allowed — two folders each holding `ads/coke.mp4` produced one id and
	// silently overwrote each other. Hashing the bytes also answers the question a path cannot:
	// *is this the same advert?*, which is what lets intake skip duplicates.
	ID string
	// Path is the clip's LOCATION under the clip folder — `a3/f9/<hash>.mp4` for anything intake
	// has filed. Data, not identity.
	Path string
	// Source is the registered source id restored from Loomarr's sidecar. Empty is a hand-copied
	// or legacy clip and resolves through the folder source policy.
	Source string
	// ParentHash is restored only from Loomarr's conditioning lineage sidecar. It reconnects a
	// reviewed child to its composite after the rebuildable clip catalog is empty.
	ParentHash string
	// LineageInvalid keeps a damaged child fail-closed during catalog reconstruction. ParentHash
	// may still name the retained parent, but the child remains held until its sidecar is repaired.
	LineageInvalid bool
	// ConditioningHold keeps a valid lineage-only child and malformed conditioning state out of
	// rotation across an empty-catalog rebuild until complete post-rewrite evidence exists.
	ConditioningHold bool
	// SidecarInvalid prevents catalog repair from overwriting or laundering unreadable durable
	// metadata. The clip is held until an operator repairs the sidecar.
	SidecarInvalid bool
	// IsComposite is derived for retained parents from valid child lineage before any rows are
	// written, so filesystem traversal order cannot briefly make the parent airable.
	IsComposite bool
	// TunarrProgramID is set only when the clip was ALSO seen through Tunarr's local
	// source, so Tunarr-backed channels can still build filler-lists. Empty on an
	// install with no Tunarr, which is a supported configuration, not a degraded one.
	TunarrProgramID string
	Name            string
	// NameSource is where the scan found Name (#1988); the catalog row takes it with the name.
	NameSource      clips.NameSource
	DurationMs      int64
	Role            clips.Role
	Era             int // initial era from filename; 0 if none
	GeographicScope pod.GeographicScope
	Country         string
	Network         string
	Station         string
	AirDate         string
	GeoEvidence     string
	// Quality is the resolution label ("1080p", "480p") derived from the probed video
	// height; "" when the file has no video stream or was never re-probed. Display-only —
	// the guide's pod hover card shows it so a grainy 240p advert is explicable rather
	// than surprising. It never affects selection (V17c adds an opt-in floor).
	Quality string
	// License is the licence URL the source declared, read from the clip's info-JSON sidecar
	// at scan time (V33). "" means UNKNOWN — the common case, ~92% of archive.org items — and
	// never "public domain".
	License string
	// Thumbnail is the extracted frame's path relative to the thumbnail cache dir; "" when
	// extraction failed or has not run. Filled by a SEPARATE pass (GenerateThumbnails), not
	// by the probe — see thumbnail.go for why it cannot ride along with ffprobe.
	Thumbnail string
	// Preview is the animated hover preview's path relative to the preview cache dir; "" when
	// the render failed or has not run (V39). A third pass again (GeneratePreviews) for the same
	// reason as the still: it is a distinct ffmpeg invocation, and a failed render must cost a
	// preview rather than a clip.
	Preview string
}

// FillerSource discovers the clips in FILLER_DIR.
//
// Implemented by DirSource (the local scan, §9.1) and satisfied in tests by a double. It was
// previously implemented by the Tunarr client — EnsureLocalSource registered the drop-folder
// as a Tunarr `local` media source and ListLocalClips read back what Tunarr had scanned.
//
// EnsureLocalSource is retained because Tunarr-backed channels still need that registration
// (their filler-lists reference Tunarr program ids). It is now BEST-EFFORT: an install with no
// Tunarr fails it harmlessly and still gets a full catalog from the local scan, which is the
// whole point of the change.
type FillerSource interface {
	EnsureLocalSource(ctx context.Context, dir string) error
	ListLocalClips(ctx context.Context) ([]RawClip, error)
}

// LocalScan is one listing of the clip folder: the clips found and the subtrees that could not be
// read, which the listing says nothing about (#2673).
type LocalScan struct {
	Clips []RawClip
	// Unread are subtrees the scan could not read, as slash paths relative to the clip folder, in
	// the same form as RawClip.Path. A catalog row under one is neither confirmed nor gone.
	Unread []string
}

// ScanningSource is a FillerSource that also reports the subtrees it could not read. The sync
// prefers it, so a clip under an unreadable subfolder is kept rather than pruned. A source that
// does not implement it reports no unreadable subtrees, which is only true of one that cannot
// have any; DirSource implements it.
type ScanningSource interface {
	FillerSource
	ScanLocalClips(ctx context.Context) (LocalScan, error)
}

var _ ScanningSource = DirSource{}

// StoreClip is the persistence view the sync round-trips (mirrors store.Clip;
// declared here so filler doesn't import store — the adapter in main bridges them).
type StoreClip struct {
	clips.Clip
	UpdatedAt time.Time
}
