package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
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
	// AutoAdmit permits grounded, sufficiently confident arrivals from this source to leave
	// Incoming without a per-clip decision (§10 V57). It is independent of Enabled: one controls
	// acquisition, the other catalog admission. Channel matching and overrides happen later.
	AutoAdmit bool
	// FetchEverySeconds overrides `filler.fetch.every` for THIS source (§10 V38c). A busy archive
	// collection and a small playlist want different numbers, and one global figure serves
	// neither well.
	//
	// ⚠ **nil = inherit the global; 0 = NEVER auto-fetch this source.** They cannot share an
	// encoding, because 0 already means "off" for the global key — so a non-pointer int would
	// make "unset" and "never" the same value, and every existing source would read as switched
	// off on upgrade. That is 00026's mistake (a default chosen for new rows applied to old
	// ones), which is why the column is nullable and this field is a pointer.
	FetchEverySeconds *int
	// FetchMaxPerRun overrides `filler.fetch.max_per_run` for this source. nil = inherit.
	//
	// ⚠ 0 is meaningless here rather than meaningful — "fetch nothing per run" is what
	// FetchEverySeconds=0 says, and saying it twice invites the two to disagree. The API rejects
	// it; the store does not need to.
	FetchMaxPerRun *int
	// Geography is asserted source coverage. Country-only sources may feed any market in that
	// country; a market-scoped source may feed only that market. Empty means unknown.
	Geography filler.Geography
}

// FetchEvery resolves this source's poll interval against the global default (§10 V38c).
//
// ⚠ Returns (interval, ok). `ok=false` means NEVER poll this source — distinct from "poll every
// zero seconds", which is what a bare int return would have to encode as. The two callers that
// matter (the fetcher's per-source decision, and the config disclosure's display) must not
// re-derive this three-state logic separately.
func (f FillerSource) FetchEvery(global time.Duration) (time.Duration, bool) {
	if f.FetchEverySeconds == nil {
		return global, global > 0
	}
	if *f.FetchEverySeconds <= 0 {
		return 0, false
	}
	return time.Duration(*f.FetchEverySeconds) * time.Second, true
}

// MaxPerRun resolves this source's per-run cap against the global default.
func (f FillerSource) MaxPerRun(global int) int {
	if f.FetchMaxPerRun == nil || *f.FetchMaxPerRun < 1 {
		return global
	}
	return *f.FetchMaxPerRun
}

// GeographicallyEligible reports whether this source may contribute to the target installation.
func (f FillerSource) GeographicallyEligible(target filler.Geography) bool {
	return filler.SourceGeographicallyEligible(f.Geography, target)
}

// Fetchable reports whether this source can be DOWNLOADED FROM — i.e. whether it may enter a
// pull plan and be handed to the ingest job.
//
// ⚠ **This invariant used to be implicit and became false when the table flattened (V37).**
// Before, `filler_sources` held only operator-registered remotes, so "every row here is
// fetchable" was true by construction and both pull paths filtered on `Enabled` alone. Migration
// 00029 seeds the config-backed singletons (`folder`, `library`) into the same table, and those
// are SCANNED, not fetched — the folder has no URL to download from and its `uri` is empty.
//
// Without this, a pull plan included a folder row and approval handed `Ingest` an empty string:
// a fetch of nothing, enqueued as real work. Caught by the existing pull tests, which is why the
// check lives on the domain type rather than being re-derived at each call site — there are two
// (propose and the approve-time re-check) and they must not disagree about what is fetchable.
func (f FillerSource) Fetchable() bool {
	switch f.Kind {
	case "folder", "library":
		return false
	default:
		return f.URI != ""
	}
}

// Scannable reports whether this source is read from local/LAN storage rather than downloaded.
//
// ⚠ **The sibling of Fetchable, and the two are deliberately not opposites of one another.** They
// answer different questions and a row can be neither: a `remote` with an empty URI is unusable
// by both. Folding them into one `!Fetchable()` would make "we cannot download this" mean "so
// scan it", which for a malformed remote row means scanning a path the operator never gave us.
//
// ⚠ V38c returned `library` to this set. V35 had it scanned by nothing at all — the media server
// was out of the filler path — and §10 records why that reversed. `folder` and `library` differ
// in WHERE they read (a filesystem path vs a media-server library) but agree on the important
// part: nothing is downloaded, so neither belongs in a pull plan.
func (f FillerSource) Scannable() bool {
	switch f.Kind {
	case "folder", "library":
		return f.URI != ""
	default:
		return false
	}
}

// Configured reports whether this source has been pointed at anything (§10 V51c).
//
// ⚠ **The third state `Fetchable`/`Scannable` can only answer by side effect.** A row can be
// neither fetchable nor scannable for two entirely different reasons — it is a grouping with
// nothing behind it yet (the seeded `youtube` row, which becomes that provider's empty state), or
// it is a malformed remote — and the Sources tab renders those differently: one is an invitation,
// the other is a fault. Reading "not fetchable and not scannable" as either would be wrong half
// the time.
//
// It replaces three inline `URI == ""` tests that each re-derived the same idea in a different
// file, which is how one of them ends up disagreeing after a kind is added.
func (f FillerSource) Configured() bool { return f.URI != "" }

// NewFillerSource builds a source that is ON.
//
// ⚠ **A constructor rather than a struct literal, because the safe value here is not the zero
// value.** `Enabled` is a bool, so `store.FillerSource{ID: …}` means *disabled* — a source that
// an operator just added and that silently never fetches, with a switch in the UI already
// showing "off" and no explanation. Every add path goes through here so that cannot happen; the
// only way to create a disabled source is to switch one off afterwards, which is what the
// operator would expect to have to do.
func NewFillerSource(id, kind, uri, label string, createdAt time.Time) FillerSource {
	return FillerSource{ID: id, Kind: kind, URI: uri, Label: label, CreatedAt: createdAt, Enabled: true, AutoAdmit: true}
}

const fillerSourceSelect = `SELECT id, kind, uri, label, license, last_fetched_at, created_at, enabled,
	auto_admit, fetch_every_seconds, fetch_max_per_run, country, market
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
			// ⚠ sql.NullInt64, because NULL is MEANINGFUL here: it is "inherit the global",
			// distinct from 0 = "never fetch this source" (§10 V38c). Scanning into a plain int
			// would collapse the two and read every unset source as switched off.
			every  sql.NullInt64
			perRun sql.NullInt64
		)
		if err := rows.Scan(&src.ID, &src.Kind, &src.URI, &src.Label, &src.License,
			&fetchedAt, &createdAt, &src.Enabled, &src.AutoAdmit, &every, &perRun,
			&src.Geography.Country, &src.Geography.Market); err != nil {
			return nil, fmt.Errorf("scan filler source: %w", err)
		}
		src.LastFetchedAt = fromEpoch(fetchedAt)
		src.CreatedAt = fromEpoch(createdAt)
		if every.Valid {
			v := int(every.Int64)
			src.FetchEverySeconds = &v
		}
		if perRun.Valid {
			v := int(perRun.Int64)
			src.FetchMaxPerRun = &v
		}
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
		// ⚠ The two override columns follow `enabled` exactly: INSERTed so a new row carries the
		// caller's choice, and absent from DO UPDATE so a caller that knows nothing about them
		// cannot blank them. A Go nil pointer writes NULL — which is "inherit" — so including
		// them in the update list would silently reset every operator's per-source tuning on the
		// next re-register. Same failure V35 nearly shipped with `enabled`, one column over.
		// SetFillerSourceFetchPolicy is their only writer.
		`INSERT INTO filler_sources (id, kind, uri, label, license, last_fetched_at, created_at, enabled,
		   auto_admit, fetch_every_seconds, fetch_max_per_run, country, market)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   kind=excluded.kind, uri=excluded.uri, label=excluded.label, license=excluded.license`),
		src.ID, src.Kind, src.URI, src.Label, src.License,
		epoch(src.LastFetchedAt), epoch(src.CreatedAt), src.Enabled, src.AutoAdmit,
		nullableInt(src.FetchEverySeconds), nullableInt(src.FetchMaxPerRun),
		src.Geography.Normalize().Country, src.Geography.Normalize().Market)
	if err != nil {
		return fmt.Errorf("upsert filler source %s: %w", src.ID, err)
	}
	return nil
}

// SetFillerSourceGeography is the sole writer for an existing source's asserted coverage.
func (s *sqlStore) SetFillerSourceGeography(ctx context.Context, id string, geography filler.Geography) error {
	geography = geography.Normalize()
	res, err := s.db.ExecContext(ctx, s.ph(
		`UPDATE filler_sources SET country = ?, market = ? WHERE id = ?`),
		geography.Country, geography.Market, id)
	if err != nil {
		return fmt.Errorf("set filler source geography %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set filler source geography %s: %w", id, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// nullableInt turns an optional override into a driver value: nil → NULL (inherit the global).
//
// ⚠ Not `int64(0)` for nil. NULL and 0 are DIFFERENT states here — inherit vs never — and
// collapsing them is the whole hazard the nullable column exists to avoid (§10 V38c).
func nullableInt(v *int) any {
	if v == nil {
		return nil
	}
	return int64(*v)
}

// SetFillerSourceFetchPolicy writes one source's per-source overrides (§10 V38c).
//
// ⚠ **The ONLY writer of those two columns**, exactly as SetFillerSourceEnabled owns `enabled`
// and MarkFillerSourceFetched owns the fetch stamp — `UpsertFillerSource` deliberately omits all
// of them from its DO UPDATE list, so a re-register cannot silently reset an operator's tuning.
//
// nil clears an override back to "inherit the global", which is a real operator action ("stop
// treating this source specially") and must be expressible.
func (s *sqlStore) SetFillerSourceFetchPolicy(ctx context.Context, id string, everySeconds, maxPerRun *int) error {
	res, err := s.db.ExecContext(ctx, s.ph(
		`UPDATE filler_sources SET fetch_every_seconds = ?, fetch_max_per_run = ? WHERE id = ?`),
		nullableInt(everySeconds), nullableInt(maxPerRun), id)
	if err != nil {
		return fmt.Errorf("set filler source fetch policy %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set filler source fetch policy %s: %w", id, err)
	}
	if n == 0 {
		return ErrNotFound
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

// SetFillerSourceAutoAdmit is the sole writer of a source's admission policy. A targeted update
// prevents an unrelated re-registration or enabled toggle from changing a trust decision.
func (s *sqlStore) SetFillerSourceAutoAdmit(ctx context.Context, id string, autoAdmit bool) error {
	res, err := s.db.ExecContext(ctx,
		s.ph(`UPDATE filler_sources SET auto_admit = ? WHERE id = ?`), autoAdmit, id)
	if err != nil {
		return fmt.Errorf("set filler source auto-admit %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set filler source auto-admit %s: %w", id, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
