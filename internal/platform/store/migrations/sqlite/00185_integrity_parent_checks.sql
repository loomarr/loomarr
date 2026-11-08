-- +goose Up
-- Schema to a 9 (#2528), integrity step 2: the state columns of tables other tables reference accept
-- only the values the code writes, and channels loses filler_ref, which nothing ever set.
-- SQLite cannot add a CHECK to an existing column, so each table is rebuilt transactionally. Renaming
-- a parent repoints its children's foreign keys at the renamed copy, and dropping that copy would
-- cascade-delete them, so every descendant is rebuilt with its parent (the 00166 pattern): children
-- are renamed first, created after the parent, and dropped first.

-- channels and its children ---------------------------------------------------------------------
DROP INDEX idx_channels_number;
DROP INDEX idx_channels_status_deadline;
DROP INDEX idx_channels_intent_ref;
DROP INDEX idx_user_favourite_channels_channel;
DROP INDEX idx_user_recent_channels_channel;
DROP INDEX idx_user_recent_channels_user_tuned;
DROP INDEX idx_airings_channel;
ALTER TABLE user_favourite_channels RENAME TO user_favourite_channels_old;
ALTER TABLE user_recent_channels RENAME TO user_recent_channels_old;
ALTER TABLE airings RENAME TO airings_old;
ALTER TABLE channels RENAME TO channels_old;
CREATE TABLE channels (
    id                 TEXT   PRIMARY KEY,
    intent_ref         TEXT   NOT NULL DEFAULT '',
    name               TEXT   NOT NULL,
    number             INTEGER NOT NULL,
    grp                TEXT   NOT NULL DEFAULT '',   -- the channel's group ("group" is reserved)
    logo               TEXT   NOT NULL DEFAULT '',
    strategy           TEXT   NOT NULL,
    tunarr_id          TEXT   NOT NULL DEFAULT '',
    status             TEXT   NOT NULL
        CHECK (status IN ('building', 'live', 'drifted', 'empty', 'detached', 'paused')),
    shuffle_seed       BIGINT NOT NULL DEFAULT 0,
    lineup_json        TEXT   NOT NULL DEFAULT '[]',
    desired_json       TEXT   NOT NULL DEFAULT '[]',
    -- reconcile_deadline drives ClaimDueChannels: a channel is due when its deadline is at or
    -- before now; claiming leases it so two replicas don't reconcile one channel at once.
    reconcile_deadline BIGINT NOT NULL DEFAULT 0,
    updated_at         BIGINT NOT NULL DEFAULT 0,
    policy_json        TEXT   NOT NULL DEFAULT '{}',
    broadcast_codec    TEXT   NOT NULL DEFAULT 'h264',
    revision           BIGINT NOT NULL DEFAULT 1 CHECK (revision >= 1),
    playout_anchor     BIGINT NOT NULL DEFAULT 0 CHECK (playout_anchor >= 0),
    created_at         BIGINT NOT NULL DEFAULT 0,
    intent_json        TEXT   NOT NULL DEFAULT '',
    requested_by       TEXT   NOT NULL DEFAULT ''
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
CREATE TABLE airings (
    channel_id      TEXT   NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    library_item_id TEXT   NOT NULL,
    aired_at        BIGINT NOT NULL,
    recorded_at     BIGINT NOT NULL,
    key             TEXT   NOT NULL DEFAULT '',
    PRIMARY KEY (channel_id, library_item_id, aired_at)
);
INSERT INTO channels (id, intent_ref, name, number, grp, logo, strategy, tunarr_id, status, shuffle_seed,
    lineup_json, desired_json, reconcile_deadline, updated_at, policy_json, broadcast_codec, revision,
    playout_anchor, created_at, intent_json, requested_by)
SELECT id, intent_ref, name, number, grp, logo, strategy, tunarr_id, status, shuffle_seed,
    lineup_json, desired_json, reconcile_deadline, updated_at, policy_json, broadcast_codec, revision,
    playout_anchor, created_at, intent_json, requested_by
  FROM channels_old;
INSERT INTO user_favourite_channels (user_id, channel_id, added_at_ms)
SELECT user_id, channel_id, added_at_ms FROM user_favourite_channels_old;
INSERT INTO user_recent_channels (user_id, channel_id, tuned_at_ms)
SELECT user_id, channel_id, tuned_at_ms FROM user_recent_channels_old;
INSERT INTO airings (channel_id, library_item_id, aired_at, recorded_at, key)
SELECT channel_id, library_item_id, aired_at, recorded_at, key FROM airings_old;
DROP TABLE user_favourite_channels_old;
DROP TABLE user_recent_channels_old;
DROP TABLE airings_old;
DROP TABLE channels_old;
CREATE UNIQUE INDEX idx_channels_number ON channels (number);
CREATE INDEX idx_channels_status_deadline ON channels (status, reconcile_deadline);
CREATE UNIQUE INDEX idx_channels_intent_ref ON channels (intent_ref) WHERE intent_ref <> '';
CREATE INDEX idx_user_favourite_channels_channel ON user_favourite_channels (channel_id);
CREATE INDEX idx_user_recent_channels_channel ON user_recent_channels (channel_id);
CREATE INDEX idx_user_recent_channels_user_tuned ON user_recent_channels (user_id, tuned_at_ms DESC);
CREATE INDEX idx_airings_channel ON airings (channel_id, recorded_at);

-- filler_acquisition_runs and filler_pulls, and the children they share -------------------------
DROP INDEX idx_filler_acquisition_runs_recent;
DROP INDEX idx_filler_acquisition_runs_source;
DROP INDEX idx_filler_acquisition_runs_pull;
DROP INDEX idx_filler_pulls_status;
DROP INDEX idx_filler_acquisition_artifacts_path;
DROP INDEX idx_filler_acquisition_artifacts_clip;
DROP INDEX idx_filler_acquisition_artifacts_recovery;
DROP INDEX idx_filler_acquisition_artifacts_remote;
DROP INDEX idx_filler_acquisition_artifacts_acquisition;
ALTER TABLE filler_acquisition_artifacts RENAME TO filler_acquisition_artifacts_old;
ALTER TABLE filler_pull_commits RENAME TO filler_pull_commits_old;
ALTER TABLE filler_acquisition_runs RENAME TO filler_acquisition_runs_old;
ALTER TABLE filler_pulls RENAME TO filler_pulls_old;
CREATE TABLE filler_acquisition_runs (
    id           TEXT    PRIMARY KEY,
    trigger      TEXT    NOT NULL,
    source_id    TEXT    NOT NULL DEFAULT '',
    pull_id      TEXT    NOT NULL DEFAULT '',
    status       TEXT    NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'success', 'error')),
    requested    INTEGER NOT NULL DEFAULT 0,
    fetched      INTEGER NOT NULL DEFAULT 0,
    skipped      INTEGER NOT NULL DEFAULT 0,
    failed       INTEGER NOT NULL DEFAULT 0,
    empty_count  INTEGER NOT NULL DEFAULT 0,
    error        TEXT    NOT NULL DEFAULT '',
    started_at   INTEGER NOT NULL,
    completed_at INTEGER NOT NULL DEFAULT 0,
    updated_at   INTEGER NOT NULL
);
CREATE TABLE filler_pulls (
    id          TEXT    PRIMARY KEY,
    title       TEXT    NOT NULL,
    reason      TEXT    NOT NULL DEFAULT '',
    proposed_by TEXT    NOT NULL DEFAULT '',
    -- A decided pull is kept, not deleted: the queue's History answers what was agreed and when.
    status      TEXT    NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'dismissed')),
    note        TEXT    NOT NULL DEFAULT '',
    -- ⚠ Rows the operator dropped stay in the plan, flagged: the record shows what was proposed
    -- and what was agreed to.
    plan_json   TEXT    NOT NULL DEFAULT '[]',
    created_at  INTEGER NOT NULL,
    decided_at  INTEGER NOT NULL DEFAULT 0,
    decided_by  TEXT    NOT NULL DEFAULT ''
);
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
    provider_archive_committed INTEGER NOT NULL DEFAULT 0 CHECK (provider_archive_committed IN (0, 1)),
    remote_id                  TEXT    NOT NULL DEFAULT '',
    gap                        TEXT    NOT NULL DEFAULT ''
);
CREATE TABLE filler_pull_commits (
    pull_id        TEXT PRIMARY KEY REFERENCES filler_pulls(id),
    acquisition_id TEXT NOT NULL UNIQUE REFERENCES filler_acquisition_runs(id)
);
INSERT INTO filler_acquisition_runs (id, trigger, source_id, pull_id, status, requested, fetched, skipped,
    failed, empty_count, error, started_at, completed_at, updated_at)
SELECT id, trigger, source_id, pull_id, status, requested, fetched, skipped,
    failed, empty_count, error, started_at, completed_at, updated_at
  FROM filler_acquisition_runs_old;
INSERT INTO filler_pulls (id, title, reason, proposed_by, status, note, plan_json, created_at, decided_at,
    decided_by)
SELECT id, title, reason, proposed_by, status, note, plan_json, created_at, decided_at,
    decided_by
  FROM filler_pulls_old;
INSERT INTO filler_acquisition_artifacts (id, acquisition_id, source_id, provider, source_url,
    staging_path, media_path, sidecar_path, media_sha256, media_bytes, clip_hash, state, repair_reason,
    completed_at, updated_at, provider_archive_entry, provider_archive_committed, remote_id, gap)
SELECT id, acquisition_id, source_id, provider, source_url,
    staging_path, media_path, sidecar_path, media_sha256, media_bytes, clip_hash, state, repair_reason,
    completed_at, updated_at, provider_archive_entry, provider_archive_committed, remote_id, gap
  FROM filler_acquisition_artifacts_old;
INSERT INTO filler_pull_commits (pull_id, acquisition_id)
SELECT pull_id, acquisition_id FROM filler_pull_commits_old;
DROP TABLE filler_acquisition_artifacts_old;
DROP TABLE filler_pull_commits_old;
DROP TABLE filler_acquisition_runs_old;
DROP TABLE filler_pulls_old;
CREATE INDEX idx_filler_acquisition_runs_recent ON filler_acquisition_runs (started_at DESC, id DESC);
CREATE INDEX idx_filler_acquisition_runs_source ON filler_acquisition_runs (source_id, started_at DESC);
CREATE INDEX idx_filler_acquisition_runs_pull ON filler_acquisition_runs (pull_id, started_at DESC);
CREATE INDEX idx_filler_pulls_status ON filler_pulls (status, created_at);
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

-- filler_sources and its check outcomes ---------------------------------------------------------
DROP INDEX idx_filler_sources_kind;
DROP INDEX idx_filler_sources_uri;
DROP INDEX idx_filler_sources_geography;
ALTER TABLE filler_source_check_outcomes RENAME TO filler_source_check_outcomes_old;
ALTER TABLE filler_sources RENAME TO filler_sources_old;
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
    enabled                INTEGER NOT NULL DEFAULT 1,
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
INSERT INTO filler_sources (id, kind, uri, label, license, last_fetched_at, created_at, enabled,
    fetch_every_seconds, fetch_max_per_run, country, last_checked_at, check_failure_count, check_retry_at,
    check_lease_until, scan_cursor, scan_watermark, scan_pending_watermark)
SELECT id, kind, uri, label, license, last_fetched_at, created_at, enabled,
    fetch_every_seconds, fetch_max_per_run, country, last_checked_at, check_failure_count, check_retry_at,
    check_lease_until, scan_cursor, scan_watermark, scan_pending_watermark
  FROM filler_sources_old;
INSERT INTO filler_source_check_outcomes (source_id, disposition, item_count)
SELECT source_id, disposition, item_count FROM filler_source_check_outcomes_old;
DROP TABLE filler_source_check_outcomes_old;
DROP TABLE filler_sources_old;
CREATE INDEX idx_filler_sources_kind ON filler_sources (kind, created_at);
CREATE UNIQUE INDEX idx_filler_sources_uri ON filler_sources (kind, uri) WHERE uri <> '';
CREATE INDEX idx_filler_sources_geography ON filler_sources (country);

-- inventory_sources and its children ------------------------------------------------------------
DROP INDEX idx_inventory_sources_item;
DROP INDEX idx_inventory_source_origins_source;
ALTER TABLE inventory_source_origins RENAME TO inventory_source_origins_old;
ALTER TABLE inventory_source_measurements RENAME TO inventory_source_measurements_old;
ALTER TABLE inventory_source_analysis RENAME TO inventory_source_analysis_old;
ALTER TABLE inventory_sources RENAME TO inventory_sources_old;
CREATE TABLE inventory_sources (
    id         TEXT   PRIMARY KEY,
    item_id    TEXT   NOT NULL REFERENCES inventory_items(id) ON DELETE CASCADE,
    kind       TEXT   NOT NULL CHECK (kind IN ('library_original', 'local_file')),
    revision   TEXT   NOT NULL,
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL
);
CREATE TABLE inventory_source_origins (
    authority_id       TEXT   NOT NULL,
    external_item_id   TEXT   NOT NULL,
    external_source_id TEXT   NOT NULL,
    source_id          TEXT   NOT NULL REFERENCES inventory_sources(id) ON DELETE CASCADE,
    locator_json       TEXT   NOT NULL,
    observation_json   TEXT   NOT NULL,
    observed_at        BIGINT NOT NULL,
    last_seen_at       BIGINT NOT NULL,
    missing_at         BIGINT,
    PRIMARY KEY (authority_id, external_item_id, external_source_id),
    FOREIGN KEY (authority_id, external_item_id)
        REFERENCES inventory_item_origins(authority_id, external_item_id) ON DELETE CASCADE
);
CREATE TABLE inventory_source_measurements (
    source_id        TEXT   PRIMARY KEY REFERENCES inventory_sources(id) ON DELETE CASCADE,
    source_revision  TEXT   NOT NULL,
    observation_json TEXT   NOT NULL,
    observed_at      BIGINT NOT NULL
);
CREATE TABLE inventory_source_analysis (
    source_id           TEXT    PRIMARY KEY REFERENCES inventory_sources(id) ON DELETE CASCADE,
    source_revision     TEXT    NOT NULL,
    schema_version      INTEGER NOT NULL,
    keyframes           BLOB    NOT NULL,
    keyframe_count      INTEGER NOT NULL,
    integrated_lufs     DOUBLE PRECISION,
    true_peak_dbtp      DOUBLE PRECISION,
    breaks_json         TEXT    NOT NULL,
    analyzed_at         BIGINT  NOT NULL,
    active_picture_json TEXT
);
INSERT INTO inventory_sources (id, item_id, kind, revision, created_at, updated_at)
SELECT id, item_id, kind, revision, created_at, updated_at FROM inventory_sources_old;
INSERT INTO inventory_source_origins (authority_id, external_item_id, external_source_id, source_id,
    locator_json, observation_json, observed_at, last_seen_at, missing_at)
SELECT authority_id, external_item_id, external_source_id, source_id,
    locator_json, observation_json, observed_at, last_seen_at, missing_at
  FROM inventory_source_origins_old;
INSERT INTO inventory_source_measurements (source_id, source_revision, observation_json, observed_at)
SELECT source_id, source_revision, observation_json, observed_at FROM inventory_source_measurements_old;
INSERT INTO inventory_source_analysis (source_id, source_revision, schema_version, keyframes,
    keyframe_count, integrated_lufs, true_peak_dbtp, breaks_json, analyzed_at, active_picture_json)
SELECT source_id, source_revision, schema_version, keyframes,
    keyframe_count, integrated_lufs, true_peak_dbtp, breaks_json, analyzed_at, active_picture_json
  FROM inventory_source_analysis_old;
DROP TABLE inventory_source_origins_old;
DROP TABLE inventory_source_measurements_old;
DROP TABLE inventory_source_analysis_old;
DROP TABLE inventory_sources_old;
CREATE INDEX idx_inventory_sources_item ON inventory_sources (item_id);
CREATE INDEX idx_inventory_source_origins_source ON inventory_source_origins (source_id);

-- library_items and its descendants -------------------------------------------------------------
DROP INDEX idx_library_items_sort;
DROP INDEX idx_library_items_catalog;
DROP INDEX idx_library_items_origin;
DROP INDEX idx_library_items_set;
DROP INDEX idx_library_items_series_origin;
DROP INDEX idx_library_items_series;
DROP INDEX idx_library_ids_value;
DROP INDEX idx_library_genres_name;
DROP INDEX idx_library_studios_name;
DROP INDEX idx_library_tags_name;
DROP INDEX idx_library_sources_path;
DROP INDEX idx_library_streams_language;
DROP INDEX idx_library_countries_name;
DROP INDEX idx_library_credits_person;
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
    grounded         INTEGER NOT NULL DEFAULT 1,
    searched         TEXT    NOT NULL DEFAULT '',
    match_basis      TEXT    NOT NULL DEFAULT '',
    match_app        TEXT    NOT NULL DEFAULT '',
    match_page       TEXT    NOT NULL DEFAULT '',
    needs_noticed    INTEGER NOT NULL DEFAULT 0,
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
    plays       BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (item_id, path)
);
CREATE TABLE library_streams (
    item_id          TEXT    NOT NULL,
    path             TEXT    NOT NULL,
    stream_index     INTEGER NOT NULL,
    kind             TEXT    NOT NULL CHECK (kind IN ('video', 'audio', 'subtitle')),
    codec            TEXT    NOT NULL DEFAULT '',
    profile          TEXT    NOT NULL DEFAULT '',
    title            TEXT    NOT NULL DEFAULT '',
    is_default       BOOLEAN NOT NULL DEFAULT FALSE,
    forced           BOOLEAN NOT NULL DEFAULT FALSE,
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
    commentary       BOOLEAN NOT NULL DEFAULT FALSE,
    hearing_impaired BOOLEAN NOT NULL DEFAULT FALSE,
    descriptive      BOOLEAN NOT NULL DEFAULT FALSE,
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
CREATE TABLE library_art (
    item_id  TEXT    NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
    kind     TEXT    NOT NULL,
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
INSERT INTO library_items (id, kind, name, sort_name, year, updated_at_ms, catalog, origin_authority,
    origin_key, grounded, searched, match_basis, match_app, match_page, needs_noticed, set_key,
    original_title, overview, premiere_date, rating, score, runtime_ms, season, episode, episode_end,
    series_authority, series_key, series_id, name_from, year_from, overview_from, premiere_from,
    rating_from, score_from, runtime_from, tagline, trailer)
SELECT id, kind, name, sort_name, year, updated_at_ms, catalog, origin_authority,
    origin_key, grounded, searched, match_basis, match_app, match_page, needs_noticed, set_key,
    original_title, overview, premiere_date, rating, score, runtime_ms, season, episode, episode_end,
    series_authority, series_key, series_id, name_from, year_from, overview_from, premiere_from,
    rating_from, score_from, runtime_from, tagline, trailer
  FROM library_items_old;
INSERT INTO library_ids (item_id, namespace, value) SELECT item_id, namespace, value FROM library_ids_old;
INSERT INTO library_genres (item_id, name, position) SELECT item_id, name, position FROM library_genres_old;
INSERT INTO library_studios (item_id, name, position) SELECT item_id, name, position FROM library_studios_old;
INSERT INTO library_tags (item_id, name, position) SELECT item_id, name, position FROM library_tags_old;
INSERT INTO library_sources (item_id, path, container, size_bytes, bitrate, duration_ms, edition, plays)
SELECT item_id, path, container, size_bytes, bitrate, duration_ms, edition, plays FROM library_sources_old;
INSERT INTO library_streams (item_id, path, stream_index, kind, codec, profile, title, is_default,
    forced, language, language_name, channels, channel_layout, sample_rate, audio_format, width,
    height, frame_rate, pixel_format, color_transfer, resolution, hdr, commentary, hearing_impaired,
    descriptive, external_ref)
SELECT item_id, path, stream_index, kind, codec, profile, title, is_default,
    forced, language, language_name, channels, channel_layout, sample_rate, audio_format, width,
    height, frame_rate, pixel_format, color_transfer, resolution, hdr, commentary, hearing_impaired,
    descriptive, external_ref
  FROM library_streams_old;
INSERT INTO library_trickplay (item_id, path, ref, format) SELECT item_id, path, ref, format FROM library_trickplay_old;
INSERT INTO library_chapters (item_id, path, position, start_ms, end_ms, title)
SELECT item_id, path, position, start_ms, end_ms, title FROM library_chapters_old;
INSERT INTO library_markers (item_id, path, position, start_ms, end_ms, kind)
SELECT item_id, path, position, start_ms, end_ms, kind FROM library_markers_old;
INSERT INTO library_art (item_id, kind, season, position, ref)
SELECT item_id, kind, season, position, ref FROM library_art_old;
INSERT INTO library_countries (item_id, name, position) SELECT item_id, name, position FROM library_countries_old;
INSERT INTO library_credits (item_id, position, person_id, role, character_name)
SELECT item_id, position, person_id, role, character_name FROM library_credits_old;
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
CREATE INDEX idx_library_streams_language ON library_streams (kind, language);
CREATE INDEX idx_library_countries_name ON library_countries (name);
CREATE INDEX idx_library_credits_person ON library_credits (person_id, role);

-- proposal_jobs and its attempts ----------------------------------------------------------------
DROP INDEX idx_proposal_jobs_status_deadline;
DROP INDEX idx_proposal_jobs_intent_hash;
DROP INDEX idx_proposal_job_attempts_job_started;
ALTER TABLE proposal_job_attempts RENAME TO proposal_job_attempts_old;
ALTER TABLE proposal_jobs RENAME TO proposal_jobs_old;
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
    reached_live       BOOLEAN NOT NULL DEFAULT 0,
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
INSERT INTO proposal_jobs (id, kind, status, intent_json, intent_hash, created_by, last_error, deadline,
    attempts, created_at, updated_at, failure_code, workflow_version, reached_live, failure_trace_json,
    channel_id)
SELECT id, kind, status, intent_json, intent_hash, created_by, last_error, deadline,
    attempts, created_at, updated_at, failure_code, workflow_version, reached_live, failure_trace_json,
    channel_id
  FROM proposal_jobs_old;
INSERT INTO proposal_job_attempts (job_id, attempt, workflow_version, status, started_at, completed_at,
    failure_code)
SELECT job_id, attempt, workflow_version, status, started_at, completed_at, failure_code
  FROM proposal_job_attempts_old;
DROP TABLE proposal_job_attempts_old;
DROP TABLE proposal_jobs_old;
CREATE INDEX idx_proposal_jobs_status_deadline ON proposal_jobs (status, deadline);
CREATE INDEX idx_proposal_jobs_intent_hash ON proposal_jobs (intent_hash);
CREATE INDEX idx_proposal_job_attempts_job_started ON proposal_job_attempts (job_id, started_at DESC);

-- The spoken-safety ledger: nothing has started a run since its evaluation cascade was deleted
-- (#1763), and production holds no events. Events go first: they reference the runs.
DROP TABLE filler_spoken_safety_events;
DROP TABLE filler_spoken_safety_runs;

-- filler_inference_evaluations and its structure-window ledger -----------------------------------
-- Without the spoken-safety ledger, outcome is '' or window_complete (a structure window answered).
DROP INDEX idx_filler_inference_evaluations_clip_created;
DROP INDEX idx_filler_inference_evaluations_cache_key;
DROP INDEX idx_filler_inference_evaluations_run_created;
DROP INDEX idx_filler_inference_evaluations_day_budget;
DROP INDEX idx_filler_structure_window_call_ledger_source;
DROP INDEX idx_filler_structure_window_call_ledger_open;
ALTER TABLE filler_structure_window_call_ledger RENAME TO filler_structure_window_call_ledger_old;
ALTER TABLE filler_inference_evaluations RENAME TO filler_inference_evaluations_old;
CREATE TABLE filler_inference_evaluations (
    id                       TEXT    PRIMARY KEY,
    cache_key                TEXT    NOT NULL,
    clip_hash                TEXT    NOT NULL,
    run_id                   TEXT    NOT NULL DEFAULT '',
    role                     TEXT    NOT NULL,
    rung                     TEXT    NOT NULL DEFAULT '',
    state                    TEXT    NOT NULL CHECK (state IN ('reserved', 'completed', 'failed', 'held_budget')),
    requested_provider       TEXT    NOT NULL,
    requested_model          TEXT    NOT NULL,
    resolved_provider        TEXT    NOT NULL DEFAULT '',
    resolved_model           TEXT    NOT NULL DEFAULT '',
    upstream_provider        TEXT    NOT NULL DEFAULT '',
    modalities_json          TEXT    NOT NULL DEFAULT '[]',
    derivative_bytes         INTEGER NOT NULL DEFAULT 0,
    derivative_duration_ms   INTEGER NOT NULL DEFAULT 0,
    derivative_pixels        INTEGER NOT NULL DEFAULT 0,
    prompt_tokens            INTEGER NOT NULL DEFAULT 0,
    completion_tokens        INTEGER NOT NULL DEFAULT 0,
    reasoning_tokens         INTEGER NOT NULL DEFAULT 0,
    cached_tokens            INTEGER NOT NULL DEFAULT 0,
    cache_write_tokens       INTEGER NOT NULL DEFAULT 0,
    image_tokens             INTEGER NOT NULL DEFAULT 0,
    audio_tokens             INTEGER NOT NULL DEFAULT 0,
    video_tokens             INTEGER NOT NULL DEFAULT 0,
    charged_amount           TEXT    NOT NULL DEFAULT '',
    charged_currency         TEXT    NOT NULL DEFAULT '',
    charged_nano_usd         INTEGER NOT NULL DEFAULT 0,
    estimated_nano_usd       INTEGER NOT NULL DEFAULT 0,
    reserved_nano_usd        INTEGER NOT NULL DEFAULT 0,
    price_snapshot           TEXT    NOT NULL DEFAULT '',
    latency_ms               INTEGER NOT NULL DEFAULT 0,
    attempts                 INTEGER NOT NULL DEFAULT 0,
    generation_id            TEXT    NOT NULL DEFAULT '',
    evidence_version         TEXT    NOT NULL,
    extractor_version        TEXT    NOT NULL,
    prompt_version           TEXT    NOT NULL,
    schema_version           TEXT    NOT NULL,
    taxonomy_version         TEXT    NOT NULL,
    admission_policy_version TEXT    NOT NULL,
    role_policy_version      TEXT    NOT NULL,
    capability_snapshot      TEXT    NOT NULL,
    outcome                  TEXT    NOT NULL DEFAULT '' CHECK (outcome IN ('', 'window_complete')),
    failure_reason           TEXT    NOT NULL DEFAULT '',
    created_at               INTEGER NOT NULL,
    updated_at               INTEGER NOT NULL
);
CREATE TABLE filler_structure_window_call_ledger (
    request_sha256   TEXT    NOT NULL,
    attempt          INTEGER NOT NULL DEFAULT 1,
    evaluation_id    TEXT    NOT NULL UNIQUE REFERENCES filler_inference_evaluations(id),
    source_sha256    TEXT    NOT NULL,
    media_set_sha256 TEXT    NOT NULL,
    window_ordinal   INTEGER NOT NULL,
    assessor_id      TEXT    NOT NULL,
    state            TEXT    NOT NULL,
    reservation_json TEXT    NOT NULL,
    record_sha256    TEXT    NOT NULL DEFAULT '',
    record_json      TEXT    NOT NULL DEFAULT '',
    requested_at     INTEGER NOT NULL,
    assessed_at      INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (request_sha256, attempt),
    CHECK (attempt >= 1),
    CHECK (window_ordinal >= 0),
    CHECK (state IN ('open', 'held_budget', 'settled')),
    CHECK (
        (state = 'settled' AND record_sha256 <> '' AND record_json <> '' AND assessed_at > 0) OR
        (state <> 'settled' AND record_sha256 = '' AND record_json = '' AND assessed_at = 0)
    )
);
INSERT INTO filler_inference_evaluations (id, cache_key, clip_hash, run_id, role, rung, state,
    requested_provider, requested_model, resolved_provider, resolved_model, upstream_provider,
    modalities_json, derivative_bytes, derivative_duration_ms, derivative_pixels, prompt_tokens,
    completion_tokens, reasoning_tokens, cached_tokens, cache_write_tokens, image_tokens, audio_tokens,
    video_tokens, charged_amount, charged_currency, charged_nano_usd, estimated_nano_usd,
    reserved_nano_usd, price_snapshot, latency_ms, attempts, generation_id, evidence_version,
    extractor_version, prompt_version, schema_version, taxonomy_version, admission_policy_version,
    role_policy_version, capability_snapshot, outcome, failure_reason, created_at, updated_at)
SELECT id, cache_key, clip_hash, run_id, role, rung, state,
    requested_provider, requested_model, resolved_provider, resolved_model, upstream_provider,
    modalities_json, derivative_bytes, derivative_duration_ms, derivative_pixels, prompt_tokens,
    completion_tokens, reasoning_tokens, cached_tokens, cache_write_tokens, image_tokens, audio_tokens,
    video_tokens, charged_amount, charged_currency, charged_nano_usd, estimated_nano_usd,
    reserved_nano_usd, price_snapshot, latency_ms, attempts, generation_id, evidence_version,
    extractor_version, prompt_version, schema_version, taxonomy_version, admission_policy_version,
    role_policy_version, capability_snapshot, outcome, failure_reason, created_at, updated_at
  FROM filler_inference_evaluations_old;
INSERT INTO filler_structure_window_call_ledger (request_sha256, attempt, evaluation_id, source_sha256,
    media_set_sha256, window_ordinal, assessor_id, state, reservation_json, record_sha256, record_json,
    requested_at, assessed_at)
SELECT request_sha256, attempt, evaluation_id, source_sha256,
    media_set_sha256, window_ordinal, assessor_id, state, reservation_json, record_sha256, record_json,
    requested_at, assessed_at
  FROM filler_structure_window_call_ledger_old;
DROP TABLE filler_structure_window_call_ledger_old;
DROP TABLE filler_inference_evaluations_old;
CREATE INDEX idx_filler_inference_evaluations_clip_created
    ON filler_inference_evaluations (clip_hash, created_at DESC, id DESC);
CREATE INDEX idx_filler_inference_evaluations_cache_key
    ON filler_inference_evaluations (cache_key, created_at DESC, id DESC);
CREATE INDEX idx_filler_inference_evaluations_run_created
    ON filler_inference_evaluations (run_id, created_at DESC, id DESC);
CREATE INDEX idx_filler_inference_evaluations_day_budget
    ON filler_inference_evaluations (created_at, reserved_nano_usd);
CREATE INDEX idx_filler_structure_window_call_ledger_source
    ON filler_structure_window_call_ledger (source_sha256, media_set_sha256, requested_at DESC, request_sha256);
CREATE INDEX idx_filler_structure_window_call_ledger_open
    ON filler_structure_window_call_ledger (state, requested_at ASC, request_sha256);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
