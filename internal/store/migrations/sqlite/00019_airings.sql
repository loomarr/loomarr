-- +goose Up
-- Airing history — what this channel actually broadcast, and when (§5, programming-design §3.1).
--
-- WHY. The scheduler had no memory of its own output. Separation (programming-design §3) is a
-- WITHIN-CYCLE rule: it bounds what recurs inside one pass of the deck, and when the deck wraps
-- the count resets and playback replays from position. So a title returns on a positional clock
-- rather than a programmed one. Reported from the dev "1980s Action Heroes": Akira aired Tue
-- 21:53, Fri 13:33, Sat 02:10 and Mon 01:30 — four times in a week, at no interval anyone chose.
--
-- This is the programme analogue of `clips.play_count` / `last_played_at` (§10 V28), which has
-- always done exactly this for commercial clips. Same write point, same best-effort posture, and
-- for the same reason: you cannot rotate what you cannot remember.
--
-- LOOMARR'S OWN OUTPUT, not viewer behaviour. Deliberately NOT the media server's per-user watch
-- state (`UserData`): that is a different signal carrying a "whose history counts?" question and
-- privacy implications for a shared channel. This table only records what Loomarr broadcast.
--
-- ONE ROW PER (channel, key) — the LAST airing, upserted — not an append-only log. The only
-- reader is "when did this last air here?", so a log would accumulate a row per programme per
-- play forever to answer a question about its own maximum. A log would also need the janitor to
-- keep it from becoming the largest table in the install; this cannot outgrow
-- channels x lineup-size.
--
-- Scoped BY CHANNEL: the same film on two channels is two independent rotations, and collapsing
-- them would let one channel's schedule suppress another's.
--
-- Forward-only (§16).
CREATE TABLE airings (
    channel_id      TEXT   NOT NULL,
    key             TEXT   NOT NULL,          -- provision.Key of the programme that aired
    library_item_id TEXT   NOT NULL DEFAULT '', -- what actually streamed (diagnostics)
    aired_at        BIGINT NOT NULL DEFAULT 0,  -- unix seconds of the most recent airing
    PRIMARY KEY (channel_id, key)
);

-- Placement reads a whole channel's history at once ("last-aired for every key on this
-- channel"), so the index matches that access pattern rather than a per-key lookup.
CREATE INDEX idx_airings_channel ON airings (channel_id, aired_at);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
