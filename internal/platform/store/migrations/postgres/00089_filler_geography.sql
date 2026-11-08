-- +goose Up
ALTER TABLE clips ADD COLUMN geographic_scope TEXT NOT NULL DEFAULT 'unknown';
ALTER TABLE clips ADD COLUMN country TEXT NOT NULL DEFAULT '';
ALTER TABLE clips ADD COLUMN market TEXT NOT NULL DEFAULT '';
ALTER TABLE clips ADD COLUMN network TEXT NOT NULL DEFAULT '';
ALTER TABLE clips ADD COLUMN station TEXT NOT NULL DEFAULT '';
ALTER TABLE clips ADD COLUMN air_date TEXT NOT NULL DEFAULT '';
ALTER TABLE clips ADD COLUMN geo_evidence TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_clips_geography ON clips (geographic_scope, country, market);
ALTER TABLE filler_sources ADD COLUMN country TEXT NOT NULL DEFAULT '';
ALTER TABLE filler_sources ADD COLUMN market TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_filler_sources_geography ON filler_sources (country, market);

-- +goose Down
DROP INDEX IF EXISTS idx_clips_geography;
DROP INDEX IF EXISTS idx_filler_sources_geography;
ALTER TABLE filler_sources DROP COLUMN market;
ALTER TABLE filler_sources DROP COLUMN country;
ALTER TABLE clips DROP COLUMN geo_evidence;
ALTER TABLE clips DROP COLUMN air_date;
ALTER TABLE clips DROP COLUMN station;
ALTER TABLE clips DROP COLUMN network;
ALTER TABLE clips DROP COLUMN market;
ALTER TABLE clips DROP COLUMN country;
ALTER TABLE clips DROP COLUMN geographic_scope;
