-- +goose Up
-- §9.1/§10: clip identity moves from the Tunarr `local`-source program uuid to the clip's
-- PATH RELATIVE TO FILLER_DIR. Forward-only (§16).
--
-- Why, in one line: internal playout needs a playable input, and a Tunarr program uuid is not
-- one. The longer version:
--
--   1. Loomarr's own encoder takes a file path or a URL. Under the old identity a channel on the
--      internal backend could assemble a pod and then have nothing to hand ffmpeg.
--   2. The dependency ran the wrong way. Clips were DISCOVERED by asking Tunarr to scan
--      FILLER_DIR, so an install running internal playout with no Tunarr at all had an empty
--      catalog and therefore no commercials — a hard requirement on a service §9.1 makes
--      optional. The files were on Loomarr's own disk the whole time.
--
-- The old arrangement's stated premise is also gone: it existed so "probing stays out of loomarr
-- entirely — the core never needs ffprobe", and §14 now bundles ffmpeg AND ffprobe as core
-- runtime dependencies precisely because internal playout owns duration and cut points.
--
-- The identity SOURCE changes, so existing rows (keyed by a Tunarr uuid) are invalid under the
-- new scheme — drop + recreate EMPTY, exactly as 00006 did for the same reason. Filler is a
-- synced CACHE, not source-of-truth data: the next /v1/filler/sync repopulates it from
-- FILLER_DIR. (Pre-1.0, dev-only; no production installs.)

DROP TABLE clips;

CREATE TABLE clips (
    -- Path RELATIVE to FILLER_DIR ("1994/toys-transformers.mp4") = identity.
    --
    -- Relative, not absolute, deliberately: FILLER_DIR is a deployment detail that differs
    -- between the host and a container (/data/filler vs ~/clips), and an absolute path would
    -- make every row invalid the first time someone moves the mount. Relative paths survive it,
    -- and the same folder mounted anywhere yields the same catalog — which also keeps pod
    -- assembly deterministic across environments, since the seed hashes clip ids.
    path              TEXT PRIMARY KEY,
    name              TEXT NOT NULL,
    -- The Tunarr program uuid, when Tunarr knows this clip. NULLABLE and no longer identity:
    -- Tunarr-backed channels still need it for filler-lists (attachFillerList), but an install
    -- with no Tunarr simply leaves it empty rather than having no catalog at all. One catalog
    -- serves both backends — internal playout reads `path`, Tunarr reads this.
    tunarr_program_id TEXT,
    kind              TEXT NOT NULL DEFAULT 'interstitial', -- commercial|bumper|station_id|psa|trailer|interstitial
    era               INTEGER NOT NULL DEFAULT 0, -- decade/year, e.g. 1994; 0 = untagged
    audience          TEXT NOT NULL DEFAULT '',   -- kids|family|general|late_night; '' = untagged
    category          TEXT NOT NULL DEFAULT '',   -- toys|cereal|…; '' = untagged
    duration_ms       BIGINT NOT NULL DEFAULT 0,  -- from ffprobe (the core probes now, §14)
    rating            TEXT NOT NULL DEFAULT '',
    source            TEXT NOT NULL DEFAULT '',   -- filler-dir|tunarr-local|manual|…
    ai_tagged         INTEGER NOT NULL DEFAULT 0, -- 1 = tags came from AI classification
    updated_at        BIGINT NOT NULL DEFAULT 0
);

-- Pod matching filters by kind/era/audience/category; index the hot paths.
CREATE INDEX idx_clips_kind ON clips (kind);
CREATE INDEX idx_clips_match ON clips (audience, era);
-- Tunarr-backed reconcile looks clips up by program id when building a filler-list.
CREATE INDEX idx_clips_tunarr ON clips (tunarr_program_id);

-- +goose Down
-- Restore the 00006 shape (Tunarr-program-id identity), empty.
DROP TABLE clips;

CREATE TABLE clips (
    tunarr_program_id TEXT PRIMARY KEY,
    name              TEXT NOT NULL,
    kind              TEXT NOT NULL DEFAULT 'interstitial',
    era               INTEGER NOT NULL DEFAULT 0,
    audience          TEXT NOT NULL DEFAULT '',
    category          TEXT NOT NULL DEFAULT '',
    duration_ms       BIGINT NOT NULL DEFAULT 0,
    rating            TEXT NOT NULL DEFAULT '',
    source            TEXT NOT NULL DEFAULT '',
    ai_tagged         INTEGER NOT NULL DEFAULT 0,
    updated_at        BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX idx_clips_kind ON clips (kind);
CREATE INDEX idx_clips_match ON clips (audience, era);
