-- +goose Up
-- A channel owns the request it was made from. Until now its intent lived only on the job its
-- `intent_ref` names, and housekeeping purges finished jobs, so a channel older than
-- `jobs.retention` lost the intent a refine or a weekly re-curation needs and the requester its
-- name. `intent_json` is the approved intent (the request without any one-run refinement);
-- `requested_by` is the person who made the request, set when the channel is created.
--
-- A refine or re-curation submits a NEW job seeded from `intent_json`; `jobs.channel_id` names the
-- channel such a job re-curates. A job that originates a channel leaves it empty and is bound by
-- the channel's `intent_ref`.
ALTER TABLE channels ADD COLUMN intent_json TEXT NOT NULL DEFAULT '';
ALTER TABLE channels ADD COLUMN requested_by TEXT NOT NULL DEFAULT '';
ALTER TABLE jobs ADD COLUMN channel_id TEXT NOT NULL DEFAULT '';

-- One-time copy from the job each channel still points at. A channel whose job was already purged
-- keeps empty values: its original request is gone.
UPDATE channels
   SET intent_json  = COALESCE((SELECT j.intent_json FROM jobs j WHERE j.id = channels.intent_ref), ''),
       requested_by = COALESCE((SELECT j.created_by FROM jobs j WHERE j.id = channels.intent_ref), '')
 WHERE intent_ref <> '';

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
