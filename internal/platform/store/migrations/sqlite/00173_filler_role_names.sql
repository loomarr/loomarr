-- +goose Up
-- #2323: after 00171 renamed clips.kind to clips.role, two stored names still said "kind" for the
-- same idea. Both are renamed here and the values are left alone:
--   - a channel's filler selection keeps its chosen roles under policy_json.filler.roles (it was
--     filler.kinds); an absent key still means the default roles;
--   - the accepted enrichment answer for a clip's role is stored under axis 'role' (it was 'kind').
UPDATE channels
SET policy_json = json_remove(
    json_set(policy_json, '$.filler.roles', json(json_extract(policy_json, '$.filler.kinds'))),
    '$.filler.kinds')
WHERE json_valid(policy_json) AND json_type(policy_json, '$.filler.kinds') IS NOT NULL;

UPDATE filler_enrichment_axes SET axis = 'role' WHERE axis = 'kind';

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
