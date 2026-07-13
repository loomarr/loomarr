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
