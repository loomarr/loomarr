-- +goose Up
-- Channel-scoped discovery feedback belongs to one currently persisted Channel identity. Older
-- binaries accepted arbitrary scope ids, so clean only rows whose identity is already absent.
-- Detached Channels retain their row and therefore their feedback; household events are outside
-- this predicate. Live AppendDiscoveryFeedback and DeleteChannel enforce the invariant atomically.
DELETE FROM discovery_feedback
 WHERE scope = 'channel'
   AND NOT EXISTS (
       SELECT 1 FROM channels WHERE channels.id = discovery_feedback.scope_id
   );

-- +goose Down
-- Forward-only (§16): deleted orphan editorial events cannot be reconstructed truthfully.
SELECT 1;
