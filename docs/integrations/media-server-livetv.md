# Media Server Live TV Integration (Emby / Jellyfin)

> **Intended repo location:** `docs/integrations/media-server-livetv.md`
> This is a summary/briefing. The design doc is authoritative — see §6 (Live TV wiring contract), §9 (guide freshness), §13 (wizard step 4), §21 Phase 0 (payload pinning).

## The one-sentence model

Loomarr channels reach Emby/Jellyfin through **one owned tuner + guide pair for the durably applied playout backend — never per-channel registration.** The pair points at Loomarr for internal playout or Tunarr for Tunarr playout, and backend cutovers publish it through one durable, cross-replica workflow.

## Division of labor

| Layer | Owns |
| --- | --- |
| **Loomarr** | *Always:* what plays and when. *Internal backend:* streaming/transcode plus M3U/XMLTV publication. *Tunarr backend:* projects the schedule to Tunarr. |
| **Tunarr** | On the Tunarr backend, owns streaming/transcode and M3U/XMLTV publication. It is not required for internal playout. |
| **Emby/Jellyfin** | *The viewer surface:* consumes the tuner + guide like any HDHomeRun; also the library + auth source Loomarr already depends on |

## How the wiring works

- Both flavors share the **write** endpoints (Emby lineage): **`POST /LiveTv/TunerHosts`** (type `m3u`, `Url` = the applied backend's playlist URL) and **`POST /LiveTv/ListingProviders`** (type `xmltv`, `Path` = its guide URL — `Path`, not `Url`; Phase-10 finding 1). Loomarr's existing admin `LIBRARY_TOKEN` has sufficient privilege.
- They do **not** share the **read** side. `GET /LiveTv/TunerHosts` and `GET /LiveTv/ListingProviders` are Emby-only — Jellyfin 10.10.3 returns **405**, making those paths write-only there. Enumeration therefore goes through **`GET /System/Configuration/livetv`** (`{TunerHosts, ListingProviders}`), which answers 200 on both. See `internal/testkit/fixtures/livetv/FINDINGS.md`; this corrects the original claim that the admin endpoints were shared wholesale.
- **M3U is preferred over HDHomeRun emulation for API wiring** — explicit and discovery-free, so the registration is deterministic.
- It is **one-time**. There is no per-channel API call to the media server, ever.

## Guide freshness

Both servers refresh guide data on a schedule (nightly by default). A reconcile that adds or removes a channel triggers a best-effort **tuner re-scan** so the media server re-reads the M3U; an existing channel's lineup change triggers a best-effort **guide refresh** for its EPG.

## Rules

1. **Never silent.** Wiring is idempotent and fully derived from relevant backend, URL, token, and media-server settings, so it runs through the durable transition coordinator when those settings are saved rather than needing its own button. `POST /v1/setup/livetv-reconnect` (admin) force-repairs the durably applied target under the same lock when a stale channel→stream binding needs clearing.
2. **Idempotent.** Enumerate existing tuners/providers first; if the applied target is already registered, ordinary publication is a no-op. Duplicate tuners are a classic Emby mess — tests assert second-call-no-op.
3. **Checklist-detected.** `GET /v1/setup/status` reports whether the applied backend's tuner + guide pair is registered and links a red result to troubleshooting; there is no manual connect button.

## Version fragility → Phase 0

The endpoints exist across both flavors, but **payload fields and the guide-refresh task id drift across versions.** Phase 0 captures the exact accepted request/response payloads and the task id from the maintainer's real Emby and Jellyfin into `internal/testkit/fixtures/`; the adapter is written against those pins, not memory. Any contract deviation ⇒ update the design doc before implementing.

## Troubleshooting

These are also surfaced in the in-app Help center (Troubleshooting → LiveTV).

- **Channel exists in the applied backend but not in the Emby/Jellyfin guide** → publication is incomplete or a tuner re-scan/guide refresh is pending; re-save the relevant connection setting or run the reconnect repair.
- **Duplicate channels in the guide** → duplicate tuner registration from manual + API wiring; remove extras, rely on the idempotent connect thereafter.
- **Channels visible but won't play** → run the reconnect repair first; on the Tunarr backend, also check Tunarr's library media-source status.
- **Live TV playback stops after ~4 seconds** → a **Firefox** client-side playback quirk, not a Loomarr/Tunarr/Emby backend fault (the backend stream is healthy). Play the channel in a different client (Chrome-based browser, the Emby/Jellyfin native app, or a set-top client). Confirmed on the maintainer's stack 2026-07-14: the stall was Firefox-specific and disappeared on another client. Not a Simkl-plugin issue (earlier suspicion — ruled out).
