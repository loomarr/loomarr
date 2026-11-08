# 0046. Loomarr scans its own media files; the media server keeps only sign-in and Live TV

- **Date:** 2026-11-03
- **Status:** accepted; two points superseded by [0047](0047-loomarr-identifies-media-without-the-arrs.md)
- **Supersedes:** the media server as the sole importer into the Media Inventory
  ([0027](0027-loomarr-owned-media-inventory.md) keeps its model and gains these importers)

## Context

Every catalog fact Loomarr uses still comes from Emby or Jellyfin: presence, episodes, durations,
guide text, ratings, file paths, and stream facts on first play. On a reference household (Unraid over
SMB on 10 GbE, 137k files, 33k videos), the media server's library scan took 9 h 51 m and was cancelled.
Measurements on that library (#2151) located the cost:

- A walk returning size and mtime for every file takes about 25 s, since SMB directory queries
  carry attributes.
- Opening a file is cheap (855/s). Its first content byte costs about 20 ms, and throughput stays
  flat at about 40 files/s from concurrency 4 to 64. The limit is the NAS's disk seek, not the
  client.
- A Go reader of the MKV or MP4 header needs one 64 KiB read for most files and agrees with ffprobe.
  ffprobe costs about 48 ms warm, of which 8 ms is the process spawn.
- Sonarr and Radarr already identify 99.7% of TV files and 94% of movie files. They return every
  episode's title, overview and air date in a few seconds, with no NAS reads.
- SMB2 change notifications (`CIFS_IOC_NOTIFY_INFO`) reach a Linux client with file names in under
  a second.

## Decision

Loomarr discovers, identifies and enriches media files itself, as additional importers into the
existing Media Inventory. The media server stays required only for sign-in and Live TV registration.

- **Never read the contents of an unchanged file.** A source revision is size plus mtime. Header
  facts are read once per revision with a native MKV/MP4 reader, and ffprobe is the fallback.
  Reads are paced for spinning disks: low fixed concurrency, and a pause while a Loomarr channel is
  playing. A faster parser or another language cannot beat a disk seek, so no Rust worker is added.
- **Changes are pushed, with a reconcile walk as the safety net.** The signals are SMB change
  notifications (or fsnotify on non-CIFS mounts), Sonarr and Radarr webhooks that Loomarr registers
  itself, and an hourly walk.
- **Identity stays grounded** (0027). Its sources, in order: Sonarr and Radarr when configured,
  then ids embedded in folder or file names, then NFO `uniqueid`. A filename plus a TMDB search
  produces only a candidate, which the maintainer confirms as an explicit link. The inventory item
  id becomes Loomarr's only content key, and provider ids are open-ended external ids.
- **Episodes bridge between providers by their own TVDB episode id, never by season and episode
  number.** TVDB and TMDB disagree on season layout for about 15% of shows.
- **Metadata providers are a registry** (a new `metadataprovider` package), shaped like source providers.
  One resolver applies a fixed precedence per field family:
  - technical facts: measured, then header, then the arr summary;
  - identity and numbering: arr, then ids embedded in names, then NFO, then confirmed link;
  - descriptive text: TMDB for movies and series, Sonarr/TVDB for episodes;
  - certification: the strictest grounded value.
  No code compares provider ids as strings.
- **Episodes inherit their series certification** unless a grounded provider rates the episode
  itself.
- **Media mounts become required.** The media-server catalog reads and the HTTP stream fallback are
  removed once the scanner passes a parity gate against the media-server origin.
- **Absence is guarded.** An unreadable root is an incomplete scan. A completed walk that would
  remove more than 10% (or 500) of a root's items is held for the maintainer to confirm.
- **Vocabulary follows the change.** Once the scanner ships, **Library** names the household's media
  as Loomarr knows it, which is also the name of the maintainer's UI section. The media server is
  called the media server. `CONTEXT.md` changes in the PR that makes this true.

Implementation is tracked in #2151. The subsystem docs change with each step as it lands.

## Consequences

- A library without Sonarr or Radarr still scans fully. Its first import costs one cold NFO or
  header read per file, and files that nothing grounds wait in Unmatched.
- An episode rated above its series passes a ceiling set for the series rating until a provider
  supplies episode ratings (TVDB is optional and follows the cutover). This is a known limit.
- A Loomarr that cannot read the media files no longer runs. The install docs require read-only media
  mounts, as the media server itself does.
- Stored media-server item ids move to inventory ids in one forward-only migration. The episode and
  path caches are dropped, not rebuilt.
- Hand-made media-server collections become Loomarr collections. Automatic ones come from TMDB.
- Loomarr gains a Library section for the maintainer: items with the provenance of each field,
  Unmatched, Collections, and Settings → Library with scan health and progress.
