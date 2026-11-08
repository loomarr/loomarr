-- +goose Up
-- V66 (§10): retire every compatibility publisher before terminal admission becomes the only
-- authority. Acquisition history, a hand copy, a high tag score, and an old `filed` pipeline row
-- do not reproduce current safety, playback-integrity, audience, and rights evidence.
--
-- ⚠ This deliberately removes existing non-composite filler from rotation on upgrade. Continuity
-- is not evidence: keeping unexplained clips playable would preserve exactly the bypass this
-- migration closes. Removed rows stay removed, and composite containers keep their separate
-- split-repair lifecycle because they are excluded from scheduling independently.
UPDATE clips
SET held = TRUE,
    auto_filed = FALSE,
    updated_at = CAST(strftime('%s', 'now') AS INTEGER)
WHERE removed_at = 0
  AND is_composite = FALSE;

-- The column remains physical migration history until a future table rebuild, but it no longer
-- carries application meaning anywhere — clear even removed/composite rows so no later repair can
-- accidentally resurrect the old audit lifecycle from stale data.
UPDATE clips SET auto_filed = FALSE WHERE auto_filed = TRUE;

-- Keep the durable conveyor projection coherent with the catalog gate. A legacy `filed` row now
-- means “needs terminal review”; no stage record is fabricated or erased.
UPDATE filler_clip_pipeline
SET disposition = 'review',
    status = 'done',
    progress = 100,
    updated_at = CAST(strftime('%s', 'now') AS INTEGER)
WHERE disposition = 'filed'
  AND clip_hash IN (
    SELECT hash FROM clips
    WHERE held = TRUE AND removed_at = 0 AND is_composite = FALSE
  );

-- Forward-only (§16). Releasing these clips without terminal evidence would restore the defect.

-- +goose Down
SELECT 1;
