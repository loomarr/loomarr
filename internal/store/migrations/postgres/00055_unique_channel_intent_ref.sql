-- +goose Up
-- One suggestion job owns at most one channel. Older databases can contain duplicate
-- bindings because the original lookup index was non-unique. Prefer a managed row over
-- a detached one, then keep the most recently updated binding (id breaks timestamp ties
-- deterministically), and clear the others;
-- channels are operator-owned records, so a migration must never delete them.
WITH ranked AS (
    SELECT id,
           ROW_NUMBER() OVER (
               PARTITION BY intent_ref
               ORDER BY CASE WHEN status = 'detached' THEN 1 ELSE 0 END,
                        updated_at DESC, id DESC
           ) AS position
      FROM channels
     WHERE intent_ref <> ''
)
UPDATE channels AS channels_to_detach
   SET intent_ref = ''
  FROM ranked
 WHERE channels_to_detach.id = ranked.id
   AND ranked.position > 1;

DROP INDEX idx_channels_intent_ref;
CREATE UNIQUE INDEX idx_channels_intent_ref
    ON channels (intent_ref)
    WHERE intent_ref <> '';

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
