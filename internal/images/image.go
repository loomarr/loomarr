// Package images is the one pipeline every image in Loomarr travels (§22).
//
// Before it there were four: uploaded channel icons as database blobs, clip stills and hover
// loops on disk inside FILLER_DIR, and TMDB posters hot-linked straight from the operator's
// browser. Four storage models, four cache policies, four identity schemes, no shared code —
// and, because nothing could produce a `srcset`, one hardcoded 500px width shipped to a 40px
// hover chip and a full channel tile alike.
//
// The seam is deliberately narrow: callers hand bytes (Ingest) or a URL (Adopt) and get back an
// Image; they ask for a Rendition and get a file. Nothing outside this package knows the disk
// layout, the hash function, the format ladder, or which encoder ran.
package images

import (
	"errors"
	"time"
)

// ErrNotFound is returned when no image exists for a hash. Callers map it to a 404; the API
// layer must not distinguish "no such image" from "not visible to you" (§22 — a distinct error
// would confirm which hashes exist).
var ErrNotFound = errors.New("images: not found")

// Origin is where an image's bytes came from, and — more importantly — whether they can be got
// back. This is the field the whole durability story hangs on (§22): the application backup is a
// DATABASE backup, so what survives losing the image directory is decided entirely here.
type Origin string

const (
	// OriginUpload is bytes a person handed us. ⚠ The ONLY origin that is unrecoverable: there
	// is no upstream to re-fetch and no source to re-derive from. The GC job counts these when
	// their file is missing and raises a system warning, because the alternative — rendering as
	// a broken image — looks like a bug rather than a known consequence.
	OriginUpload Origin = "upload"
	// OriginRemote is bytes fetched from upstream (TMDB, the media server). Recoverable from
	// SourceURL, which is why that column is load-bearing rather than informational.
	OriginRemote Origin = "remote"
	// OriginExtracted is bytes we produced from local media — a clip's still frame. Recoverable
	// by re-running the ffmpeg pass that made it.
	OriginExtracted Origin = "extracted"
	// OriginGenerated is bytes a model produced. Recoverable only if the generation parameters
	// in Meta are enough to reproduce it, which for most models they are not — treat as
	// upload-like for durability purposes.
	OriginGenerated Origin = "generated"
)

// Recoverable reports whether the bytes can be obtained again without a human. It is the
// predicate the GC job's missing-file warning uses, stated once here rather than re-derived as a
// switch at each call site.
func (o Origin) Recoverable() bool {
	return o == OriginRemote || o == OriginExtracted
}

// Visibility is a property of the IMAGE, not of the route that serves it (§22).
//
// ⚠ This is the one place the image service departs from every other raw-byte route in §7.1,
// which each carry a fixed role. It cannot: a channel icon MUST be reachable without credentials
// because Tunarr fetches it machine-to-machine exactly as it would fetch a TMDB poster, while a
// clip still must not be. One route with a per-row check is the only shape that serves both
// without duplicating the serve path.
type Visibility string

const (
	// VisibilityPublic is fetchable with no session. Used for images a machine must fetch.
	VisibilityPublic Visibility = "public"
	// VisibilityMember requires a signed-in user. ⚠ A miss is answered 404, never 403 — the same
	// convention the clip-thumbnail route already follows, because a distinct status confirms
	// which hashes exist to someone enumerating them.
	VisibilityMember Visibility = "member"
)

// Format is an output encoding. The input format is recorded as a MIME string on the row; this
// type is only ever about what we PRODUCE, which is a closed set.
type Format string

const (
	// FormatAVIF is the smallest and by far the most expensive to produce — 300–1200ms for a
	// ~1000px image. ⚠ Produced by a background job, never on a request (§22); a serve that
	// finds no AVIF derivative simply omits the <source> and the browser takes WebP.
	FormatAVIF Format = "avif"
	// FormatWebP is the workhorse: ~97% support, generated lazily on first request.
	FormatWebP Format = "webp"
	// FormatJPEG is the floor, kept for old iOS and legacy Android WebViews — over-represented
	// among a self-hosted media server's clients (televisions, ageing tablets) in a way they are
	// not on the general web.
	FormatJPEG Format = "jpeg"
)

// Ext is the filename extension for a format. Separate from the Format string because "jpeg" the
// format is "jpg" the extension, and hardcoding that mapping at each call site is how a
// derivative gets written to one path and looked up at another.
func (f Format) Ext() string {
	if f == FormatJPEG {
		return "jpg"
	}
	return string(f)
}

// MIME is the Content-Type to serve a rendition of this format with.
func (f Format) MIME() string {
	switch f {
	case FormatAVIF:
		return "image/avif"
	case FormatWebP:
		return "image/webp"
	case FormatJPEG:
		return "image/jpeg"
	}
	return "application/octet-stream"
}

// Role is what an image is FOR, which is what decides its width ladder and aspect (§22). Roles
// are a closed set rather than a free string because the ladder is code — a caller inventing
// "banner" would silently get the fallback ladder and no one would notice.
type Role string

const (
	RolePoster   Role = "poster"   // 2:3 — channel tiles, title art
	RoleBackdrop Role = "backdrop" // 16:9 — episode stills, hero images
	RoleIcon     Role = "icon"     // channel icons and logos
	RoleThumb    Role = "thumb"    // clip stills in the filler catalog
)

// Image is one logical image: an original plus everything derived from or known about it.
//
// The bytes are NOT here. They are on disk, addressed by Hash — this struct is the record, and
// keeping them apart is what lets the GC delete a derivative without touching the row and lets a
// restore find rows whose files are gone (which is a state the system must handle, not prevent).
type Image struct {
	// Hash is the sha256 of the ORIGINAL bytes, hex-encoded. Identity, filename, ETag and cache
	// key all at once — which is what makes `immutable` an honest header rather than a wish.
	//
	// ⚠ Never equal to a path. §10 shipped two live bugs (DeleteClipsNotIn wiping the catalog;
	// the tagger never tagging) because fixtures set a clip's hash and path to the SAME STRING,
	// making a hash-keyed call and a path-keyed one indistinguishable to every test. Tests here
	// must derive one from the other and never equate them.
	Hash string

	Origin     Origin
	Visibility Visibility
	Role       Role

	// SourceURL is the upstream this was fetched from, for OriginRemote only. Empty otherwise.
	// This is what makes a remote image recoverable rather than merely cached.
	SourceURL string

	// MIME, Width, Height, Bytes describe the ORIGINAL, not any derivative. Width/Height are
	// served to the frontend so an <img> can carry real width/height attributes and contribute
	// zero layout shift.
	MIME   string
	Width  int
	Height int
	Bytes  int64

	// Animated marks an image whose original carries motion (the clip hover loop). Such images
	// have exactly one rendition and skip the ladder — resizing an animation per breakpoint
	// costs far more than it saves, and the hover asset is already sized for its card.
	Animated bool

	// Placeholder is a ThumbHash, base64-encoded (~25 bytes raw). Rendered as an LQIP while the
	// real image loads. ThumbHash over BlurHash because it carries ALPHA — channel logos are
	// routinely transparent PNGs, which BlurHash renders as black.
	Placeholder string

	// DominantHex is the average colour as "#rrggbb", used to tint a skeleton before even the
	// placeholder decodes.
	DominantHex string

	// Meta is free-form JSON: attribution, licence, upstream ids, generation parameters. Kept
	// opaque at this layer on purpose — the service must not grow a schema per producer.
	//
	// A string rather than []byte, matching the store: the Postgres column is JSONB, and a []byte
	// parameter binds as bytea while a string binds as text and casts cleanly.
	Meta string

	// OriginFetchedAt is when the bytes were obtained from upstream.
	//
	// ⚠ This exists to serve a COMPLIANCE ceiling, not a cache heuristic: TMDB's API terms
	// forbid caching their content for longer than six months, so remote images carry an expiry
	// the GC job enforces. It is on the row from the first migration because retrofitting expiry
	// into a content-addressed store, after a year of artwork has accumulated, is not something
	// adding a column later can fix.
	OriginFetchedAt time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
	// LastUsedAt drives derivative eviction under the cache budget.
	LastUsedAt time.Time
}

// Ref is what an image decorates. An image can have several (the same poster may be a channel's
// icon and a title's art), which is why this is a table rather than a column.
//
// ⚠ Refs live apart from the domains they point at so the GC can find orphans WITHOUT every
// domain knowing about images — the same reasoning that keeps clip identity out of the channel
// schema. A domain adds a ref; it never gains an images import.
type Ref struct {
	ImageHash string
	OwnerKind string // "channel" | "clip" | "title" | …
	OwnerID   string
	Role      Role
}

// Derivative is one encoded rendition on disk. Regenerable by construction: nothing here is
// worth backing up, and deleting one costs a re-encode, never data.
type Derivative struct {
	ImageHash string
	Format    Format
	Width     int
	Bytes     int64
	Path      string // relative to the images dir
	CreatedAt time.Time
}

// Rendition is what a serve handler needs: where the bytes are and what to call them. Returned
// rather than the bytes themselves so the handler can hand the file to http.ServeContent and get
// Range and conditional requests for free.
type Rendition struct {
	Path        string
	ContentType string
	Bytes       int64
	// Hash is the parent image's hash — the strong ETag. Carried here so the handler never has
	// to re-derive it from the path it was handed.
	Hash string
}
