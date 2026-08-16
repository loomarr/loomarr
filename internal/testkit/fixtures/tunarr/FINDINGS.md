# Tunarr contract spike — Phase 0 findings

**Captured:** 2026-07-13, against a local throwaway Tunarr instance.
**Version:** `chrisbenincasa/tunarr:latest` → `{"tunarr":"1.3.8","ffmpeg":"7.1.1","nodejs":"22.20.0"}`.
**Vendored spec:** `api/vendor/tunarr-openapi.json` (OpenAPI 3.0.3, 117 paths).
**Base URL used:** `http://localhost:8000`.

> **Re-verified 2026-07-23 against Tunarr `1.3.9`** (live homelab). The `POST /api/channels`
> create body Loomarr sends (`internal/programmer/tunarr.go` `channelBody` +
> `tunarrChannel`) matches 1.3.9's `required` channel set — `disableFillerOverlay`,
> `duration`, `groupTitle`, `guideMinimumDuration`, `icon`, `id`, `name`, `number`,
> `offline`, `startTime`, `stealth`, `streamMode`, `transcodeConfigId`, `subtitlesEnabled`
> — confirmed by a live `201` and a full channel create→reconcile→read-back. No payload
> change needed for 1.3.8 → 1.3.9. `/api/media-sources` (tunarr-connect) is likewise
> unchanged. `/api/version` and `/openapi.json` are the endpoints to re-run on the next bump.

## Settled: the §6 API-key question

**Tunarr 1.3.8 requires no API key.** The vendored spec declares
`components.securitySchemes = {}` and no global `security`. Unauthenticated GET *and*
POST/DELETE all succeed (channel create returned 201, delete 200). Record the tested
version in the README; re-verify if the target Tunarr version changes.

> **Update (2026-07):** `TUNARR_API_KEY` / `tunarr.api_key` was **removed entirely** — the <!-- retired-ok -->
> optional bearer field only confused operators (Tunarr has no login to get a key *from*), and
> Loomarr talks to Tunarr machine-to-machine on the same network. An operator fronting Tunarr with
> their own auth proxy terminates that at the proxy; Loomarr no longer models a Tunarr key.

## Contract surprises (must inform the Phase-10 Programmer adapter)

1. **Server assigns the channel `id`; the client-supplied `id` is ignored.**
   POST `/api/channels` with `{type:"new", channel:{id:"1111...", ...}}` returned 201 with
   a *different* server-generated `id` (`2540b613-…`). The adapter MUST read the id from the
   create response and use it for all subsequent GET/PUT/DELETE/programming calls. Assuming
   the requested id sticks produces 404s on every follow-up. (Fixture pair:
   `channel_create_response.json` shows the returned id ≠ requested id.)

2. **Create body is a `oneOf` discriminated envelope**, not a bare channel:
   - `{"type":"new","channel":{…}}` — full channel object
   - `{"type":"copy","channelId":"…"}` — clone an existing channel
   The `new` variant's `channel.required` is large (14 fields incl. `icon`, `offline`,
   `streamMode`, `transcodeConfigId`, `guideMinimumDuration`, `startTime`). A minimal valid
   body is captured in the fixtures.

3. **`transcodeConfigId` must reference a real config** (uuid) — the instance ships a
   `Default` config; the nil (`00000000-…`) and all-`f` UUIDs are also accepted by the
   pattern. Fetch valid ids from `GET /api/transcode_configs`.

4. **`GET /api/channels/{id}/lineup` 400s on a channel with no programming** — empty lineup
   is an error, not an empty-200. Program the channel first. The batch
   `GET /api/channels/all/lineups` needs `from`/`to` query params (returns 500 without,
   200 with a date range). `includePrograms` is an optional query param.

5. **`streamMode` enum:** `hls | hls_slower | mpegts | hls_direct | hls_direct_v2`.

## Endpoints the Programmer adapter (§9) will use

| Op | Path |
| --- | --- |
| List / create channels | `GET,POST /api/channels` |
| Get / update / delete channel | `GET,PUT,DELETE /api/channels/{id}` |
| Get / set programming (lineup) | `GET,POST /api/channels/{id}/programming` |
| Read lineup | `GET /api/channels/{id}/lineup?from&to&includePrograms` |
| Schedule slots | `POST /api/channels/{channelId}/schedule-slots`, `.../schedule-time-slots` |
| Filler lists | `GET,POST /api/filler-lists`, `GET,PUT,DELETE /api/filler-lists/{id}` |
| Filler-list programs | `GET /api/filler-lists/{id}/programs` |
| Program search / lookup | `POST /api/programs/search`, `POST /api/programming/batch/lookup` |
| Custom shows | `GET,POST /api/custom-shows`, `GET,PUT,DELETE /api/custom-shows/{id}` |
| Version | `GET /api/version` |

## Fixtures captured (this dir)

- `channel_create_response.json` — 201 body; **demonstrates server-assigned id**.
- `channel_get_response.json` — 200 read-back by the server-assigned id.
- `channel_lineup_empty.json` — the 400 `{"error":"Bad Request"}` for an unprogrammed channel.
- `filler_lists_response.json` — `[]` (empty filler-lists shape).

## Cleanup

The throwaway channel (name `loomarr-phase0-spike`, number 9901) was deleted (DELETE 200,
GET-after → 404). No residue left on the instance. Spike container: `tunarr-spike`
(remove with `docker rm -f tunarr-spike && docker volume rm tunarr-spike-data`).

## Not deviations from the design doc

None of the above contradicts §6/§9 — they are *details under-specified by the doc* that
the doc explicitly deferred to "verify during phase 10." No `loomarr-design.md` edit
required. The server-assigned-id behavior (finding 1) should be called out in the Phase-10
Programmer adapter code + §9 when we get there.
