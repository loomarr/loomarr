package images

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"golang.org/x/sync/singleflight"
)

// The service seam (§22). Callers hand over bytes (Ingest) or a URL (Adopt) and get an Image; they
// ask for a Rendition and get a file. Nothing outside this package knows the disk layout, the hash
// function, the ladder, or which encoder ran — which is what lets all of that change without a
// migration of call sites.

// Store is the persistence this service needs, declared HERE with this package's own types.
//
// ⚠ Narrow and locally-owned on purpose: no domain package in this codebase imports internal/store,
// and an adapter in internal/app bridges the two. Following that rule rather than reaching for the
// concrete store is what keeps this package testable without a database.
type Store interface {
	PutImage(ctx context.Context, img Image) error
	GetImage(ctx context.Context, hash string) (Image, error)
	// GetFetchedBySourceURL finds the row holding the bytes already downloaded from a source URL.
	// Adopt needs it because the fetch re-key deletes the row Adopt itself created — see there.
	GetFetchedBySourceURL(ctx context.Context, src string) (Image, error)
	TouchImage(ctx context.Context, hash string, at time.Time) error
	PutRef(ctx context.Context, ref Ref) error
	PutDerivative(ctx context.Context, d Derivative) error
	ListDerivatives(ctx context.Context, hash string) ([]Derivative, error)
}

// IngestRequest is everything about an image that its bytes do not say.
type IngestRequest struct {
	Role       Role
	Visibility Visibility
	Origin     Origin
	// Owner, when set, records a Ref in the same call — the common case, since an image almost
	// always arrives because something needs to display it.
	OwnerKind string
	OwnerID   string
	Meta      string
}

// Config is the service's tunables.
//
// ⚠ The shape encodes WHICH knobs hot-apply (config-design §3). `Dir` is a plain value because the
// blob store is built from it once, and re-pointing it at runtime would orphan every file already
// written — that one genuinely needs a restart. The rest are funcs, read per call, so an operator
// saving `server.public_url` in the wizard gets absolute image URLs on the next request instead of
// after a reboot. A struct of plain values would have quietly frozen all four at boot while this
// comment claimed otherwise, which is the failure worth designing out.
//
// All three funcs tolerate being nil; New fills in the declared defaults.
type Config struct {
	Dir string
	// MaxUploadBytes caps an ingested original. Enforced on the READ, never on a declared size.
	MaxUploadBytes func() int64
	// PublicBaseURL is the operator-configured server.public_url.
	//
	// ⚠ Never derived from request headers. Host and X-Forwarded-Host are attacker-controllable and
	// these URLs are STORED and fetched downstream by Tunarr, so a spoofed header would poison an
	// icon URL persistently. Empty falls back to a relative URL, which is safe and works whenever
	// the fetcher resolves Loomarr at the same origin.
	PublicBaseURL func() string
	// Formats is the rendition set, in <picture> preference order — `images.formats`.
	//
	// ⚠ This was declared and never read: the field existed, New defaulted it, and nothing
	// consulted it, so dropping `avif` or `jpeg` from the setting would have changed nothing while
	// the docs said it saved CPU or storage. `Produces` is the reader; the AVIF job and the record
	// handler both go through it.
	Formats func() []Format
}

// DefaultFormats is the rendition set when `images.formats` says nothing — §22's full ladder.
func DefaultFormats() []Format { return []Format{FormatAVIF, FormatWebP, FormatJPEG} }

// Service is the concrete implementation.
type Service struct {
	cfg   Config
	store Store
	blob  *blobStore
	// group collapses concurrent identical rendition requests into one encode. Without it, a cold
	// poster grid asking fifty browsers for the same width would run fifty identical encodes and
	// race each other writing the same file.
	group singleflight.Group
	now   func() time.Time
}

// New builds a Service. `now` is injectable because several behaviours here are time-dependent
// (fetch stamps, last-used) and a test that cannot control the clock asserts on wall time.
func New(cfg Config, store Store, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	if cfg.MaxUploadBytes == nil {
		cfg.MaxUploadBytes = func() int64 { return defaultMaxUploadBytes }
	}
	if cfg.PublicBaseURL == nil {
		cfg.PublicBaseURL = func() string { return "" }
	}
	if cfg.Formats == nil {
		cfg.Formats = DefaultFormats
	}
	return &Service{cfg: cfg, store: store, blob: newBlobStore(cfg.Dir), now: now}
}

// defaultMaxUploadBytes mirrors `images.max_upload_bytes` (§15). Declared here too so a Service
// built without a settings service — a test, or a boot with no store — still refuses an unbounded
// read rather than treating a nil func as "no limit".
const defaultMaxUploadBytes = 8 << 20

// Produces reports whether this install emits a format, per `images.formats`.
//
// The one reader of cfg.Formats, so the setting has exactly one meaning. AVIF asks before
// encoding; the record handler asks before advertising a <source> that will never exist.
func (s *Service) Produces(f Format) bool {
	for _, want := range s.cfg.Formats() {
		if want == f {
			return true
		}
	}
	return false
}

// HasFormat reports whether a rendition in this format ALREADY EXISTS for an image.
//
// ⚠ **This is the difference between "we would emit AVIF" and "AVIF is there right now", and
// conflating them shipped a bug that broke every image in the app.** `Produces` answers the first
// from `images.formats`; only this answers the second. AVIF is job-produced (§22 makes its coverage
// eventually consistent on purpose), so a freshly-ingested image has none for up to an hour.
//
// The consequence is not a missing optimisation, it is a BROKEN IMAGE: `<picture>` selects a source
// by declared `type` and COMMITS to it. Its fallback chain is for format SUPPORT, not for
// availability — a 404 on the chosen source renders nothing and does not fall through to the next
// one. Advertising an AVIF that 404s therefore breaks the image for every browser that supports
// AVIF, which is all current ones. Measured in a real browser: the request for `w92.avif` failed
// and the surface rendered its fallback while the WebP and JPEG renditions both served 200.
func (s *Service) HasFormat(ctx context.Context, hash string, f Format) (bool, error) {
	if !s.Produces(f) {
		return false, nil
	}
	ds, err := s.store.ListDerivatives(ctx, hash)
	if err != nil {
		return false, err
	}
	for _, d := range ds {
		if d.Format == f {
			return true, nil
		}
	}
	return false, nil
}

// Ingest stores bytes and returns the canonical record.
//
// Idempotent by construction: identical bytes produce an identical hash, so re-ingesting something
// already held updates the row's mutable half and rewrites nothing on disk. That is what makes the
// upload path safe to retry.
func (s *Service) Ingest(ctx context.Context, r io.Reader, req IngestRequest) (Image, error) {
	data, err := readCapped(r, s.cfg.MaxUploadBytes())
	if err != nil {
		return Image{}, err
	}

	// ⚠ Decode BEFORE storing. It is both the format allowlist (a sniff the client cannot lie past)
	// and the only proof the bytes are a real image rather than something merely labelled as one —
	// and storing first would mean a rejected upload had already touched the disk.
	img, mime, err := Decode(data)
	if err != nil {
		return Image{}, err
	}

	hash := HashBytes(data)
	dst, err := s.blob.OriginalPath(hash, extForMIME(mime))
	if err != nil {
		return Image{}, err
	}
	if _, exists := s.blob.Stat(dst); !exists {
		if err := s.blob.Write(dst, data); err != nil {
			return Image{}, err
		}
	}

	now := s.now()
	b := img.Bounds()
	rec := Image{
		Hash:        hash,
		Origin:      orDefault(req.Origin, OriginUpload),
		Visibility:  orDefault(req.Visibility, VisibilityMember),
		Role:        orDefault(req.Role, RoleIcon),
		MIME:        mime,
		Width:       b.Dx(),
		Height:      b.Dy(),
		Bytes:       int64(len(data)),
		Animated:    isAnimatedWebP(data, mime),
		Placeholder: Placeholder(img),
		DominantHex: DominantHex(img),
		Meta:        req.Meta,
		CreatedAt:   now,
		UpdatedAt:   now,
		LastUsedAt:  now,
		// Locally-produced bytes are "fetched" now: it is what the expiry sweep reads, and leaving
		// it zero would put an upload in the fetch job's queue forever.
		OriginFetchedAt: now,
	}
	if err := s.store.PutImage(ctx, rec); err != nil {
		return Image{}, err
	}
	if req.OwnerKind != "" && req.OwnerID != "" {
		if err := s.store.PutRef(ctx, Ref{
			ImageHash: hash, OwnerKind: req.OwnerKind, OwnerID: req.OwnerID, Role: rec.Role,
		}); err != nil {
			return Image{}, err
		}
	}
	return rec, nil
}

// Adopt records a remote image WITHOUT fetching it, and returns immediately.
//
// ⚠ **Returning before the bytes exist is the point.** The guide and the timeline render many
// images at once; blocking any of them on a TMDB round trip would put a third party's latency on
// Loomarr's own page load. The fetch job pulls the bytes, and until then Rendition has nothing to
// serve — so the frontend shows the placeholder, which is a better first paint than the remote
// fetch this replaces anyway.
//
// The identity problem: the content hash cannot be known before the bytes arrive, so the row is
// keyed on a hash of the SOURCE URL until the fetch replaces it. That is why `hashOfURL` exists and
// why it is namespaced — see its comment.
func (s *Service) Adopt(ctx context.Context, srcURL string, req IngestRequest) (Image, error) {
	u, err := url.Parse(srcURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return Image{}, fmt.Errorf("images: adopt needs an absolute https URL, got %q", srcURL)
	}

	// ⚠ **Check the FETCHED row first, and the order is the whole fix.** The placeholder lookup
	// below cannot find an image that already succeeded: the fetch re-keys the row onto the content
	// hash and deletes the URL-keyed one, so `GetImage(hashOfURL(src))` misses forever afterwards.
	// Without this, re-adopting a URL minted a second placeholder and the fetch job re-downloaded
	// bytes already on disk — once per adopt, forever. Harmless while nothing called Adopt twice;
	// a per-open cost the moment the icon picker did (V52 phase 7).
	if existing, err := s.store.GetFetchedBySourceURL(ctx, srcURL); err == nil {
		if req.OwnerKind != "" && req.OwnerID != "" {
			if err := s.store.PutRef(ctx, Ref{
				ImageHash: existing.Hash, OwnerKind: req.OwnerKind, OwnerID: req.OwnerID, Role: existing.Role,
			}); err != nil {
				return Image{}, err
			}
		}
		return existing, nil
	} else if !errors.Is(err, ErrNotFound) {
		return Image{}, err
	}

	hash := hashOfURL(srcURL)
	if existing, err := s.store.GetImage(ctx, hash); err == nil {
		return existing, nil // adopted and still pending; the fetch job owns it from here
	} else if !errors.Is(err, ErrNotFound) {
		return Image{}, err
	}

	now := s.now()
	rec := Image{
		Hash:       hash,
		Origin:     OriginRemote,
		SourceURL:  srcURL,
		Visibility: orDefault(req.Visibility, VisibilityMember),
		Role:       orDefault(req.Role, RolePoster),
		MIME:       mimeFromURLPath(u.Path),
		Meta:       req.Meta,
		CreatedAt:  now,
		UpdatedAt:  now,
		LastUsedAt: now,
		// ⚠ Left ZERO deliberately: it is the "never fetched" sentinel the fetch job selects on,
		// and it is also why the expiry sweep must exclude zero explicitly — otherwise every
		// pending image looks infinitely stale and the sweep purges the backlog.
		OriginFetchedAt: time.Time{},
	}
	if err := s.store.PutImage(ctx, rec); err != nil {
		return Image{}, err
	}
	if req.OwnerKind != "" && req.OwnerID != "" {
		if err := s.store.PutRef(ctx, Ref{
			ImageHash: hash, OwnerKind: req.OwnerKind, OwnerID: req.OwnerID, Role: rec.Role,
		}); err != nil {
			return Image{}, err
		}
	}
	return rec, nil
}

// Get returns a record.
func (s *Service) Get(ctx context.Context, hash string) (Image, error) {
	if !validHash(hash) {
		return Image{}, ErrNotFound // ⚠ never ErrBadHash to a caller: both are 404 to a client
	}
	return s.store.GetImage(ctx, hash)
}

// Rendition returns a servable file for a hash at a width in a format, generating it if needed.
//
// The generation split (§22): WebP and JPEG are produced here, synchronously, behind singleflight.
// AVIF is NOT — it is job-produced, and a missing AVIF returns ErrNotFound so the caller simply
// omits that <source> and the browser takes WebP.
func (s *Service) Rendition(ctx context.Context, hash string, f Format, width int) (Rendition, error) {
	rec, err := s.Get(ctx, hash)
	if err != nil {
		return Rendition{}, err
	}

	// ⚠ Snap the requested width to the role's ladder. Honouring an arbitrary width would let an
	// unauthenticated caller request ten thousand distinct sizes and make the box encode ten
	// thousand renditions — CPU and disk amplification with no login.
	w := rec.Role.NearestWidth(width)
	if rec.Animated {
		// An animation has one rendition and skips the ladder entirely; resizing it per breakpoint
		// costs far more than it saves.
		w = rec.Width
		if f == FormatWebP && rec.MIME == "image/webp" {
			// The original is already the card-sized animated WebP. Passing it through Decode +
			// Encode would preserve only frame zero, which is precisely the hover regression this
			// branch prevents. It is the one rendition; no derivative row or duplicate file exists.
			path, pathErr := s.blob.OriginalPath(rec.Hash, extForMIME(rec.MIME))
			if pathErr != nil {
				return Rendition{}, ErrNotFound
			}
			size, ok := s.blob.Stat(path)
			if !ok {
				return Rendition{}, ErrNotFound
			}
			_ = s.store.TouchImage(ctx, hash, s.now())
			return Rendition{Path: path, ContentType: rec.MIME, Bytes: size, Hash: hash}, nil
		}
	}

	dst, err := s.blob.DerivativePath(hash, w, f)
	if err != nil {
		return Rendition{}, ErrNotFound
	}
	if size, ok := s.blob.Stat(dst); ok {
		_ = s.store.TouchImage(ctx, hash, s.now())
		return Rendition{Path: dst, ContentType: f.MIME(), Bytes: size, Hash: hash}, nil
	}

	if f == FormatAVIF {
		// Job-only. Absent is a normal, expected state rather than an error worth logging.
		return Rendition{}, ErrNotFound
	}

	// Collapse concurrent identical requests: fifty browsers asking for the same cold poster must
	// produce one encode, not fifty racing writes to one path.
	key := dst
	v, err, _ := s.group.Do(key, func() (any, error) {
		return s.encodeRendition(ctx, rec, f, w, dst)
	})
	if err != nil {
		return Rendition{}, err
	}
	return v.(Rendition), nil
}

// encodeRendition does the actual work behind singleflight.
func (s *Service) encodeRendition(ctx context.Context, rec Image, f Format, w int, dst string) (Rendition, error) {
	origPath, err := s.blob.OriginalPath(rec.Hash, extForMIME(rec.MIME))
	if err != nil {
		return Rendition{}, err
	}
	if _, ok := s.blob.Stat(origPath); !ok {
		// The row exists but the bytes do not — a remote image not yet fetched, or an upload lost
		// with the image directory. Both are ErrNotFound to a caller; the GC job is what tells an
		// operator about the second, since it is the unrecoverable one.
		return Rendition{}, ErrNotFound
	}

	data, err := os.ReadFile(origPath) //nolint:gosec // path built from a validated hash
	if err != nil {
		return Rendition{}, fmt.Errorf("images: read original %s: %w", rec.Hash, err)
	}
	img, _, err := Decode(data)
	if err != nil {
		return Rendition{}, err
	}

	var buf bytes.Buffer
	if err := Encode(&buf, Resize(img, w), f); err != nil {
		return Rendition{}, err
	}
	out := buf.Bytes()
	if err := s.blob.Write(dst, out); err != nil {
		return Rendition{}, err
	}

	now := s.now()
	if err := s.store.PutDerivative(ctx, Derivative{
		ImageHash: rec.Hash, Format: f, Width: w,
		Bytes: int64(len(out)), Path: dst, CreatedAt: now,
	}); err != nil {
		return Rendition{}, err
	}
	_ = s.store.TouchImage(ctx, rec.Hash, now)

	return Rendition{Path: dst, ContentType: f.MIME(), Bytes: int64(len(out)), Hash: rec.Hash}, nil
}

// PathFor builds the same-origin path of one rendition for an in-app browser.
//
// A browser already has the right origin. Binding its image requests to server.public_url would
// make every real rendition fail when that machine-client address is container-only or otherwise
// unreachable from the viewer, while the inline ThumbHash misleadingly keeps painting.
func (s *Service) PathFor(hash string, width int, f Format) string {
	return fmt.Sprintf("/v1/images/%s/w%d.%s", hash, width, f.Ext())
}

// URLFor builds the public URL of one rendition for a machine/off-origin client.
//
// ⚠ Built from the operator-configured public base, never from a request header — see Config.
func (s *Service) URLFor(hash string, width int, f Format) string {
	base := strings.TrimRight(strings.TrimSpace(s.cfg.PublicBaseURL()), "/")
	return base + s.PathFor(hash, width, f)
}

// SrcSet builds a same-origin `srcset` value for a role's whole ladder in one format.
//
// Width descriptors (`w`), not density (`2x`): the browser multiplies our `sizes` by the device's
// DPR when choosing, so `w` already covers retina and a density list would be a second, redundant
// axis that cannot express a fluid grid.
func (s *Service) SrcSet(hash string, role Role, f Format) string {
	widths := role.Widths()
	parts := make([]string, 0, len(widths))
	for _, w := range widths {
		parts = append(parts, fmt.Sprintf("%s %dw", s.PathFor(hash, w, f), w))
	}
	return strings.Join(parts, ", ")
}

// hashOfURL is the placeholder identity for an adopted-but-unfetched image.
//
// ⚠ **Namespaced with a prefix so it can never collide with a real content hash.** Both are
// sha256 hex of the same length, and an adopted row whose id happened to equal some real image's
// content hash would make the fetch overwrite an unrelated image — silently, because the content
// address IS the identity. Hashing "url:"+the URL puts them in disjoint preimage spaces.
func hashOfURL(srcURL string) string { return HashBytes([]byte("url:" + srcURL)) }

// mimeFromURLPath guesses a stored extension for an adopted image before its bytes arrive. A guess
// is acceptable here and nowhere else: the fetch replaces it with the sniffed truth, and until then
// it only decides a filename.
func mimeFromURLPath(p string) string {
	switch strings.ToLower(path.Ext(p)) {
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	}
	return "image/jpeg"
}

func orDefault[T ~string](v, def T) T {
	if v == "" {
		return def
	}
	return v
}
