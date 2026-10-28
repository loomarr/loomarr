-- +goose Up
-- The broadcast station directory (#2060): every over-the-air TV station a filler clip can name, and
-- the dated call signs it held, so "WEWS NewsChannel 5 1991" resolves to a station in Cleveland. Rows
-- are SEEDED AT BOOT from broadcaststation.SeedRevisions(); this migration creates the empty tables.
-- Unlike taxa and holidays the directory is shipped data, not operator data: applying a newer
-- revision replaces every row.
--
-- latitude and longitude are the community of license, the point a household's radius is measured
-- from. from_date and to_date are YYYY-MM-DD; '' is an open bound and to_date is exclusive.
CREATE TABLE broadcast_stations (
  id        TEXT PRIMARY KEY,
  country   TEXT NOT NULL,
  city      TEXT NOT NULL DEFAULT '',
  region    TEXT NOT NULL DEFAULT '',
  latitude  DOUBLE PRECISION NOT NULL,
  longitude DOUBLE PRECISION NOT NULL
);

CREATE TABLE broadcast_station_call_signs (
  station_id TEXT NOT NULL REFERENCES broadcast_stations(id) ON DELETE CASCADE,
  call_sign  TEXT NOT NULL,
  from_date  TEXT NOT NULL DEFAULT '',
  to_date    TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (station_id, call_sign, from_date)
);

CREATE INDEX broadcast_station_call_signs_by_sign ON broadcast_station_call_signs (call_sign);

CREATE TABLE broadcast_station_seed_revisions (
  version    INTEGER PRIMARY KEY,
  applied_at BIGINT NOT NULL
);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
