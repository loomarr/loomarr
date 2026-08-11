# Troubleshooting

Every red check in the setup wizard and the Settings → Connections checklist deep-links
into this page. The checklist is executable documentation; this page is its narrative twin
(design §13).

Each section below is the target of a `docHref` the API emits, so the heading anchors here
are part of the contract — renaming one breaks a link the backend is already sending.

## Media server

**Check name:** `media_server` · **Settings:** Connections → Media server

Loomarr talks to Emby/Jellyfin for two things: the title library it grounds suggestions
against, and (optionally) the user accounts it imports.

- **"Connection refused" / timeout** — the URL is wrong or unreachable *from Loomarr's
  container*. `localhost` inside a container is the container, not your host: use the
  service name (`http://emby:8096`) on a shared compose network, or the host's LAN IP.
- **401 / "invalid token"** — the API key is wrong, or it belongs to a non-admin user.
  Loomarr needs an **admin** key: it reads the user list and every library.
- **Connects, but suggestions say nothing is in your library** — the token is valid but
  the libraries aren't visible to that user. Check the user's library access in Emby.

## Seerr

**Check name:** `requester` · **Settings:** Connections → Requester

Seerr (Jellyseerr/Overseerr) is how Loomarr acquires titles it doesn't have. Without a
requester, proposals still generate from what's already in your library — you just can't
fill gaps.

- **401** — the API key is wrong. It's under Seerr's Settings → General → API Key.
- **Requests are accepted but nothing downloads** — that's downstream of Loomarr. Seerr
  forwards to Sonarr/Radarr; check Seerr's own request log first.
- **Re-requesting an already-available title returns success with nothing queued.** That's
  expected, not a failure (Seerr returns 201 with the existing media).

## Tunarr

**Check name:** `tunarr` · **Settings:** Connections → Tunarr

This section only applies if you chose Tunarr as your playout backend. On a default install,
Loomarr streams its own channels and a red `tunarr` check blocks nothing — you don't need it.

On the Tunarr backend, Tunarr builds the streams and the guide; Loomarr decides what plays and
when, and pushes that to Tunarr.

- **Cannot reach Tunarr** — same container-networking rule as the media server. Tunarr's
  default port is 8000.
- **Channels exist in Loomarr but not in Tunarr** — the channel hasn't rebuilt yet, or the
  rebuild is failing. Press **Rebuild now** on the channel page and read the error.
- **A channel exists but plays nothing** — see *Tunarr library* below: Tunarr needs its own
  media source wired and scanned, or its programs table is empty and every slot degrades
  to flex.

### Tunarr library

**Check name:** `tunarr_library`

Tunarr must have your Emby/Jellyfin server configured as *its* media source, with the
movie and show libraries **enabled and scanned**. Loomarr wires this for you
(Settings → Connections → "Connect Tunarr"), but the scan is what populates Tunarr's
program table.

This check exists because its absence is a **silent** failure: without the scan, channels
build, the rebuild reports success, and every program slot quietly becomes dead air.

## LLM

**Check name:** `llm` · **Settings:** AI

Suggestions need a model that supports **tool calling** — Loomarr grounds the model in your
real library through tools, and a model without tool support cannot be grounded.

- **"No tool-calling support"** — the model is reachable but unsuitable. Pick one the model
  picker marks as recommended.
- **Ollama unreachable** — check the URL. On Docker Desktop, Ollama running on the host is
  `http://host.docker.internal:11434`, not `localhost`.
- **A downloaded model isn't selected** — downloading and selecting are separate steps on
  purpose: selecting hot-swaps the running suggester, so Loomarr never changes what's
  running as a side effect of a download.

## TMDB

**Check name:** `tmdb` · **Settings:** AI → Grounding

TMDB provides the metadata that grounds suggestions for titles you don't own yet. Without
it, Loomarr can only suggest from your existing library.

- **401** — the API key is wrong. Use the **API Read Access Token** or v3 key from your
  TMDB account settings.

## Filler

**Check name:** `filler` · **Settings:** Filler

This check appears only once a drop-folder is configured.

- **No clips found** — the drop-folder path must be readable *by Loomarr's container*, and
  it must be the same folder Tunarr scans as a `local` media source. Run a sync after
  adding files.
- **Clips exist but channels play no commercials** — check a channel's pod preview. If it
  shows only bumpers, your commercials likely lack the tags used for matching.
- **"Ingest unavailable"** — the yt-dlp and ffmpeg tooling ships in the Loomarr image, so
  this means the running install cannot execute it: usually a custom image built without the
  vendored binaries, or `INGEST_YTDLP_PATH` / `INGEST_FFMPEG_PATH` pointing at a path that
  is missing or not executable. Clips placed in the drop-folder by hand or by another tool
  are unaffected.

## LiveTV

**Check name:** `livetv`

This is the wiring that makes Loomarr's channels appear in your media server's Live TV
guide: Tunarr registered as a **tuner** (M3U) and a **guide provider** (XMLTV).

- It is a **one-time** setup, not per channel. Once wired, every channel Loomarr creates,
  renames, or deletes propagates automatically.
- **Channels are in Tunarr but not in the Emby/Jellyfin guide** — the tuner is wired but
  the guide hasn't refreshed. Media servers refresh on a schedule (often nightly); Loomarr
  pokes a refresh after each rebuild, but the media server decides when to honor it.
- **Duplicate channels in the guide** — a tuner was registered twice (manual + Loomarr).
  Remove the extras in your media server; Loomarr's connect is idempotent and won't add
  more.
- **Channels show but won't play** — that's not a Live TV problem. Tunarr's library
  source doesn't match, so the programs resolve to nothing. Re-run **Wire Tunarr to your
  library** (the `tunarr_library` check above).
- **Playback stops after ~4 seconds in Firefox** — a Firefox playback quirk, not a Loomarr
  or Tunarr fault; the stream is fine. Watch in a Chrome-based browser or the Emby/Jellyfin
  app.
- Loomarr's admin token needs privilege to register tuners — the same admin key used for
  the media-server connection.

## Downloads not appearing

Loomarr learns a title finished by **polling** — a scheduled library scan (and, with the
direct Sonarr/Radarr requester, a download-queue poll), not an inbound webhook. A title moves
*downloading* → *available* once it shows up in the media-server library.

- **Stuck in *requested* / *downloading*** — check the acquisition backend is reachable
  (Settings → Connections → Test) and that Sonarr/Radarr actually grabbed a release. The
  library scan runs every few minutes; the Tasks page (Settings → Tasks) shows each poll's
  last run and lets you **Run now**.
- **Downloaded but not showing** — confirm the item landed in the same media-server library
  Loomarr and Tunarr read, and that the library has been scanned. Availability follows the
  library, so a title the media server hasn't indexed yet won't flip to *available*.
