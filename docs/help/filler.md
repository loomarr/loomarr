# Filler guide

Filler is what plays between programs — commercials, bumpers, station IDs. It's optional:
without it, channels just leave the gaps empty and play fine.

## The drop-folder

Everything starts with a folder of clips.

- `FILLER_DIR` — the folder Loomarr watches (usually `/filler`).
- Add clips any way you like: copy them in, use a tool like MeTube, or use Loomarr's
  built-in ingest (below).
- After adding files, run a sync on the **Filler** page.

Loomarr reads the folder itself, so clips are available whether or not you run Tunarr. Your
media server is not involved either: filler never lives in an Emby or Jellyfin library, which
is why a commercial can never turn up in a channel's programming.

If you do use Tunarr, Loomarr also registers the folder with it so Tunarr can play the same
clips into its breaks. That happens on its own and needs no setup from you.

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

## Recordings of several adverts

A file holding twenty adverts back to back is a **recording**, not a clip — it can't play in a
30-second break. Loomarr finds the cuts inside it and files the ones it is confident about, so most
of a recording turns into clips with no work from you.

Cuts it is **not** confident about wait under **Filler → Incoming** for you to look at. Each one has
a ▶ so you can watch it before deciding.

> ⚠ **Cuts you never review don't wait forever, and the original recording is deleted with them.**
> After the time set by `FILLER_SPLIT_REVIEW_WINDOW` (30 days by default), Loomarr gives up on the
> leftover cuts and **removes the original recording** to reclaim the space — these files are
> commonly 1–2 GB each.
>
> **The clips already made from it are never touched**, and a recording that produced no clips at
> all is never removed — if Loomarr could not use it, it stays where you put it.
>
> This is the **only** thing in Loomarr that deletes your media. Everything else keeps your files:
> removing a clip from the catalog leaves it on disk, and so does disabling or deleting a source.
> Set `FILLER_SPLIT_REVIEW_WINDOW` to `0s` to keep every recording forever.
