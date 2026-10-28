-- +goose Up
-- #2062: a local clip is local to its broadcast station, and a household is a picked place with a
-- radius. The typed market name could never be matched against a station, so it goes everywhere:
-- the clip and source columns and the installation setting (now `filler.home_place`).

DROP INDEX idx_clips_geography;
ALTER TABLE clips DROP COLUMN market;
CREATE INDEX idx_clips_geography ON clips (geographic_scope, country);

DROP INDEX idx_filler_sources_geography;
ALTER TABLE filler_sources DROP COLUMN market;
CREATE INDEX idx_filler_sources_geography ON filler_sources (country);

DELETE FROM settings WHERE key = 'filler.home_market';

-- Forward-only (§16).

-- +goose Down
SELECT 1;
