-- +goose Up
-- #4476: a Needs you answer can keep a title as a home video. An answer is exactly one of a link
-- (external_id), ignored or kept.
ALTER TABLE library_answers ADD COLUMN kept BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE library_answers ADD CONSTRAINT library_answers_one_answer_check
    CHECK ((external_id <> '')::int + ignored::int + kept::int = 1);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
