package images

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/mantonx/loomarr/internal/images/rustgen"
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
	PutDerivatives(ctx context.Context, derivatives []Derivative) error
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
// Both funcs tolerate being nil; New fills in the declared defaults.
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
	// Observer reports the bounded worker and queue vocabulary defined by V59a. It is optional;
	// nil callbacks make instrumentation a no-op without changing image behavior.
	Observer Observer
}

// Observer is the composition-root seam for image worker telemetry.
type Observer struct {
	QueueWait func(time.Duration)
	InFlight  func(delta int)
	Worker    func(rustgen.Observation)
}

// DefaultFormats is §22's fixed compatibility ladder.
func DefaultFormats() []Format { return []Format{FormatAVIF, FormatWebP, FormatJPEG} }

// Service is the concrete implementation.
type Service struct {
	cfg      Config
	store    Store
	blob     *blobStore
	renderer Renderer
	slots    chan struct{}
	// group collapses concurrent identical rendition requests into one encode. Without it, a cold
	// poster grid asking fifty browsers for the same width would run fifty identical encodes and
	// race each other writing the same file.
	group singleflight.Group
	now   func() time.Time
}

// Renderer is the required Rust worker seam. It is deliberately the complete operation rather
// than codec-shaped methods: product policy sends a plan; the worker owns all pixel work.
type Renderer interface {
	Generate(context.Context, rustgen.Request) (rustgen.Manifest, error)
}

const renditionRecipe = "loomarr-rendition-v2"

// New builds a Service. `now` is injectable because several behaviours here are time-dependent
// (fetch stamps, last-used) and a test that cannot control the clock asserts on wall time.
func New(cfg Config, store Store, renderer Renderer, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	if cfg.MaxUploadBytes == nil {
		cfg.MaxUploadBytes = func() int64 { return defaultMaxUploadBytes }
	}
	if cfg.PublicBaseURL == nil {
		cfg.PublicBaseURL = func() string { return "" }
	}
	workers := min(4, max(1, runtime.GOMAXPROCS(0)))
	return &Service{cfg: cfg, store: store, blob: newBlobStore(cfg.Dir), renderer: renderer,
		slots: make(chan struct{}, workers), now: now}
}

// defaultMaxUploadBytes mirrors `images.max_upload_bytes` (§15). Declared here too so a Service
// built without a settings service — a test, or a boot with no store — still refuses an unbounded
// read rather than treating a nil func as "no limit".
const defaultMaxUploadBytes = 8 << 20

// Produces reports whether the image module's fixed compatibility ladder emits a format.
func (s *Service) Produces(f Format) bool {
	for _, want := range DefaultFormats() {
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
// from the fixed compatibility ladder; only this answers the second. AVIF is job-produced (§22 makes its coverage
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
		if d.Recipe == renditionRecipe && d.Format == f {
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

	hash := HashBytes(data)
	manifest, cleanup, err := s.inspect(ctx, data, hash)
	if err != nil {
		return Image{}, err
	}
	defer cleanup()

	dst, err := s.blob.OriginalPath(hash, extForMIME(manifest.Source.MIME))
	if err != nil {
		return Image{}, err
	}
	if _, exists := s.blob.Stat(dst); !exists {
		if err := s.blob.Write(dst, data); err != nil {
			return Image{}, err
		}
	}

	now := s.now()
	rec := Image{
		Hash:        hash,
		Origin:      orDefault(req.Origin, OriginUpload),
		Visibility:  orDefault(req.Visibility, VisibilityMember),
		Role:        orDefault(req.Role, RoleIcon),
		MIME:        manifest.Source.MIME,
		Width:       manifest.Source.Width,
		Height:      manifest.Source.Height,
		Bytes:       int64(len(data)),
		Animated:    manifest.Source.Animated,
		FrameCount:  manifest.Source.FrameCount,
		DurationMS:  manifest.Source.DurationMS,
		LoopCount:   manifest.Source.LoopCount,
		Placeholder: manifest.Source.Placeholder,
		DominantHex: manifest.Source.DominantHex,
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

func (s *Service) inspect(ctx context.Context, data []byte, hash string) (rustgen.Manifest, func(), error) {
	if s.renderer == nil {
		return rustgen.Manifest{}, func() {}, fmt.Errorf("images: required Rust renderer unavailable")
	}
	stagingRoot := filepath.Join(s.cfg.Dir, "staging")
	if err := os.MkdirAll(stagingRoot, 0o750); err != nil {
		return rustgen.Manifest{}, func() {}, fmt.Errorf("images: create staging root: %w", err)
	}
	staging, err := os.MkdirTemp(stagingRoot, "request-*")
	if err != nil {
		return rustgen.Manifest{}, func() {}, fmt.Errorf("images: create staging: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(staging) }
	source := filepath.Join(staging, "source")
	if err := os.WriteFile(source, data, 0o600); err != nil {
		cleanup()
		return rustgen.Manifest{}, func() {}, fmt.Errorf("images: stage source: %w", err)
	}
	workerCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	manifest, err := s.generate(workerCtx, s.workerRequest(hash, source, staging, nil))
	if err != nil {
		cleanup()
		return rustgen.Manifest{}, func() {}, err
	}
	return manifest, cleanup, nil
}

func (s *Service) generate(ctx context.Context, request rustgen.Request) (rustgen.Manifest, error) {
	waitStarted := time.Now()
	select {
	case s.slots <- struct{}{}:
	case <-ctx.Done():
		if s.cfg.Observer.QueueWait != nil {
			s.cfg.Observer.QueueWait(time.Since(waitStarted))
		}
		return rustgen.Manifest{}, ctx.Err()
	}
	if s.cfg.Observer.QueueWait != nil {
		s.cfg.Observer.QueueWait(time.Since(waitStarted))
	}
	if s.cfg.Observer.InFlight != nil {
		s.cfg.Observer.InFlight(1)
	}
	defer func() {
		<-s.slots
		if s.cfg.Observer.InFlight != nil {
			s.cfg.Observer.InFlight(-1)
		}
	}()
	ctx = rustgen.WithObserver(ctx, s.cfg.Observer.Worker)
	return s.renderer.Generate(ctx, request)
}

func (s *Service) workerRequest(hash, source, staging string, targets []rustgen.Target) rustgen.Request {
	if targets == nil {
		targets = []rustgen.Target{}
	}
	return rustgen.Request{
		RequestID:  filepath.Base(staging),
		Source:     rustgen.Source{Path: source, ExpectedSHA256: hash},
		StagingDir: staging,
		Targets:    targets,
		Budget: rustgen.Budget{
			MaxInputBytes: s.cfg.MaxUploadBytes(), MaxWidth: 16_384, MaxHeight: 16_384,
			MaxCanvasPixels: 40_000_000, MaxFrames: 600,
			MaxTotalFramePixels: 600_000_000, MaxDurationMS: 60_000,
			MaxOutputBytes: 64 << 20,
		},
	}
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
	dst, err := s.blob.DerivativePath(hash, w, f)
	if err != nil {
		return Rendition{}, ErrNotFound
	}
	derivatives, err := s.store.ListDerivatives(ctx, hash)
	if err != nil {
		return Rendition{}, err
	}
	for _, derivative := range derivatives {
		if derivative.Recipe != renditionRecipe || derivative.Format != f || derivative.Width != w {
			continue
		}
		if size, ok := s.blob.Stat(derivative.Path); ok {
			_ = s.store.TouchImage(ctx, hash, s.now())
			return Rendition{Path: derivative.Path, ContentType: f.MIME(), Bytes: size, Hash: hash}, nil
		}
	}
	// A file without its provenance row is an interrupted publication. It is not accepted as a
	// cache hit: regenerate so output hash, recipe and animation state become durable together.
	_ = s.blob.Remove(dst)

	if f == FormatAVIF {
		// Job-only. Absent is a normal, expected state rather than an error worth logging.
		return Rendition{}, ErrNotFound
	}

	// Collapse concurrent identical requests: fifty browsers asking for the same cold poster must
	// produce one encode, not fifty racing writes to one path.
	key := dst
	v, err, _ := s.group.Do(key, func() (any, error) {
		return s.encodeRendition(ctx, rec, f, w)
	})
	if err != nil {
		return Rendition{}, err
	}
	return v.(Rendition), nil
}

// encodeRendition does the actual work behind singleflight.
func (s *Service) encodeRendition(ctx context.Context, rec Image, f Format, w int) (Rendition, error) {
	rends, err := s.encodeRenditions(ctx, rec, f, []int{w})
	if err != nil {
		return Rendition{}, err
	}
	return rends[0], nil
}

// encodeRenditions renders one same-format ladder in one worker process. The worker returns only a
// complete, validated manifest; Go retains ownership of canonical paths and durable publication.
func (s *Service) encodeRenditions(ctx context.Context, rec Image, f Format, widths []int) ([]Rendition, error) {
	origPath, err := s.blob.OriginalPath(rec.Hash, extForMIME(rec.MIME))
	if err != nil {
		return nil, err
	}
	if _, ok := s.blob.Stat(origPath); !ok {
		// The row exists but the bytes do not — a remote image not yet fetched, or an upload lost
		// with the image directory. Both are ErrNotFound to a caller; the GC job is what tells an
		// operator about the second, since it is the unrecoverable one.
		return nil, ErrNotFound
	}

	if s.renderer == nil {
		return nil, fmt.Errorf("images: required Rust renderer unavailable")
	}
	stagingRoot := filepath.Join(s.cfg.Dir, "staging")
	if err := os.MkdirAll(stagingRoot, 0o750); err != nil {
		return nil, fmt.Errorf("images: create staging root: %w", err)
	}
	staging, err := os.MkdirTemp(stagingRoot, "request-*")
	if err != nil {
		return nil, fmt.Errorf("images: create staging: %w", err)
	}
	defer func() { _ = os.RemoveAll(staging) }()
	motion := "first_frame"
	if rec.Animated && f == FormatWebP {
		motion = "preserve"
	}
	targets := make([]rustgen.Target, 0, len(widths))
	for _, width := range widths {
		targets = append(targets, rustgen.Target{
			ID: fmt.Sprintf("%s-w%d", f, width), Format: string(f), Width: width, Motion: motion,
		})
	}
	timeout := 30 * time.Second
	if f == FormatAVIF || rec.Animated {
		timeout = 2 * time.Minute
	}
	workerCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	manifest, err := s.generate(workerCtx, s.workerRequest(rec.Hash, origPath, staging, targets))
	if err != nil {
		return nil, err
	}

	rends := make([]Rendition, 0, len(manifest.Outputs))
	derivatives := make([]Derivative, 0, len(manifest.Outputs))
	published := make([]string, 0, len(manifest.Outputs))
	now := s.now()
	rollback := func() {
		for _, path := range published {
			_ = s.blob.Remove(path)
		}
	}
	for _, output := range manifest.Outputs {
		dst, pathErr := s.blob.DerivativePath(rec.Hash, output.RequestedWidth, f)
		if pathErr != nil {
			rollback()
			return nil, pathErr
		}
		if err := s.blob.Promote(filepath.Join(staging, output.RelativePath), dst); err != nil {
			rollback()
			return nil, err
		}
		published = append(published, dst)

		derivatives = append(derivatives, Derivative{
			ImageHash: rec.Hash, Recipe: output.RecipeID, Format: f, Width: output.RequestedWidth,
			Bytes: output.Bytes, OutputHash: output.SHA256, Path: dst,
			Animated: output.Animated, CreatedAt: now,
		})
		rends = append(rends, Rendition{Path: dst, ContentType: f.MIME(), Bytes: output.Bytes, Hash: rec.Hash})
	}
	if err := s.store.PutDerivatives(ctx, derivatives); err != nil {
		rollback()
		return nil, err
	}
	_ = s.store.TouchImage(ctx, rec.Hash, now)
	return rends, nil
}

// PathFor builds the same-origin path of one rendition for an in-app browser.
//
// A browser already has the right origin. Binding its image requests to server.public_url would
// make every real rendition fail when that machine-client address is container-only or otherwise
// unreachable from the viewer, while the inline ThumbHash misleadingly keeps painting.
func (s *Service) PathFor(hash string, width int, f Format) string {
	return fmt.Sprintf("/v1/images/%s/w%d.%s?r=%s", hash, width, f.Ext(), renditionRecipe)
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
