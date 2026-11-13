package librarystore

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

// Header states of a library file.
const (
	LibraryHeaderPending = "pending"
	LibraryHeaderRead    = "read"
	LibraryHeaderFailed  = "failed"
)

// LibraryFile is one media file the scanner found under an in-use root (#2151 step 5).
type LibraryFile struct {
	// Path is the file as Loomarr opens it; Root is the scan root it was found under.
	Path string
	Root string
	// Size and ModifiedNs are the last walked stat, which together are the file's revision.
	Size       int64
	ModifiedNs int64
	// Identity digests what said what the file is when it was last written to the inventory.
	Identity string
	// Kind is what the file was last written as (movie, episode, extra), "" for a problem.
	Kind string
	// Header is LibraryHeaderPending, LibraryHeaderRead or LibraryHeaderFailed.
	Header string
	// Problem names why the file is skipped, "" for none.
	Problem string
	// Held marks a file a walk no longer found while that walk's removals wait for confirmation.
	// A held file is still present (zero MissingAt) until the removal is confirmed.
	Held      bool
	SeenAt    time.Time
	MissingAt time.Time
	// ItemID is the inventory item the file was last written as; SeriesID is an episode's series.
	// Either is "" until known.
	ItemID   string
	SeriesID string
}

// LibraryFileCounts counts the present files of every root.
type LibraryFileCounts struct {
	Files          int
	HeadersPending int
	Problems       int
	// Held counts held files by root and kind.
	Held []LibraryHeldCount
}

// LibraryHeldCount is how many files of one kind a root's held removal would remove.
type LibraryHeldCount struct {
	Root  string
	Kind  string
	Count int
}

// LibraryFileStore keeps the scanner's file table.
type LibraryFileStore interface {
	// LibraryAudioTracks is a file's own audio tracks (none of its external audio files), in
	// stream order, as the Library lists them (#2151 decision 47): Index, Language, Default,
	// Commentary, Descriptive, Codec and Channels. ok is false unless the scanner read the file's
	// header at this size and modification time and lists it with a title, so a file changed since
	// is never answered from its old tracks.
	LibraryAudioTracks(ctx context.Context, path string, sizeBytes, modifiedNs int64) (tracks []LibraryStream, ok bool, err error)
	// LibrarySubtitleTracks is a file's subtitle tracks, its own then the subtitle files beside it,
	// in stream order (#2088 decision 5): Index, Codec, Title, Language, Forced, HearingImpaired and
	// ExternalRef. ok is false on the same terms as LibraryAudioTracks.
	LibrarySubtitleTracks(ctx context.Context, path string, sizeBytes, modifiedNs int64) (tracks []LibraryStream, ok bool, err error)
	// LibraryPlayingParts is the files a library item plays, in the order they play: its source
	// marked plays, while the scanner sees it present, or each part of a movie in parts, in part
	// order. A movie that plays one in each edition (decision 54) airs the default edition's, else
	// the edition whose file's path sorts first. None for an id that is not a library item, or one
	// with no present file.
	LibraryPlayingParts(ctx context.Context, itemID string) ([]LibraryPlayingPart, error)
	// LibraryPlayingVideo is the first video stream of the first file a library item plays, as the
	// scanner last read its header: Index, Codec, Width, Height, FrameRate, PixelFormat and
	// ColorTransfer. ok is false when LibraryPlayingParts has none, or the file's header has not
	// been read with a video stream.
	LibraryPlayingVideo(ctx context.Context, itemID string) (video LibraryStream, ok bool, err error)
	// LibraryFiles reads every file row of one root, missing ones included, in path order.
	LibraryFiles(ctx context.Context, root string) ([]LibraryFile, error)
	// SaveLibraryFiles writes rows whole, a chunk per transaction, which keeps each transaction and
	// the WAL small. Every row is checked before any is written; a failure part-way leaves the
	// earlier chunks written, which the next pass writes again.
	SaveLibraryFiles(ctx context.Context, files []LibraryFile) error
	// LibraryFileRoots lists every root that has file rows.
	LibraryFileRoots(ctx context.Context) ([]string, error)
	// DeleteLibraryFiles forgets every file row of a root.
	DeleteLibraryFiles(ctx context.Context, root string) error
	// SetLibraryRootScanned records that a walk of the root completed at `at`.
	SetLibraryRootScanned(ctx context.Context, root string, at time.Time) error
	// LibraryFileCounts counts present files across roots.
	LibraryFileCounts(ctx context.Context) (LibraryFileCounts, error)
	// LibraryHeldTitles names up to limit of the titles whose held files a root no longer found, a
	// show once for all its episodes, by name, and counts them all: what the maintainer is asked
	// to let go.
	LibraryHeldTitles(ctx context.Context, root string, limit int) ([]string, int, error)
}

// LibraryPlayingPart is one file of what a library item plays, with its header's length (0 until
// it is read).
type LibraryPlayingPart struct {
	Path       string
	DurationMs int64
}

func (s *Store) LibraryPlayingParts(ctx context.Context, itemID string) ([]LibraryPlayingPart, error) {
	rows, err := s.h.QueryContext(ctx, s.ph(`SELECT s.path, s.duration_ms, s.edition, s.part FROM library_sources s
		JOIN library_files f ON f.path = s.path
		WHERE s.item_id = ? AND s.plays AND f.missing_at_ms = 0 AND f.problem = ''
		ORDER BY s.edition <> '', s.path`), itemID)
	if err != nil {
		return nil, fmt.Errorf("library playing parts: %w", err)
	}
	type row struct {
		LibraryPlayingPart
		edition string
		part    int
	}
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.Path, &r.DurationMs, &r.edition, &r.part); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("library playing parts: %w", err)
		}
		got = append(got, r)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("library playing parts: %w", err)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("library playing parts: %w", err)
	}
	if len(got) == 0 {
		return nil, nil
	}
	// The edition that airs, then its parts in order; an edition that is not in parts has one file.
	edition := got[0].edition
	got = slices.DeleteFunc(got, func(r row) bool { return r.edition != edition })
	slices.SortStableFunc(got, func(a, b row) int { return cmp.Compare(a.part, b.part) })
	if got[0].part == 0 {
		got = got[:1]
	}
	out := make([]LibraryPlayingPart, len(got))
	for i, r := range got {
		out[i] = r.LibraryPlayingPart
	}
	return out, nil
}

func (s *Store) LibraryPlayingVideo(ctx context.Context, itemID string) (LibraryStream, bool, error) {
	parts, err := s.LibraryPlayingParts(ctx, itemID)
	if err != nil || len(parts) == 0 {
		return LibraryStream{}, false, err
	}
	v := LibraryStream{Kind: "video"}
	err = s.h.QueryRowContext(ctx, s.ph(`SELECT st.stream_index, st.codec, st.width, st.height, st.frame_rate,
		st.pixel_format, st.color_transfer FROM library_streams st
		JOIN library_files f ON f.path = st.path
		WHERE st.item_id = ? AND st.path = ? AND st.kind = 'video' AND st.external_ref = '' AND f.header = ?
		ORDER BY st.stream_index LIMIT 1`), itemID, parts[0].Path, LibraryHeaderRead).Scan(
		&v.Index, &v.Codec, &v.Width, &v.Height, &v.FrameRate, &v.PixelFormat, &v.ColorTransfer)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return LibraryStream{}, false, nil
	case err != nil:
		return LibraryStream{}, false, fmt.Errorf("library playing video: %w", err)
	}
	return v, true, nil
}

// libraryFileCurrent reports whether the scanner read path's header at this size and modification
// time and a title lists the file: the condition every per-file track read answers on.
func (s *Store) libraryFileCurrent(ctx context.Context, path string, sizeBytes, modifiedNs int64) (bool, error) {
	var listed int
	err := s.h.QueryRowContext(ctx, s.ph(`SELECT COUNT(*) FROM library_files f
		JOIN library_sources src ON src.path = f.path
		WHERE f.path = ? AND f.size_bytes = ? AND f.modified_ns = ? AND f.header = ? AND f.missing_at_ms = 0`),
		path, sizeBytes, modifiedNs, LibraryHeaderRead).Scan(&listed)
	return listed > 0, err
}

func (s *Store) LibrarySubtitleTracks(ctx context.Context, path string, sizeBytes, modifiedNs int64) ([]LibraryStream, bool, error) {
	current, err := s.libraryFileCurrent(ctx, path, sizeBytes, modifiedNs)
	if err != nil {
		return nil, false, fmt.Errorf("library subtitle tracks: %w", err)
	}
	if !current {
		return nil, false, nil
	}
	rows, err := s.h.QueryContext(ctx, s.ph(`SELECT stream_index, codec, title, language, forced, hearing_impaired,
		external_ref FROM library_streams WHERE path = ? AND kind = 'subtitle' ORDER BY stream_index`), path)
	if err != nil {
		return nil, false, fmt.Errorf("library subtitle tracks: %w", err)
	}
	defer func() { _ = rows.Close() }()
	tracks := []LibraryStream{}
	for rows.Next() {
		t := LibraryStream{Kind: "subtitle"}
		if err := rows.Scan(&t.Index, &t.Codec, &t.Title, &t.Language, &t.Forced, &t.HearingImpaired, &t.ExternalRef); err != nil {
			return nil, false, fmt.Errorf("library subtitle tracks: %w", err)
		}
		tracks = append(tracks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("library subtitle tracks: %w", err)
	}
	return tracks, true, nil
}

func (s *Store) LibraryAudioTracks(ctx context.Context, path string, sizeBytes, modifiedNs int64) ([]LibraryStream, bool, error) {
	current, err := s.libraryFileCurrent(ctx, path, sizeBytes, modifiedNs)
	if err != nil {
		return nil, false, fmt.Errorf("library audio tracks: %w", err)
	}
	if !current {
		return nil, false, nil
	}
	rows, err := s.h.QueryContext(ctx, s.ph(`SELECT stream_index, language, is_default, commentary, descriptive,
		codec, channels FROM library_streams WHERE path = ? AND kind = 'audio' AND external_ref = ''
		ORDER BY stream_index`), path)
	if err != nil {
		return nil, false, fmt.Errorf("library audio tracks: %w", err)
	}
	defer func() { _ = rows.Close() }()
	tracks := []LibraryStream{}
	for rows.Next() {
		t := LibraryStream{Kind: "audio"}
		if err := rows.Scan(&t.Index, &t.Language, &t.Default, &t.Commentary, &t.Descriptive, &t.Codec, &t.Channels); err != nil {
			return nil, false, fmt.Errorf("library audio tracks: %w", err)
		}
		tracks = append(tracks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("library audio tracks: %w", err)
	}
	return tracks, true, nil
}

// LibraryLink writes a link to another library_items row: NULL for "" (not known yet), which the
// foreign key allows, since no row has "" as its id. Reads turn it back with COALESCE(col, ”).
func LibraryLink(id string) any {
	if id == "" {
		return nil
	}
	return id
}

func (s *Store) LibraryFiles(ctx context.Context, root string) ([]LibraryFile, error) {
	rows, err := s.h.QueryContext(ctx, s.ph(`SELECT path, root, size_bytes, modified_ns, identity, kind, header,
		problem, held, seen_at_ms, missing_at_ms, COALESCE(inventory_item_id, ''), COALESCE(inventory_series_id, '')
		FROM library_files WHERE root = ? ORDER BY path`), root)
	if err != nil {
		return nil, fmt.Errorf("list library files: %w", err)
	}
	var out []LibraryFile
	for rows.Next() {
		var f LibraryFile
		var seen, missing int64
		if err := rows.Scan(&f.Path, &f.Root, &f.Size, &f.ModifiedNs, &f.Identity, &f.Kind, &f.Header, &f.Problem,
			&f.Held, &seen, &missing, &f.ItemID, &f.SeriesID); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("list library files: %w", err)
		}
		f.SeenAt = time.UnixMilli(seen).UTC()
		if missing > 0 {
			f.MissingAt = time.UnixMilli(missing).UTC()
		}
		out = append(out, f)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("list library files: %w", err)
	}
	return out, nil
}

// LibraryFilesChunk is how many file rows one transaction writes. Measured on 30,684 episode rows
// (SQLite, 2026-11-09): one transaction took 491 ms, chunks of 500 take 302 ms. Chunks do not let
// another writer in between them: it waits in SQLite's busy handler, which sleeps longer than the
// gap, so it waits out the whole save (333 ms here), far inside busy_timeout.
const LibraryFilesChunk = 500

func (s *Store) SaveLibraryFiles(ctx context.Context, files []LibraryFile) error {
	for _, f := range files {
		if f.Path == "" || f.Root == "" || f.SeenAt.IsZero() {
			return fmt.Errorf("save library files: path, root and seen time are required")
		}
	}
	for start := 0; start < len(files); start += LibraryFilesChunk {
		if err := s.saveLibraryFiles(ctx, files[start:min(start+LibraryFilesChunk, len(files))]); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) saveLibraryFiles(ctx context.Context, files []LibraryFile) error {
	tx, err := s.h.Begin(ctx)
	if err != nil {
		return fmt.Errorf("save library files: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx, s.ph(`INSERT INTO library_files
		(path, root, size_bytes, modified_ns, identity, kind, header, problem, held, seen_at_ms, missing_at_ms,
		inventory_item_id, inventory_series_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (path) DO UPDATE SET root = excluded.root, size_bytes = excluded.size_bytes,
		modified_ns = excluded.modified_ns, identity = excluded.identity, kind = excluded.kind,
		header = excluded.header, problem = excluded.problem, held = excluded.held,
		seen_at_ms = excluded.seen_at_ms, missing_at_ms = excluded.missing_at_ms,
		inventory_item_id = excluded.inventory_item_id, inventory_series_id = excluded.inventory_series_id`))
	if err != nil {
		return fmt.Errorf("save library files: %w", err)
	}
	defer func() { _ = stmt.Close() }()
	for _, f := range files {
		header := f.Header
		if header == "" {
			header = LibraryHeaderPending
		}
		var missing int64
		if !f.MissingAt.IsZero() {
			missing = f.MissingAt.UnixMilli()
		}
		if _, err := stmt.ExecContext(ctx, f.Path, f.Root, f.Size, f.ModifiedNs, f.Identity, f.Kind, header,
			f.Problem, f.Held, f.SeenAt.UnixMilli(), missing, LibraryLink(f.ItemID), LibraryLink(f.SeriesID)); err != nil {
			return fmt.Errorf("save library file: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save library files: %w", err)
	}
	return nil
}

func (s *Store) LibraryFileRoots(ctx context.Context) ([]string, error) {
	rows, err := s.h.QueryContext(ctx, `SELECT DISTINCT root FROM library_files ORDER BY root`)
	if err != nil {
		return nil, fmt.Errorf("list library file roots: %w", err)
	}
	var out []string
	for rows.Next() {
		var root string
		if err := rows.Scan(&root); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("list library file roots: %w", err)
		}
		out = append(out, root)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("list library file roots: %w", err)
	}
	return out, nil
}

func (s *Store) DeleteLibraryFiles(ctx context.Context, root string) error {
	if _, err := s.h.ExecContext(ctx, s.ph(`DELETE FROM library_files WHERE root = ?`), root); err != nil {
		return fmt.Errorf("delete library files: %w", err)
	}
	return nil
}

func (s *Store) SetLibraryRootScanned(ctx context.Context, root string, at time.Time) error {
	result, err := s.h.ExecContext(ctx, s.ph(`UPDATE library_roots SET scanned_at_ms = ? WHERE path = ?`),
		at.UnixMilli(), root)
	if err != nil {
		return fmt.Errorf("set library root scanned: %w", err)
	}
	if n, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("set library root scanned: %w", err)
	} else if n == 0 {
		return storedb.ErrNotFound
	}
	return nil
}

func (s *Store) LibraryHeldTitles(ctx context.Context, root string, limit int) ([]string, int, error) {
	// An episode's title is its show; any other file's is its own item.
	rows, err := s.h.QueryContext(ctx, s.ph(`WITH held AS (
		SELECT DISTINCT i.id, i.name, i.sort_name FROM library_files f JOIN library_items i
			ON i.id = CASE WHEN COALESCE(f.inventory_series_id, '') <> '' THEN f.inventory_series_id ELSE f.inventory_item_id END
		WHERE f.root = ? AND f.held AND f.missing_at_ms = 0)
		SELECT name, (SELECT COUNT(*) FROM held) FROM held ORDER BY sort_name, id LIMIT ?`), root, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("library held titles: %w", err)
	}
	names, total := []string{}, 0
	for rows.Next() {
		var name string
		if err := rows.Scan(&name, &total); err != nil {
			_ = rows.Close()
			return nil, 0, fmt.Errorf("library held titles: %w", err)
		}
		names = append(names, name)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, 0, fmt.Errorf("library held titles: %w", err)
	}
	return names, total, nil
}

func (s *Store) LibraryFileCounts(ctx context.Context) (LibraryFileCounts, error) {
	var c LibraryFileCounts
	err := s.h.QueryRowContext(ctx, s.ph(`SELECT
		COUNT(*),
		COALESCE(SUM(CASE WHEN header = ? AND problem = '' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN problem <> '' THEN 1 ELSE 0 END), 0)
		FROM library_files WHERE missing_at_ms = 0`), LibraryHeaderPending).
		Scan(&c.Files, &c.HeadersPending, &c.Problems)
	if err != nil {
		return c, fmt.Errorf("count library files: %w", err)
	}
	rows, err := s.h.QueryContext(ctx, `SELECT root, kind, COUNT(*) FROM library_files
		WHERE held AND missing_at_ms = 0 GROUP BY root, kind ORDER BY root, kind`)
	if err != nil {
		return c, fmt.Errorf("count held library files: %w", err)
	}
	for rows.Next() {
		var h LibraryHeldCount
		if err := rows.Scan(&h.Root, &h.Kind, &h.Count); err != nil {
			_ = rows.Close()
			return c, fmt.Errorf("count held library files: %w", err)
		}
		c.Held = append(c.Held, h)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return c, fmt.Errorf("count held library files: %w", err)
	}
	return c, nil
}
