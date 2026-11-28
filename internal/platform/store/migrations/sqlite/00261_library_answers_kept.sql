-- +goose Up
-- #4476: a Needs you answer can keep a title as a home video. An answer is exactly one of a link
-- (external_id), ignored or kept.
ALTER TABLE library_answers RENAME TO library_answers_old;
CREATE TABLE library_answers (
    origin_authority TEXT    NOT NULL,
    origin_key       TEXT    NOT NULL,
    kind             TEXT    NOT NULL CHECK (kind IN ('movie', 'series')),
    external_id      TEXT    NOT NULL DEFAULT '',
    ignored          BOOLEAN NOT NULL DEFAULT FALSE CHECK (ignored IN (0, 1)),
    answered_at_ms   INTEGER NOT NULL,
    answered_by      TEXT    NOT NULL DEFAULT 'you',
    namespace        TEXT    NOT NULL DEFAULT '',
    kept             BOOLEAN NOT NULL DEFAULT FALSE CHECK (kept IN (0, 1)),
    PRIMARY KEY (origin_authority, origin_key),
    CONSTRAINT library_answers_one_answer_check
        CHECK ((external_id <> '') + (ignored <> 0) + (kept <> 0) = 1)
);
INSERT INTO library_answers (origin_authority, origin_key, kind, external_id, ignored, answered_at_ms,
    answered_by, namespace)
SELECT origin_authority, origin_key, kind, external_id, ignored, answered_at_ms, answered_by, namespace
  FROM library_answers_old;
DROP TABLE library_answers_old;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
