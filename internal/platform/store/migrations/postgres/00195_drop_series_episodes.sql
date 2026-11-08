-- +goose Up
-- #2151 step 16: a lineup's series expands from the Library's own tables, so the media server's
-- episode list cache (00018) has nothing left to save and goes.
DROP TABLE series_episodes;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
