-- +goose Up
-- PostgreSQL mirror of SQLite 00173; the full rationale lives there. policy_json is TEXT.
UPDATE channels
SET policy_json = jsonb_set(policy_json::jsonb #- '{filler,kinds}', '{filler,roles}',
    policy_json::jsonb #> '{filler,kinds}')::text
WHERE policy_json::jsonb #> '{filler,kinds}' IS NOT NULL;

UPDATE filler_enrichment_axes SET axis = 'role' WHERE axis = 'kind';

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
