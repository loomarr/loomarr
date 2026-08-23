# Filler guide

Filler is what plays between programs — commercials, bumpers, station IDs. It's optional:
without it, channels just leave the gaps empty and play fine.

## How clips arrive

Loomarr keeps arrivals separate from the filed clip library:

- **Clip library** (`filler.dir`, normally `/data/filler`) holds content-addressed filed clips.
- **Drop folder** (`filler.watch_dir`) is where new files arrive. Leave it blank to use
  `<clip library>/_watch`.
- **Filler → Sources** registers folders, media libraries, YouTube playlists, and Archive.org
  collections. Enabled remote sources can fetch on their own schedule.

Anything dropped or downloaded is measured, checked, classified, and filed through the same
pipeline. **Filler → Incoming** shows where each clip is on that conveyor; there is no sync or
whole-library tag button to remember.

Loomarr reads the folder itself, so clips are available whether or not you run Tunarr. Your
media server is not involved either: filler never lives in an Emby or Jellyfin library, which
is why a commercial can never turn up in a channel's programming.

If you do use Tunarr, Loomarr also registers the folder with it so Tunarr can play the same
clips into its breaks. That happens on its own and needs no setup from you.

## Automatic downloads and limits

The bundled `yt-dlp` and `ffmpeg` tools let Sources fetch clips without a sidecar image. Four
settings keep unattended acquisition bounded: how often to check, downloads per source check,
maximum catalog clips, and maximum filler storage.

Use **Fetch now** on a source when you want one bounded check immediately. It checks only that row,
then puts new downloads through the same Incoming review pipeline; it does not start every remote
collection or bypass the storage and catalog limits. A downloaded clip remains unavailable to
channels until Loomarr can file it confidently or you approve it in Incoming.

When the catalog or storage ceiling is reached, the Filler page names the ceiling and its current
measurement. This pauses only automatic fetching; manually queued clips and approved pulls still
work. Curate the library or raise the named limit to resume.

Settings → Connections also probes the effective clip and drop folders. It verifies that they
exist and are readable and writable; a saved path alone does not count as healthy.

## Tagging

Matching a clip to a channel needs several separate facts:

- **Kind** — commercial, bumper, station ID, PSA, trailer, or interstitial.
- **Era** and **audience** — scheduling facts with their own validation.
- **Brand** — shown only when grounded in the clip's text or picture.
- **Taxonomy tags** — what the clip contains, across products/topics, format, seasonal cues,
  and audience cues.

Kind controls how Loomarr may place a clip. A format tag is descriptive vocabulary for browsing and
curation; changing one does not silently change the clip's playout role.

**Filler → Taxonomy** shows that vocabulary as a hierarchy, its direct and descendant clip counts,
overall coverage, and coverage on each independent axis. Axis coverage matters because a seasonal
cue alone does not explain what a clip advertises; select **Browse without** to inspect clips without
one dimension. Absence is not automatically a problem—seasonal and audience cues are intentionally
sparse. Selecting a broad parent such as
Food matches clips assigned more-specific descendants such as Cereal. The clip editor stores only
the tags actually selected; inherited parents remain derived, so changing the hierarchy later does
not turn old rollups into false operator decisions.

Enable **Tag clips with AI** to classify arrivals automatically. The classifier may resolve only
known slugs, synonyms, and retired aliases; it cannot invent new taxonomy nodes. An admin owns
vocabulary changes from the Taxonomy page. Brand is a separate grounded fact; correct or clear it
beside kind, era, audience, and tags in a clip's editor rather than creating a brand taxon.
Both text and vision classification use the current taxonomy shown on that page. Confirmed segments
from compilation recordings keep their grounded taxonomy tags when they become individual clips.

Untagged commercials are a last-resort rung only for general and late-night channels. They are
excluded from kids and family channels because an unknown audience must never be treated as safe
for children. If a themed channel falls back to bumpers, check its **pod preview** and tag the clips.

## If a clip gets stuck

Pipeline stages retry with backoff. Expand a failed row in **Filler → Incoming** to retry that stage
immediately after fixing its cause. A clip handed over for a decision also has an advanced
**Re-run AI** action. Both preserve completed upstream work and restart only the selected stage and
its dependants. The retry request is durable across a Loomarr restart, and AI classification merges
new grounded facts without erasing operator-authored tags. Deleting and re-importing the clip is not
the recovery path.

Re-running transcode can replace playable bytes and is therefore not exposed as a routine UI
action. The admin API requires an explicit force flag for it.

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

Cuts it is **not** confident about wait under **Filler → Incoming** with boundary evidence and the
whole recording available for context. Confirmed segments become individual clips; duplicates and
segments too short to be useful are discarded automatically. The non-airable original is preserved
for lineage and recovery, while only its filed segments may play.

Removing a clip from the catalog is a tombstone: it stops being scheduled, but Loomarr does not
delete the source file.
