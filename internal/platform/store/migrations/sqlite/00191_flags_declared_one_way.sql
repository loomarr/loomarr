-- +goose Up
-- Schema to a 10 (#2793): a flag is declared one way on both engines. In SQLite that is
-- BOOLEAN NOT NULL CHECK (col IN (0, 1)), the twin of Postgres boolean; before this, some flags
-- were INTEGER and some BOOLEAN, and none refused a 2. images.meta, jsonb in Postgres, gains its
-- SQLite twin CHECK (json_valid(meta)). The parity test holds the engines to this.
-- SQLite cannot add a CHECK to an existing column, so each table is rebuilt transactionally. Renaming
-- a parent repoints its children's foreign keys at the renamed copy, and dropping that copy would
-- cascade-delete them, so every descendant is rebuilt with its parent (the 00188 pattern): children
-- are renamed first, created after the parent, and dropped first. Each copy keeps its columns in
-- order, so INSERT ... SELECT * carries every value across; dropping a copy drops its indexes, which
-- are created again at the end of each section.

-- users and its children ------------------------------------------------------------------------
ALTER TABLE sessions RENAME TO sessions_old;
ALTER TABLE device_pairings RENAME TO device_pairings_old;
ALTER TABLE device_tokens RENAME TO device_tokens_old;
ALTER TABLE password_recovery_grants RENAME TO password_recovery_grants_old;
ALTER TABLE password_recoveries RENAME TO password_recoveries_old;
ALTER TABLE user_hidden_ideas RENAME TO user_hidden_ideas_old;
ALTER TABLE client_crash_reports RENAME TO client_crash_reports_old;
ALTER TABLE user_favourite_channels RENAME TO user_favourite_channels_old;
ALTER TABLE user_recent_channels RENAME TO user_recent_channels_old;
ALTER TABLE users RENAME TO users_old;
CREATE TABLE users (
    id                  TEXT    PRIMARY KEY,          -- media-server user id (§11)
    name                TEXT    NOT NULL,
    role                TEXT    NOT NULL DEFAULT 'member', -- admin | member
    disabled            BOOLEAN NOT NULL DEFAULT FALSE CHECK (disabled IN (0, 1)),
    quota               INTEGER NOT NULL DEFAULT 0,  -- pending-acquisition cap; 0 = default
    auto_approve        BOOLEAN NOT NULL DEFAULT FALSE CHECK (auto_approve IN (0, 1)),
    created_at          BIGINT  NOT NULL DEFAULT 0,
    updated_at          BIGINT  NOT NULL DEFAULT 0,
    password_hash       TEXT,
    media_server_linked BOOLEAN NOT NULL DEFAULT FALSE CHECK (media_server_linked IN (0, 1)),
    last_seen_at        BIGINT  NOT NULL DEFAULT 0
);
CREATE TABLE sessions (
    token_hash   TEXT   PRIMARY KEY,          -- SHA-256 of the cookie token (§11)
    user_id      TEXT   NOT NULL,
    created_at   BIGINT NOT NULL DEFAULT 0,
    expires_at   BIGINT NOT NULL DEFAULT 0,   -- sliding TTL; janitor purges past this
    last_seen_at BIGINT NOT NULL DEFAULT 0,
    client_label TEXT   NOT NULL DEFAULT '',
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE TABLE device_pairings (
    device_code TEXT PRIMARY KEY,          -- SHA-256 of the device's polling secret, never plaintext
    user_code   TEXT NOT NULL UNIQUE,      -- the short human-typed code, shown on the TV
    user_id     TEXT,                      -- set at approval; NULL while pending
    device_name TEXT NOT NULL DEFAULT '',  -- client-supplied label ("Living Room Shield")
    created_at  BIGINT NOT NULL DEFAULT 0,
    expires_at  BIGINT NOT NULL DEFAULT 0, -- the pairing window closes fast; a stale code must not linger
    approved_at BIGINT NOT NULL DEFAULT 0, -- 0 = still pending
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE TABLE device_tokens (
    token_hash   TEXT PRIMARY KEY,          -- SHA-256 of the device token (§11)
    user_id      TEXT NOT NULL,             -- the member who approved it; the device inherits ITS role
    device_name  TEXT NOT NULL DEFAULT '',
    created_at   BIGINT NOT NULL DEFAULT 0,
    last_seen_at BIGINT NOT NULL DEFAULT 0, -- so the revocation UI can show what is actually in use
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE TABLE password_recoveries (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status      TEXT NOT NULL CHECK (status IN ('pending', 'redeemed', 'expired', 'revoked')),
    created_at  BIGINT NOT NULL,
    expires_at  BIGINT NOT NULL,
    terminal_at BIGINT NOT NULL DEFAULT 0,
    CHECK (expires_at = created_at + 1800),
    CHECK (
        (status = 'pending' AND terminal_at = 0) OR
        (status = 'expired' AND terminal_at = expires_at) OR
        (status IN ('redeemed', 'revoked') AND terminal_at > 0)
    )
);
CREATE TABLE password_recovery_grants (
    token_hash   TEXT PRIMARY KEY CHECK (length(token_hash) = 64 AND token_hash NOT GLOB '*[^0-9a-f]*'),
    recovery_id  TEXT NOT NULL REFERENCES password_recoveries(id) ON DELETE CASCADE,
    created_at   BIGINT NOT NULL,
    expires_at   BIGINT NOT NULL,
    consumed_at  BIGINT NOT NULL DEFAULT 0,
    revoked_at   BIGINT NOT NULL DEFAULT 0,
    CHECK (expires_at > created_at AND expires_at <= created_at + 1800),
    CHECK (consumed_at = 0 OR revoked_at = 0)
);
CREATE TABLE user_hidden_ideas (
    user_id      TEXT   NOT NULL,
    idea_id      TEXT   NOT NULL,
    hidden_at_ms BIGINT NOT NULL CHECK (hidden_at_ms > 0),
    PRIMARY KEY (user_id, idea_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE TABLE client_crash_reports (
    id               TEXT PRIMARY KEY,
    received_at_ms   BIGINT NOT NULL CHECK (received_at_ms > 0),
    occurred_at_ms   BIGINT NOT NULL,
    user_id          TEXT REFERENCES users(id) ON DELETE SET NULL,
    device_id        TEXT NOT NULL DEFAULT '',
    device_name      TEXT NOT NULL DEFAULT '',
    kind             TEXT NOT NULL CHECK (kind IN ('js-fatal', 'js-error', 'native')),
    platform         TEXT NOT NULL CHECK (platform IN ('ios', 'android', 'tvos', 'android-tv')),
    app_version      TEXT NOT NULL,
    app_build        TEXT NOT NULL,
    os_version       TEXT NOT NULL,
    device_model     TEXT NOT NULL,
    message          TEXT NOT NULL,
    stack            TEXT NOT NULL DEFAULT '',
    breadcrumbs_json TEXT NOT NULL DEFAULT '[]',
    native_payload   TEXT NOT NULL DEFAULT ''
);
CREATE TABLE user_favourite_channels (
    user_id     TEXT   NOT NULL,
    channel_id  TEXT   NOT NULL,
    added_at_ms BIGINT NOT NULL CHECK (added_at_ms > 0),
    PRIMARY KEY (user_id, channel_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE
);
CREATE TABLE user_recent_channels (
    user_id     TEXT   NOT NULL,
    channel_id  TEXT   NOT NULL,
    tuned_at_ms BIGINT NOT NULL CHECK (tuned_at_ms > 0),
    PRIMARY KEY (user_id, channel_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE
);
INSERT INTO users SELECT * FROM users_old;
INSERT INTO sessions SELECT * FROM sessions_old;
INSERT INTO device_pairings SELECT * FROM device_pairings_old;
INSERT INTO device_tokens SELECT * FROM device_tokens_old;
INSERT INTO password_recoveries SELECT * FROM password_recoveries_old;
INSERT INTO password_recovery_grants SELECT * FROM password_recovery_grants_old;
INSERT INTO user_hidden_ideas SELECT * FROM user_hidden_ideas_old;
INSERT INTO client_crash_reports SELECT * FROM client_crash_reports_old;
INSERT INTO user_favourite_channels SELECT * FROM user_favourite_channels_old;
INSERT INTO user_recent_channels SELECT * FROM user_recent_channels_old;
DROP TABLE sessions_old;
DROP TABLE device_pairings_old;
DROP TABLE device_tokens_old;
DROP TABLE password_recovery_grants_old;
DROP TABLE password_recoveries_old;
DROP TABLE user_hidden_ideas_old;
DROP TABLE client_crash_reports_old;
DROP TABLE user_favourite_channels_old;
DROP TABLE user_recent_channels_old;
DROP TABLE users_old;
CREATE INDEX idx_sessions_user ON sessions (user_id);
CREATE INDEX idx_sessions_expires ON sessions (expires_at);
CREATE INDEX idx_device_pairings_expires ON device_pairings (expires_at);
CREATE INDEX idx_device_pairings_user ON device_pairings (user_id);
CREATE INDEX idx_device_tokens_user ON device_tokens (user_id);
CREATE UNIQUE INDEX idx_password_recoveries_pending_user
    ON password_recoveries (user_id) WHERE status = 'pending';
CREATE INDEX idx_password_recoveries_terminal
    ON password_recoveries (terminal_at) WHERE terminal_at > 0;
CREATE INDEX idx_password_recoveries_user ON password_recoveries (user_id);
CREATE INDEX idx_password_recovery_grants_recovery
    ON password_recovery_grants (recovery_id, created_at);
CREATE INDEX idx_client_crash_reports_received ON client_crash_reports (received_at_ms, id);
CREATE INDEX idx_client_crash_reports_user ON client_crash_reports (user_id);
CREATE INDEX idx_user_favourite_channels_channel ON user_favourite_channels (channel_id);
CREATE INDEX idx_user_recent_channels_channel ON user_recent_channels (channel_id);
CREATE INDEX idx_user_recent_channels_user_tuned ON user_recent_channels (user_id, tuned_at_ms DESC);

-- images and its children -----------------------------------------------------------------------
ALTER TABLE image_derivatives RENAME TO image_derivatives_old;
ALTER TABLE image_refs RENAME TO image_refs_old;
ALTER TABLE images RENAME TO images_old;
CREATE TABLE images (
    -- sha256 of the ORIGINAL bytes, hex. Identity, filename, ETag and cache key at once, which is
    -- what makes `Cache-Control: immutable` an honest header rather than a hope.
    hash         TEXT PRIMARY KEY,

    -- upload | remote | extracted | generated. ⚠ This is the durability column: it decides what
    -- can be got back after a restore. Only `remote` and `extracted` are recoverable.
    origin       TEXT NOT NULL,

    -- Upstream URL for `remote`, empty otherwise. Load-bearing rather than informational: it is
    -- the difference between an image being cached and an image being recoverable.
    source_url   TEXT NOT NULL DEFAULT '',

    -- public | member. ⚠ Visibility is a property of the IMAGE, not of the route — a channel icon
    -- must be fetchable by Tunarr with no credentials while a clip still must not be, and one
    -- serve path with a per-row check is the only shape that serves both.
    visibility   TEXT NOT NULL DEFAULT 'member',

    -- poster | backdrop | icon | thumb. Decides the width ladder and the fallback aspect.
    role         TEXT NOT NULL DEFAULT 'icon',

    -- Describes the ORIGINAL, never a derivative. width/height are served to the frontend so an
    -- <img> can carry real dimensions and contribute zero layout shift.
    mime         TEXT NOT NULL,
    width        INTEGER NOT NULL DEFAULT 0,
    height       INTEGER NOT NULL DEFAULT 0,
    bytes        INTEGER NOT NULL DEFAULT 0,

    -- Motion in the original (the clip hover loop). Such images have one rendition and skip the
    -- ladder: resizing an animation per breakpoint costs far more than it saves.
    animated     BOOLEAN NOT NULL DEFAULT FALSE CHECK (animated IN (0, 1)),

    -- ThumbHash, base64 (~25 bytes raw). ThumbHash rather than BlurHash because it carries ALPHA;
    -- channel logos are routinely transparent PNGs, which BlurHash renders as black.
    placeholder  TEXT NOT NULL DEFAULT '',
    dominant_hex TEXT NOT NULL DEFAULT '',

    -- Free-form JSON: attribution, licence, upstream ids, generation parameters. Deliberately
    -- opaque at this layer so the service does not grow a schema per producer.
    meta         TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(meta)),

    -- ⚠ When the bytes were obtained upstream — a COMPLIANCE field, not a cache heuristic. TMDB's
    -- API terms forbid caching their content beyond six months, so remote images carry an expiry
    -- the GC enforces.
    origin_fetched_at INTEGER NOT NULL DEFAULT 0,

    created_at   INTEGER NOT NULL DEFAULT 0,
    updated_at   INTEGER NOT NULL DEFAULT 0,
    -- Drives derivative eviction under the cache budget.
    last_used_at INTEGER NOT NULL DEFAULT 0,
    frame_count  INTEGER NOT NULL DEFAULT 1,
    duration_ms  INTEGER NOT NULL DEFAULT 0,
    loop_count   INTEGER
);
CREATE TABLE image_derivatives (
    image_hash  TEXT    NOT NULL REFERENCES images(hash) ON DELETE CASCADE,
    recipe      TEXT    NOT NULL,
    format      TEXT    NOT NULL,
    width       INTEGER NOT NULL,
    bytes       INTEGER NOT NULL DEFAULT 0,
    output_hash TEXT    NOT NULL DEFAULT '',
    path        TEXT    NOT NULL,
    animated    BOOLEAN NOT NULL DEFAULT FALSE CHECK (animated IN (0, 1)),
    created_at  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (image_hash, recipe, format, width)
);
CREATE TABLE image_refs (
    image_hash TEXT NOT NULL REFERENCES images(hash) ON DELETE CASCADE,
    owner_kind TEXT NOT NULL,
    owner_id   TEXT NOT NULL,
    role       TEXT NOT NULL DEFAULT 'icon',
    PRIMARY KEY (image_hash, owner_kind, owner_id, role)
);
INSERT INTO images SELECT * FROM images_old;
INSERT INTO image_derivatives SELECT * FROM image_derivatives_old;
INSERT INTO image_refs SELECT * FROM image_refs_old;
DROP TABLE image_derivatives_old;
DROP TABLE image_refs_old;
DROP TABLE images_old;
CREATE INDEX idx_images_origin_fetched ON images (origin, origin_fetched_at);
CREATE INDEX idx_images_source_url ON images (source_url);
CREATE INDEX idx_image_derivatives_format ON image_derivatives (format, created_at);
CREATE INDEX idx_image_refs_owner ON image_refs (owner_kind, owner_id);

-- clips and its child; clip_tags ----------------------------------------------------------------
ALTER TABLE filler_research_web_attempts RENAME TO filler_research_web_attempts_old;
ALTER TABLE clips RENAME TO clips_old;
ALTER TABLE clip_tags RENAME TO clip_tags_old;
CREATE TABLE clips (
    -- The sparse content hash: 64 lowercase hex characters (see filler.ClipID).
    hash                TEXT    PRIMARY KEY,
    -- Location under the clip folder — `a3/f9/<hash>.mp4`. ⚠ Data, not identity: two folders can
    -- hold the same bytes, and the point of hashing is that they are ONE clip.
    path                TEXT    NOT NULL,
    tunarr_program_id   TEXT,
    name                TEXT    NOT NULL,
    role                TEXT    NOT NULL,
    era                 INTEGER NOT NULL DEFAULT 0,
    audience            TEXT    NOT NULL DEFAULT '',
    duration_ms         INTEGER NOT NULL,
    source              TEXT    NOT NULL DEFAULT '',
    quality             TEXT    NOT NULL DEFAULT '',
    license             TEXT    NOT NULL DEFAULT '',
    thumbnail           TEXT    NOT NULL DEFAULT '',
    play_count          INTEGER NOT NULL DEFAULT 0,
    last_played_at      INTEGER NOT NULL DEFAULT 0,
    removed_at          INTEGER NOT NULL DEFAULT 0,
    held                BOOLEAN NOT NULL DEFAULT FALSE CHECK (held IN (0, 1)),
    updated_at          INTEGER NOT NULL,
    preview             TEXT    NOT NULL DEFAULT '',
    language            TEXT    NOT NULL DEFAULT '',
    transcript          TEXT    NOT NULL DEFAULT '',
    brand               TEXT    NOT NULL DEFAULT '',
    visible_text        TEXT    NOT NULL DEFAULT '',
    is_composite        BOOLEAN NOT NULL DEFAULT FALSE CHECK (is_composite IN (0, 1)),
    parent_hash         TEXT    NOT NULL DEFAULT '',
    created_at          INTEGER NOT NULL DEFAULT 0,
    thumb_image_hash    TEXT    NOT NULL DEFAULT '',
    hover_image_hash    TEXT    NOT NULL DEFAULT '',
    reaped_at           INTEGER,
    geographic_scope    TEXT    NOT NULL DEFAULT 'unknown',
    country             TEXT    NOT NULL DEFAULT '',
    network             TEXT    NOT NULL DEFAULT '',
    station             TEXT    NOT NULL DEFAULT '',
    air_date            TEXT    NOT NULL DEFAULT '',
    geo_evidence        TEXT    NOT NULL DEFAULT '',
    placement           TEXT    NOT NULL DEFAULT 'not_playable',
    enrichment_revision INTEGER NOT NULL DEFAULT 1,
    name_source         TEXT    NOT NULL DEFAULT 'unknown'
);
CREATE TABLE filler_research_web_attempts (
    clip_hash       TEXT    NOT NULL REFERENCES clips(hash) ON DELETE CASCADE,
    input_revision  INTEGER NOT NULL CHECK (input_revision > 0),
    adapter_version TEXT    NOT NULL CHECK (adapter_version <> ''),
    provider        TEXT    NOT NULL CHECK (provider IN ('brave', 'searxng')),
    reserved_at     INTEGER NOT NULL CHECK (reserved_at > 0),
    PRIMARY KEY (clip_hash, input_revision, adapter_version)
);
CREATE TABLE clip_tags (
    clip_hash TEXT    NOT NULL,
    taxon     TEXT    NOT NULL,
    leaf      BOOLEAN NOT NULL DEFAULT FALSE CHECK (leaf IN (0, 1)),
    PRIMARY KEY (clip_hash, taxon)
);
INSERT INTO clips SELECT * FROM clips_old;
INSERT INTO filler_research_web_attempts SELECT * FROM filler_research_web_attempts_old;
INSERT INTO clip_tags SELECT * FROM clip_tags_old;
DROP TABLE filler_research_web_attempts_old;
DROP TABLE clips_old;
DROP TABLE clip_tags_old;
CREATE INDEX idx_clips_removed_at ON clips (removed_at);
CREATE INDEX idx_clips_held ON clips (held);
CREATE INDEX idx_clips_path ON clips (path);
CREATE INDEX idx_clips_geography ON clips (geographic_scope, country);
CREATE INDEX idx_clip_tags_taxon ON clip_tags (taxon);

-- filler_sources and its child; filler_clip_pipeline; holidays ----------------------------------
ALTER TABLE filler_source_check_outcomes RENAME TO filler_source_check_outcomes_old;
ALTER TABLE filler_sources RENAME TO filler_sources_old;
ALTER TABLE filler_clip_pipeline RENAME TO filler_clip_pipeline_old;
ALTER TABLE holidays RENAME TO holidays_old;
CREATE TABLE filler_sources (
    -- A stable id, not the uri: re-pointing a source keeps its history.
    id                     TEXT    PRIMARY KEY,
    -- The providers Loomarr is built with. Remote sources nest under the Sources tab's derived
    -- remote row, so this doesn't reuse its folder/library/remote vocabulary.
    kind                   TEXT    NOT NULL CHECK (kind IN ('archive', 'youtube', 'folder', 'library')),
    uri                    TEXT    NOT NULL,
    label                  TEXT    NOT NULL DEFAULT '',
    -- ⚠ Empty means unknown, never "public domain": most archive.org items declare no licence.
    license                TEXT    NOT NULL DEFAULT '',
    last_fetched_at        INTEGER NOT NULL DEFAULT 0,
    created_at             INTEGER NOT NULL,
    enabled                BOOLEAN NOT NULL DEFAULT TRUE CHECK (enabled IN (0, 1)),
    fetch_every_seconds    INTEGER,
    fetch_max_per_run      INTEGER,
    country                TEXT    NOT NULL DEFAULT '',
    last_checked_at        INTEGER NOT NULL DEFAULT 0,
    check_failure_count    INTEGER NOT NULL DEFAULT 0,
    check_retry_at         INTEGER NOT NULL DEFAULT 0,
    check_lease_until      INTEGER NOT NULL DEFAULT 0,
    scan_cursor            TEXT    NOT NULL DEFAULT '',
    scan_watermark         TEXT    NOT NULL DEFAULT '',
    scan_pending_watermark TEXT    NOT NULL DEFAULT ''
);
CREATE TABLE filler_source_check_outcomes (
    source_id   TEXT    NOT NULL REFERENCES filler_sources(id) ON DELETE CASCADE,
    disposition TEXT    NOT NULL CHECK (disposition IN (
        'queued', 'already_known', 'too_short', 'too_long', 'live', 'upcoming',
        'private', 'unavailable', 'metadata_incomplete'
    )),
    item_count  INTEGER NOT NULL CHECK (item_count >= 0),
    PRIMARY KEY (source_id, disposition)
);
CREATE TABLE filler_clip_pipeline (
    clip_hash                TEXT    PRIMARY KEY,
    stage                    TEXT    NOT NULL DEFAULT 'probe',
    -- ⚠ failed retries with backoff, skipped is re-evaluated every pass but never re-run, done is
    -- never redone: three meanings the vocabulary exists to keep apart.
    status                   TEXT    NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'running', 'done', 'failed', 'skipped')),
    progress                 INTEGER NOT NULL DEFAULT 0,
    disposition              TEXT    NOT NULL DEFAULT 'running',
    reject_reason            TEXT    NOT NULL DEFAULT '',
    reject_detail            TEXT    NOT NULL DEFAULT '',
    attempts                 INTEGER NOT NULL DEFAULT 0,
    next_run                 INTEGER NOT NULL DEFAULT 0,
    stages_json              TEXT    NOT NULL DEFAULT '[]',
    enrolled_at              INTEGER NOT NULL,
    updated_at               INTEGER NOT NULL,
    force_run                BOOLEAN NOT NULL DEFAULT FALSE CHECK (force_run IN (0, 1)),
    acquisition_id           TEXT    NOT NULL DEFAULT '',
    preparation_attempt      INTEGER NOT NULL DEFAULT 0,
    preparation_started_at   INTEGER NOT NULL DEFAULT 0,
    preparation_start_reason TEXT    NOT NULL DEFAULT '',
    preparation_progress     INTEGER NOT NULL DEFAULT -1,
    stage_queued_at          INTEGER NOT NULL DEFAULT 0,
    stage_started_at         INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE holidays (
    id                       TEXT    PRIMARY KEY REFERENCES taxa(slug),
    label                    TEXT    NOT NULL,
    position                 INTEGER NOT NULL,
    words                    TEXT    NOT NULL DEFAULT '{}',
    kept                     TEXT    NOT NULL DEFAULT '[]',
    air_window               TEXT    NOT NULL DEFAULT '',
    members                  TEXT    NOT NULL DEFAULT '[]',
    name_needs_corroboration BOOLEAN NOT NULL DEFAULT FALSE CHECK (name_needs_corroboration IN (0, 1)),
    updated_at               INTEGER NOT NULL DEFAULT 0
);
INSERT INTO filler_sources SELECT * FROM filler_sources_old;
INSERT INTO filler_source_check_outcomes SELECT * FROM filler_source_check_outcomes_old;
INSERT INTO filler_clip_pipeline SELECT * FROM filler_clip_pipeline_old;
INSERT INTO holidays SELECT * FROM holidays_old;
DROP TABLE filler_source_check_outcomes_old;
DROP TABLE filler_sources_old;
DROP TABLE filler_clip_pipeline_old;
DROP TABLE holidays_old;
CREATE INDEX idx_filler_sources_kind ON filler_sources (kind, created_at);
CREATE UNIQUE INDEX idx_filler_sources_uri ON filler_sources (kind, uri) WHERE uri <> '';
CREATE INDEX idx_filler_sources_geography ON filler_sources (country);
CREATE INDEX idx_filler_clip_pipeline_work ON filler_clip_pipeline (disposition, next_run);
CREATE INDEX idx_filler_clip_pipeline_acquisition ON filler_clip_pipeline (acquisition_id);

-- the scanner's leaf tables ---------------------------------------------------------------------
ALTER TABLE library_answers RENAME TO library_answers_old;
ALTER TABLE library_candidates RENAME TO library_candidates_old;
ALTER TABLE library_files RENAME TO library_files_old;
ALTER TABLE library_root_paths RENAME TO library_root_paths_old;
ALTER TABLE library_roots RENAME TO library_roots_old;
CREATE TABLE library_answers (
    origin_authority TEXT    NOT NULL,
    origin_key       TEXT    NOT NULL,
    kind             TEXT    NOT NULL CHECK (kind IN ('movie', 'series')),
    tmdb_id          TEXT    NOT NULL DEFAULT '',
    ignored          BOOLEAN NOT NULL DEFAULT FALSE CHECK (ignored IN (0, 1)),
    answered_at_ms   INTEGER NOT NULL,
    answered_by      TEXT    NOT NULL DEFAULT 'you',
    PRIMARY KEY (origin_authority, origin_key)
);
CREATE TABLE library_candidates (
    inventory_item_id TEXT    NOT NULL,
    rank              INTEGER NOT NULL,
    kind              TEXT    NOT NULL CHECK (kind IN ('movie', 'series')),
    tmdb_id           TEXT    NOT NULL,
    name              TEXT    NOT NULL,
    year              INTEGER NOT NULL DEFAULT 0,
    overview          TEXT    NOT NULL DEFAULT '',
    poster            TEXT    NOT NULL DEFAULT '',
    name_match        BOOLEAN NOT NULL DEFAULT FALSE CHECK (name_match IN (0, 1)),
    year_match        BOOLEAN NOT NULL DEFAULT FALSE CHECK (year_match IN (0, 1)),
    length_match      BOOLEAN NOT NULL DEFAULT FALSE CHECK (length_match IN (0, 1)),
    PRIMARY KEY (inventory_item_id, rank)
);
CREATE TABLE library_files (
    path                TEXT    PRIMARY KEY,
    root                TEXT    NOT NULL,
    size_bytes          INTEGER NOT NULL,
    modified_ns         INTEGER NOT NULL,
    identity            TEXT    NOT NULL DEFAULT '',
    kind                TEXT    NOT NULL DEFAULT '' CHECK (kind IN ('', 'movie', 'episode', 'series')),
    header              TEXT    NOT NULL DEFAULT 'pending',
    problem             TEXT    NOT NULL DEFAULT '',
    held                BOOLEAN NOT NULL DEFAULT FALSE CHECK (held IN (0, 1)),
    seen_at_ms          INTEGER NOT NULL,
    missing_at_ms       INTEGER NOT NULL DEFAULT 0,
    inventory_item_id   TEXT    NOT NULL DEFAULT '',
    inventory_series_id TEXT    NOT NULL DEFAULT ''
);
CREATE TABLE library_root_paths (
    authority      TEXT    NOT NULL,
    authority_path TEXT    NOT NULL,
    root_path      TEXT    NOT NULL DEFAULT '',
    by_hand        BOOLEAN NOT NULL DEFAULT FALSE CHECK (by_hand IN (0, 1)),
    seen_at_ms     INTEGER NOT NULL,
    PRIMARY KEY (authority, authority_path)
);
CREATE TABLE library_roots (
    path          TEXT    PRIMARY KEY,
    in_use        BOOLEAN NOT NULL DEFAULT FALSE CHECK (in_use IN (0, 1)),
    readable      BOOLEAN NOT NULL DEFAULT FALSE CHECK (readable IN (0, 1)),
    checked_at_ms INTEGER NOT NULL DEFAULT 0,
    by_hand       BOOLEAN NOT NULL DEFAULT FALSE CHECK (by_hand IN (0, 1)),
    added_at_ms   INTEGER NOT NULL,
    scanned_at_ms INTEGER NOT NULL DEFAULT 0
);
INSERT INTO library_answers SELECT * FROM library_answers_old;
INSERT INTO library_candidates SELECT * FROM library_candidates_old;
INSERT INTO library_files SELECT * FROM library_files_old;
INSERT INTO library_root_paths SELECT * FROM library_root_paths_old;
INSERT INTO library_roots SELECT * FROM library_roots_old;
DROP TABLE library_answers_old;
DROP TABLE library_candidates_old;
DROP TABLE library_files_old;
DROP TABLE library_root_paths_old;
DROP TABLE library_roots_old;
CREATE INDEX idx_library_files_root ON library_files (root);
CREATE INDEX idx_library_files_inventory_item ON library_files (inventory_item_id);
CREATE INDEX idx_library_files_inventory_series ON library_files (inventory_series_id);

-- library_items and its descendants -------------------------------------------------------------
ALTER TABLE library_streams RENAME TO library_streams_old;
ALTER TABLE library_trickplay RENAME TO library_trickplay_old;
ALTER TABLE library_chapters RENAME TO library_chapters_old;
ALTER TABLE library_markers RENAME TO library_markers_old;
ALTER TABLE library_ids RENAME TO library_ids_old;
ALTER TABLE library_genres RENAME TO library_genres_old;
ALTER TABLE library_studios RENAME TO library_studios_old;
ALTER TABLE library_tags RENAME TO library_tags_old;
ALTER TABLE library_sources RENAME TO library_sources_old;
ALTER TABLE library_art RENAME TO library_art_old;
ALTER TABLE library_countries RENAME TO library_countries_old;
ALTER TABLE library_credits RENAME TO library_credits_old;
ALTER TABLE library_items RENAME TO library_items_old;
CREATE TABLE library_items (
    id               TEXT    PRIMARY KEY,
    kind             TEXT    NOT NULL CHECK (kind IN ('movie', 'series', 'episode')),
    name             TEXT    NOT NULL,
    sort_name        TEXT    NOT NULL,
    year             INTEGER NOT NULL DEFAULT 0,
    updated_at_ms    INTEGER NOT NULL,
    catalog          TEXT    NOT NULL DEFAULT '',
    origin_authority TEXT    NOT NULL DEFAULT '',
    origin_key       TEXT    NOT NULL DEFAULT '',
    grounded         BOOLEAN NOT NULL DEFAULT TRUE CHECK (grounded IN (0, 1)),
    searched         TEXT    NOT NULL DEFAULT '',
    match_basis      TEXT    NOT NULL DEFAULT '',
    match_app        TEXT    NOT NULL DEFAULT '',
    match_page       TEXT    NOT NULL DEFAULT '',
    needs_noticed    BOOLEAN NOT NULL DEFAULT FALSE CHECK (needs_noticed IN (0, 1)),
    set_key          TEXT    NOT NULL DEFAULT '',
    original_title   TEXT    NOT NULL DEFAULT '',
    overview         TEXT    NOT NULL DEFAULT '',
    premiere_date    TEXT    NOT NULL DEFAULT '',
    rating           TEXT    NOT NULL DEFAULT '',
    score            REAL    NOT NULL DEFAULT 0,
    runtime_ms       INTEGER NOT NULL DEFAULT 0,
    season           INTEGER NOT NULL DEFAULT 0,
    episode          INTEGER NOT NULL DEFAULT 0,
    episode_end      INTEGER NOT NULL DEFAULT 0,
    series_authority TEXT    NOT NULL DEFAULT '',
    series_key       TEXT    NOT NULL DEFAULT '',
    series_id        TEXT    NOT NULL DEFAULT '',
    name_from        TEXT    NOT NULL DEFAULT '',
    year_from        TEXT    NOT NULL DEFAULT '',
    overview_from    TEXT    NOT NULL DEFAULT '',
    premiere_from    TEXT    NOT NULL DEFAULT '',
    rating_from      TEXT    NOT NULL DEFAULT '',
    score_from       TEXT    NOT NULL DEFAULT '',
    runtime_from     TEXT    NOT NULL DEFAULT '',
    tagline          TEXT    NOT NULL DEFAULT '',
    trailer          TEXT    NOT NULL DEFAULT ''
);
CREATE TABLE library_ids (
    item_id   TEXT NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
    namespace TEXT NOT NULL,
    value     TEXT NOT NULL,
    PRIMARY KEY (item_id, namespace, value)
);
CREATE TABLE library_genres (
    item_id  TEXT    NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
    name     TEXT    NOT NULL,
    position INTEGER NOT NULL,
    PRIMARY KEY (item_id, name)
);
CREATE TABLE library_studios (
    item_id  TEXT    NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
    name     TEXT    NOT NULL,
    position INTEGER NOT NULL,
    PRIMARY KEY (item_id, name)
);
CREATE TABLE library_tags (
    item_id  TEXT    NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
    name     TEXT    NOT NULL,
    position INTEGER NOT NULL,
    PRIMARY KEY (item_id, name)
);
CREATE TABLE library_sources (
    item_id     TEXT    NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
    path        TEXT    NOT NULL,
    container   TEXT    NOT NULL DEFAULT '',
    size_bytes  INTEGER NOT NULL DEFAULT 0,
    bitrate     INTEGER NOT NULL DEFAULT 0,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    edition     TEXT    NOT NULL DEFAULT '',
    plays       BOOLEAN NOT NULL DEFAULT FALSE CHECK (plays IN (0, 1)),
    PRIMARY KEY (item_id, path)
);
CREATE TABLE library_art (
    item_id  TEXT    NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
    kind     TEXT    NOT NULL CHECK (kind IN ('poster', 'fanart', 'clearlogo', 'banner', 'landscape',
        'clearart', 'discart', 'thumb', 'season-poster')),
    season   INTEGER NOT NULL DEFAULT 0,
    position INTEGER NOT NULL,
    ref      TEXT    NOT NULL,
    PRIMARY KEY (item_id, kind, season, position)
);
CREATE TABLE library_countries (
    item_id  TEXT    NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
    name     TEXT    NOT NULL,
    position INTEGER NOT NULL,
    PRIMARY KEY (item_id, name)
);
CREATE TABLE library_credits (
    item_id        TEXT    NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
    position       INTEGER NOT NULL,
    person_id      TEXT    NOT NULL REFERENCES library_people(id),
    role           TEXT    NOT NULL,
    character_name TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (item_id, position)
);
CREATE TABLE library_streams (
    item_id          TEXT    NOT NULL,
    path             TEXT    NOT NULL,
    stream_index     INTEGER NOT NULL,
    kind             TEXT    NOT NULL CHECK (kind IN ('video', 'audio', 'subtitle')),
    codec            TEXT    NOT NULL DEFAULT '',
    profile          TEXT    NOT NULL DEFAULT '',
    title            TEXT    NOT NULL DEFAULT '',
    is_default       BOOLEAN NOT NULL DEFAULT FALSE CHECK (is_default IN (0, 1)),
    forced           BOOLEAN NOT NULL DEFAULT FALSE CHECK (forced IN (0, 1)),
    language         TEXT    NOT NULL DEFAULT '',
    language_name    TEXT    NOT NULL DEFAULT '',
    channels         INTEGER NOT NULL DEFAULT 0,
    channel_layout   TEXT    NOT NULL DEFAULT '',
    sample_rate      INTEGER NOT NULL DEFAULT 0,
    audio_format     TEXT    NOT NULL DEFAULT '',
    width            INTEGER NOT NULL DEFAULT 0,
    height           INTEGER NOT NULL DEFAULT 0,
    frame_rate       TEXT    NOT NULL DEFAULT '',
    pixel_format     TEXT    NOT NULL DEFAULT '',
    color_transfer   TEXT    NOT NULL DEFAULT '',
    resolution       TEXT    NOT NULL DEFAULT '',
    hdr              TEXT    NOT NULL DEFAULT '',
    commentary       BOOLEAN NOT NULL DEFAULT FALSE CHECK (commentary IN (0, 1)),
    hearing_impaired BOOLEAN NOT NULL DEFAULT FALSE CHECK (hearing_impaired IN (0, 1)),
    descriptive      BOOLEAN NOT NULL DEFAULT FALSE CHECK (descriptive IN (0, 1)),
    external_ref     TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (item_id, path, stream_index),
    FOREIGN KEY (item_id, path) REFERENCES library_sources(item_id, path) ON DELETE CASCADE
);
CREATE TABLE library_trickplay (
    item_id TEXT NOT NULL,
    path    TEXT NOT NULL,
    ref     TEXT NOT NULL,
    format  TEXT NOT NULL,
    PRIMARY KEY (item_id, path, ref),
    FOREIGN KEY (item_id, path) REFERENCES library_sources(item_id, path) ON DELETE CASCADE
);
CREATE TABLE library_chapters (
    item_id  TEXT    NOT NULL,
    path     TEXT    NOT NULL,
    position INTEGER NOT NULL,
    start_ms INTEGER NOT NULL,
    end_ms   INTEGER NOT NULL,
    title    TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (item_id, path, position),
    FOREIGN KEY (item_id, path) REFERENCES library_sources(item_id, path) ON DELETE CASCADE
);
CREATE TABLE library_markers (
    item_id  TEXT    NOT NULL,
    path     TEXT    NOT NULL,
    position INTEGER NOT NULL,
    start_ms INTEGER NOT NULL,
    end_ms   INTEGER NOT NULL,
    kind     TEXT    NOT NULL CHECK (kind IN ('cut', 'mute', 'scene', 'commercial')),
    PRIMARY KEY (item_id, path, position),
    FOREIGN KEY (item_id, path) REFERENCES library_sources(item_id, path) ON DELETE CASCADE
);
INSERT INTO library_items SELECT * FROM library_items_old;
INSERT INTO library_ids SELECT * FROM library_ids_old;
INSERT INTO library_genres SELECT * FROM library_genres_old;
INSERT INTO library_studios SELECT * FROM library_studios_old;
INSERT INTO library_tags SELECT * FROM library_tags_old;
INSERT INTO library_sources SELECT * FROM library_sources_old;
INSERT INTO library_art SELECT * FROM library_art_old;
INSERT INTO library_countries SELECT * FROM library_countries_old;
INSERT INTO library_credits SELECT * FROM library_credits_old;
INSERT INTO library_streams SELECT * FROM library_streams_old;
INSERT INTO library_trickplay SELECT * FROM library_trickplay_old;
INSERT INTO library_chapters SELECT * FROM library_chapters_old;
INSERT INTO library_markers SELECT * FROM library_markers_old;
DROP TABLE library_streams_old;
DROP TABLE library_trickplay_old;
DROP TABLE library_chapters_old;
DROP TABLE library_markers_old;
DROP TABLE library_ids_old;
DROP TABLE library_genres_old;
DROP TABLE library_studios_old;
DROP TABLE library_tags_old;
DROP TABLE library_sources_old;
DROP TABLE library_art_old;
DROP TABLE library_countries_old;
DROP TABLE library_credits_old;
DROP TABLE library_items_old;
CREATE INDEX idx_library_items_sort ON library_items (kind, sort_name);
CREATE INDEX idx_library_items_catalog ON library_items (catalog);
CREATE INDEX idx_library_items_origin ON library_items (origin_authority, origin_key);
CREATE INDEX idx_library_items_set ON library_items (set_key);
CREATE INDEX idx_library_items_series_origin ON library_items (series_authority, series_key);
CREATE INDEX idx_library_items_series ON library_items (series_id);
CREATE INDEX idx_library_ids_value ON library_ids (namespace, value);
CREATE INDEX idx_library_genres_name ON library_genres (name);
CREATE INDEX idx_library_studios_name ON library_studios (name);
CREATE INDEX idx_library_tags_name ON library_tags (name);
CREATE INDEX idx_library_sources_path ON library_sources (path);
CREATE INDEX idx_library_countries_name ON library_countries (name);
CREATE INDEX idx_library_credits_person ON library_credits (person_id, role);
CREATE INDEX idx_library_streams_language ON library_streams (kind, language);

-- proposal_jobs and its child; quality_run_snapshots and its child; scheduled_jobs; settings -----
ALTER TABLE proposal_job_attempts RENAME TO proposal_job_attempts_old;
ALTER TABLE proposal_jobs RENAME TO proposal_jobs_old;
ALTER TABLE quality_receipts RENAME TO quality_receipts_old;
ALTER TABLE quality_run_snapshots RENAME TO quality_run_snapshots_old;
ALTER TABLE scheduled_jobs RENAME TO scheduled_jobs_old;
ALTER TABLE settings RENAME TO settings_old;
CREATE TABLE proposal_jobs (
    id                 TEXT    PRIMARY KEY,
    kind               TEXT    NOT NULL DEFAULT 'suggest' CHECK (kind IN ('suggest', 'recurate')),
    status             TEXT    NOT NULL CHECK (status IN ('queued', 'running', 'done', 'failed')),
    intent_json        TEXT    NOT NULL DEFAULT '{}',
    intent_hash        TEXT    NOT NULL DEFAULT '',
    created_by         TEXT    NOT NULL DEFAULT '',
    last_error         TEXT    NOT NULL DEFAULT '',
    -- deadline drives ClaimDueJobs: a queued job is due at or before now; claiming leases it so
    -- two workers or replicas don't run it twice.
    deadline           BIGINT  NOT NULL DEFAULT 0,
    attempts           INTEGER NOT NULL DEFAULT 0,
    created_at         BIGINT  NOT NULL DEFAULT 0,
    updated_at         BIGINT  NOT NULL DEFAULT 0,
    failure_code       TEXT    NOT NULL DEFAULT '',
    workflow_version   INTEGER NOT NULL DEFAULT 0,
    reached_live       BOOLEAN NOT NULL DEFAULT FALSE CHECK (reached_live IN (0, 1)),
    failure_trace_json TEXT    NOT NULL DEFAULT '',
    channel_id         TEXT    NOT NULL DEFAULT ''
);
CREATE TABLE proposal_job_attempts (
    job_id           TEXT    NOT NULL REFERENCES proposal_jobs(id) ON DELETE CASCADE,
    attempt          INTEGER NOT NULL,
    workflow_version INTEGER NOT NULL,
    status           TEXT    NOT NULL CHECK (status IN ('running', 'succeeded', 'failed', 'interrupted')),
    started_at       BIGINT  NOT NULL,
    completed_at     BIGINT  NOT NULL DEFAULT 0,
    failure_code     TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (job_id, attempt)
);
CREATE TABLE quality_run_snapshots (
    id                   TEXT    PRIMARY KEY,
    schema_version       INTEGER NOT NULL,
    corpus_version       TEXT    NOT NULL,
    requested_model      TEXT    NOT NULL,
    resolved_model       TEXT    NOT NULL,
    provider             TEXT    NOT NULL,
    budget_profile       TEXT    NOT NULL,
    application_version  TEXT    NOT NULL,
    accounting_available BOOLEAN NOT NULL CHECK (accounting_available IN (0, 1)),
    created_at           BIGINT  NOT NULL
);
CREATE TABLE quality_receipts (
    idempotency_key TEXT    PRIMARY KEY,
    occurred_at     BIGINT  NOT NULL,
    stage           TEXT    NOT NULL,
    outcome         TEXT    NOT NULL CHECK (outcome IN ('succeeded', 'empty', 'failed', 'abstained',
        'accepted', 'rejected', 'approved', 'declined', 'playable', 'scheduled')),
    duration_ms     BIGINT  NOT NULL,
    tool_calls      INTEGER NOT NULL,
    candidate_count INTEGER NOT NULL,
    cost_nanos      BIGINT  NOT NULL,
    run_snapshot_id TEXT    REFERENCES quality_run_snapshots(id)
);
CREATE TABLE scheduled_jobs (
    name        TEXT    PRIMARY KEY,
    last_run    BIGINT  NOT NULL DEFAULT 0,
    last_result TEXT    NOT NULL DEFAULT '',
    last_error  TEXT    NOT NULL DEFAULT '',
    next_run    BIGINT  NOT NULL DEFAULT 0,
    updated_at  BIGINT  NOT NULL DEFAULT 0,
    paused      BOOLEAN NOT NULL DEFAULT FALSE CHECK (paused IN (0, 1))
);
CREATE TABLE settings (
    key          TEXT    PRIMARY KEY,
    value        TEXT    NOT NULL,
    updated_at   BIGINT  NOT NULL DEFAULT 0,
    updated_by   TEXT,
    env_override BOOLEAN NOT NULL DEFAULT FALSE CHECK (env_override IN (0, 1))
);
INSERT INTO proposal_jobs SELECT * FROM proposal_jobs_old;
INSERT INTO proposal_job_attempts SELECT * FROM proposal_job_attempts_old;
INSERT INTO quality_run_snapshots SELECT * FROM quality_run_snapshots_old;
INSERT INTO quality_receipts SELECT * FROM quality_receipts_old;
INSERT INTO scheduled_jobs SELECT * FROM scheduled_jobs_old;
INSERT INTO settings SELECT * FROM settings_old;
DROP TABLE proposal_job_attempts_old;
DROP TABLE proposal_jobs_old;
DROP TABLE quality_receipts_old;
DROP TABLE quality_run_snapshots_old;
DROP TABLE scheduled_jobs_old;
DROP TABLE settings_old;
CREATE INDEX idx_proposal_jobs_status_deadline ON proposal_jobs (status, deadline);
CREATE INDEX idx_proposal_jobs_intent_hash ON proposal_jobs (intent_hash);
CREATE INDEX idx_proposal_job_attempts_job_started ON proposal_job_attempts (job_id, started_at DESC);
CREATE INDEX idx_quality_receipts_rollup ON quality_receipts (occurred_at, stage, outcome);
CREATE INDEX idx_quality_receipts_run_snapshot ON quality_receipts (run_snapshot_id);
CREATE INDEX idx_scheduled_jobs_next_run ON scheduled_jobs (next_run);

-- flags already CHECKed, but typed INTEGER ------------------------------------------------------
ALTER TABLE filler_acquisition_artifacts RENAME TO filler_acquisition_artifacts_old;
ALTER TABLE filler_providers RENAME TO filler_providers_old;
ALTER TABLE notification_destinations RENAME TO notification_destinations_old;
ALTER TABLE secret_data_keys RENAME TO secret_data_keys_old;
CREATE TABLE filler_acquisition_artifacts (
    id                         TEXT    PRIMARY KEY,
    acquisition_id             TEXT    NOT NULL REFERENCES filler_acquisition_runs(id),
    source_id                  TEXT    NOT NULL DEFAULT '',
    provider                   TEXT    NOT NULL,
    source_url                 TEXT    NOT NULL,
    staging_path               TEXT    NOT NULL DEFAULT '',
    media_path                 TEXT    NOT NULL,
    sidecar_path               TEXT    NOT NULL DEFAULT '',
    media_sha256               TEXT    NOT NULL,
    media_bytes                INTEGER NOT NULL,
    clip_hash                  TEXT    NOT NULL DEFAULT '',
    state                      TEXT    NOT NULL CHECK (state IN ('staged', 'published', 'consumed', 'repair')),
    repair_reason              TEXT    NOT NULL DEFAULT '',
    completed_at               INTEGER NOT NULL,
    updated_at                 INTEGER NOT NULL,
    provider_archive_entry     TEXT    NOT NULL DEFAULT '',
    provider_archive_committed BOOLEAN NOT NULL DEFAULT FALSE CHECK (provider_archive_committed IN (0, 1)),
    remote_id                  TEXT    NOT NULL DEFAULT '',
    gap                        TEXT    NOT NULL DEFAULT ''
);
CREATE TABLE filler_providers (
    kind    TEXT    PRIMARY KEY CHECK (kind IN ('archive', 'youtube')),
    enabled BOOLEAN NOT NULL DEFAULT TRUE CHECK (enabled IN (0, 1))
);
CREATE TABLE notification_destinations (
    id                       TEXT    PRIMARY KEY,
    means                    TEXT    NOT NULL CHECK (means IN (
        'email', 'webhook', 'discord', 'ntfy', 'gotify', 'apprise', 'pushover',
        'telegram', 'mattermost', 'matrix', 'web_push', 'mqtt', 'slack'
    )),
    label                    TEXT    NOT NULL,
    scope                    TEXT    NOT NULL CHECK (scope IN ('installation', 'person')),
    owner_id                 TEXT    NOT NULL DEFAULT '',
    audience                 TEXT    NOT NULL CHECK (audience IN ('person', 'approvers', 'operators')),
    topics_json              TEXT    NOT NULL,
    enabled                  BOOLEAN NOT NULL CHECK (enabled IN (0, 1)),
    configuration_json       TEXT    NOT NULL DEFAULT '{}',
    credentials_encrypted    TEXT    NOT NULL,
    created_at               BIGINT  NOT NULL,
    updated_at               BIGINT  NOT NULL,
    credential_keys_json     TEXT,
    subscription_fingerprint TEXT    NOT NULL DEFAULT '',
    CHECK (
        (scope = 'installation' AND owner_id = '' AND audience IN ('approvers', 'operators')) OR
        (scope = 'person' AND owner_id <> '' AND audience IN ('person', 'approvers', 'operators'))
    )
);
CREATE TABLE secret_data_keys (
    id          TEXT    PRIMARY KEY,
    wrapped_key TEXT    NOT NULL,
    active      BOOLEAN NOT NULL CHECK (active IN (0, 1)),
    created_at  BIGINT  NOT NULL
);
INSERT INTO filler_acquisition_artifacts SELECT * FROM filler_acquisition_artifacts_old;
INSERT INTO filler_providers SELECT * FROM filler_providers_old;
INSERT INTO notification_destinations SELECT * FROM notification_destinations_old;
INSERT INTO secret_data_keys SELECT * FROM secret_data_keys_old;
DROP TABLE filler_acquisition_artifacts_old;
DROP TABLE filler_providers_old;
DROP TABLE notification_destinations_old;
DROP TABLE secret_data_keys_old;
CREATE INDEX idx_filler_acquisition_artifacts_path
    ON filler_acquisition_artifacts (media_path, updated_at DESC);
CREATE INDEX idx_filler_acquisition_artifacts_clip
    ON filler_acquisition_artifacts (clip_hash) WHERE clip_hash <> '';
CREATE INDEX idx_filler_acquisition_artifacts_recovery
    ON filler_acquisition_artifacts (state, updated_at);
CREATE INDEX idx_filler_acquisition_artifacts_remote
    ON filler_acquisition_artifacts (provider, source_id, remote_id) WHERE remote_id <> '';
CREATE INDEX idx_filler_acquisition_artifacts_acquisition
    ON filler_acquisition_artifacts (acquisition_id);
CREATE INDEX idx_notification_destinations_routing
    ON notification_destinations (enabled, scope, audience, means, id);
CREATE UNIQUE INDEX idx_notification_destinations_web_push_subscription
    ON notification_destinations (owner_id, subscription_fingerprint)
    WHERE means = 'web_push' AND subscription_fingerprint <> '';
CREATE UNIQUE INDEX idx_secret_data_keys_one_active
    ON secret_data_keys (active) WHERE active = 1;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
