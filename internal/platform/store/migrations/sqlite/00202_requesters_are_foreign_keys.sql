-- +goose Up
-- Schema to a 10 (#2793, item 1): every reference is a foreign key. notification_reference_recipients
-- named what was asked for as (reference_kind, reference_id) text, so a nightly sweep had to find
-- requesters whose title, channel or person had gone. Approval writes the title rows and the
-- channel in the same transaction as their requesters, so each half can be a real foreign key
-- that cascades, and the sweep goes.
--
-- Only rows whose title or channel and person still exist are copied.
CREATE TABLE title_requesters (
    title_key  TEXT NOT NULL REFERENCES titles(key) ON DELETE CASCADE,
    person_id  TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at BIGINT NOT NULL,
    PRIMARY KEY (title_key, person_id)
);
CREATE INDEX idx_title_requesters_person ON title_requesters (person_id);

CREATE TABLE channel_requesters (
    channel_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    person_id  TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at BIGINT NOT NULL,
    PRIMARY KEY (channel_id, person_id)
);
CREATE INDEX idx_channel_requesters_person ON channel_requesters (person_id);

INSERT INTO title_requesters (title_key, person_id, created_at)
SELECT r.reference_id, r.person_id, r.created_at FROM notification_reference_recipients r
WHERE r.reference_kind = 'title'
  AND EXISTS (SELECT 1 FROM titles t WHERE t.key = r.reference_id)
  AND EXISTS (SELECT 1 FROM users u WHERE u.id = r.person_id);
INSERT INTO channel_requesters (channel_id, person_id, created_at)
SELECT r.reference_id, r.person_id, r.created_at FROM notification_reference_recipients r
WHERE r.reference_kind = 'channel'
  AND EXISTS (SELECT 1 FROM channels c WHERE c.id = r.reference_id)
  AND EXISTS (SELECT 1 FROM users u WHERE u.id = r.person_id);

DROP TABLE notification_reference_recipients;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
