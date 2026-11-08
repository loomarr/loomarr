-- +goose Up
-- #2151 step 16: playout reads only the files Loomarr's own scan found, so nothing resolves an
-- item through the media server's file path any more, and its cache (00122, #1456) goes.
DROP TABLE library_item_paths;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
