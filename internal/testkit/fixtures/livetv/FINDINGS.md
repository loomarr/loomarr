# Live TV wiring contract spike — Phase 10 findings

**Captured:** 2026-07-13, maintainer-supervised, against the real Emby via Tailscale.
**Version:** Emby **4.10.0.17** ("Fictional Emby"), reached via Tailscale direct (see the
`loomarr-homelab-access` memory for addressing).
**Method:** reversible register→capture→delete (Phase-0 style). A throwaway `loomarr-phase10-capture`
m3u tuner + xmltv provider (URLs → the dev Tunarr instance, reachable from Emby over Tailscale) were registered,
their accepted payloads captured, then DELETEd. **Emby verified reverted** to its original state
(only the pre-existing HDHomeRun tuner + `embygn` "Emby Guide Data" provider remain). No residue.

## Endpoints + accepted payloads (pinned)

| Op | Method + Path | Body / params | Result |
| --- | --- | --- | --- |
| List tuners | `GET /LiveTv/TunerHosts` | — | `[]tunerHost` (see `tuner_hosts_list.json`) |
| Add m3u tuner | `POST /LiveTv/TunerHosts` | `{"Type":"m3u","Url":"<m3u>","FriendlyName":"loomarr"}` | **200**, returns the created host incl. server-assigned `Id` (`tuner_add_{request,response}.json`) |
| Delete tuner | `DELETE /LiveTv/TunerHosts?Id=<id>` | — | **204** |
| List providers | `GET /LiveTv/ListingProviders` | — | `[]listingProvider` (`listing_providers_list.json`) |
| Add xmltv provider | `POST /LiveTv/ListingProviders` | `{"Type":"xmltv","Path":"<xmltv>"}` | **200**, returns the created provider incl. `Id` (`listing_add_{request,response}.json`) |
| Delete provider | `DELETE /LiveTv/ListingProviders?Id=<id>` | — | **204** |
| List tasks | `GET /ScheduledTasks` | — | find the task with `Key=="RefreshGuide"` |
| Run guide refresh | `POST /ScheduledTasks/Running/<id>` | — | **204** |

## Contract findings (informed the adapter)

1. **The xmltv provider URL field is `Path`** (not `Url`). Confirmed accepted (200). The adapter's
   `listingProvider{Type,Path}` shape was already correct.
2. **The m3u tuner URL field is `Url`.** Correct as written.
   - **`FriendlyName` is REQUIRED on the add** (the capture pins `"loomarr"`). Emby 4.10 **404s**
     the add when it's omitted — a distinct failure from the fetch-validation 500 in #3. The
     adapter's `tunerHost` struct originally modeled only `{Type, Url}` ("only the fields the
     idempotency check needs"), which drifted from THIS captured payload and made every real
     `AddTuner` 404. Re-added `FriendlyName` + a regression test (2026-07). Lesson: the struct is a
     lossy remembering; `tuner_add_request.json` is the truth — model every field it carries.
3. **Emby validates the M3U tuner by FETCHING the playlist synchronously at registration.** An
   unreachable URL → **HTTP 500, no row created** (verified: `localhost:8000` — Emby's own host —
   500s cleanly). So a real connect requires a Tunarr URL reachable from the media-server's network.
   This is why the §21 DoD splits: automated gate runs vs the mock; the real connect is the manual
   smoke on the maintainer's co-networked stack.
4. **Guide-refresh task id is per-install and version-fragile** (`9492d30c70f7f1bec3757c9d0a4feb45`
   here), BUT its **`Key` is the stable `"RefreshGuide"`**. The run endpoint takes the **Id** in the
   path (`POST /ScheduledTasks/Running/<id>` → 204); the Key form **404s** (`/Running/RefreshGuide`).
   ⇒ The adapter resolves the id at runtime: `GET /ScheduledTasks` → find `Key=="RefreshGuide"` →
   POST its Id. This survives the id differing per install / across versions without a hardcoded id.
5. **Delete takes `?Id=` as a query param** (not a path segment), returns 204.

## Not deviations from the design doc

§6 already declared these payloads + the guide-refresh task id "version-fragile → pin via capture."
The findings *fill in* the under-specified details (xmltv uses `Path`; refresh is a Key→Id resolve;
delete is `?Id=`; M3U registration is fetch-validating). None contradict §6/§9. The design doc's
`docs-livetv-integration.md` seed says "M3U preferred, one-time, idempotent" — all confirmed.

## Fixtures (this dir)

- `tuner_hosts_list.json` / `listing_providers_list.json` — original enumerate shapes (pre-wiring).
- `tuner_add_request.json` / `tuner_add_response.json` — accepted m3u tuner add (scrubbed: URL →
  `http://TUNARR_HOST:8000/...`, Id → `<server-assigned>`).
- `listing_add_request.json` / `listing_add_response.json` — accepted xmltv provider add (scrubbed).
- `guide_refresh_task.json` — the `RefreshGuide` scheduled task (Id + Key + State).

---

# Jellyfin capture — the enumerate endpoints DIVERGE (2026-07-20)

**Captured:** 2026-07-20 against a **throwaway** Jellyfin **10.10.3** container (`make smoke-livetv`
stands one up, wires it, and destroys it). No real media server was involved, so there was nothing
to revert.

**Why it happened:** the Phase-10 capture above was **Emby-only**, while §6 has always claimed both
flavors. The maintainer smoke wired a real Jellyfin for the first time and the adapter failed
immediately.

## The deviation

| Op | Emby 4.10 | Jellyfin 10.10.3 |
| --- | --- | --- |
| `GET /LiveTv/TunerHosts` | **200** | **405** |
| `GET /LiveTv/ListingProviders` | **200** | **405** |
| `POST /LiveTv/TunerHosts` | 200 | **200** |
| `POST /LiveTv/ListingProviders` | 200 | **200** |
| `GET /System/Configuration/livetv` | **200** | **200** |

Jellyfin makes the lineage Live TV admin endpoints **write-only**: the POSTs the adapter already
used are fine, but the GETs it enumerated through return 405. The §6 "enumerate-first" idempotency
check therefore **errored on every Jellyfin install** — so the connect either failed outright or
re-registered the tuner on each attempt, which is exactly the duplicate-tuner mess §6 set out to
avoid.

`GET /System/Configuration/livetv` returns `{TunerHosts, ListingProviders}` (plus recording/padding
settings we neither read nor write) and answers **200 on both flavors**. The adapter now reads only
that: one code path, no flavor branch — strictly simpler than what it replaced.

**Verified on the real Emby too** (read-only GETs, nothing written): both `/LiveTv/TunerHosts` and
`/System/Configuration/livetv` answer 200, so moving to the config endpoint loses nothing on Emby.

## Fixture

- `livetv_config_jellyfin.json` — a real `GET /System/Configuration/livetv` from Jellyfin 10.10.3
  **after** wiring, so it carries a populated `TunerHosts` (m3u) and `ListingProviders` (xmltv).
  URLs are the throwaway host's (`192.168.1.79:8001` — the smoke Tunarr); ids are Jellyfin's own.
  Unscrubbed on purpose: nothing here is secret, and the whole server has ceased to exist.
