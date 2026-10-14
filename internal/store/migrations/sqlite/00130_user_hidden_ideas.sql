-- +goose Up
-- The channel ideas each person hid on Home (#1665). Ideas are rebuilt from the library on every
-- read and never stored, so a hide keys on the idea's stable id ("genre:comedy"). Hides go with
-- their person.
CREATE TABLE user_hidden_ideas (
    user_id   TEXT NOT NULL,
    idea_id   TEXT NOT NULL,
    hidden_at BIGINT NOT NULL CHECK (hidden_at > 0),
    PRIMARY KEY (user_id, idea_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- +goose Down
SELECT 1;
