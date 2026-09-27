-- +goose Up
-- The measured active picture of a source (#1512 phase 1d): the part of the coded frame that is
-- picture, letterbox and pillarbox bars excluded, as JSON {x,y,w,h} in source pixels. The channel
-- watermark anchors to its corner. NULL is unknown (no video, every sampled frame black, or a row
-- measured before this column; those are re-measured by schema version).
ALTER TABLE inventory_source_analysis ADD COLUMN active_picture_json TEXT;

-- +goose Down
ALTER TABLE inventory_source_analysis DROP COLUMN active_picture_json;
