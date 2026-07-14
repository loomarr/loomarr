-- +goose Up
-- §10 filler redesign: clip identity moves from the media-server item id to the
-- Tunarr `local`-source program uuid (the media server is no longer in the filler
-- path — clips are scanned by Tunarr, not Emby/Jellyfin). Forward-only (§16).
--
-- The identity SOURCE changes, so existing rows (keyed by a media-server id) are
-- invalid under the new scheme — drop + recreate EMPTY. Filler is a synced cache,
-- not source-of-truth data: the next /v1/filler/sync repopulates the catalog from
-- Tunarr's local filler source. (Pre-1.0, dev-only; no production installs.)

DROP TABLE clips;

CREATE TABLE clips (
    tunarr_program_id TEXT PRIMARY KEY,           -- Tunarr `local`-source program uuid = identity
    name              TEXT NOT NULL,
    kind              TEXT NOT NULL DEFAULT 'interstitial', -- commercial|bumper|station_id|psa|trailer|interstitial
    era               INTEGER NOT NULL DEFAULT 0, -- decade/year, e.g. 1994; 0 = untagged
    audience          TEXT NOT NULL DEFAULT '',   -- kids|family|general|late_night; '' = untagged
    category          TEXT NOT NULL DEFAULT '',   -- toys|cereal|…; '' = untagged
    duration_ms       BIGINT NOT NULL DEFAULT 0,  -- from Tunarr's scan (the core never probes)
    rating            TEXT NOT NULL DEFAULT '',
    source            TEXT NOT NULL DEFAULT '',   -- tunarr-local|manual|…
    ai_tagged         INTEGER NOT NULL DEFAULT 0, -- 1 = tags came from AI classification
    updated_at        BIGINT NOT NULL DEFAULT 0
);

-- Pod matching filters by kind/era/audience/category; index the hot paths.
CREATE INDEX idx_clips_kind ON clips (kind);
CREATE INDEX idx_clips_match ON clips (audience, era);

-- +goose Down
-- Restore the 00005 shape (media-server-item-id identity), empty.
DROP TABLE clips;

CREATE TABLE clips (
    library_item_id TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    kind            TEXT NOT NULL DEFAULT 'interstitial',
    era             INTEGER NOT NULL DEFAULT 0,
    audience        TEXT NOT NULL DEFAULT '',
    category        TEXT NOT NULL DEFAULT '',
    duration_ms     BIGINT NOT NULL DEFAULT 0,
    rating          TEXT NOT NULL DEFAULT '',
    source          TEXT NOT NULL DEFAULT '',
    ai_tagged       INTEGER NOT NULL DEFAULT 0,
    updated_at      BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX idx_clips_kind ON clips (kind);
CREATE INDEX idx_clips_match ON clips (audience, era);
