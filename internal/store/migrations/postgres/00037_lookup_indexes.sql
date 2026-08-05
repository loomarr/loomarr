-- +goose Up
-- V41: two lookup indexes for the "find the row by something other than its id" queries.
--
-- ⚠ Both back scans that were previously done in Go over a full table read. `channels.intent_ref`
-- answers "which channel is bound to this suggestion job?" — asked once per bind (binder) and
-- once per auto-curate consideration (recurate), each of which was `ListChannels` plus a linear
-- walk. `proposals.job_id` answers "the newest approved proposal for this job", which ran over
-- EVERY approved proposal in the install.
--
-- ⚠ The proposals one matters more than its size suggests. Retention deliberately never purges
-- approved proposals — they are the audit trail (`internal/retention`) — so that table grows
-- monotonically for the life of the install while the denied ones are swept. Measured on sqlite:
-- 0.38ms at 100 rows, 3.45ms at 1k, 19.4ms at 5k, linear. Household-scale today; the index is
-- cheap now and awkward to add once someone notices.
--
-- ⚠ Non-unique on purpose. A job legitimately has SEVERAL approved proposals over its life: a
-- refine re-runs the channel's own job and the newest approved one wins (see
-- `binder.ApprovedProposalForJob`, and TestRefine_NewestApprovedWins). A UNIQUE index here would
-- reject the second refine of any channel.
CREATE INDEX idx_channels_intent_ref ON channels (intent_ref);
CREATE INDEX idx_proposals_job_id ON proposals (job_id);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
