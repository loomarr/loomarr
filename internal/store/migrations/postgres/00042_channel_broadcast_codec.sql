-- +goose Up
-- V50 (§9.1): the channel's BROADCAST CODEC follows its CONTENT, not the client.
-- A channel whose library is majority-HEVC broadcasts HEVC end-to-end (making fMP4
-- legal — one uniform codec, no mid-stream switch); an h264-majority channel stays
-- h264/TS. Minority-codec titles and all filler normalize (transcode) to this codec.
-- The client DeviceProfile is a SEPARATE axis: it gates copy-native-vs-down-convert
-- per client, it does NOT pick the channel's codec.
--
-- Set at CURATION time (when the lineup is written) from the majority of the titles'
-- probed codecs (V50 Q1: STORED column, not runtime-probed; Q2: MAJORITY WINS). The
-- resolver reads this column on the session hot path — no per-start library round-trip.
--
-- Forward-only (§16). Default 'h264' means every existing (un-backfilled) channel keeps
-- today's TS/h264 behaviour verbatim: a channel only becomes HEVC once curation next
-- recomputes it. So this DDL changes nothing observable on its own — no data migration.
ALTER TABLE channels ADD COLUMN broadcast_codec TEXT NOT NULL DEFAULT 'h264';

-- +goose Down
ALTER TABLE channels DROP COLUMN broadcast_codec;
