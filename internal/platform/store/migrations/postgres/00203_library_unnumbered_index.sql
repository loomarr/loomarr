-- +goose Up
-- #2151 decision 53: Needs you, and its badge on every status read, find the shows whose episodes
-- wait on their episode order by the episodes marked unnumbered. Few or none are, so a partial
-- index on them makes a library where nothing waits read nothing.
CREATE INDEX idx_library_items_unnumbered ON library_items (series_id) WHERE unnumbered;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
