# Tunarr guide (now/next) — contract capture

**Captured:** 2026-07-19, local dev Tunarr (`docker/compose.dev.yaml`).
**Version:** `chrisbenincasa/tunarr:1.3.8` → `{"tunarr":"1.3.8","ffmpeg":"7.1.1","nodejs":"22.20.0"}`.
**Source of truth:** `types/src/schemas/guideApiSchemas.ts` @ tag `v1.3.8` — the vendored
OpenAPI (`api/vendor/tunarr-openapi.json`) does NOT type the guide response (no
guide/TvGuide component schemas), so the shape comes from Tunarr's own zod schemas,
confirmed against a live capture.
**Fixture:** `guide_channels_response.json`.

## Use the PLURAL endpoint

`GET /api/guide/channels?dateFrom=&dateTo=` → `Record<channelId, ChannelLineup>`

```
ChannelLineup { icon?, id, name, number, programs: TvGuideProgram[] }
TvGuideProgram = discriminatedUnion('type') of content | custom | redirect | flex
  common: { start, stop }   // epoch ms — REAL airtimes
  content: { duration, id, program: { uuid, sourceType, type, title, identifiers[] } }
  flex:    { title }        // flex carries its own title
```

`identifiers[]` includes `tmdb`, so a guide entry maps onto Loomarr's provisioning key
(`movie:tmdb:14412`) without a second lookup.

## Two traps this capture closes

1. **The singular endpoint is a different shape.** `GET /api/guide/channels/{id}` returns
   `[{index, lineupItem:{type,id,durationMs}, startTimeMs}]` — no `start`/`stop`, and **no
   title**. Building against it would have produced a title-less strip plus an unnecessary
   second lookup to resolve names. Use the plural one.
2. **One call covers every channel.** The Channels list needs now/next per card; the plural
   endpoint is keyed by channel id, so the list costs ONE Tunarr call, not N.

## Still to decide when implementing

- Cache/TTL: `GET /api/guide/status` returns per-channel `lastUpdate` + `guideTimes`
  (start/end of the generated window) — cheap staleness signal.
- `flex` entries are gaps (commercial pods / dead air) and carry their own `title`; the UI
  should render them as a gap, not as a program (NowNextStrip already has a `gap` flag).
