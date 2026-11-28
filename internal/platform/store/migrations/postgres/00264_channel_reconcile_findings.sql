-- +goose Up
-- #4706: what a reconcile finds about a channel (the relaxations it applied, the lineup titles that
-- cannot air) is not policy. It moves out of policy_json into its own column, so policy_json holds
-- only what an operator or a proposal decided.
ALTER TABLE channels ADD COLUMN reconcile_findings_json jsonb NOT NULL DEFAULT '{}'::jsonb;

-- Each channel keeps its last findings until its next reconcile rewrites them; the column names only
-- the findings the policy held, a null one dropped. Removing the two keys from the jsonb policy
-- leaves every other key and value as they were. A policy that is not an object is left alone (on
-- an array, `-` would remove string elements) and keeps the empty findings.
UPDATE channels SET
  reconcile_findings_json = (
    SELECT COALESCE(jsonb_object_agg(f.key, f.value), '{}'::jsonb)
    FROM jsonb_each(policy_json) AS f
    WHERE f.key IN ('applied', 'unairable') AND jsonb_typeof(f.value) <> 'null'),
  policy_json = policy_json - 'applied' - 'unairable'
WHERE jsonb_typeof(policy_json) = 'object'
  AND (policy_json -> 'applied' IS NOT NULL OR policy_json -> 'unairable' IS NOT NULL);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
