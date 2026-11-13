package clipstore

import (
	"context"

	"encoding/json"
	"errors"
	"fmt"

	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/taxonomy"
	"github.com/loomarr/loomarr/internal/media/defects"
	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

// Clip is the persisted form of a filler clip (§10). It embeds the catalog's value type,
// clipcatalog.Clip; the store owns the persistence concerns (UpdatedAt). Identity is
// the clip's sparse content HASH (§10 V38c) — see clipcatalog.Clip.Hash for why the
// path could not stay the key once many watched folders were allowed. The Tunarr
// program uuid rides alongside, nullable, for Tunarr-backed filler-lists.
//
// ⚠ This comment said PATH until 2026-08-10, two identity changes after the fact.
// Several methods below are still path-keyed on purpose (a scan job carries a path,
// not a hash); read each one's doc rather than assuming a single key.
type Clip struct {
	clips.Clip
	UpdatedAt time.Time
	// CreatedAt is when the clip ENTERED the catalog (§10 V51d, migration 00046) — the "recently
	// added" sort order, and the only honest answer to "what arrived while I was away?".
	//
	// ⚠ A separate column rather than a reading of UpdatedAt, because a folder re-sync bumps
	// UpdatedAt on every clip it touches: an "added" order backed by it would reshuffle the whole
	// catalog after a routine scan. Written ONCE, at insert — `UpsertClip` omits it from the
	// DO UPDATE list for the same reason it omits held/removed_at and the counters.
	//
	// ⚠ A catalog fact, so it lives HERE beside UpdatedAt rather than on clipcatalog.Clip. Nothing in
	// pod assembly, matching or playout reads it; the domain has no opinion about when a row
	// appeared, only about what the clip is.
	CreatedAt time.Time
}

// ClipFilter narrows a ListClips query. Any zero-value field is a wildcard, so a
// zero ClipFilter lists everything (the pod-assembly catalog load).
type ClipFilter struct {
	Role            clips.Role
	Era             int
	Audience        clips.Audience
	Category        string
	GeographicScope clips.GeographicScope
	Country         string
	// Taxon matches the denormalised full tag set, so selecting a parent includes descendants.
	Taxon string
	// WithoutTaxonomyTags restricts to playable clips with no asserted taxonomy rows.
	// It answers the taxonomy manager's coverage gap; unlike UntaggedOnly it is not commercial-only
	// and says nothing about era/audience completeness. Derived-only rows do not count as knowledge.
	WithoutTaxonomyTags bool
	// WithoutTaxonomyAxis restricts to playable clips with no direct assertion on one axis. Axis
	// absence is a neutral browse fact, not necessarily a defect: seasonal and audience cues are
	// intentionally sparse.
	WithoutTaxonomyAxis taxonomy.Axis
	// UntaggedOnly restricts to the clips clipcatalog.Clip.NeedsDetails selects — commercials
	// missing era/audience/category, the AI tagging job's work list (§10).
	UntaggedOnly bool
	// IncludeRemoved lifts the default exclusion of tombstoned clips (V35).
	//
	// ⚠ The DEFAULT is to exclude, and it has to be: pod assembly loads the catalog through
	// this same call with a zero filter, so an opt-OUT would mean a clip the operator removed
	// keeps airing until somebody remembers to pass a flag. Opt-in is the safe polarity.
	IncludeRemoved bool
	// HeldOnly restricts to clips waiting to be filed (§10 V38) — the Incoming queue's work
	// list. The inverse of the default exclusion below, not a lifting of it.
	HeldOnly bool
	// IncludeHeld lifts the default exclusion of HELD clips (§10 V38).
	//
	// ⚠ Same polarity as IncludeRemoved and for a sharper reason. Pod assembly loads the catalog
	// through this same call with a ZERO filter, so an opt-OUT would put every untagged,
	// unreviewed clip Loomarr has ever downloaded straight into a channel's breaks — the exact
	// thing holding exists to prevent. Opt-in means a new caller is safe by default and an
	// unsafe one has to say so in writing.
	IncludeHeld bool
	// Query is a case-insensitive substring match on the clip name — the `name LIKE`
	// filter §7.2 prescribes for the clip corpus. Clip search lives here rather than in
	// /v1/search because a clip is not a provisionable title (§10), so it cannot be a
	// federated Candidate without leaking a non-title into the LLM grounding path.
	Query string
	// IncludeComposites lifts the default exclusion of COMPOSITE clips (§10 V45).
	//
	// ⚠ Same polarity and same reason as IncludeHeld. A composite is a recorded break, NOT airable —
	// pod assembly loads the catalog through this same call with a ZERO filter, so an opt-OUT would
	// put a 16-minute block into a channel's break as one "commercial". Its segments air, it does
	// not. Opt-in means only a caller that explicitly wants to SEE composites (the catalog listing,
	// a lineage view) passes this.
	IncludeComposites bool
	// CompositesOnly restricts to composites — the "recorded breaks awaiting split" work list and the
	// lineage parent lookup. The inverse of the default exclusion, like HeldOnly.
	CompositesOnly bool
	// ParentHash restricts to the SEGMENTS of one composite (§10 V45) — the lineage query
	// ("show me the adverts split out of this break").
	ParentHash string
	// TopLevelOnly restricts to clips with NO parent (§10 V51d) — composites and standalone clips,
	// but not the segments split out of a break. The catalog listing's filter, so a break paginates
	// as ONE container row rather than as the twenty adverts inside it.
	//
	// ⚠ Opt-in, with a sharper argument than its siblings. For Held and IncludeComposites the
	// excluded rows are the unsafe ones; here it is the reverse — **segments are the airable
	// clips**. Pod assembly loads the catalog through the zero filter, so an opt-OUT would remove
	// every advert split out of a recorded break from every channel's breaks: not a degraded pool
	// but the exact inverse of what V45 exists to achieve, and silent.
	//
	// ⚠ Ignored when ParentHash is set — see clipWhere.
	TopLevelOnly bool
	// Hashes restricts to a specific set of clip identities (§10 V51d) — the batch read behind
	// "resolve these N pinned ids to their names".
	//
	// ⚠ It exists because paging made the alternative impossible: the channel pin/exclude editor
	// used to load the WHOLE catalog and build an id→clip map in the client, which a 100-row
	// default page silently truncates. Ask for what you hold.
	//
	// ⚠ The CALLER bounds the list. One bind parameter per hash, and Postgres caps a statement at
	// 65535 — the same ceiling attachTags carries a warning about.
	Hashes []string
	// QueryTranscript widens Query to also search the persisted transcript (§10 V51d).
	//
	// ⚠ Opt-in, and NOT because of safety this time but because of cost and noise. `transcript` is
	// the one long column — a few KB per clip, so a 500-row page scans megabytes — and the one
	// noisy one: "ford" matches "afford" with no ranking available to explain why the row came
	// back. A caller that wants to search what a clip SAYS asks for it.
	QueryTranscript bool
	// Sort selects the ORDER BY column; "" keeps the historical `path` order (see clipOrderBy).
	Sort ClipSort
	// Desc reverses the sort, tie-break included, so a descending page is the exact reverse of the
	// ascending list rather than a differently-tied approximation of it.
	Desc bool
	// Limit caps the rows returned; Offset skips that many first (§10 V51d).
	//
	// ⚠ **`Limit == 0` means NO `LIMIT` CLAUSE, and the default lives in the API — never here.**
	// This is the single most important rule in the paging change: pod assembly loads the catalog
	// through the ZERO filter, so a store-side default of 100 would quietly cut every channel's
	// break pool to the first hundred clips. No error, no log line, just a channel that plays the
	// same few adverts. The API layer applies the operator-facing default; the store obeys.
	//
	// ⚠ Offset without Limit is IGNORED, not emulated: sqlite rejects a bare OFFSET, and a page
	// with no size is not a page.
	Limit  int
	Offset int
}

// ClipSort is the catalog's sort vocabulary (§10 V51d) — a CLOSED set, mapped to a column by a
// fixed switch in clipOrderBy.
//
// ⚠ A typed string rather than a raw one so the column can never be concatenated from client
// input, and an unrecognised value is an error rather than a silent fall-back to the default. A
// silent fall-back turns "the sort control does nothing" into a bug that looks like a UI glitch.
type ClipSort string

const (
	ClipSortName     ClipSort = "name"
	ClipSortDuration ClipSort = "duration"
	ClipSortAdded    ClipSort = "added"
	ClipSortPlays    ClipSort = "plays"
)

// ErrUnknownClipSort is returned by ListClips for a Sort value outside the closed set above. The
// API maps it to a 422; nothing falls back.
var ErrUnknownClipSort = errors.New("unknown clip sort")

// clipOrderBy renders the ORDER BY for a filter (§10 V51d).
//
// ⚠ **Every ordering appends `hash` as a tie-break, and that is a correctness requirement, not
// tidiness.** Without a total order, `ORDER BY duration_ms` under LIMIT/OFFSET may return the same
// row on two pages and skip another entirely — SQL promises nothing about the relative order of
// tied rows between statements, and duration/plays tie constantly.
//
// ⚠ **`name` sorts as `LOWER(name)` on BOTH dialects.** SQLite's default BINARY collation puts
// 'Z' before 'a'; Postgres's locale collation typically does not. Without the LOWER() this one
// function produces two different orders — the per-dialect fork §5's store rules forbid — and the
// conformance fixture carries case-mixed names so a regression fails on exactly one backend.
func clipOrderBy(f ClipFilter) (string, error) {
	var col string
	switch f.Sort {
	case "":
		// The historical order, preserved exactly so the zero filter (pod assembly, coverage, the
		// filler-list builder) is byte-for-byte unchanged by this phase. `hash` still rides along:
		// paths are unique in practice, and "in practice" is not a total order.
		col = "path"
	case ClipSortName:
		col = "LOWER(name)"
	case ClipSortDuration:
		col = "duration_ms"
	case ClipSortAdded:
		col = "created_at"
	case ClipSortPlays:
		col = "play_count"
	default:
		return "", fmt.Errorf("%w: %q", ErrUnknownClipSort, f.Sort)
	}
	dir := "ASC"
	if f.Desc {
		dir = "DESC"
	}
	// The tie-break flips WITH the sort, so a descending page is the exact reverse of the
	// ascending list. Either direction would be total; matching makes the reversal a property the
	// conformance suite can assert.
	return " ORDER BY " + col + " " + dir + ", hash " + dir, nil
}

func (s *Store) UpsertClip(ctx context.Context, c Clip) error {
	_, err := s.h.ExecContext(ctx, s.h.Rebind(
		// ⚠ play_count / last_played_at are INSERTed (so a new row starts at 0) but deliberately
		// NOT in the DO UPDATE list. A re-sync knows nothing about plays, so writing
		// ⚠ `removed_at` is omitted from DO UPDATE for the same reason, and it is the thing that
		// makes "Remove from catalog" survive a re-scan. `clips` is a synced CACHE, so the next
		// pass finds the file still on disk and upserts it; if the tombstone rode along it would
		// be reset to the scan's zero and the clip would silently reappear. SetClipsRemoved is
		// its only writer, exactly like RecordClipPlay is the counters'.
		//
		// ⚠ `held` is omitted for the SAME reason, and the failure is worse
		// (V38). The folder scan re-upserts every file it finds with `held = false`; if that rode
		// along in the update list, one scan pass would file every held clip — clearing the
		// review queue and putting untagged, unreviewed clips straight into channels, with no
		// operator action and nothing in the logs. Enrollment holds new clips; only the atomic
		// terminal-ready transaction may lift one after the pipeline completes.
		//
		// excluded.play_count would reset every counter to the scan's zero on each pass —
		// silently, and only noticeable as "usage never goes up". Tags survive re-sync because
		// sync.go merges them before calling this; the counters survive because the SQL simply
		// never touches them after insert. RecordClipPlay is their only writer.
		// ⚠ Keyed on `hash` since V38c — `path` is ordinary data now, and IS in the DO UPDATE list
		// because a clip's location can legitimately change (re-filed under a new extension, or
		// found in a different folder) while its identity does not. That is the whole point of
		// content addressing: the same bytes are the same clip wherever they sit.
		// ⚠ transcript / brand / visible_text are INSERTed (a new row starts empty) but
		// deliberately OMITTED from DO UPDATE (§10 V44, migration 00038) — the same rule the block
		// above states for language/removed_at/held/counters. The folder scan knows none of them;
		// if they rode the update list, one re-sync would blank a transcribed or vision-read clip.
		// Their writers are the job methods below — SetClipTranscript owns `transcript`,
		// ApplyClipVision owns `visible_text`, and `brand` is written by whichever grounded it
		// (SetClipBrand from text, ApplyClipVision from the frame).
		//
		// ⚠ is_composite / parent_hash (§10 V45, migration 00039) are also INSERTed but OMITTED from
		// DO UPDATE. Set by intake/detection and by split Confirm; the folder scan does not know a
		// file is a composite or whose segment it is, so a re-sync must not flip a confirmed composite
		// back or blank a segment's lineage. Written by SetClipComposite / SetClipParent below.
		//
		// ⚠ `created_at` (§10 V51d, migration 00046) is the FOURTH column with this rule, and this
		// block is where people look for the list. INSERTed so a new row records when it arrived;
		// omitted from DO UPDATE because the scan supplies a fresh timestamp on every pass — riding
		// the update list would mark the entire catalog "just added" after each sync, which is the
		// exact failure the column was added to avoid. It has NO other writer: unlike its
		// neighbours here there is no Set… method, because nothing may ever change when a clip
		// arrived. See clipCreatedAt below for the insert-time value.
		//
		// ⚠ `thumb_image_hash` / `hover_image_hash` (§22 V52 phase 6, migration 00047) are the
		// FIFTH and SIXTH, and they DO have a writer — SetClipArtworkImages, after the adoption
		// job ingests the files. The scan knows nothing about image identities, so including them
		// in the update list would blank every clip's artwork on re-sync.
		`INSERT INTO clips (hash, path, tunarr_program_id, name, name_source, role, placement, era, audience, geographic_scope, country, network, station, air_date, geo_evidence, duration_ms, source, quality, license, thumbnail, preview, thumb_image_hash, hover_image_hash, language, transcript, brand, visible_text, is_composite, parent_hash, play_count, last_played_at, removed_at, held, updated_at, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(hash) DO UPDATE SET
		   enrichment_revision=CASE
		     WHEN clips.name <> excluded.name OR clips.role <> excluded.role OR clips.source <> excluded.source
		     THEN clips.enrichment_revision + 1 ELSE clips.enrichment_revision END,
		   path=excluded.path,
		   tunarr_program_id=excluded.tunarr_program_id,
		   -- name_source travels with name: every writer of a name says where it came from (#1988).
		   name=excluded.name, name_source=excluded.name_source, role=excluded.role,
		   era=excluded.era, audience=excluded.audience,
		   duration_ms=excluded.duration_ms,
		   source=excluded.source, quality=excluded.quality,
		   license=excluded.license,
		   thumbnail=excluded.thumbnail,
		   preview=excluded.preview,
		   -- ⚠ thumb_image_hash / hover_image_hash are DELIBERATELY ABSENT from this list, and
		   -- their absence is load-bearing (§22, V52 phase 6). They are written by ONE writer —
		   -- SetClipArtworkImages, after the ingest — and the folder scan that also calls this
		   -- upsert has no idea what they are. Including them would blank a clip's artwork on
		   -- every re-sync, which is the same class as is_composite/parent_hash above.
		   language=excluded.language,
		   updated_at=excluded.updated_at`),
		c.Hash, c.Path, nullIfEmpty(c.TunarrProgramID), c.Name, clipNameSource(c), string(c.Role), string(clipPlacement(c)), c.Era, string(c.Audience),
		clipGeographicScope(c), c.Country, c.Network, c.Station, c.AirDate, c.GeoEvidence,
		c.DurationMs, c.Source,
		// ⚠ Flags are bound as real bools: they are BOOLEAN on Postgres (storage.md).
		c.Quality, c.License, c.Thumbnail, c.Preview, c.ThumbImageHash, c.HoverImageHash, c.Language,
		c.Transcript, c.Brand, c.VisibleText,
		c.IsComposite, c.ParentHash,
		c.PlayCount, epoch(c.LastPlayedAt), epoch(c.RemovedAt),
		c.Held, epoch(c.UpdatedAt), epoch(clipCreatedAt(c)))
	if err != nil {
		return fmt.Errorf("upsert clip %s: %w", c.Hash, err)
	}
	return nil
}

// clipCreatedAt is the value `created_at` takes at INSERT (§10 V51d).
//
// ⚠ It falls back to UpdatedAt when the caller left CreatedAt zero, and the fallback is the point:
// every existing writer — the folder scan, intake, split confirm — builds a Clip with a fresh
// UpdatedAt and knows nothing about this column. Requiring each of them to set it would mean five
// call sites that must remember, and the one that forgets writes a 0 that sorts to the far end of
// "recently added" forever. The store answers the question it can answer correctly.
//
// ⚠ It is NOT a general default. Only the INSERT reads this; a re-sync never reaches it, because
// `created_at` is absent from the DO UPDATE list.
func clipCreatedAt(c Clip) time.Time {
	if !c.CreatedAt.IsZero() {
		return c.CreatedAt
	}
	return c.UpdatedAt
}

// clipNameSource stores an unset provenance as 'unknown', the value migration 00134 gives a row it
// could not classify, so the column never holds a second spelling of "not recorded".
func clipNameSource(c Clip) string {
	if !c.NameSource.Known() {
		return string(clips.NameFromUnknown)
	}
	return string(c.NameSource)
}

func clipGeographicScope(c Clip) string {
	if c.GeographicScope == "" {
		return string(clips.GeographicUnknown)
	}
	return string(c.GeographicScope)
}

// clipPlacement gives a direct insert of an already-unheld known role its natural placement. Real
// intake inserts held rows and therefore starts not_playable until CommitFillerReady.
func clipPlacement(c Clip) clips.Placement {
	if c.Held {
		return clips.PlacementNotPlayable
	}
	return c.EffectivePlacement()
}

func (s *Store) UpdateClipClassification(ctx context.Context, id string, era int, audience string, updatedAt time.Time) error {
	res, err := s.h.ExecContext(ctx, s.h.Rebind(
		`UPDATE clips SET era = ?, audience = ?, updated_at = ? WHERE hash = ?`),
		era, audience, epoch(updatedAt), id)
	if err != nil {
		return fmt.Errorf("update clip classification %s: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// RecordClipPlay increments a clip's play counter and stamps when it last aired (V28).
//
// ⚠ Called from PLAYOUT — when a filler item actually starts encoding — never from pod
// assembly. Assembly re-runs on every reconcile sweep and would count SCHEDULED rather than
// AIRED, inflating without bound (see migration 00017).
//
// `at` is the scheduled clip start, not the wall clock when the resolver happened to arrive.
// A finite encoder child normally requests its successor milliseconds after that boundary, and
// a viewer reconnect or retry may resolve it again. One airing is one (channel, clip, start)
// row, so all callbacks for it are one durable write without process-local memory; only that
// first write increments the global catalog counter. `recordedAt` is the wall clock of that
// write, which a break's snapshot needs to stay fixed while it airs (pod.ExposuresBefore).
//
// A clip keeps its newest fillerAiringsKept airings and every airing younger than
// fillerAiringsTrimAge: rotation reads the latest airing a break may count, and supply reads every
// airing of the last eight weeks to size the pool. The trim never removes the airing just written,
// so a re-resolve stays a no-op.
//
// A missing clip is NOT an error. Playout resolves a path that the catalog may have pruned
// between the schedule being built and the break airing; failing here would turn a stale
// catalog row into a playback error, and the counter is telemetry, not correctness. The row
// count is deliberately ignored for the same reason.
//
// updated_at is left alone: this is not a catalog edit, and touching it would make every clip
// look freshly re-synced in the UI's "last updated" column.
func (s *Store) RecordClipPlay(ctx context.Context, channelID, id string, at, recordedAt time.Time) (bool, error) {
	tx, err := s.h.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("record clip play %s: %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, s.h.Rebind(`INSERT INTO filler_airings (channel_id, clip_hash, aired_at_ms, recorded_at_ms)
		VALUES (?, ?, ?, ?) ON CONFLICT(channel_id, clip_hash, aired_at_ms) DO NOTHING`),
		channelID, id, at.UnixMilli(), recordedAt.UnixMilli())
	if err != nil {
		return false, fmt.Errorf("record channel clip play %s/%s: %w", channelID, id, err)
	}
	changed, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read channel clip play result %s/%s: %w", channelID, id, err)
	}
	if changed == 0 {
		if err := tx.Commit(); err != nil {
			return false, fmt.Errorf("commit duplicate clip play %s: %w", id, err)
		}
		return false, nil
	}
	// With fewer than fillerAiringsKept rows the subquery is NULL and nothing is deleted. Only airings
	// past the history age go, and never the one just written: a break on air may count any recent airing, and
	// a deleted row would let the same airing be counted again at its next resolve.
	if _, err := tx.ExecContext(ctx, s.h.Rebind(`DELETE FROM filler_airings WHERE clip_hash = ? AND aired_at_ms < ?
		AND NOT (channel_id = ? AND aired_at_ms = ?) AND aired_at_ms <
		(SELECT aired_at_ms FROM filler_airings WHERE clip_hash = ? ORDER BY aired_at_ms DESC LIMIT 1 OFFSET ?)`),
		id, recordedAt.Add(-fillerAiringsTrimAge).UnixMilli(), channelID, at.UnixMilli(), id, fillerAiringsKept-1); err != nil {
		return false, fmt.Errorf("trim clip airings %s: %w", id, err)
	}
	if _, err := tx.ExecContext(ctx, s.h.Rebind(
		`UPDATE clips SET play_count = play_count + 1,
		 last_played_at = CASE WHEN ? > last_played_at THEN ? ELSE last_played_at END
		 WHERE hash = ?`), epoch(at), epoch(at), id); err != nil {
		return false, fmt.Errorf("record clip play %s: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("record clip play %s: %w", id, err)
	}
	return true, nil
}

// fillerAiringsKept is how many of a clip's newest airings RecordClipPlay keeps, beyond those
// younger than fillerAiringsTrimAge, which it always keeps.
const (
	fillerAiringsKept    = 4
	fillerAiringsTrimAge = clips.AiringHistoryAge
)

// FillerAirings returns the household's stored clip airings, every channel's (§10 rotation). The
// per-break snapshot is cut from them by pod.ExposuresBefore, so one read serves a whole window.
func (s *Store) FillerAirings(ctx context.Context) ([]clips.Airing, error) {
	rows, err := s.h.QueryContext(ctx, `SELECT channel_id, clip_hash, aired_at_ms, recorded_at_ms FROM filler_airings`)
	if err != nil {
		return nil, fmt.Errorf("list filler airings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []clips.Airing
	for rows.Next() {
		var a clips.Airing
		var airedMs, recordedMs int64
		if err := rows.Scan(&a.ChannelID, &a.ClipHash, &airedMs, &recordedMs); err != nil {
			return nil, fmt.Errorf("scan filler airing: %w", err)
		}
		a.AiredAt, a.RecordedAt = exposureTime(airedMs), exposureTime(recordedMs)
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list filler airings: %w", err)
	}
	return out, nil
}

// exposureTime maps a stored epoch-ms exposure timestamp to a time; 0 (never) is the zero time.
func exposureTime(ms int64) time.Time {
	if ms <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}

// UpdateClipRole corrects a clip's role (§10). The role decides where a clip sits in a pod — a
// bumper bookends a pod while a commercial fills it — so a mis-detected role produces
// structurally wrong pods, not merely a mis-tagged clip.
func (s *Store) UpdateClipRole(ctx context.Context, id, role string, updatedAt time.Time) error {
	res, err := s.h.ExecContext(ctx, s.h.Rebind(
		`UPDATE clips SET role = ?, updated_at = ? WHERE hash = ?`),
		role, epoch(updatedAt), id)
	if err != nil {
		return fmt.Errorf("update clip role %s: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// RenameClip sets a clip's name and its provenance together (#1990): name_source travels with
// name, as in UpsertClip.
func (s *Store) RenameClip(ctx context.Context, hash, name string, source clips.NameSource, updatedAt time.Time) error {
	res, err := s.h.ExecContext(ctx, s.h.Rebind(
		`UPDATE clips SET name = ?, name_source = ?, updated_at = ? WHERE hash = ?`),
		name, string(source), epoch(updatedAt), hash)
	if err != nil {
		return fmt.Errorf("rename clip %s: %w", hash, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) UpdateClipGeography(ctx context.Context, hash, scope, country, network, station, airDate, evidence string, updatedAt time.Time) error {
	res, err := s.h.ExecContext(ctx, s.h.Rebind(`UPDATE clips SET geographic_scope = ?, country = ?,
		network = ?, station = ?, air_date = ?, geo_evidence = ?, updated_at = ? WHERE hash = ?`),
		scope, country, network, station, airDate, evidence, epoch(updatedAt), hash)
	if err != nil {
		return fmt.Errorf("update clip geography %s: %w", hash, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteClip removes ONE clip by identity. Used by split confirm: the
// compilation's identity is a path that after the cut means twenty clips, not
// one (§10 V34). ErrNotFound for an unknown path.
func (s *Store) DeleteClip(ctx context.Context, id string) error {
	res, err := s.h.ExecContext(ctx, s.h.Rebind(`DELETE FROM clips WHERE path = ?`), id)
	if err != nil {
		return fmt.Errorf("delete clip %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete clip %s: %w", id, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkClipReaped records that a composite's recording has been reclaimed (§10 V54). The row stays;
// only the bytes are gone.
func (s *Store) MarkClipReaped(ctx context.Context, hash string, at time.Time) error {
	res, err := s.h.ExecContext(ctx, s.h.Rebind(
		`UPDATE clips SET reaped_at = ?, updated_at = ? WHERE hash = ?`), epoch(at), epoch(at), hash)
	if err != nil {
		return fmt.Errorf("mark clip %s reaped: %w", hash, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark clip %s reaped: %w", hash, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// pinnedClipsTx is every clip some channel pins. On Postgres it locks the channel rows until tx
// ends, so a concurrent channel write cannot add a pin between this read and the caller's write.
func (s *Store) pinnedClipsTx(ctx context.Context, tx storedb.Querier) (map[string]struct{}, error) {
	rows, err := s.channels.LockChannelPolicies(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("read channel pins: %w", err)
	}
	defer func() { _ = rows.Close() }()
	pinned := map[string]struct{}{}
	for rows.Next() {
		var id, raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, fmt.Errorf("scan channel policy: %w", err)
		}
		var policy schedule.ChannelPolicy
		if err := json.Unmarshal([]byte(raw), &policy); err != nil {
			return nil, fmt.Errorf("decode channel policy %s: %w", id, err)
		}
		if policy.Filler != nil {
			for _, hash := range policy.Filler.Pinned {
				if hash != "" {
					pinned[hash] = struct{}{}
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read channel pins: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close channel pins: %w", err)
	}
	return pinned, nil
}

// RetireDefectiveCopy takes a copy that failed the defect floor off the catalog for good (#2036):
// removed, so nothing airs it, and reaped, so a sync that no longer finds its file keeps the row.
// The row, its fingerprint, its defect record and its airings stay: together they are the "wanted:
// better copy" entry that a cleaner copy of the same ad is matched against, and that copy inherits
// its rotation. The caller deletes the file after this commits.
func (s *Store) RetireDefectiveCopy(ctx context.Context, hash string, at time.Time) error {
	res, err := s.h.ExecContext(ctx, s.h.Rebind(
		`UPDATE clips SET removed_at = ?, reaped_at = ?, updated_at = ? WHERE hash = ? AND is_composite = ?`),
		epoch(at), epoch(at), epoch(at), hash, false)
	if err != nil {
		return fmt.Errorf("retire defective copy %s: %w", hash, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("retire defective copy %s: %w", hash, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// WantedCopy is an ad whose only copy failed the defect floor and was retired: Loomarr wants a
// better copy of it (#2036).
type WantedCopy struct {
	ClipHash   string
	Path       string
	Name       string
	DurationMs int64
	// Reason is the failing verdict's reason; RetiredAt is when the copy came off the catalog.
	Reason    string
	RetiredAt time.Time
}

// ListWantedCopies is every retired defective copy still waiting for a better one. A copy found
// later folds the entry into itself (MergeInferiorCopy), which is what removes it from this list.
func (s *Store) ListWantedCopies(ctx context.Context) ([]WantedCopy, error) {
	rows, err := s.h.QueryContext(ctx, s.h.Rebind(
		`SELECT c.hash, c.path, c.name, c.duration_ms, d.verdict_reason, c.reaped_at
		   FROM clips c JOIN filler_clip_defects d ON d.clip_hash = c.hash
		  WHERE c.reaped_at IS NOT NULL AND c.is_composite = ? AND d.verdict = ?
		  ORDER BY c.reaped_at, c.hash`), false, string(defects.VerdictFail))
	if err != nil {
		return nil, fmt.Errorf("list wanted copies: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []WantedCopy
	for rows.Next() {
		var w WantedCopy
		var retiredAt int64
		if err := rows.Scan(&w.ClipHash, &w.Path, &w.Name, &w.DurationMs, &w.Reason, &retiredAt); err != nil {
			return nil, fmt.Errorf("scan wanted copy: %w", err)
		}
		w.RetiredAt = fromEpoch(retiredAt)
		out = append(out, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list wanted copies: %w", err)
	}
	return out, nil
}

// LiveClipPaths maps every clip DeleteClipsNotIn can delete (not reaped, held and removed ones
// included) to its location under the clip folder. The sync weighs a listing against it before it
// prunes (#2673).
func (s *Store) LiveClipPaths(ctx context.Context) (map[string]string, error) {
	rows, err := s.h.QueryContext(ctx, `SELECT hash, path FROM clips WHERE reaped_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("list live clip paths: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var hash, path string
		if err := rows.Scan(&hash, &path); err != nil {
			return nil, fmt.Errorf("scan live clip path: %w", err)
		}
		out[hash] = path
	}
	return out, rows.Err()
}

// DeleteClipsNotIn prunes clips absent from the given id set (the sync reconcile).
//
// ⚠ An empty keep set deletes EVERY live clip, with no question asked: this is the primitive, not
// the policy. The sync (internal/filler) is where a listing is weighed against the catalog, so it
// never hands an empty or shrunken keep set to this on the strength of a folder that merely
// looked empty (#2673). Returns the count removed.
func (s *Store) DeleteClipsNotIn(ctx context.Context, keepIDs []string) (int, error) {
	// Pruning by "no matching clip" rather than by the keep-set means one statement covers both
	// branches below and cannot disagree with whichever DELETE ran.
	defer func() { _ = s.pruneOrphanClipFingerprints(ctx); _ = s.pruneOrphanClipDefects(ctx) }()

	// The pipeline rows and split proposals, the no-foreign-key siblings of `clips`, are the filler
	// store's: its DeleteClipsNotIn runs this one and then prunes them (internal/filler/records).

	if len(keepIDs) == 0 {
		// ⚠ Reaped composites survive an EMPTY keep set too, which is exactly when a swept reel
		// looks most like a deleted one — taking the tombstones here would dangle their children
		// just as surely. (An unreadable clip folder never reaches this: the scan errors first,
		// and the sync holds a prune that would empty a non-empty catalog.)
		res, err := s.h.ExecContext(ctx, `DELETE FROM clips WHERE reaped_at IS NULL`)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		return int(n), nil
	}
	placeholders := make([]string, len(keepIDs))
	args := make([]any, len(keepIDs))
	for i, id := range keepIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	// ⚠ Keyed on `hash`, the clip's IDENTITY since V38c — not on `path`.
	//
	// This said `path NOT IN (…)` and was missed when identity moved off the path. The sync's
	// `keep` set carries hashes, and a hash never equals a path, so EVERY clip matched
	// "not in the keep set" and the whole catalog was deleted on every single sync. The route
	// reported "1 added, 1 pruned" forever and the catalog stayed empty — filler silently never
	// worked. Found by running the real binary; the conformance suite passed throughout because
	// its fixtures set `path` and `hash` to the same string.
	// ⚠ **A REAPED composite is exempt** (§10 V54). The split sweep deletes a spent compilation's
	// recording on purpose, so its absence from the scan is expected rather than news. Without this
	// the very next sync would prune the row — and every clip cut out of that reel carries
	// `parent_hash` pointing at it, so one sweep would dangle all 47 children and take V45's
	// lineage with it. The row is a tombstone; only the bytes are gone.
	q := s.h.Rebind(`DELETE FROM clips WHERE reaped_at IS NULL AND hash NOT IN (` + strings.Join(placeholders, ",") + `)`)
	res, err := s.h.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, fmt.Errorf("prune clips: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ⚠ There is deliberately no bool-to-int helper here. Every bool column is BOOLEAN on Postgres
// (00033 rebuilt `clips`), so binding an int is a 42804; bind the Go bool directly.

// nullIfEmpty writes "" as SQL NULL.
//
// For tunarr_program_id specifically: the column is nullable because an install with no Tunarr
// has no uuid for its clips. Storing "" instead would make every such clip share a value, and
// any future UNIQUE constraint on it would then reject the second clip — whereas SQL NULLs do
// not collide. Distinguishing "no Tunarr" from "the empty uuid" also keeps the sync's
// preserve-a-known-uuid branch honest.
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// escapeLike neutralizes LIKE metacharacters so a search term is matched literally.
// Without it, a query of "%" matches every clip and "_" matches any single character —
// surprising for a plain search box, and a needless full-table scan.
func escapeLike(term string) string {
	r := strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`)
	return r.Replace(term)
}
