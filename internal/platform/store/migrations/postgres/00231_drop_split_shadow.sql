-- +goose Up
-- #2147: the V43 auto-confirm gate is gone, so there is nothing left to compare the certified V67
-- gate against. Its comparison history goes with it, and so does a stored value for
-- `filler.autosplit.min_confidence`, the threshold only that gate read.
DROP TABLE filler_split_shadow_decisions;
DELETE FROM settings WHERE key = 'filler.autosplit.min_confidence';

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
