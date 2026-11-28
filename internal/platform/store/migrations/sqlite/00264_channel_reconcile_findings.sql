-- +goose Up
-- #4706: what a reconcile finds about a channel (the relaxations it applied, the lineup titles that
-- cannot air) is not policy. It moves out of policy_json into its own column, so policy_json holds
-- only what an operator or a proposal decided.
ALTER TABLE channels ADD COLUMN reconcile_findings_json TEXT NOT NULL DEFAULT '{}'
    CHECK (json_valid(reconcile_findings_json));

-- Each channel keeps its last findings until its next reconcile rewrites them. json_patch drops a
-- key whose value is null, so the column names only the findings the policy held. json_remove
-- rewrites the policy with every other key, in its order and with its own text. policy_json is
-- CHECKed as JSON; one that is not an object is left alone and keeps the empty findings.
UPDATE channels SET
  reconcile_findings_json = json_patch('{}', json_object(
    'applied', json_extract(policy_json, '$.applied'),
    'unairable', json_extract(policy_json, '$.unairable'))),
  policy_json = json_remove(policy_json, '$.applied', '$.unairable')
WHERE json_type(policy_json) = 'object'
  AND (json_type(policy_json, '$.applied') IS NOT NULL OR json_type(policy_json, '$.unairable') IS NOT NULL);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
