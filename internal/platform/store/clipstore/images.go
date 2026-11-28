package clipstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// The image service's persistence (§22, V52). One record per logical image; the BYTES are on disk
// under a content hash and never in this database.
//
// ⚠ That is the deliberate reversal of `channel_icons`, which stored upload bytes here precisely so retired-ok
// they would ride the §16 backup. It worked, and it makes the database the wrong shape for a general
// image service — so §22 accepts the narrower guarantee instead: an operator upload is the one thing
// that does not survive losing /data/images, which is why `ListUnrecoverableImages` exists for the
// GC job to warn about rather than for anything to repair.

// Image is one logical image's record. Mirrors the `images` table; the domain type in
// internal/media/images is separate and an adapter bridges them, following the same rule every other
// domain here follows — no domain package imports this one.
type Image struct {
	Hash       string
	Origin     string // upload | remote | extracted | generated
	SourceURL  string
	Visibility string // public | member
	Role       string

	MIME   string
	Width  int
	Height int
	Bytes  int64

	Animated    bool
	FrameCount  int
	DurationMS  int64
	LoopCount   *int
	Placeholder string
	DominantHex string
	// Meta is a JSON document. Carried as a string rather than []byte because the Postgres column
	// is JSONB and a []byte parameter binds as bytea; a string binds as text and casts cleanly.
	Meta string

	OriginFetchedAt time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	LastUsedAt      time.Time

	// FetchFailures counts the downloads that failed in a row, FetchFailedAt is when the last was
	// tried, and FetchRetryAt is when the fetch queue takes the image again; zero for none. A write
	// whose OriginFetchedAt is set clears all three: the bytes landed.
	FetchFailures int
	FetchFailedAt time.Time
	FetchRetryAt  time.Time
	// FetchQueuedAt is when the image joined the fetch queue: its adoption, or the expiry purge's
	// requeue. The queue is served in this order. A pending row written with none joins at its
	// CreatedAt.
	FetchQueuedAt time.Time
	// FetchPriority orders the fetch queue ahead of FetchQueuedAt: 0 adopted artwork, 1 a refresh
	// the expiry purge requeued, 2 bulk artwork fetched last.
	FetchPriority int
}

// ImageRef records what an image decorates. OwnerKind picks the owner's table (imageOwners); a
// kind with no table is refused, because nothing could ever delete its refs.
type ImageRef struct {
	ImageHash string
	OwnerKind string
	OwnerID   string
	Role      string
}

// imageOwner is where one owner kind's image refs live: a table whose owner column is a foreign
// key to the owner, so deleting the owner deletes its refs and frees its images for the GC.
type imageOwner struct {
	kind, table, column string
}

// imageOwners is every kind of thing an image can decorate (#2793).
var imageOwners = []imageOwner{
	{"clip", "clip_images", "clip_hash"},
	{"filler_split_proposal", "filler_split_proposal_images", "proposal_id"},
	{"channel", "channel_images", "channel_id"},
	{"library_item", "library_item_images", "item_id"},
	{"library_candidate", "library_candidate_images", "item_id"},
	{"library_person", "library_person_images", "person_id"},
}

// ImageOwnerKinds lists the owner kinds an image ref may name, in table order.
func ImageOwnerKinds() []string {
	kinds := make([]string, len(imageOwners))
	for i, o := range imageOwners {
		kinds[i] = o.kind
	}
	return kinds
}

func ownerOf(kind string) (imageOwner, error) {
	for _, o := range imageOwners {
		if o.kind == kind {
			return o, nil
		}
	}
	return imageOwner{}, fmt.Errorf("image owner kind %q has no table", kind)
}

// ImageDerivative is one rendition on disk. Regenerable — deleting a row costs a re-encode.
type ImageDerivative struct {
	ImageHash  string
	Recipe     string
	Format     string
	Width      int
	Bytes      int64
	OutputHash string
	Path       string
	Animated   bool
	CreatedAt  time.Time
}

// ExpiryCursor is a position in the expiry sweep's order: the fetch time and hash of the last row
// read. The zero cursor is the start.
type ExpiryCursor struct {
	FetchedAt time.Time
	Hash      string
}

// ImageStore is the image half of the Store contract.
type ImageStore interface {
	PutImage(ctx context.Context, img Image) error
	GetImage(ctx context.Context, hash string) (Image, error)
	// GetImageBySourceURL finds the image of a source URL, so a re-adopt reuses it instead of
	// queueing a second download: the row whose bytes have landed if there is one, else the row
	// still waiting for them (an adopted placeholder, or an image the expiry purge requeued under
	// its content hash). ErrNotFound when no row has that URL.
	GetImageBySourceURL(ctx context.Context, src string) (Image, error)
	DeleteImage(ctx context.Context, hash string) error
	TouchImage(ctx context.Context, hash string, at time.Time) error

	// ListImagesAwaitingFetch returns remote images whose bytes have not landed, by fetch priority
	// and then by when each joined the queue. The fetch job's work list; `origin_fetched_at = 0` is
	// the "never fetched" sentinel. An image whose last download failed waits until its
	// fetch_retry_at is at or before now.
	ListImagesAwaitingFetch(ctx context.Context, now time.Time, limit int) ([]Image, error)
	// RecordImageFetchFailure counts one more failed download of a never-fetched image, at, and
	// holds it off the fetch queue until retryAt. An image whose bytes have landed is left alone.
	RecordImageFetchFailure(ctx context.Context, hash string, at, retryAt time.Time) error
	// DeferImageFetch holds a never-fetched image off the fetch queue until retryAt without
	// counting a failure: the fetch pass that tried it found the network down.
	DeferImageFetch(ctx context.Context, hash string, retryAt time.Time) error
	// ListImagesExpiredBefore returns remote images fetched before a cutoff — the TMDB six-month
	// compliance sweep — in (origin_fetched_at, hash) order, starting after the cursor. The sweep
	// passes the last row of each page as the next cursor, so a row it failed to purge is passed
	// over instead of read again.
	ListImagesExpiredBefore(ctx context.Context, cutoff time.Time, after ExpiryCursor, limit int) ([]Image, error)
	// ListOrphanImages returns images no longer referenced by anything.
	ListOrphanImages(ctx context.Context, limit int) ([]Image, error)
	// ListUnrecoverableImages returns images whose bytes cannot be re-obtained (upload/generated).
	// The GC checks these against the disk and warns about any whose file is gone.
	ListUnrecoverableImages(ctx context.Context, limit int) ([]Image, error)

	PutImageRef(ctx context.Context, ref ImageRef) error
	DeleteImageRefs(ctx context.Context, ownerKind, ownerID string) error
	// DeleteImageRefsByRole lets go of what an owner holds in one role (a title's poster), leaving
	// its other roles (its logo, its backdrop) alone.
	DeleteImageRefsByRole(ctx context.Context, ownerKind, ownerID, role string) error
	ImagesForOwner(ctx context.Context, ownerKind, ownerID string) ([]Image, error)

	// ListImagesByOrigin returns images of one origin, oldest first. The rehydrate job's work
	// list: "every remote image" is a set no other method describes, because
	// ListImagesAwaitingFetch is scoped to the never-fetched sentinel and the expiry sweep is
	// scoped to a cutoff, while a file can go missing under a row in either state.
	ListImagesByOrigin(ctx context.Context, origin string, limit int) ([]Image, error)

	// RepointImageRefs moves every ref from one image hash to another.
	//
	// ⚠ This exists for exactly one caller and it is not a convenience. `Adopt` keys a
	// not-yet-fetched remote row on a hash of its SOURCE URL, because the content hash cannot be
	// known before the bytes arrive; when the fetch lands, identity becomes the real content hash
	// or `Cache-Control: immutable` stops being true. Re-keying a row that an image ref already
	// points at needs the refs to move with it, and a naive UPDATE collides whenever two source
	// URLs turn out to hold the same bytes for the same owner — so this is insert-then-delete,
	// not an update.
	RepointImageRefs(ctx context.Context, from, to string) error

	PutImageDerivative(ctx context.Context, d ImageDerivative) error
	PutImageDerivatives(ctx context.Context, derivatives []ImageDerivative) error
	ListImageDerivatives(ctx context.Context, hash string) ([]ImageDerivative, error)
	// ImagesByHash reads the rows of hashes and every derivative of them, in a bounded number of
	// statements (two per imagesByHashChunk hashes). A hash with no row is absent; the images come
	// in hash order and the derivatives by image, recipe, format and width.
	ImagesByHash(ctx context.Context, hashes []string) ([]Image, []ImageDerivative, error)
	DeleteImageDerivatives(ctx context.Context, hash string) error
	// DeleteImageDerivative removes one rendition's row. The GC's eviction unit: a derivative is
	// regenerable, so dropping one costs a re-encode and never data.
	DeleteImageDerivative(ctx context.Context, hash, recipe, format string, width int) error
	// TotalImageDerivativeBytes is the disk the cache budget is measured against.
	//
	// ⚠ Sums the RECORDED sizes rather than walking the directory. The two can disagree — a file
	// written and never recorded, or a row whose file was removed by hand — and the row is the
	// right authority here because it is what the eviction loop deletes: a budget computed from
	// bytes the GC cannot free would make the loop run forever without ever getting under it.
	TotalImageDerivativeBytes(ctx context.Context) (int64, error)
	// ListColdestDerivatives returns derivatives ordered by their parent image's last use,
	// coldest first — the eviction order.
	//
	// ⚠ Ordered by the IMAGE's last_used_at, not the derivative's own created_at, and the
	// difference is the whole policy (V52 phase 3b). `image_derivatives` has no last_used_at and
	// deliberately gains none: a per-derivative LRU would make every image request a row WRITE,
	// so a fifty-poster grid would be fifty writes per page load against SQLite's single writer.
	// Image-level LRU costs one write per image request, which `TouchImage` already does.
	ListColdestDerivatives(ctx context.Context, limit int) ([]ImageDerivative, error)
	// ListImagesMissingFormat returns images that have at least one derivative but lack, in the
	// given format, a rung of rungs they are owed — the AVIF job's work list, expressed without the
	// job knowing SQL. An animated image is never listed.
	ListImagesMissingFormat(ctx context.Context, recipe, format string, rungs []ImageRung, limit int) ([]Image, error)
}

// ImageRung is one rung an image of Role is owed once its original is at least MinSourceWidth
// wide; the image package derives the set from its ladders.
type ImageRung struct {
	Role           string
	Width          int
	MinSourceWidth int
}

const imageColumns = `hash, origin, source_url, visibility, role, mime, width, height, bytes,
	animated, frame_count, duration_ms, loop_count, placeholder, dominant_hex, meta,
	origin_fetched_at, created_at, updated_at, last_used_at, fetch_failures, fetch_failed_at, fetch_retry_at,
	fetch_queued_at, fetch_priority`

func (s *Store) PutImage(ctx context.Context, img Image) error {
	if epoch(img.OriginFetchedAt) > 0 {
		// Bytes that landed end the failure record.
		img.FetchFailures, img.FetchFailedAt, img.FetchRetryAt = 0, time.Time{}, time.Time{}
	} else if img.FetchQueuedAt.IsZero() {
		img.FetchQueuedAt = img.CreatedAt
	}
	_, err := s.h.ExecContext(ctx, s.h.Rebind(
		`INSERT INTO images (`+imageColumns+`)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (hash) DO UPDATE SET
		     origin = excluded.origin,
		     source_url = excluded.source_url,
		     visibility = excluded.visibility,
		     role = excluded.role,
		     mime = excluded.mime,
		     width = excluded.width,
		     height = excluded.height,
		     bytes = excluded.bytes,
		     animated = excluded.animated,
		     frame_count = excluded.frame_count,
		     duration_ms = excluded.duration_ms,
		     loop_count = excluded.loop_count,
		     placeholder = excluded.placeholder,
		     dominant_hex = excluded.dominant_hex,
		     meta = excluded.meta,
		     origin_fetched_at = excluded.origin_fetched_at,
		     updated_at = excluded.updated_at,
		     fetch_failures = excluded.fetch_failures,
		     fetch_failed_at = excluded.fetch_failed_at,
		     fetch_retry_at = excluded.fetch_retry_at,
		     fetch_queued_at = excluded.fetch_queued_at,
		     fetch_priority = excluded.fetch_priority`),
		img.Hash, img.Origin, img.SourceURL, img.Visibility, img.Role, img.MIME,
		img.Width, img.Height, img.Bytes, img.Animated, stillFrameCount(img.FrameCount), img.DurationMS,
		img.LoopCount, img.Placeholder, img.DominantHex,
		metaOrEmpty(img.Meta), epoch(img.OriginFetchedAt), epoch(img.CreatedAt),
		epoch(img.UpdatedAt), epoch(img.LastUsedAt),
		img.FetchFailures, epoch(img.FetchFailedAt), epoch(img.FetchRetryAt), epoch(img.FetchQueuedAt), img.FetchPriority)
	if err != nil {
		return fmt.Errorf("put image %s: %w", img.Hash, err)
	}
	return nil
}

// ⚠ `created_at` and `last_used_at` are deliberately absent from the DO UPDATE list. Re-ingesting
// identical bytes yields the identical hash — that is the whole point of content addressing — so an
// upsert is a re-import of something already held, and letting it reset the creation time would
// make the row lie about its own age. The GC's expiry sweep reads exactly these columns.
// Single-writer columns omitted from an upsert is the same rule V44's clip metadata follows.

func (s *Store) GetImage(ctx context.Context, hash string) (Image, error) {
	row := s.h.QueryRowContext(ctx, s.h.Rebind(
		`SELECT `+imageColumns+` FROM images WHERE hash = ?`), hash)
	img, err := scanImage(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Image{}, ErrNotFound
	}
	if err != nil {
		return Image{}, fmt.Errorf("get image %s: %w", hash, err)
	}
	return img, nil
}

// GetFetchedImageBySourceURL resolves a source URL to the row that actually holds its bytes.
//
// ⚠ **`origin_fetched_at > 0` is the load-bearing half, not a filter for tidiness.** Two rows share
// a source URL by design for the width of a re-key — `fetchOne` writes the content-hash row before
// deleting the URL-keyed placeholder, so that no ref ever points at a hash with no row. Asking only
// for fetched rows collapses that to at most one, and it is also the only answer a caller can use:
// a placeholder's hash is about to stop resolving.
//
// ORDER BY makes the result deterministic rather than incidentally-single, which is the property a
// later change to the re-key ordering would otherwise break silently.
func (s *Store) GetImageBySourceURL(ctx context.Context, src string) (Image, error) {
	if src == "" {
		return Image{}, ErrNotFound // every non-remote row has source_url '' — never a match
	}
	// A fetched row sorts before a pending one (origin_fetched_at 0), and the latest fetch first.
	row := s.h.QueryRowContext(ctx, s.h.Rebind(
		`SELECT `+imageColumns+` FROM images
		 WHERE source_url = ?
		 ORDER BY origin_fetched_at DESC, hash LIMIT 1`), src)
	img, err := scanImage(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Image{}, ErrNotFound
	}
	if err != nil {
		return Image{}, fmt.Errorf("get image by source url: %w", err)
	}
	return img, nil
}

func (s *Store) DeleteImage(ctx context.Context, hash string) error {
	// Refs and derivatives cascade (ON DELETE CASCADE), so this is one statement rather than a
	// transaction the caller could get half-right.
	if _, err := s.h.ExecContext(ctx, s.h.Rebind(`DELETE FROM images WHERE hash = ?`), hash); err != nil {
		return fmt.Errorf("delete image %s: %w", hash, err)
	}
	return nil
}

func (s *Store) TouchImage(ctx context.Context, hash string, at time.Time) error {
	if _, err := s.h.ExecContext(ctx, s.h.Rebind(
		`UPDATE images SET last_used_at = ? WHERE hash = ?`), epoch(at), hash); err != nil {
		return fmt.Errorf("touch image %s: %w", hash, err)
	}
	return nil
}

// awaitingFetchQuery is the fetch queue's read: by priority, then by when an image joined the
// queue. idx_images_fetch_queue leads with its two equalities and then this order, so a pass reads
// the front of the queue instead of sorting every waiting row.
const awaitingFetchQuery = `SELECT ` + imageColumns + ` FROM images
	WHERE origin = 'remote' AND origin_fetched_at = 0 AND fetch_retry_at <= ?
	ORDER BY fetch_priority, fetch_queued_at, hash LIMIT ?`

// AwaitingFetchQuery is the fetch queue's SQL, for the store's query-plan check.
func AwaitingFetchQuery() string { return awaitingFetchQuery }

func (s *Store) ListImagesAwaitingFetch(ctx context.Context, now time.Time, limit int) ([]Image, error) {
	return s.queryImages(ctx, awaitingFetchQuery, epoch(now), limit)
}

func (s *Store) RecordImageFetchFailure(ctx context.Context, hash string, at, retryAt time.Time) error {
	if _, err := s.h.ExecContext(ctx, s.h.Rebind(
		`UPDATE images SET fetch_failures = fetch_failures + 1, fetch_failed_at = ?, fetch_retry_at = ?
		 WHERE hash = ? AND origin_fetched_at = 0`), epoch(at), epoch(retryAt), hash); err != nil {
		return fmt.Errorf("record image fetch failure %s: %w", hash, err)
	}
	return nil
}

func (s *Store) DeferImageFetch(ctx context.Context, hash string, retryAt time.Time) error {
	if _, err := s.h.ExecContext(ctx, s.h.Rebind(
		`UPDATE images SET fetch_retry_at = ? WHERE hash = ? AND origin_fetched_at = 0`), epoch(retryAt), hash); err != nil {
		return fmt.Errorf("defer image fetch %s: %w", hash, err)
	}
	return nil
}

func (s *Store) ListImagesExpiredBefore(ctx context.Context, cutoff time.Time, after ExpiryCursor, limit int) ([]Image, error) {
	// origin_fetched_at > 0 excludes the never-fetched rows, which are the fetch job's business
	// rather than the expiry sweep's — otherwise every pending image would look infinitely stale.
	// The zero cursor sorts before every fetched row (origin_fetched_at 0, hash '').
	at := epoch(after.FetchedAt)
	return s.queryImages(ctx,
		`SELECT `+imageColumns+` FROM images
		 WHERE origin = 'remote' AND origin_fetched_at > 0 AND origin_fetched_at < ?
		   AND (origin_fetched_at > ? OR (origin_fetched_at = ? AND hash > ?))
		 ORDER BY origin_fetched_at, hash LIMIT ?`, epoch(cutoff), at, at, after.Hash, limit)
}

func (s *Store) ListOrphanImages(ctx context.Context, limit int) ([]Image, error) {
	var unowned strings.Builder
	for _, o := range imageOwners {
		fmt.Fprintf(&unowned, " AND NOT EXISTS (SELECT 1 FROM %s r WHERE r.image_hash = i.hash)", o.table)
	}
	return s.queryImages(ctx,
		`SELECT `+imageColumns+` FROM images i WHERE TRUE`+unowned.String()+`
		 ORDER BY i.created_at LIMIT ?`, limit)
}

func (s *Store) ListUnrecoverableImages(ctx context.Context, limit int) ([]Image, error) {
	return s.queryImages(ctx,
		`SELECT `+imageColumns+` FROM images
		 WHERE origin IN ('upload', 'generated')
		 ORDER BY created_at LIMIT ?`, limit)
}

func (s *Store) ListImagesMissingFormat(ctx context.Context, recipe, format string, rungs []ImageRung, limit int) ([]Image, error) {
	if len(rungs) == 0 {
		return nil, nil
	}
	// "Has some derivative but lacks a rung it is owed in this format." The first EXISTS is what
	// keeps the AVIF job from picking up images whose bytes have not been processed at all yet —
	// those belong to the fetch job, and encoding AVIF for an image with no WebP would invert the
	// priority the split exists to create. A rung is owed when the image's role has it and the
	// original is at least its MinSourceWidth wide, or of unknown width (0), which is owed every
	// rung. An animated image is never encoded as AVIF, so it is never on the list: otherwise the
	// oldest animated images would fill every batch and the job would make no progress.
	var owed strings.Builder
	args := make([]any, 0, 5*len(rungs)+2)
	args = append(args, false)
	for i, r := range rungs {
		if i > 0 {
			owed.WriteString(" OR ")
		}
		owed.WriteString(`(i.role = ? AND (i.width >= ? OR i.width <= 0) AND NOT EXISTS (
			SELECT 1 FROM image_derivatives d2
			WHERE d2.image_hash = i.hash AND d2.recipe = ? AND d2.format = ? AND d2.width = ?))`)
		args = append(args, r.Role, r.MinSourceWidth, recipe, format, r.Width)
	}
	args = append(args, limit)
	return s.queryImages(ctx,
		`SELECT `+imageColumns+` FROM images i
		 WHERE i.animated = ?
		   AND EXISTS (SELECT 1 FROM image_derivatives d WHERE d.image_hash = i.hash)
		   AND (`+owed.String()+`)
		 ORDER BY i.created_at LIMIT ?`, args...)
}

func (s *Store) PutImageRef(ctx context.Context, ref ImageRef) error {
	o, err := ownerOf(ref.OwnerKind)
	if err != nil {
		return fmt.Errorf("put image ref %s->%s/%s: %w", ref.ImageHash, ref.OwnerKind, ref.OwnerID, err)
	}
	_, err = s.h.ExecContext(ctx, s.h.Rebind(
		`INSERT INTO `+o.table+` (`+o.column+`, role, image_hash) VALUES (?, ?, ?)
		 ON CONFLICT (`+o.column+`, role, image_hash) DO NOTHING`),
		ref.OwnerID, ref.Role, ref.ImageHash)
	if err != nil {
		return fmt.Errorf("put image ref %s->%s/%s: %w", ref.ImageHash, ref.OwnerKind, ref.OwnerID, err)
	}
	return nil
}

func (s *Store) DeleteImageRefs(ctx context.Context, ownerKind, ownerID string) error {
	o, err := ownerOf(ownerKind)
	if err == nil {
		_, err = s.h.ExecContext(ctx, s.h.Rebind(`DELETE FROM `+o.table+` WHERE `+o.column+` = ?`), ownerID)
	}
	if err != nil {
		return fmt.Errorf("delete image refs %s/%s: %w", ownerKind, ownerID, err)
	}
	return nil
}

func (s *Store) DeleteImageRefsByRole(ctx context.Context, ownerKind, ownerID, role string) error {
	o, err := ownerOf(ownerKind)
	if err == nil {
		_, err = s.h.ExecContext(ctx, s.h.Rebind(
			`DELETE FROM `+o.table+` WHERE `+o.column+` = ? AND role = ?`), ownerID, role)
	}
	if err != nil {
		return fmt.Errorf("delete %s image refs %s/%s: %w", role, ownerKind, ownerID, err)
	}
	return nil
}

func (s *Store) ImagesForOwner(ctx context.Context, ownerKind, ownerID string) ([]Image, error) {
	o, err := ownerOf(ownerKind)
	if err != nil {
		return nil, fmt.Errorf("images for %s/%s: %w", ownerKind, ownerID, err)
	}
	return s.queryImages(ctx,
		`SELECT `+prefixed(imageColumns, "i.")+` FROM images i
		 JOIN `+o.table+` r ON r.image_hash = i.hash
		 WHERE r.`+o.column+` = ?
		 ORDER BY i.created_at`, ownerID)
}

func (s *Store) PutImageDerivative(ctx context.Context, d ImageDerivative) error {
	_, err := s.h.ExecContext(ctx, s.h.Rebind(
		`INSERT INTO image_derivatives (image_hash, recipe, format, width, bytes, output_hash, path, animated, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (image_hash, recipe, format, width) DO UPDATE SET
		     bytes = excluded.bytes, output_hash = excluded.output_hash, path = excluded.path,
		     animated = excluded.animated, created_at = excluded.created_at`),
		d.ImageHash, derivativeRecipe(d.Recipe), d.Format, d.Width, d.Bytes, d.OutputHash, d.Path,
		d.Animated, epoch(d.CreatedAt))
	if err != nil {
		return fmt.Errorf("put derivative %s %s w%d: %w", d.ImageHash, d.Format, d.Width, err)
	}
	return nil
}

// PutImageDerivatives publishes one complete Rendition ladder transactionally. The worker manifest
// is all-or-nothing, so persistence must not turn a valid manifest into a partially visible ladder.
func (s *Store) PutImageDerivatives(ctx context.Context, derivatives []ImageDerivative) error {
	if len(derivatives) == 0 {
		return nil
	}
	tx, err := s.h.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin derivative batch: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	query := s.h.Rebind(
		`INSERT INTO image_derivatives (image_hash, recipe, format, width, bytes, output_hash, path, animated, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (image_hash, recipe, format, width) DO UPDATE SET
		     bytes = excluded.bytes, output_hash = excluded.output_hash, path = excluded.path,
		     animated = excluded.animated, created_at = excluded.created_at`)
	for _, d := range derivatives {
		if _, err := tx.ExecContext(ctx, query,
			d.ImageHash, derivativeRecipe(d.Recipe), d.Format, d.Width, d.Bytes, d.OutputHash, d.Path,
			d.Animated, epoch(d.CreatedAt)); err != nil {
			return fmt.Errorf("put derivative batch %s %s w%d: %w", d.ImageHash, d.Format, d.Width, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit derivative batch: %w", err)
	}
	return nil
}

func (s *Store) ListImageDerivatives(ctx context.Context, hash string) ([]ImageDerivative, error) {
	rows, err := s.h.QueryContext(ctx, s.h.Rebind(
		`SELECT `+derivativeColumns+` FROM image_derivatives WHERE image_hash = ? ORDER BY recipe, format, width`), hash)
	if err != nil {
		return nil, fmt.Errorf("list derivatives %s: %w", hash, err)
	}
	return scanDerivatives(rows)
}

// imagesByHashChunk bounds one statement's IN list, well under SQLite's 32,766 bound parameters
// and Postgres's 65,535: a read of n hashes is two statements per chunk.
const imagesByHashChunk = 500

func (s *Store) ImagesByHash(ctx context.Context, hashes []string) ([]Image, []ImageDerivative, error) {
	// Sorted and unique, so a hash asked for twice in different chunks comes back once and the
	// result stays in hash order across chunks.
	hashes = slices.Compact(slices.Sorted(slices.Values(hashes)))
	var imgs []Image
	var derivatives []ImageDerivative
	for start := 0; start < len(hashes); start += imagesByHashChunk {
		chunk := hashes[start:min(start+imagesByHashChunk, len(hashes))]
		in := strings.TrimSuffix(strings.Repeat("?, ", len(chunk)), ", ")
		args := make([]any, len(chunk))
		for i, h := range chunk {
			args[i] = h
		}
		got, err := s.queryImages(ctx, `SELECT `+imageColumns+` FROM images WHERE hash IN (`+in+`) ORDER BY hash`, args...)
		if err != nil {
			return nil, nil, err
		}
		imgs = append(imgs, got...)
		rows, err := s.h.QueryContext(ctx, s.h.Rebind(`SELECT `+derivativeColumns+` FROM image_derivatives
			WHERE image_hash IN (`+in+`) ORDER BY image_hash, recipe, format, width`), args...)
		if err != nil {
			return nil, nil, fmt.Errorf("list derivatives by hash: %w", err)
		}
		ds, err := scanDerivatives(rows)
		if err != nil {
			return nil, nil, err
		}
		derivatives = append(derivatives, ds...)
	}
	return imgs, derivatives, nil
}

const derivativeColumns = `image_hash, recipe, format, width, bytes, output_hash, path, animated, created_at`

// scanDerivatives scans rows of derivativeColumns and closes them.
func scanDerivatives(rows *sql.Rows) ([]ImageDerivative, error) {
	defer func() { _ = rows.Close() }()
	var out []ImageDerivative
	for rows.Next() {
		var d ImageDerivative
		var created int64
		if err := rows.Scan(&d.ImageHash, &d.Recipe, &d.Format, &d.Width, &d.Bytes, &d.OutputHash,
			&d.Path, &d.Animated, &created); err != nil {
			return nil, fmt.Errorf("scan derivative: %w", err)
		}
		d.CreatedAt = time.Unix(created, 0)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) DeleteImageDerivatives(ctx context.Context, hash string) error {
	_, err := s.h.ExecContext(ctx, s.h.Rebind(
		`DELETE FROM image_derivatives WHERE image_hash = ?`), hash)
	if err != nil {
		return fmt.Errorf("delete derivatives %s: %w", hash, err)
	}
	return nil
}

func (s *Store) DeleteImageDerivative(ctx context.Context, hash, recipe, format string, width int) error {
	_, err := s.h.ExecContext(ctx, s.h.Rebind(
		`DELETE FROM image_derivatives WHERE image_hash = ? AND recipe = ? AND format = ? AND width = ?`),
		hash, recipe, format, width)
	if err != nil {
		return fmt.Errorf("delete derivative %s %s w%d: %w", hash, format, width, err)
	}
	return nil
}

func (s *Store) TotalImageDerivativeBytes(ctx context.Context) (int64, error) {
	// COALESCE because SUM over no rows is NULL in both dialects, and a fresh install has no
	// derivatives — scanning NULL into int64 is an error, not a zero.
	var total int64
	err := s.h.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(bytes), 0) FROM image_derivatives`).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("total derivative bytes: %w", err)
	}
	return total, nil
}

func (s *Store) ListColdestDerivatives(ctx context.Context, limit int) ([]ImageDerivative, error) {
	// The join is what makes this image-level LRU: the order comes from the parent row's
	// last_used_at, which every serve already updates, rather than from a column on the
	// derivative that would need a write per rendition served.
	rows, err := s.h.QueryContext(ctx, s.h.Rebind(
		`SELECT d.image_hash, d.recipe, d.format, d.width, d.bytes, d.output_hash, d.path, d.animated, d.created_at
		 FROM image_derivatives d
		 JOIN images i ON i.hash = d.image_hash
		 ORDER BY i.last_used_at, d.image_hash, d.format, d.width
		 LIMIT ?`), limit)
	if err != nil {
		return nil, fmt.Errorf("list coldest derivatives: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []ImageDerivative
	for rows.Next() {
		var d ImageDerivative
		var created int64
		if err := rows.Scan(&d.ImageHash, &d.Recipe, &d.Format, &d.Width, &d.Bytes, &d.OutputHash,
			&d.Path, &d.Animated, &created); err != nil {
			return nil, fmt.Errorf("scan derivative: %w", err)
		}
		d.CreatedAt = time.Unix(created, 0)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) ListImagesByOrigin(ctx context.Context, origin string, limit int) ([]Image, error) {
	return s.queryImages(ctx,
		`SELECT `+imageColumns+` FROM images
		 WHERE origin = ?
		 ORDER BY created_at LIMIT ?`, origin, limit)
}

func (s *Store) RepointImageRefs(ctx context.Context, from, to string) error {
	// ⚠ INSERT-then-DELETE rather than `UPDATE ... SET image_hash = ?`. The update looks
	// obviously right and breaks the day two source URLs resolve to the same bytes: both rows
	// re-key onto one content hash, and the second update violates the primary key
	// (owner, role, image_hash). DO NOTHING makes the collision the no-op it should be — the ref
	// already says what the update was trying to say.
	for _, o := range imageOwners {
		if _, err := s.h.ExecContext(ctx, s.h.Rebind(
			`INSERT INTO `+o.table+` (`+o.column+`, role, image_hash)
			 SELECT `+o.column+`, role, ? FROM `+o.table+` WHERE image_hash = ?
			 ON CONFLICT (`+o.column+`, role, image_hash) DO NOTHING`), to, from); err != nil {
			return fmt.Errorf("repoint %s image refs %s->%s: %w", o.kind, from, to, err)
		}
		if _, err := s.h.ExecContext(ctx, s.h.Rebind(
			`DELETE FROM `+o.table+` WHERE image_hash = ?`), from); err != nil {
			return fmt.Errorf("clear %s image refs %s: %w", o.kind, from, err)
		}
	}
	return nil
}

// queryImages runs a SELECT of imageColumns with args and scans the lot. Every list method shares
// it so a column added to the table is added in exactly one scan path — the alternative is five
// hand-written Scan calls that drift, which is a shape this repo has been bitten by.
func (s *Store) queryImages(ctx context.Context, query string, args ...any) ([]Image, error) {
	rows, err := s.h.QueryContext(ctx, s.h.Rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("list images: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Image
	for rows.Next() {
		img, err := scanImage(rows)
		if err != nil {
			return nil, fmt.Errorf("scan image: %w", err)
		}
		out = append(out, img)
	}
	return out, rows.Err()
}

func scanImage(sc scannable) (Image, error) {
	var (
		img                             Image
		fetched, created, updated, used int64
		failedAt, retryAt, queuedAt     int64
		loop                            sql.NullInt64
	)
	err := sc.Scan(&img.Hash, &img.Origin, &img.SourceURL, &img.Visibility, &img.Role,
		&img.MIME, &img.Width, &img.Height, &img.Bytes, &img.Animated, &img.FrameCount,
		&img.DurationMS, &loop, &img.Placeholder,
		&img.DominantHex, &img.Meta, &fetched, &created, &updated, &used,
		&img.FetchFailures, &failedAt, &retryAt, &queuedAt, &img.FetchPriority)
	if err != nil {
		return Image{}, err
	}
	img.FetchFailedAt, img.FetchRetryAt, img.FetchQueuedAt = fromEpoch(failedAt), fromEpoch(retryAt), fromEpoch(queuedAt)
	// 0 is the never-fetched sentinel, read back as the zero time that every caller checks with
	// IsZero; time.Unix(0, 0) would read as fetched at the epoch.
	img.OriginFetchedAt = fromEpoch(fetched)
	img.CreatedAt = time.Unix(created, 0)
	img.UpdatedAt = time.Unix(updated, 0)
	img.LastUsedAt = time.Unix(used, 0)
	if loop.Valid {
		value := int(loop.Int64)
		img.LoopCount = &value
	}
	return img, nil
}

func stillFrameCount(count int) int {
	if count == 0 {
		return 1
	}
	return count
}

func derivativeRecipe(recipe string) string {
	if recipe == "" {
		return "legacy-go-v1"
	}
	return recipe
}

// metaOrEmpty keeps the JSON column valid. An empty string is not a JSON document, and Postgres
// rejects it outright on a JSONB column — so the zero value becomes an empty object rather than
// failing an insert for a field nobody set.
func metaOrEmpty(meta string) string {
	if meta == "" {
		return "{}"
	}
	return meta
}

// prefixed qualifies a column list for a joined query. Mechanical, but writing the qualified list
// out by hand would be a second copy of imageColumns to keep in step — and a column added to one
// copy and not the other is a scan that fails only on the joined path.
func prefixed(columns, prefix string) string {
	parts := strings.Split(columns, ",")
	for i, c := range parts {
		parts[i] = prefix + strings.TrimSpace(c)
	}
	return strings.Join(parts, ", ")
}
