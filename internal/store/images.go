package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// The image service's persistence (§22, V52). One record per logical image; the BYTES are on disk
// under a content hash and never in this database.
//
// ⚠ That is the deliberate reversal of `channel_icons`, which stored upload bytes here precisely so
// they would ride the §16 backup. It worked, and it makes the database the wrong shape for a general
// image service — so §22 accepts the narrower guarantee instead: an operator upload is the one thing
// that does not survive losing /data/images, which is why `ListUnrecoverableImages` exists for the
// GC job to warn about rather than for anything to repair.

// Image is one logical image's record. Mirrors the `images` table; the domain type in
// internal/images is separate and an adapter bridges them, following the same rule every other
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
	Placeholder string
	DominantHex string
	// Meta is a JSON document. Carried as a string rather than []byte because the Postgres column
	// is JSONB and a []byte parameter binds as bytea; a string binds as text and casts cleanly.
	Meta string

	OriginFetchedAt time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	LastUsedAt      time.Time
}

// ImageRef records what an image decorates. Its own table so the GC can find orphans without any
// domain schema referencing images.
type ImageRef struct {
	ImageHash string
	OwnerKind string
	OwnerID   string
	Role      string
}

// ImageDerivative is one rendition on disk. Regenerable — deleting a row costs a re-encode.
type ImageDerivative struct {
	ImageHash string
	Format    string
	Width     int
	Bytes     int64
	Path      string
	CreatedAt time.Time
}

// ImageStore is the image half of the Store contract.
type ImageStore interface {
	PutImage(ctx context.Context, img Image) error
	GetImage(ctx context.Context, hash string) (Image, error)
	DeleteImage(ctx context.Context, hash string) error
	TouchImage(ctx context.Context, hash string, at time.Time) error

	// ListImagesAwaitingFetch returns remote images whose bytes have not landed. The fetch job's
	// work list; `origin_fetched_at = 0` is the "never fetched" sentinel.
	ListImagesAwaitingFetch(ctx context.Context, limit int) ([]Image, error)
	// ListImagesExpiredBefore returns remote images fetched before a cutoff — the TMDB six-month
	// compliance sweep.
	ListImagesExpiredBefore(ctx context.Context, cutoff time.Time, limit int) ([]Image, error)
	// ListOrphanImages returns images no longer referenced by anything.
	ListOrphanImages(ctx context.Context, limit int) ([]Image, error)
	// ListUnrecoverableImages returns images whose bytes cannot be re-obtained (upload/generated).
	// The GC checks these against the disk and warns about any whose file is gone.
	ListUnrecoverableImages(ctx context.Context, limit int) ([]Image, error)

	PutImageRef(ctx context.Context, ref ImageRef) error
	DeleteImageRefs(ctx context.Context, ownerKind, ownerID string) error
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
	// or `Cache-Control: immutable` stops being true. Re-keying a row that `image_refs` already
	// points at needs the refs to move with it, and a naive UPDATE collides whenever two source
	// URLs turn out to hold the same bytes for the same owner — so this is insert-then-delete,
	// not an update.
	RepointImageRefs(ctx context.Context, from, to string) error

	PutImageDerivative(ctx context.Context, d ImageDerivative) error
	ListImageDerivatives(ctx context.Context, hash string) ([]ImageDerivative, error)
	DeleteImageDerivatives(ctx context.Context, hash string) error
	// DeleteImageDerivative removes one rendition's row. The GC's eviction unit: a derivative is
	// regenerable, so dropping one costs a re-encode and never data.
	DeleteImageDerivative(ctx context.Context, hash, format string, width int) error
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
	// ListImagesMissingFormat returns images that have at least one derivative but none in the
	// given format — the AVIF job's work list, expressed without the job knowing SQL.
	ListImagesMissingFormat(ctx context.Context, format string, limit int) ([]Image, error)
}

const imageColumns = `hash, origin, source_url, visibility, role, mime, width, height, bytes,
	animated, placeholder, dominant_hex, meta, origin_fetched_at, created_at, updated_at, last_used_at`

func (s *sqlStore) PutImage(ctx context.Context, img Image) error {
	_, err := s.db.ExecContext(ctx, s.ph(
		`INSERT INTO images (`+imageColumns+`)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
		     placeholder = excluded.placeholder,
		     dominant_hex = excluded.dominant_hex,
		     meta = excluded.meta,
		     origin_fetched_at = excluded.origin_fetched_at,
		     updated_at = excluded.updated_at`),
		img.Hash, img.Origin, img.SourceURL, img.Visibility, img.Role, img.MIME,
		img.Width, img.Height, img.Bytes, img.Animated, img.Placeholder, img.DominantHex,
		metaOrEmpty(img.Meta), epoch(img.OriginFetchedAt), epoch(img.CreatedAt),
		epoch(img.UpdatedAt), epoch(img.LastUsedAt))
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

func (s *sqlStore) GetImage(ctx context.Context, hash string) (Image, error) {
	row := s.db.QueryRowContext(ctx, s.ph(
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

func (s *sqlStore) DeleteImage(ctx context.Context, hash string) error {
	// Refs and derivatives cascade (ON DELETE CASCADE), so this is one statement rather than a
	// transaction the caller could get half-right.
	if _, err := s.db.ExecContext(ctx, s.ph(`DELETE FROM images WHERE hash = ?`), hash); err != nil {
		return fmt.Errorf("delete image %s: %w", hash, err)
	}
	return nil
}

func (s *sqlStore) TouchImage(ctx context.Context, hash string, at time.Time) error {
	if _, err := s.db.ExecContext(ctx, s.ph(
		`UPDATE images SET last_used_at = ? WHERE hash = ?`), epoch(at), hash); err != nil {
		return fmt.Errorf("touch image %s: %w", hash, err)
	}
	return nil
}

func (s *sqlStore) ListImagesAwaitingFetch(ctx context.Context, limit int) ([]Image, error) {
	return s.queryImages(ctx,
		`SELECT `+imageColumns+` FROM images
		 WHERE origin = 'remote' AND origin_fetched_at = 0
		 ORDER BY created_at LIMIT ?`, limit)
}

func (s *sqlStore) ListImagesExpiredBefore(ctx context.Context, cutoff time.Time, limit int) ([]Image, error) {
	// origin_fetched_at > 0 excludes the never-fetched rows, which are the fetch job's business
	// rather than the expiry sweep's — otherwise every pending image would look infinitely stale.
	return s.queryImages(ctx,
		`SELECT `+imageColumns+` FROM images
		 WHERE origin = 'remote' AND origin_fetched_at > 0 AND origin_fetched_at < ?
		 ORDER BY origin_fetched_at LIMIT ?`, epoch(cutoff), limit)
}

func (s *sqlStore) ListOrphanImages(ctx context.Context, limit int) ([]Image, error) {
	return s.queryImages(ctx,
		`SELECT `+imageColumns+` FROM images i
		 WHERE NOT EXISTS (SELECT 1 FROM image_refs r WHERE r.image_hash = i.hash)
		 ORDER BY i.created_at LIMIT ?`, limit)
}

func (s *sqlStore) ListUnrecoverableImages(ctx context.Context, limit int) ([]Image, error) {
	return s.queryImages(ctx,
		`SELECT `+imageColumns+` FROM images
		 WHERE origin IN ('upload', 'generated')
		 ORDER BY created_at LIMIT ?`, limit)
}

func (s *sqlStore) ListImagesMissingFormat(ctx context.Context, format string, limit int) ([]Image, error) {
	// "Has some derivative but not this one." The first EXISTS is what keeps the AVIF job from
	// picking up images whose bytes have not been processed at all yet — those belong to the fetch
	// job, and encoding AVIF for an image with no WebP would invert the priority the split exists
	// to create.
	return s.queryImages(ctx,
		`SELECT `+imageColumns+` FROM images i
		 WHERE EXISTS (SELECT 1 FROM image_derivatives d WHERE d.image_hash = i.hash)
		   AND NOT EXISTS (SELECT 1 FROM image_derivatives d2
		                   WHERE d2.image_hash = i.hash AND d2.format = ?)
		 ORDER BY i.created_at LIMIT ?`, format, limit)
}

func (s *sqlStore) PutImageRef(ctx context.Context, ref ImageRef) error {
	_, err := s.db.ExecContext(ctx, s.ph(
		`INSERT INTO image_refs (image_hash, owner_kind, owner_id, role)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT (image_hash, owner_kind, owner_id, role) DO NOTHING`),
		ref.ImageHash, ref.OwnerKind, ref.OwnerID, ref.Role)
	if err != nil {
		return fmt.Errorf("put image ref %s->%s/%s: %w", ref.ImageHash, ref.OwnerKind, ref.OwnerID, err)
	}
	return nil
}

func (s *sqlStore) DeleteImageRefs(ctx context.Context, ownerKind, ownerID string) error {
	_, err := s.db.ExecContext(ctx, s.ph(
		`DELETE FROM image_refs WHERE owner_kind = ? AND owner_id = ?`), ownerKind, ownerID)
	if err != nil {
		return fmt.Errorf("delete image refs %s/%s: %w", ownerKind, ownerID, err)
	}
	return nil
}

func (s *sqlStore) ImagesForOwner(ctx context.Context, ownerKind, ownerID string) ([]Image, error) {
	return s.queryImages(ctx,
		`SELECT `+prefixed(imageColumns, "i.")+` FROM images i
		 JOIN image_refs r ON r.image_hash = i.hash
		 WHERE r.owner_kind = ? AND r.owner_id = ?
		 ORDER BY i.created_at`, ownerKind, ownerID)
}

func (s *sqlStore) PutImageDerivative(ctx context.Context, d ImageDerivative) error {
	_, err := s.db.ExecContext(ctx, s.ph(
		`INSERT INTO image_derivatives (image_hash, format, width, bytes, path, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (image_hash, format, width) DO UPDATE SET
		     bytes = excluded.bytes, path = excluded.path, created_at = excluded.created_at`),
		d.ImageHash, d.Format, d.Width, d.Bytes, d.Path, epoch(d.CreatedAt))
	if err != nil {
		return fmt.Errorf("put derivative %s %s w%d: %w", d.ImageHash, d.Format, d.Width, err)
	}
	return nil
}

func (s *sqlStore) ListImageDerivatives(ctx context.Context, hash string) ([]ImageDerivative, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(
		`SELECT image_hash, format, width, bytes, path, created_at
		 FROM image_derivatives WHERE image_hash = ? ORDER BY format, width`), hash)
	if err != nil {
		return nil, fmt.Errorf("list derivatives %s: %w", hash, err)
	}
	defer func() { _ = rows.Close() }()

	var out []ImageDerivative
	for rows.Next() {
		var d ImageDerivative
		var created int64
		if err := rows.Scan(&d.ImageHash, &d.Format, &d.Width, &d.Bytes, &d.Path, &created); err != nil {
			return nil, fmt.Errorf("scan derivative: %w", err)
		}
		d.CreatedAt = time.Unix(created, 0)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *sqlStore) DeleteImageDerivatives(ctx context.Context, hash string) error {
	_, err := s.db.ExecContext(ctx, s.ph(
		`DELETE FROM image_derivatives WHERE image_hash = ?`), hash)
	if err != nil {
		return fmt.Errorf("delete derivatives %s: %w", hash, err)
	}
	return nil
}

func (s *sqlStore) DeleteImageDerivative(ctx context.Context, hash, format string, width int) error {
	_, err := s.db.ExecContext(ctx, s.ph(
		`DELETE FROM image_derivatives WHERE image_hash = ? AND format = ? AND width = ?`),
		hash, format, width)
	if err != nil {
		return fmt.Errorf("delete derivative %s %s w%d: %w", hash, format, width, err)
	}
	return nil
}

func (s *sqlStore) TotalImageDerivativeBytes(ctx context.Context) (int64, error) {
	// COALESCE because SUM over no rows is NULL in both dialects, and a fresh install has no
	// derivatives — scanning NULL into int64 is an error, not a zero.
	var total int64
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(bytes), 0) FROM image_derivatives`).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("total derivative bytes: %w", err)
	}
	return total, nil
}

func (s *sqlStore) ListColdestDerivatives(ctx context.Context, limit int) ([]ImageDerivative, error) {
	// The join is what makes this image-level LRU: the order comes from the parent row's
	// last_used_at, which every serve already updates, rather than from a column on the
	// derivative that would need a write per rendition served.
	rows, err := s.db.QueryContext(ctx, s.ph(
		`SELECT d.image_hash, d.format, d.width, d.bytes, d.path, d.created_at
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
		if err := rows.Scan(&d.ImageHash, &d.Format, &d.Width, &d.Bytes, &d.Path, &created); err != nil {
			return nil, fmt.Errorf("scan derivative: %w", err)
		}
		d.CreatedAt = time.Unix(created, 0)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *sqlStore) ListImagesByOrigin(ctx context.Context, origin string, limit int) ([]Image, error) {
	return s.queryImages(ctx,
		`SELECT `+imageColumns+` FROM images
		 WHERE origin = ?
		 ORDER BY created_at LIMIT ?`, origin, limit)
}

func (s *sqlStore) RepointImageRefs(ctx context.Context, from, to string) error {
	// ⚠ INSERT-then-DELETE rather than `UPDATE image_refs SET image_hash = ?`. The update looks
	// obviously right and breaks the day two source URLs resolve to the same bytes: both rows
	// re-key onto one content hash, and the second update violates the primary key
	// (image_hash, owner_kind, owner_id, role). DO NOTHING makes the collision the no-op it
	// should be — the ref already says what the update was trying to say.
	if _, err := s.db.ExecContext(ctx, s.ph(
		`INSERT INTO image_refs (image_hash, owner_kind, owner_id, role)
		 SELECT ?, owner_kind, owner_id, role FROM image_refs WHERE image_hash = ?
		 ON CONFLICT (image_hash, owner_kind, owner_id, role) DO NOTHING`), to, from); err != nil {
		return fmt.Errorf("repoint image refs %s->%s: %w", from, to, err)
	}
	if _, err := s.db.ExecContext(ctx, s.ph(
		`DELETE FROM image_refs WHERE image_hash = ?`), from); err != nil {
		return fmt.Errorf("clear image refs %s: %w", from, err)
	}
	return nil
}

// queryImages runs a SELECT of imageColumns with args and scans the lot. Every list method shares
// it so a column added to the table is added in exactly one scan path — the alternative is five
// hand-written Scan calls that drift, which is a shape this repo has been bitten by.
func (s *sqlStore) queryImages(ctx context.Context, query string, args ...any) ([]Image, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(query), args...)
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

// scanner is what both *sql.Row and *sql.Rows satisfy, so one scan body serves the single-row and
// multi-row paths.
type scanner interface{ Scan(dest ...any) error }

func scanImage(sc scanner) (Image, error) {
	var (
		img                             Image
		fetched, created, updated, used int64
	)
	err := sc.Scan(&img.Hash, &img.Origin, &img.SourceURL, &img.Visibility, &img.Role,
		&img.MIME, &img.Width, &img.Height, &img.Bytes, &img.Animated, &img.Placeholder,
		&img.DominantHex, &img.Meta, &fetched, &created, &updated, &used)
	if err != nil {
		return Image{}, err
	}
	img.OriginFetchedAt = time.Unix(fetched, 0)
	img.CreatedAt = time.Unix(created, 0)
	img.UpdatedAt = time.Unix(updated, 0)
	img.LastUsedAt = time.Unix(used, 0)
	return img, nil
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
