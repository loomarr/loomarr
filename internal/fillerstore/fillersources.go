package fillerstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/store"
)

// FillerProvider is persisted policy shared by every source of one remote kind.
type FillerProvider struct {
	Kind    string
	Enabled bool
}

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
	// LastCheckedAt records a successful bounded source check even when nothing new was found.
	// Automatic-download due planning uses this fact; LastFetchedAt remains the narrower
	// provenance fact that at least one item was actually queued.
	LastCheckedAt time.Time
	// CheckFailureCount and CheckRetryAt retain provider-listing backoff across restarts.
	// CheckLeaseUntil is the atomic source-level claim shared by scheduled and manual checks.
	CheckFailureCount int
	CheckRetryAt      time.Time
	CheckLeaseUntil   time.Time
	// ScanCheckpoint and LastCheckSummary are the restart-safe automatic-acquisition read model.
	ScanCheckpoint   filler.SourceScanCheckpoint
	LastCheckSummary filler.SourceCheckSummary
	CreatedAt        time.Time
	// Enabled is the Sources tab's on/off switch (V35). A disabled source is not scanned, not
	// searched and not downloaded from.
	//
	// ⚠ **Disabling is not deleting, and nothing here may blur that.** The row keeps its
	// licence and its fetch history, so switching it back on restores what was there. Clips it
	// already brought in are untouched either way — they are real files an operator may have
	// tagged and pinned.
	Enabled bool
	// ProviderEnabled is the provider-level half of effective acquisition policy. It is populated
	// by ListFillerSources for remote kinds and deliberately does not overwrite Enabled, which is
	// the operator's remembered choice for this exact target.
	ProviderEnabled bool
	// OnForPlace reports a curated source Loomarr switched on because it covers a station near a
	// current place (#2059 decision 10), so Sources can say why it is on.
	OnForPlace bool
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
	// Geography is asserted source coverage: the country it serves (a source has no place).
	// Empty means unknown.
	Geography filler.Geography
}

// EffectiveEnabled is the one source-policy projection acquisition callers use. Local sources
// have no provider policy; remote sources require both their own and their provider's switches.
func (f FillerSource) EffectiveEnabled() bool {
	switch f.Kind {
	case "archive", "youtube":
		return f.Enabled && f.ProviderEnabled
	default:
		return f.Enabled
	}
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
// An empty source value means "follow this installation", not "ask the operator to repeat the
// same location on every row". Candidate geography is still checked independently by the pull
// planner, so this inheritance cannot make missing or conflicting item evidence pass.
func (f FillerSource) GeographicallyEligible(target filler.Geography) bool {
	return filler.SourceGeographicallyEligible(f.EffectiveGeography(target), target)
}

// EffectiveGeography resolves where a source may FETCH for: its own assertion, or the
// installation's home geography when it asserts no country. It answers "may we pull from it",
// never "where did its clips air": clip enrichment reads only the declared Geography, because
// copying this inheritance into clips labelled British archive ads as US (#1670).
func (f FillerSource) EffectiveGeography(home filler.Geography) filler.Geography {
	coverage := filler.Geography{Country: f.Geography.Normalize().Country}
	if coverage.Country == "" {
		return filler.Geography{Country: home.Normalize().Country} // a source covers a country, never a place
	}
	return coverage
}

// ServesHome reports whether this source may fetch for the installation as configured: the
// installation names a country and the source serves it. Unlike GeographicallyEligible, which
// admits everything for an installation with no country, nothing is served before a location is set.
func (f FillerSource) ServesHome(home filler.Geography) bool {
	home = home.Normalize()
	return home.Country != "" && f.GeographicallyEligible(home)
}

// FetchPlan is this source as the fetcher schedules it — the one projection the fetch job and the
// Sources page's next-check time both read, so they cannot disagree about whether or when a source
// is checked. eligible is the caller's geography verdict (normally ServesHome); region its rank.
//
// ⚠ The three-state override is resolved HERE, by FetchEvery, and handed to the fetcher already
// decided (§10 V38c). Re-deriving nil-vs-0-vs-N in the fetcher is how the two would drift, and the
// way they would drift is toward treating "never" as "inherit" — fetching from a source the
// operator opted out of.
func (f FillerSource) FetchPlan(eligible bool, region int, globalEvery time.Duration) filler.FetchSource {
	every, pollable := f.FetchEvery(globalEvery)
	if !pollable {
		every = 0
	}
	return filler.FetchSource{
		ID: f.ID, Kind: f.Kind, URI: f.URI, Enabled: f.EffectiveEnabled() && eligible, Region: region,
		NeverFetch:        !pollable,
		Every:             every,
		LastCheckedAt:     f.LastCheckedAt,
		CheckFailureCount: f.CheckFailureCount,
		CheckRetryAt:      f.CheckRetryAt,
		CheckLeaseUntil:   f.CheckLeaseUntil,
		MaxPerRun:         f.MaxPerRun(0),
		ScanCheckpoint:    f.ScanCheckpoint,
	}
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
	return FillerSource{ID: id, Kind: kind, URI: uri, Label: label, CreatedAt: createdAt, Enabled: true}
}

const fillerSourceSelect = `SELECT s.id, s.kind, s.uri, s.label, s.license, s.last_fetched_at, s.last_checked_at,
	s.check_failure_count, s.check_retry_at, s.check_lease_until, s.created_at, s.enabled,
	s.fetch_every_seconds, s.fetch_max_per_run, s.country,
	s.scan_cursor, s.scan_watermark, s.scan_pending_watermark,
	CASE WHEN s.kind IN ('archive', 'youtube') THEN COALESCE(p.enabled, FALSE) ELSE TRUE END,
	COALESCE(o.local_state, '') = 'on'
	FROM filler_sources s LEFT JOIN filler_providers p ON p.kind = s.kind
	LEFT JOIN filler_curated_offers o ON o.source_id = s.id`

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
			src        FillerSource
			fetchedAt  int64
			checkedAt  int64
			retryAt    int64
			leaseUntil int64
			createdAt  int64
			// ⚠ sql.NullInt64, because NULL is MEANINGFUL here: it is "inherit the global",
			// distinct from 0 = "never fetch this source" (§10 V38c). Scanning into a plain int
			// would collapse the two and read every unset source as switched off.
			every  sql.NullInt64
			perRun sql.NullInt64
		)
		if err := rows.Scan(&src.ID, &src.Kind, &src.URI, &src.Label, &src.License,
			&fetchedAt, &checkedAt, &src.CheckFailureCount, &retryAt, &leaseUntil,
			&createdAt, &src.Enabled, &every, &perRun,
			&src.Geography.Country,
			&src.ScanCheckpoint.Cursor, &src.ScanCheckpoint.Watermark,
			&src.ScanCheckpoint.PendingWatermark, &src.ProviderEnabled, &src.OnForPlace); err != nil {
			return nil, fmt.Errorf("scan filler source: %w", err)
		}
		src.LastFetchedAt = fromEpoch(fetchedAt)
		src.LastCheckedAt = fromEpoch(checkedAt)
		src.CheckRetryAt = fromEpoch(retryAt)
		src.CheckLeaseUntil = fromEpoch(leaseUntil)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close filler source rows: %w", err)
	}
	if err := s.loadFillerSourceCheckSummaries(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *sqlStore) loadFillerSourceCheckSummaries(ctx context.Context, sources []FillerSource) error {
	if len(sources) == 0 {
		return nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT source_id, disposition, item_count
		FROM filler_source_check_outcomes`)
	if err != nil {
		return fmt.Errorf("list filler source check outcomes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	byID := make(map[string]*FillerSource, len(sources))
	for i := range sources {
		byID[sources[i].ID] = &sources[i]
	}
	for rows.Next() {
		var id string
		var disposition filler.SourceOutcome
		var count int
		if err := rows.Scan(&id, &disposition, &count); err != nil {
			return fmt.Errorf("scan filler source check outcome: %w", err)
		}
		if source := byID[id]; source != nil {
			if source.LastCheckSummary == nil {
				source.LastCheckSummary = filler.SourceCheckSummary{}
			}
			source.LastCheckSummary[disposition] = count
		}
	}
	return rows.Err()
}

// ListFillerProviders returns the two built-in remote providers in a stable UI order.
func (s *sqlStore) ListFillerProviders(ctx context.Context) ([]FillerProvider, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(`SELECT kind, enabled FROM filler_providers
		ORDER BY CASE kind WHEN 'archive' THEN 0 WHEN 'youtube' THEN 1 ELSE 2 END, kind`))
	if err != nil {
		return nil, fmt.Errorf("list filler providers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []FillerProvider
	for rows.Next() {
		var provider FillerProvider
		if err := rows.Scan(&provider.Kind, &provider.Enabled); err != nil {
			return nil, fmt.Errorf("scan filler provider: %w", err)
		}
		out = append(out, provider)
	}
	return out, rows.Err()
}

// SetFillerProviderEnabled is the only provider-policy writer.
func (s *sqlStore) SetFillerProviderEnabled(ctx context.Context, kind string, enabled bool) error {
	res, err := s.db.ExecContext(ctx, s.ph(`UPDATE filler_providers SET enabled = ? WHERE kind = ?`), enabled, kind)
	if err != nil {
		return fmt.Errorf("set filler provider enabled: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set filler provider enabled rows: %w", err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
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
		`INSERT INTO filler_sources (id, kind, uri, label, license, last_fetched_at, last_checked_at,
		   check_failure_count, check_retry_at, check_lease_until, created_at, enabled,
		   fetch_every_seconds, fetch_max_per_run, country)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   kind=excluded.kind, uri=excluded.uri, label=excluded.label, license=excluded.license`),
		src.ID, src.Kind, src.URI, src.Label, src.License,
		epoch(src.LastFetchedAt), epoch(src.LastCheckedAt), src.CheckFailureCount,
		epoch(src.CheckRetryAt), epoch(src.CheckLeaseUntil), epoch(src.CreatedAt), src.Enabled,
		nullableInt(src.FetchEverySeconds), nullableInt(src.FetchMaxPerRun),
		src.Geography.Normalize().Country)
	if err != nil {
		return fmt.Errorf("upsert filler source %s: %w", src.ID, err)
	}
	return nil
}

// OfferCuratedSources records each new offer and inserts its source switched off, in one
// transaction, so a crash cannot leave an offer without the row it promised.
//
// ⚠ Never an upsert of the source. A row that exists, added by hand or offered before, is the
// operator's: its label, switch and geography are left exactly as they are. The bare ON CONFLICT
// also covers the unique (kind, uri) index, so a row holding the same target under another id is
// kept rather than failing the whole offer on every boot.
func (s *sqlStore) OfferCuratedSources(ctx context.Context, list filler.CuratedList, now time.Time) (int, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("offer curated sources: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	added := 0
	for _, c := range list.Sources {
		res, err := tx.ExecContext(ctx, s.ph(`INSERT INTO filler_curated_offers (source_id, list_version, offered_at)
			VALUES (?, ?, ?) ON CONFLICT(source_id) DO NOTHING`), c.SourceID(), list.Version, epoch(now))
		if err != nil {
			return 0, fmt.Errorf("offer curated source %s: %w", c.SourceID(), err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("offer curated source %s: %w", c.SourceID(), err)
		}
		// The covered stations are the shipped list's, not the operator's: every version refreshes
		// them, an earlier offer's included (#2059 decision 10).
		if _, err := tx.ExecContext(ctx, s.ph(`UPDATE filler_curated_offers SET stations = ? WHERE source_id = ?`),
			strings.Join(c.Stations, ","), c.SourceID()); err != nil {
			return 0, fmt.Errorf("tag curated source %s: %w", c.SourceID(), err)
		}
		if n == 0 {
			continue
		}
		geography := filler.Geography{Country: c.Country}.Normalize()
		res, err = tx.ExecContext(ctx, s.ph(`INSERT INTO filler_sources (id, kind, uri, label, license,
			  last_fetched_at, created_at, enabled, country)
			VALUES (?, ?, ?, ?, '', 0, ?, ?, ?) ON CONFLICT DO NOTHING`),
			c.SourceID(), c.Kind, c.Target, c.Label, epoch(now), false, geography.Country)
		if err != nil {
			return 0, fmt.Errorf("add curated source %s: %w", c.SourceID(), err)
		}
		if n, err := res.RowsAffected(); err != nil {
			return 0, fmt.Errorf("add curated source %s: %w", c.SourceID(), err)
		} else {
			added += int(n)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("offer curated sources: %w", err)
	}
	return added, nil
}

// SwitchLocalCuratedSources switches curated sources on and off for the stations near the current
// places (#2059 decision 10, clarified: setting a place is the consent for its local sources), and
// returns the ids it switched on and off. One transaction, following each offer's local_state:
//   - near, untouched and off: switched on ('on').
//   - near, untouched and already on: the operator's ('operator'), never touched again.
//   - 'on' but now off: the operator switched it off, so it stays off ('off').
//   - 'on', on, and no longer near: switched back off and untouched again (”).
func (s *sqlStore) SwitchLocalCuratedSources(ctx context.Context, near map[string]bool) (on, off []string, err error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("switch local curated sources: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `SELECT o.source_id, o.stations, o.local_state, s.enabled
		FROM filler_curated_offers o JOIN filler_sources s ON s.id = o.source_id
		WHERE o.stations <> '' ORDER BY o.source_id`)
	if err != nil {
		return nil, nil, fmt.Errorf("list local curated sources: %w", err)
	}
	type offer struct {
		id, stations, state string
		enabled             bool
	}
	var offers []offer
	for rows.Next() {
		var o offer
		if err := rows.Scan(&o.id, &o.stations, &o.state, &o.enabled); err != nil {
			_ = rows.Close()
			return nil, nil, fmt.Errorf("scan local curated source: %w", err)
		}
		offers = append(offers, o)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, nil, fmt.Errorf("list local curated sources: %w", err)
	}
	set := func(id, state string, enabled *bool) error {
		if _, err := tx.ExecContext(ctx, s.ph(`UPDATE filler_curated_offers SET local_state = ? WHERE source_id = ?`), state, id); err != nil {
			return fmt.Errorf("record local curated source %s: %w", id, err)
		}
		if enabled == nil {
			return nil
		}
		if _, err := tx.ExecContext(ctx, s.ph(`UPDATE filler_sources SET enabled = ? WHERE id = ?`), *enabled, id); err != nil {
			return fmt.Errorf("switch local curated source %s: %w", id, err)
		}
		return nil
	}
	yes, no := true, false
	for _, o := range offers {
		covers := slices.ContainsFunc(strings.Split(o.stations, ","), func(id string) bool { return near[id] })
		switch {
		case o.state == "" && covers && !o.enabled:
			err, on = set(o.id, "on", &yes), append(on, o.id)
		case o.state == "" && covers:
			err = set(o.id, "operator", nil)
		case o.state == "on" && !o.enabled:
			err = set(o.id, "off", nil)
		case o.state == "on" && !covers:
			err, off = set(o.id, "", &no), append(off, o.id)
		}
		if err != nil {
			return nil, nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("switch local curated sources: %w", err)
	}
	return on, off, nil
}

// SetFillerSourceGeography is the sole writer for an existing source's asserted coverage.
func (s *sqlStore) SetFillerSourceGeography(ctx context.Context, id string, geography filler.Geography) error {
	geography = geography.Normalize()
	res, err := s.db.ExecContext(ctx, s.ph(
		`UPDATE filler_sources SET country = ? WHERE id = ?`),
		geography.Country, id)
	if err != nil {
		return fmt.Errorf("set filler source geography %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set filler source geography %s: %w", id, err)
	}
	if n == 0 {
		return store.ErrNotFound
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
		return store.ErrNotFound
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
		return store.ErrNotFound
	}
	return nil
}

// MarkFillerSourceFetched stamps a successful fetch. Returns store.ErrNotFound for an unknown id
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
		return store.ErrNotFound
	}
	return nil
}

// ClaimFillerSourceCheck atomically leases one source against the last-check snapshot the planner
// observed. The CAS closes both scheduled-vs-manual overlap and stale-planner races.
func (s *sqlStore) ClaimFillerSourceCheck(
	ctx context.Context, id string, observedLastCheck, now, leaseUntil time.Time,
) (bool, error) {
	res, err := s.db.ExecContext(ctx, s.ph(`UPDATE filler_sources SET check_lease_until = ?
		WHERE id = ? AND check_lease_until <= ? AND last_checked_at = ?`),
		epoch(leaseUntil), id, epoch(now), epoch(observedLastCheck))
	if err != nil {
		return false, fmt.Errorf("claim filler source check %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("claim filler source check rows %s: %w", id, err)
	}
	return n == 1, nil
}

// CompleteFillerSourceCheck commits a successful check only for the lease that performed it.
// The guarded completion prevents a timed-out stale worker from clearing a newer worker's claim.
func (s *sqlStore) CompleteFillerSourceCheck(
	ctx context.Context, id string, leaseUntil time.Time, completion filler.SourceCheckCompletion,
) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin filler source check completion %s: %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, s.ph(`UPDATE filler_sources
		SET last_checked_at = ?, check_failure_count = 0, check_retry_at = 0, check_lease_until = 0,
			scan_cursor = ?, scan_watermark = ?, scan_pending_watermark = ?
		WHERE id = ? AND check_lease_until = ?`), epoch(completion.CheckedAt),
		completion.Checkpoint.Cursor, completion.Checkpoint.Watermark,
		completion.Checkpoint.PendingWatermark, id, epoch(leaseUntil))
	if err != nil {
		return fmt.Errorf("complete filler source check %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("complete filler source check rows %s: %w", id, err)
	}
	if n != 1 {
		return filler.ErrSourceCheckClaimLost
	}
	if completion.ReplaceOutcomes {
		if _, err := tx.ExecContext(ctx, s.ph(`DELETE FROM filler_source_check_outcomes WHERE source_id = ?`), id); err != nil {
			return fmt.Errorf("replace filler source check outcomes %s: %w", id, err)
		}
	}
	for disposition, count := range completion.Outcomes {
		if count <= 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO filler_source_check_outcomes
			(source_id, disposition, item_count) VALUES (?, ?, ?)
			ON CONFLICT(source_id, disposition) DO UPDATE SET
			item_count = filler_source_check_outcomes.item_count + excluded.item_count`),
			id, disposition, count); err != nil {
			return fmt.Errorf("record filler source check outcome %s/%s: %w", id, disposition, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit filler source check completion %s: %w", id, err)
	}
	return nil
}

// FailFillerSourceCheck records provider-listing backoff and releases only the matching lease.
func (s *sqlStore) FailFillerSourceCheck(
	ctx context.Context, id string, leaseUntil, retryAt time.Time,
) error {
	res, err := s.db.ExecContext(ctx, s.ph(`UPDATE filler_sources
		SET check_failure_count = check_failure_count + 1, check_retry_at = ?, check_lease_until = 0
		WHERE id = ? AND check_lease_until = ?`), epoch(retryAt), id, epoch(leaseUntil))
	if err != nil {
		return fmt.Errorf("fail filler source check %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("fail filler source check rows %s: %w", id, err)
	}
	if n != 1 {
		return filler.ErrSourceCheckClaimLost
	}
	return nil
}

// SetFillerSourceEnabled switches a source on or off. Returns store.ErrNotFound for an unknown id
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
		return store.ErrNotFound
	}
	return nil
}
