-- +goose Up
-- #2323: the clips columns that no code reads any more.
--   ai_tagged, vision_tagged, auto_filed — the pipeline ladder records which rungs ran.
--   confidence — the score rung became finish and the score is gone.
--   suggested_era — an era guess is now an uncited Context observation (filler_context_reports).
--   category — derived from the taxonomy tags on every read.
-- filler_enrichment_backfills marked a one-time backfill that no longer exists.
--
-- No values are carried forward: every one of these is either derivable or retired (§16 no legacy paths).
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
