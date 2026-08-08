# Media Server Live TV Integration (Emby / Jellyfin)

> **Intended repo location:** `docs/integrations/media-server-livetv.md`
> This is a summary/briefing. The design doc is authoritative — see §6 (Live TV wiring contract), §9 (guide freshness), §13 (wizard step 4), §21 Phase 0 (payload pinning).

## The one-sentence model

Loomarr channels reach Emby/Jellyfin through **one-time wiring of Tunarr as a tuner + guide source — never per-channel registration.** Once wired, every channel Loomarr creates, renames, or deletes propagates automatically through Tunarr's tuner surface; Loomarr just pokes the guide refresh so changes show up in minutes instead of after the nightly refresh.

## Division of labor (why we build none of the middle)

| Layer | Owns |
| --- | --- |
| **Loomarr** | *What plays and when:* intent → proposal → acquisition → lineup + commercial pods → pushed to Tunarr |
| **Tunarr** | *Making it watchable:* playout math, join-in-progress, ffmpeg transcode pipeline (HW accel), program-boundary stitching, HDHR/M3U/XMLTV tuner surface, EPG generation |
| **Emby/Jellyfin** | *The viewer surface:* consumes the tuner + guide like any HDHomeRun; also the library + auth source Loomarr already depends on |

The middle row is years of accumulated media-pipeline scar tissue (see design doc §1 non-goals). Loomarr's escape hatch if Tunarr ever stalls is a second `Programmer` adapter (ErsatzTV), never building streaming itself.

## How the wiring works

- Both flavors share the **write** endpoints (Emby lineage): **`POST /LiveTv/TunerHosts`** (type `m3u`, `Url` = Tunarr's playlist URL) and **`POST /LiveTv/ListingProviders`** (type `xmltv`, `Path` = Tunarr's guide URL — `Path`, not `Url`; Phase-10 finding 1). Loomarr's existing admin `LIBRARY_TOKEN` has sufficient privilege.
- They do **not** share the **read** side. `GET /LiveTv/TunerHosts` and `GET /LiveTv/ListingProviders` are Emby-only — Jellyfin 10.10.3 returns **405**, making those paths write-only there. Enumeration therefore goes through **`GET /System/Configuration/livetv`** (`{TunerHosts, ListingProviders}`), which answers 200 on both. See `internal/testkit/fixtures/livetv/FINDINGS.md`; this corrects the original claim that the admin endpoints were shared wholesale.
- **M3U is preferred over HDHomeRun emulation for API wiring** — explicit and discovery-free, so the registration is deterministic.
- It is **one-time**. There is no per-channel API call to the media server, ever.

## Guide freshness

Both servers refresh guide data on a schedule (nightly by default). After any reconcile that creates/renames/deletes channels, the scheduler triggers the media server's **guide-refresh scheduled task** (best-effort). This is the difference between "live in Tunarr" and "visible in the family's guide right now."

## Rules

1. **Never silent.** Wiring follows from an explicit operator action — saving the Tunarr connection. It is idempotent and fully derived from that connection, so it auto-runs on save rather than needing its own button, and the `livetv` setup check reports the result. Loomarr does not reconfigure someone's media server unasked: saving the connection *is* the ask. `POST /v1/setup/livetv-reconnect` (admin) force re-wires when a stale channel→stream binding needs clearing.
2. **Idempotent.** Enumerate existing tuners/providers first; if Tunarr is already registered, the call is a no-op. Duplicate tuners are a classic Emby mess — tests assert second-call-no-op.
3. **Checklist-detected.** `GET /v1/setup/status` includes a "media server has Tunarr wired as tuner + guide" check; a red check surfaces the connect button.

## Version fragility → Phase 0

The endpoints exist across both flavors, but **payload fields and the guide-refresh task id drift across versions.** Phase 0 captures the exact accepted request/response payloads and the task id from the maintainer's real Emby and Jellyfin into `internal/testkit/fixtures/`; the adapter is written against those pins, not memory. Any contract deviation ⇒ update the design doc before implementing.

## Troubleshooting

These are also surfaced in the in-app Help center (Troubleshooting → LiveTV).

- **Channel exists in Tunarr but not in the Emby/Jellyfin guide** → wiring missing (run the connect) or guide refresh pending (poke/refresh manually).
- **Duplicate channels in the guide** → duplicate tuner registration from manual + API wiring; remove extras, rely on the idempotent connect thereafter.
- **Channels visible but won't play** → Tunarr ↔ library media-source mismatch (the *other* checklist item, design doc §6) — not a wiring problem.
- **Live TV playback stops after ~4 seconds** → a **Firefox** client-side playback quirk, not a Loomarr/Tunarr/Emby backend fault (the backend stream is healthy). Play the channel in a different client (Chrome-based browser, the Emby/Jellyfin native app, or a set-top client). Confirmed on the maintainer's stack 2026-07-14: the stall was Firefox-specific and disappeared on another client. Not a Simkl-plugin issue (earlier suspicion — ruled out).
