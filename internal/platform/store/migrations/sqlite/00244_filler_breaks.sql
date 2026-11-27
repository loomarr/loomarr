-- +goose Up
-- #3838 step 2: each break is assembled once and stored. Reconcile plans a channel's breaks ahead,
-- in order, and cuts every break slot to the length of the pod stored for it, so the programme
-- resumes when the break's own clips end. The guide, the previews and the stream read the stored
-- pod, so what the guide shows is what airs.
--
-- One row per break, keyed by its channel and its start in Unix milliseconds. `ceiling_ms` is the
-- slot length the pod was assembled to, and `length_ms` is the media it holds (the slot's length
-- once planned). `pod_json` is the pod's entries. A break whose start moves has no row at its new
-- start and is assembled fresh.
CREATE TABLE filler_breaks (
    channel_id  TEXT    NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    start_ms   INTEGER NOT NULL,
    ceiling_ms  INTEGER NOT NULL CHECK (ceiling_ms > 0),
    length_ms   INTEGER NOT NULL CHECK (length_ms >= 0),
    pod_json    TEXT    NOT NULL CHECK (json_valid(pod_json)),
    planned_at  INTEGER NOT NULL,
    PRIMARY KEY (channel_id, start_ms)
);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
