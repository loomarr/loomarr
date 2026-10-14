-- +goose Up
-- PostgreSQL mirror of approval-owned requester provenance (§11).
CREATE TABLE notification_reference_recipients (
    reference_kind  TEXT NOT NULL CHECK (reference_kind IN ('title', 'channel')),
    reference_id    TEXT NOT NULL,
    person_id       TEXT NOT NULL,
    created_at      BIGINT NOT NULL,
    PRIMARY KEY (reference_kind, reference_id, person_id)
);

CREATE INDEX idx_notification_reference_recipients_person
    ON notification_reference_recipients (person_id, reference_kind, reference_id);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
