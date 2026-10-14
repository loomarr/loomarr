-- +goose Up
-- Activity feed — what Loomarr did, for the Dashboard's Recent activity panel (§5, §12, V32).
--
-- WHY A TABLE AND NOT THE EVENT BUS. Loomarr already fans state changes out over SSE, and
-- persisting there would be one line. But `internal/events/bus.go` is explicit that the bus is
-- in-memory, single-process, and DROPS events for a slow subscriber — "a dropped event is a
-- latency bug, not a correctness bug", because the GET endpoints are always the truth. A feed
-- built on that would lose rows exactly when the install is busiest, which is when the operator
-- most wants to read it. V32's gate is "a persisted feed that survives restart"; the bus can
-- satisfy neither half.
--
-- The bus is also domain-NEUTRAL: it carries {type:"title"}, not "Darkwing Duck landed — CH 42
-- slot 05 backfilled in place". Only the subsystem making the change knows what happened, so
-- rows are written at each domain transition (reconcile, suggest, filler, channels) rather than
-- centrally. Best-effort, exactly like `airings`: a failed insert is logged and the operation
-- continues, because recording that a title landed must never be able to stop it landing.
--
-- APPEND-ONLY, unlike `airings`. A feed IS its history — collapsing to one row per subject would
-- answer "what is the latest?" and destroy the only question this table exists for ("what has
-- been going on?"). That makes it the one table here that grows without bound, which is why it
-- ships with a purge job and a retention setting in the same PR (§18.1 `activity-purge`,
-- `activity.retention`).
--
-- `text` is COMPOSED AT THE WRITE POINT and stored, never templated at read time: a feed row is
-- a historical record, and re-rendering it later against current data would let last week's
-- entry silently change its wording when a channel is renamed.
--
-- Forward-only (§16).
CREATE TABLE activity (
    id         TEXT   NOT NULL PRIMARY KEY,
    at         BIGINT NOT NULL DEFAULT 0,        -- unix seconds
    kind       TEXT   NOT NULL DEFAULT '',       -- domain that wrote it: title|channel|proposal|filler|system
    level      TEXT   NOT NULL DEFAULT 'info',   -- info|warn|error — drives the UI dot; bounded, never free text
    text       TEXT   NOT NULL DEFAULT '',       -- the operator-facing line, composed at write time
    subject_id TEXT   NOT NULL DEFAULT ''        -- the channel/title/proposal it concerns, for a future deep-link
);

-- The ONLY read is "newest first, limit N" (§7 GET /v1/activity), and the purge deletes by age.
-- Both are served by one descending index on time.
CREATE INDEX idx_activity_at ON activity (at DESC);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
