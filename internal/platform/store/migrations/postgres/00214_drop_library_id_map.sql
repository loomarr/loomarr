-- +goose Up
-- #2151 step 16: the cutover moved every stored media-server id to its Library id on the release
-- before this one, so the id map it read from has no reader left.
DROP TABLE library_id_map;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
