-- +goose Up
-- #2151: a movie can be split over several files ("cd1", "cd2"). `part` is which part a file is,
-- from 1, and 0 for a whole file; every part of the edition that plays is marked `plays`, and they
-- play in part order.
ALTER TABLE library_sources ADD COLUMN part BIGINT NOT NULL DEFAULT 0 CHECK (part >= 0);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
