-- +goose Up
DELETE FROM filler_enrichment_axes
WHERE axis = 'geography'
  AND state = 'complete'
  AND evidence_kind <> 'operator'
  AND value_json = '{"geography":{}}';

-- Forward-only (§16).

-- +goose Down
SELECT 1;
