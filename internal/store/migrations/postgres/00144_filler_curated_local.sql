-- +goose Up
-- A curated source's covered stations and the planner's switch for local material (#2059 decision
-- 10). `stations` is the comma-separated station ids the shipped list tags the source with,
-- refreshed on every offer. `local_state` records what the planner did with it:
--   ''         untouched;
--   'on'       the planner switched it on for a nearby place;
--   'operator' it was already on when a place came near, so it is the operator's and left alone;
--   'off'      the operator switched off what the planner switched on, so it stays off.
ALTER TABLE filler_curated_offers ADD COLUMN stations TEXT NOT NULL DEFAULT '';
ALTER TABLE filler_curated_offers ADD COLUMN local_state TEXT NOT NULL DEFAULT '';

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
