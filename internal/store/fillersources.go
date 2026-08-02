package store

import (
	"context"
	"fmt"
	"time"
)

// The persisted REMOTE filler-source registry (§10, V33).
//
// ⚠ **Remote sources only, and that boundary is the design.** V28 made `GET /v1/filler/sources`
// a READ-MODEL over config — three fixed rows for the drop-folder, the media-server library, and
// remote ingest — precisely so a row and a setting never compete to say where the folder is.
// That still holds. These rows describe the specific archive.org collections an operator ADDED,
// which carry state no setting can express: a licence, a last-fetch time, the fact that someone
// chose this one. On the Sources tab they nest UNDER the read-model's `remote` row.
//
// The alternative — one flat table seeded from `filler.dir` — was considered and dropped before
// it shipped: a table of things that EXIST cannot express "you could set up a library but have
// not", which is what the read-model's `configured:false` invitation is for.

// FillerSource is one registered remote source.
type FillerSource struct {
	// ID is stable and independent of URI, so re-pointing a source keeps its history.
	ID string
	// Kind discriminates the source type. "archive" today; the column exists so the next
	// one does not need a migration.
	Kind string
	// URI is the archive.org identifier or URL.
	URI string
	// Label is operator-facing; empty falls back to the URI at render time.
	Label string
	// License is what the source declared. ⚠ EMPTY MEANS UNKNOWN, never "public domain" —
	// ~92% of archive.org items declare none (667 of 8362 measured in classic_tv_commercials,
	// 2026-07-31), so absence is the common case and carries no permission.
	License string
	// LastFetchedAt is zero when never fetched, which renders as "never" rather than as an
	// epoch date nobody meant.
	LastFetchedAt time.Time
	CreatedAt     time.Time
	// Enabled is the Sources tab's on/off switch (V35). A disabled source is not scanned, not
	// searched and not downloaded from.
	//
	// ⚠ **Disabling is not deleting, and nothing here may blur that.** The row keeps its
	// licence and its fetch history, so switching it back on restores what was there. Clips it
	// already brought in are untouched either way — they are real files an operator may have
	// tagged and pinned.
	Enabled bool
}

// NewFillerSource builds a source that is ON.
//
// ⚠ **A constructor rather than a struct literal, because the safe value here is not the zero
// value.** `Enabled` is a bool, so `store.FillerSource{ID: …}` means *disabled* — a source that
// an operator just added and that silently never fetches, with a switch in the UI already
// showing "off" and no explanation. Every add path goes through here so that cannot happen; the
// only way to create a disabled source is to switch one off afterwards, which is what the
// operator would expect to have to do.
func NewFillerSource(id, kind, uri, label string, createdAt time.Time) FillerSource {
	return FillerSource{ID: id, Kind: kind, URI: uri, Label: label, CreatedAt: createdAt, Enabled: true}
}

const fillerSourceSelect = `SELECT id, kind, uri, label, license, last_fetched_at, created_at, enabled
	FROM filler_sources`

// ListFillerSources returns every source, OLDEST FIRST. Ordering is explicit rather than
// left to the engine: an unordered list reshuffles between reads on Postgres, and a Sources
// tab whose rows move under the pointer is its own small bug.
func (s *sqlStore) ListFillerSources(ctx context.Context) ([]FillerSource, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(fillerSourceSelect+` ORDER BY created_at, id`))
	if err != nil {
		return nil, fmt.Errorf("list filler sources: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []FillerSource
	for rows.Next() {
		var (
			src       FillerSource
			fetchedAt int64
			createdAt int64
		)
		if err := rows.Scan(&src.ID, &src.Kind, &src.URI, &src.Label, &src.License,
			&fetchedAt, &createdAt, &src.Enabled); err != nil {
			return nil, fmt.Errorf("scan filler source: %w", err)
		}
		src.LastFetchedAt = fromEpoch(fetchedAt)
		src.CreatedAt = fromEpoch(createdAt)
		out = append(out, src)
	}
	return out, rows.Err()
}

// UpsertFillerSource adds or updates a source by id.
//
// ⚠ `last_fetched_at` is INSERTed but never in the DO UPDATE list — the same shape as clips'
// play counters, and for the same reason. Re-registering a source (an operator fixing its
// label) knows nothing about fetches, so writing excluded.last_fetched_at would silently reset
// "last fetched" to zero and make a working source look like it had never run.
// MarkFillerSourceFetched is its only writer.
//
// ⚠ **`enabled` is the same shape, and V35 nearly got it wrong.** The first version of this
// change added `enabled=excluded.enabled` to the DO UPDATE list, which is worse than the reset
// above: Go's zero value for a bool is `false`, so ANY existing caller that re-registered a
// source without knowing about the new field would have silently switched it OFF — a source
// that stops fetching with nothing in the UI to explain why. It is INSERTed (so a new row
// carries the caller's choice, and `NewFillerSource` makes that choice `true`) and thereafter
// only `SetFillerSourceEnabled` writes it.
func (s *sqlStore) UpsertFillerSource(ctx context.Context, src FillerSource) error {
	_, err := s.db.ExecContext(ctx, s.ph(
		`INSERT INTO filler_sources (id, kind, uri, label, license, last_fetched_at, created_at, enabled)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   kind=excluded.kind, uri=excluded.uri, label=excluded.label, license=excluded.license`),
		src.ID, src.Kind, src.URI, src.Label, src.License,
		epoch(src.LastFetchedAt), epoch(src.CreatedAt), src.Enabled)
	if err != nil {
		return fmt.Errorf("upsert filler source %s: %w", src.ID, err)
	}
	return nil
}

// DeleteFillerSource forgets a source.
//
// ⚠ Clips it brought in are deliberately NOT deleted. They are real files in the drop-folder,
// already tagged and possibly pinned into a channel; forgetting where something came from is
// not a reason to throw it away. Removing the files is the operator's call, in the folder.
func (s *sqlStore) DeleteFillerSource(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, s.ph(`DELETE FROM filler_sources WHERE id = ?`), id)
	if err != nil {
		return fmt.Errorf("delete filler source %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete filler source %s: %w", id, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkFillerSourceFetched stamps a successful fetch. Returns ErrNotFound for an unknown id
// rather than silently doing nothing, so a caller cannot believe it recorded something.
func (s *sqlStore) MarkFillerSourceFetched(ctx context.Context, id string, at time.Time) error {
	res, err := s.db.ExecContext(ctx,
		s.ph(`UPDATE filler_sources SET last_fetched_at = ? WHERE id = ?`), epoch(at), id)
	if err != nil {
		return fmt.Errorf("mark filler source fetched %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark filler source fetched %s: %w", id, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetFillerSourceEnabled switches a source on or off. Returns ErrNotFound for an unknown id
// rather than silently doing nothing, so a caller cannot believe it recorded something.
//
// A targeted UPDATE rather than a read-modify-Upsert: the switch is the one field the Sources
// tab toggles, and round-tripping the whole row to change a boolean would let a concurrent
// re-registration lose its label to a stale copy.
func (s *sqlStore) SetFillerSourceEnabled(ctx context.Context, id string, enabled bool) error {
	res, err := s.db.ExecContext(ctx,
		s.ph(`UPDATE filler_sources SET enabled = ? WHERE id = ?`), enabled, id)
	if err != nil {
		return fmt.Errorf("set filler source enabled %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set filler source enabled %s: %w", id, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
