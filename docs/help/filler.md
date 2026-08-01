# Filler guide

Filler is what plays between programs — commercials, bumpers, station IDs. It's optional:
without it, channels just leave the gaps empty and play fine.

## The drop-folder

Everything starts with a folder of clips.

- `FILLER_DIR` — the folder Loomarr registers as a Tunarr source (usually `/filler`).
- Add clips any way you like: copy them in, use a tool like MeTube, or use Loomarr's
  built-in ingest (below).
- After adding files, run a sync on the **Filler** page.

Your media server isn't involved — Tunarr scans the folder and Loomarr reads the catalog
from Tunarr.

## Downloading clips in-app (optional)

Loomarr can pull clips straight into the folder from a YouTube playlist or video URL, as a
background job.

The tooling this needs (`yt-dlp` + `ffmpeg`) ships in the Loomarr image, so downloading
works out of the box — there is no tag to switch or profile to enable.

If the download button is disabled, this install cannot run that tooling: usually a custom
or hand-built image without the vendored binaries, or a `INGEST_YTDLP_PATH` /
`INGEST_FFMPEG_PATH` pointing somewhere wrong. Clips you drop into the folder by hand
always work, whatever the image.

## Tagging

Matching a clip to a channel needs to know what it is (kids, era, vibe). Tag clips
manually, or set `FILLER_AI_TAGGING=true` to have Loomarr tag them at sync.

Untagged commercials still play but only match broadly — so a themed channel may fall back
to just bumpers. If a channel plays no commercials, check its **pod preview**.

## Tuning

- `FILLER_BREAKS_PER_HOUR` — breaks per hour (default 4)
- `FILLER_POD_MAX` — clips per break (default 4)
- `FILLER_COOLDOWN_SECONDS` — before a clip repeats (default 30)
- `FILLER_SYNC_EVERY` — catalog re-sync interval (default 15m)

## Preview

Each channel has a **pod preview** showing exactly what plays in its breaks — the same
computation the scheduler uses. It's the fastest way to check your tags are matching.
