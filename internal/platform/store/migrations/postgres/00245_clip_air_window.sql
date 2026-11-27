-- +goose Up
-- A clip's air window (#3384): the part of the file that airs, in milliseconds from its start.
-- Black that is also silent at either edge is dead air; the window leaves it out but for a short
-- pad, so a break never shows seconds of black between two ads. The file is not touched: playout
-- seeks to air_start_ms and the break books air_end_ms - air_start_ms.
--
-- air_end_ms = 0 means the window has not been measured yet, and the clip airs whole. A measured
-- clip with nothing to trim has the window [0, duration_ms].
ALTER TABLE clips ADD COLUMN air_start_ms BIGINT NOT NULL DEFAULT 0 CHECK (air_start_ms >= 0);
ALTER TABLE clips ADD COLUMN air_end_ms BIGINT NOT NULL DEFAULT 0 CHECK (air_end_ms >= 0);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
