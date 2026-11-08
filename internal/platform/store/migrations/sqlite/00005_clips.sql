-- +goose Up
-- Filler clip catalog (§10). Forward-only (§16). Clips are synced FROM the media
-- server's filler library — identity is the media-server item id (library is
-- source of truth, §4), duration comes from the server (the core never probes
-- media). Distinct from titles: clips are not in TMDB and have no acquisition
-- loop. Metadata (kind/era/audience/category) is what enables pod matching (§10).

CREATE TABLE clips (
    library_item_id TEXT PRIMARY KEY,           -- media-server item id = identity
    name            TEXT NOT NULL,
    kind            TEXT NOT NULL DEFAULT 'interstitial', -- commercial|bumper|station_id|psa|trailer|interstitial
    era             INTEGER NOT NULL DEFAULT 0, -- decade/year, e.g. 1994; 0 = untagged
    audience        TEXT NOT NULL DEFAULT '',   -- kids|family|general|late_night; '' = untagged
    category        TEXT NOT NULL DEFAULT '',   -- toys|cereal|…; '' = untagged
    duration_ms     BIGINT NOT NULL DEFAULT 0,  -- from the media server
    rating          TEXT NOT NULL DEFAULT '',
    source          TEXT NOT NULL DEFAULT '',   -- archive|youtube|manual|…
    ai_tagged       INTEGER NOT NULL DEFAULT 0, -- 1 = tags came from AI classification
    updated_at      BIGINT NOT NULL DEFAULT 0
);

-- Pod matching filters by kind/era/audience/category; index the hot paths.
CREATE INDEX idx_clips_kind ON clips (kind);
CREATE INDEX idx_clips_match ON clips (audience, era);

-- +goose Down
DROP TABLE clips;
