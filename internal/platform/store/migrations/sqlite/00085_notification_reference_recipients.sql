-- +goose Up
-- Approval-owned requester provenance is the durable backstop for later Title and Channel events.
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
