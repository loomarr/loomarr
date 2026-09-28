# Live repros on the demo library

Isolated lane backend (its own port and SQLite database), demo library from `make demo-library` +
`make demo-seed`, background media jobs paused. All titles are the demo library's invented ones.
Times are UTC. Base commit `33b35dbe`.

## A. One tune-in plus one reconcile rewrites the programme on air

Channel 101 (shuffle, 36 episodes, pool fits inside the 24h window). Guide for the next three
hours, programmes only (`GET /v1/guide`), then ~24 s of HLS playlist reads through
`POST /v1/channels/{id}/play-url` (under the GPU lock), then `POST /v1/channels/{id}/reconcile`,
then the same guide read.

```
BEFORE 02:45:15 Robo-Rabbits of Zone Nine e4 | 02:56:15 The Marvelous Mudpuddles e6 | 03:07:15 Quasar Quartet e1
       03:18:15 Robo-Rabbits of Zone Nine e5 | 03:29:15 Quasar Quartet e3            | 03:40:15 Snorkelsaurus e1
AFTER  02:45:15 The Marvelous Mudpuddles e6  | 02:56:15 Quasar Quartet e1            | 03:07:15 Quasar Quartet e3
       03:18:15 Snorkelsaurus e1             | 03:29:15 The Marvelous Mudpuddles e1  | 03:40:15 Moth & Moonbeam e5
```

The block on air (started 02:45:15, the one being watched) changed identity at the same start
time, and the watched series left the next three hours entirely. Mechanism: playout recorded an
airing keyed by the SERIES (`RecordAiring`), reconcile recomputed `Desired` with that history,
`byRecency` moved every episode of that series to the back of the deck, and the channel's playout
anchor is fixed, so the same wall-clock instant now maps to a different programme.

Not verified here: whether the running HLS encoder switches mid-file or at its next resolve. The
guide and `Desired` changed; the report treats the viewer-facing stream behaviour as open.

## B. A daily cut mid-programme at the rolling-window boundary (00:00 UTC)

Channel 190, made through the channels API from the union of all six demo lineups (32 titles,
117 episodes/films, ~41 h, larger than the 24 h window). Guide around the next 00:00 UTC:

```
22:59:55 → 23:21:55  22.0 of 22 min  The Lantern Street Files e5
23:21:55 → 23:43:55  22.0 of 22 min  Cold Relay e3
23:43:55 → 00:00:00  16.1 of 22 min  The Lantern Street Files e2   ← cut
00:00:00 → 00:13:55  13.9 of 22 min  Dust Road Marshal e2          ← joined 8 min in
00:13:55 → 00:35:55  22.0 of 22 min  The Wendelmeyer Family Hour e3
```

`windowIndex` is `unix / window`, so a 24 h window turns over at 00:00 UTC (20:00 US Eastern
daylight time, primetime). The new slice is walked from the channel's fixed playout anchor, so it
is entered mid-programme, and `segmentedBroadcasts` clips both sides at the boundary. Every
channel whose pool is longer than its window does this once per window.

## C. Every channel has shuffle seed 0

`GET /v1/channels/{id}/cycle` → `trace.seed` is `"0"` on all seven channels. In product code only
`cmd/seed` ever sets `Channel.Shuffle.Seed`; channels created through the binder keep the zero
value, so every shuffle/syndication channel uses `rand.NewSource(0)`.
