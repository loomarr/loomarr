-- +goose Up
-- PostgreSQL mirror of SQLite 00170; the full rationale lives there.
ALTER TABLE clips DROP COLUMN ai_tagged;
ALTER TABLE clips DROP COLUMN vision_tagged;
ALTER TABLE clips DROP COLUMN auto_filed;
ALTER TABLE clips DROP COLUMN confidence;
ALTER TABLE clips DROP COLUMN suggested_era;
ALTER TABLE clips DROP COLUMN category;
DROP TABLE filler_enrichment_backfills;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
