# Set up filler

**For:** household admins who want commercials, bumpers or station IDs between shows.
**You'll get:** clips arriving, checked and filed, and playing in the right channels' breaks.

Filler is what plays between programs: commercials, bumpers, station IDs. It's optional. Without
it, channels leave the gaps empty and play fine. For how Loomarr picks a clip for a break, see
[How curation works](../explanation/curation.md#filler-in-the-breaks).

## 1. Set your area

Open **Settings → Your location** and fill in **Your area and language**: your country, and
a city or TV market if you want local adverts. Channels inherit it. Once it's set, Loomarr fetches
only from sources in your country, and your market if you gave one.

## 2. Add clips

There are two ways in, and both go through the same checks.

- **Drop files** into the drop folder (`filler.watch_dir`, by default `_watch` inside the clip
  library at `/data/filler`). Loomarr empties it at least every 15 minutes.
- **Add a source** under **Filler → Sources**: a folder, a media library, a YouTube playlist or
  an Archive.org collection. Give each source a country, and a market if it's local. Use
  **Fetch now** to check one source straight away; otherwise enabled sources are checked every
  6 hours.

Filler never lives in your Emby or Jellyfin library, so a commercial can't turn up as a program.
If you use Tunarr, Loomarr registers the clip folder with it for you.

## 3. Watch clips arrive

**Filler → Incoming** shows each clip moving through the checks: measured, tagged, filed. Most
clips need nothing from you. A clip Loomarr can't identify confidently waits there for your
decision, and can't play until you approve it.

A file holding many adverts back to back is a **recording**, not a clip. Loomarr finds the cuts
inside it and files the segments it's sure of. Cuts it isn't sure of wait in Incoming, with the
whole recording to look at. The original file is kept, but only its segments ever play.

## 4. Check a channel's breaks

Open a channel's **Filler** section. The commercial-break preview shows exactly what would play
in its breaks, using the same rules as the scheduler. If a themed channel falls back to bumpers,
the clips it should use are missing tags: open a clip in **Filler → Library** and fix its kind,
era, audience or tags.

On the same section you can change how often the channel breaks, and which clips it draws from.
Once you edit that choice, it's yours: later refinements don't overwrite it.

## If a clip gets stuck

Each check retries on its own, with backoff. To retry now, fix the cause, then expand the clip's
row in **Filler → Incoming** and retry that step. Earlier work and your own tags are kept, and the
retry survives a restart. Don't delete and re-import the clip.

Removing a clip from the library stops it being scheduled, but never deletes the file.

## Limits

Automatic fetching stops at a clip count or a storage size, and the **Filler** page names the
limit it hit. Clips you add by hand, and pulls you approve, still work. Tidy the library or raise
the limit to resume.

| Setting | Default | What it does |
| --- | --- | --- |
| `filler.breaks_per_hour` | 4 | Breaks per hour for channels that don't set their own; 0 turns breaks off |
| `filler.pod_max` | 4 | Preferred clips per break; exceeded when short clips need more slots |
| `filler.cooldown_seconds` | 1800 | Preferred gap before a commercial airs again |
| `filler.fetch.every` | 6h | How often enabled sources are checked |
| `filler.fetch.max_per_run` | 10 | Most new clips per source per check |
| `filler.fetch.max_catalog_clips` | 2000 | Stop fetching automatically at this many clips |

All of them, and the storage limit, are in the
[settings reference](https://mantonx.github.io/loomarr/reference/settings/).
