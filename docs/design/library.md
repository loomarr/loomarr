# Library and inventory

Formerly `design.md` §5 (media inventory, cached series episodes), §6 (Emby and Jellyfin) and §7.2
(search). How Loomarr builds the household's Library from its own media folders (#2151): the
scanner, the metadata providers and the inventory it keeps, and what it still asks the media
server for.

## The media server contract

Emby and Jellyfin share one adapter; `library.flavor` selects the auth header, never the logic.
Since #2151 step 16 the adapter reads no catalog: presence, search, bulk scans, episodes and item
metadata come from the Library's own tables.

- **Auth is a header, never the `api_key` query parameter** (it leaks into logs): Emby
  `X-Emby-Token`, Jellyfin `Authorization: MediaBrowser Token="…"`.
- **Collections are not read from the media server.** The household's collections live in the
  Library ("Your collections" under [Scanner](#scanner)) and back `scope.collections`; the
  adapter has no collection call.
- **Users:** `POST /Users/AuthenticateByName` checks credentials and `GET /Users` with the admin
  `library.token` lists accounts for import ([`auth.md`](auth.md#import-and-sync)).
- **Configuration is read per operation.** Each public operation resolves `{flavor, url, token}`
  from one settings snapshot and carries it through every nested request, so a save takes effect on
  the next operation without a restart and can never send one server's token to another.

## Media inventory

The media server is an availability authority and importer, not Loomarr's database of record
(decision [0027](decisions/0027-loomarr-owned-media-inventory.md)). `inventory.Service` owns one durable,
provider-neutral aggregate of **Media Items**, **Media Sources**, their **Origins** and current
**Observations**. Emby and Jellyfin populate it. Playout, scheduling and search never see provider
response types.

```go
type Service interface {
    ApplySnapshot(context.Context, Snapshot) (ItemID, error)
    Item(context.Context, ItemRef) (Item, bool, error)
    ResolveSource(context.Context, SourceRequest) (ResolvedSource, bool, error)
    RecordMeasurement(context.Context, Measurement) error
    MarkUnseen(context.Context, AuthorityID, time.Time, []OriginKey) error
}
```

- **Items and sources.** An item's kind is extensible; series, seasons and collections may have no
  source, and a movie or episode is playable only when a usable source resolves. Inventory membership
  grants neither acquisition state nor programme or filler authority.
- **Identity merges only on a grounded external id or an explicit operator link**, never on names or
  filenames. Re-import is idempotent.
- **Absence is recorded only after a completed scan.** A timeout, auth failure or partial scan never
  calls `MarkUnseen`, so it cannot erase the last good inventory.
- **What is kept:** hierarchy, external ids, descriptive metadata and artwork references, and ordered
  stream facts (codecs, languages, channels, dimensions, colour and HDR, interlace). Unknown importer
  fields go into a bounded, sanitized extension document. Only the current observation per origin is
  kept.
- **What is never kept:** credentials, authenticated or transcode URLs, playback state, sessions,
  tokens and artwork bytes. Locators are protected data, excluded from diagnostics exports. Bounds are
  enforced at the domain and store boundaries by the shared conformance suite.
- **Freshness is per source revision** (size and modification time locally; an upstream revision
  otherwise). A measurement applies only while its revision matches.
- **Source analysis.** One low-priority worker measures each source revision once: a keyframe index
  from the container's own index, integrated loudness and true peak from sampled windows, natural break
  candidates (chapters first, otherwise coinciding black and silence near quarter-hour points), and the
  active picture that the channel watermark anchors to. Reads are byte-budgeted, so no file is decoded
  whole. Results live in `inventory_source_analysis`, are dropped when the revision changes, and are
  read through `inventory.AnalysisReader`. Each reconcile of an internal-playout channel queues
  the library files it airs in the next 48 hours that are not yet measured at their current
  revision, so a first airing usually starts with its keyframe index; nothing past that horizon is
  measured ahead, and Tunarr-backed channels queue nothing. Playout never asks the media server at
  airtime: a source still unmeasured gets one stream-facts probe on first play.
- **Playout moves onto the Library tables one reader at a time.** Audio-track choice reads first: a
  local file the scanner read at its current size and modification time takes its own audio tracks
  from `library_streams` (`LibraryAudioTracks`), with external audio files excluded. Any other file
  falls back to inventory, then to the probe.
- `ResolveSource` returns identity, revision, safe observations and a protected locator. Inventory
  owns what a source is, not the credentials for opening it.

## Header facts

`internal/media/header` is the only code that parses container bytes (decision
[0046](decisions/0046-loomarr-scans-its-own-media-files.md)). It reads a file's duration, streams
(codec, size, frame rate, pixel format, colour and HDR, the Dolby Vision profile, field order,
languages, default, forced, commentary, hearing-impaired and described flags, channels, sample
rate) and chapters from the Matroska/WebM header or the MP4/MOV `moov`, plus the keyframe index
from Matroska Cues or the MP4 sample tables, which `media/measure` uses.

- **A reader that learns more reads every file once more.** `header.Version` goes up with each new
  fact. The scanner records it with the facts it stores and folds it into each file's digest, so
  the next pass reaches every file. A file read by an older reader keeps its facts, and its title
  stays ready, while it waits in the header queue. It keeps its old digest until it is read
  again, so a stage the share cuts short is taken up on the next pass.

- **Reads are few and random.** Most Matroska files take one 64 KiB read; the SeekHead points to
  anything past it. An MP4's `moov` is found by box headers, never by reading `mdat`. Files are
  opened with `POSIX_FADV_RANDOM`, since each read on a NAS is one disk seek.
- **Native or nothing.** `Read` answers only when every fact playout's copy decision uses is fixed
  by the header; otherwise it declines and `Probe` asks the shared ffprobe observation. It declines
  for AVI and MPEG-TS, a header that does not parse, an MP4 QuickTime chapter track, ≥10-bit video
  whose colour neither container nor parameter set signals, bitstream field coding, and VP9 or
  MPEG-2 in Matroska without a codec configuration.
- **Values are ffprobe's spellings** (`hevc`, `matroska,webm`, `yuv420p10le`, `24000/1001`), held
  to ffprobe by a parity test. Colour is compared with the first decoded frame: ffprobe's stream
  summary trusts a partial Matroska `Colour` element over the HEVC parameter set and can call a PQ
  file SDR, which the header reader does not.

## Metadata providers

`internal/library/metadataprovider` is the registry of systems that already know what a media file is
(#2151, decision [0046](decisions/0046-loomarr-scans-its-own-media-files.md)). It imports nothing from
Loomarr. A provider declares capabilities: the item kinds it describes, the `(kind, namespace)` pairs it
fetches by, the field families it supplies (technical, identity and numbering, descriptive,
certification, artwork), its **basis** (the kind of evidence it is: manager, sidecar, catalog …), the
catalog its text comes from, and rate hints. It fetches one item by id and may also list everything
(`Lister`) or report changes since a cursor (`ChangeFeed`). Provider ids are a closed set (`sonarr`,
`radarr`, `nfo`, `tmdb`, `localart`), and a guard test fails on any code that compares them.

- **Sonarr and Radarr** (`metadataprovider/arr`) are managers. Sonarr returns a series by TVDB id with
  every episode: the episode's own TVDB id, its numbering including the air-order position of specials,
  its current file and the arr's media summary. Radarr returns a movie by TMDB id. Ratings from Radarr
  carry its configured certification country. The connection is read once per operation.
- **NFO** (`metadataprovider/nfo`) is a sidecar, resolved by the media path: `<file>.nfo`, `movie.nfo`
  when the folder holds only that video, or `tvshow.nfo` in a series folder. It reads the whole title
  (decision 59): typed `uniqueid` elements and the older `<id>`, `<imdbid>`, `<tmdbid>` and `<tvdbid>`
  tags (a bare numeric `<id>` is TMDB's for a movie and TVDB's for a show, by Kodi's convention; a
  `uniqueid` wins its namespace), guide text, `mpaa`, numbering, tags, the default `<ratings>` score,
  tagline, countries, trailer (Kodi's YouTube add-on link becomes the YouTube page), `<edition>`,
  `<set>` (linked when it carries `tmdbcolid`), and people: the cast in billing order with character,
  photo and TMDB id, then directors and writers. Watched state and the scraper's stream details are
  not read. Its details rank below every service. The default `uniqueid` names its catalog. Reads are
  capped at 1 MiB, refuse symlinks and any DOCTYPE, accept UTF-8, Latin-1 and Windows-1252, and drop
  malformed ids.
- **TMDB** (`metadataprovider/tmdb`) is a catalog over the shared `internal/library/tmdb` client. A movie or
  series by TMDB id is one detail call with `append_to_response` (external ids, US release dates or
  content ratings, keywords, images), so the record carries guide text, genres, keywords, the
  community rating, series status, the US certification (the strictest of a movie's releases),
  poster, fanart and logo references, and imdb and tvdb ids. A movie's TMDB collection rides on its
  descriptive record, since the movie call already returns it. An episode is fetched by its own
  TVDB episode id through `/find`, never by season and episode number, and its record carries no
  numbering: TMDB and TVDB lay out many shows differently, so only the id bridges the two. Its
  `vote_average` is the episode's community rating. Language and country are the installation's
  `metadata.language` and `metadata.country`; a country other than the US is refused. The change
  feed reads `/movie/changes` and `/tv/changes` and answers within TMDB's 14-day window; an older
  cursor is `ErrUnsupported` and the caller refetches. Keywords per item are capped at 40.
- **Local artwork** (`metadataprovider/localart`) is a sidecar too, found by one listing of the media's
  folder in Kodi's naming: a movie's `<file>-poster.jpg` (and `-fanart`, `-clearlogo`, `-banner`,
  `-landscape`, `-clearart`, `-discart`), then the folder's `poster`, `folder`, `fanart`, `clearlogo` …
  when it holds only that video; a series folder's same names; an episode's `<file>-thumb.jpg`. Any of
  `.jpg`, `.jpeg`, `.png`, `.webp`, matched whatever the case. Records carry the image's path, never
  its bytes, and only regular files: the image service imports a poster when the scanner writes its
  title (see Posters below). Season art is not read; Loomarr has no season items.
- **Arr requests never follow a redirect**, so the API key cannot reach another host.

**Precedence** lives in one place, `metadataprovider/precedence`, and ranks bases and catalogs, never
providers. A field takes the highest-ranked source that has a value for it. Numbering and the file list
move whole.

| Family | Order |
| --- | --- |
| Technical | measured, header, manager |
| Identity and numbering | manager, id in a name, sidecar, confirmed link |
| Descriptive (text, genres, keywords, studios or networks, countries, tagline, trailer, people, community rating, collection) | a service (catalog or manager) whose text comes from TMDB for movies and series, or TVDB for episodes; then other services; then sidecars, which never outrank a service |
| Certification | the strictest grounded rating on the US ladder |
| Artwork | sidecar, catalog, manager, per artwork kind |

Certification uses the audience ladder that enforcement uses. Ratings in another country's system and
unrated values ground nothing, and a resolver for a country other than the US is refused. An episode
that no grounded source rates inherits its series' strictest rating. One that a source rates keeps
that rating, whether it is stricter or not.

People (decision 49) are a title's main cast in billing order (TMDB's top 20), then its key crew:
a series' creators, then directors and writers. Each is a `library_people` row keyed by the
catalog id a source gave (`namespace`, `external_id`; row id `tmdb:<id>` for TMDB) wherever they
are credited. A person an NFO or the media server names without an id is kept by name, unlinked.
An episode's cast is its show's. `scan.Filmographer` keeps a filmography (`library_filmography`,
decision 50) from the catalog provider for each person linked in its namespace who directed or
created a title, is
among a title's ten top-billed actors, or is credited on three or more titles. It is fetched when
they first qualify and monthly after, and people no title credits any more are removed each hour.
The household's metadata language
(`metadata.language`, decision 21) defaults to the media server's; changing it fetches every
catalog record again.

## Media folders

`internal/library/scan` keeps the folders Loomarr reads media from (decision
[0046](decisions/0046-loomarr-scans-its-own-media-files.md); #2151 decision 6), shown on
**Settings → Library**.

- **Discovered, then confirmed.** One pass asks the media server (`/Library/VirtualFolders`, video
  libraries only) and Sonarr and Radarr (`/api/v3/rootfolder`, whatever requester is chosen) for
  their folders. Each folder maps to a local folder: the `library.path_map` translation when Loomarr
  can open it, else the same path, else the one mount whose folder carries the same trailing
  names. The path map comes first because in Docker the media server's name for a folder (`/data/tv`)
  can also exist in Loomarr's container as something else (Loomarr's data folder is `/data`). A root
  is always absolute; an ambiguous or missing folder stays unmapped. A discovered folder waits for
  the maintainer's **Use this folder**, which starts a pass; one typed by hand is in use at once
  and must open. The pass runs when Settings → Library or the wizard first finds no folders, on Check
  again, and before a full scan when a connected arr has none of its folders recorded (an arr
  connected after the folders were set up).
- **One path map.** `library_root_paths` holds every authority's name for every folder. A row the
  maintainer edits under "Folders have different names in other apps" is never rewritten by
  discovery; clearing it hands it back.
- **A failing source forgets nothing.** Only an authority that answered drops the folders it no
  longer reports, and only a folder that is neither in use, typed by hand nor named by any mapping
  disappears. Every folder is re-checked on each pass, within 5 s, so an offline NAS shows as
  "Loomarr can't open this folder" rather than hanging the page.
- `GET /v1/library/status` reports `needs_folders`, `folder_offline` or `ok`, with counts.

## Scanner

`scan.Importer` writes what is on disk to the inventory, alongside the media server's origin (#2151
step 5). Nothing reads the scanner's origins until the cutover.

- **One origin per file.** Each root is an authority (`scan:<digest>`); each media file under it
  is an origin keyed by its path within the root. Files that carry the same ids join one item, as
  the media server's origin of the same title does; an id joins only items of its own kind, since
  TVDB numbers shows and episodes from one sequence. A series is an origin keyed by its folder. A
  multi-episode file is one item, its first episode numbered through its last, carrying every
  episode's own ids (Sonarr's records, or the NFO's further `<episodedetails>`).
- **Identity** is the precedence resolver's, over Sonarr's or Radarr's record for the file (its
  path translated by the path map), ids in names (`{tmdb-1234}`), the name's `S01E02`, and the
  file's own NFO (`<name>.nfo`, or a movie folder's `movie.nfo`). A movie takes ids only from its
  file's or its own folder's name, never a tvdb id; an episode's series is the folder above its
  season folder, else the nearest folder named by an id, else its own. An episode loose at a root's
  top, or in a season folder there, is the show its name gives before `S01E02`
  (`Example.Show.S01E02.mkv` is "Example Show"); a season folder never names a show. A file nothing grounds is
  still written, with no ids. Only movies and episodes are titles (0047): extras (trailers,
  `-trailer` files, `extras/` folders and the like) and theme videos (`backdrops/`) are not
  imported, and a skipped extra does not take a movie's folder art from it. An origin whose ids later name another item of its kind
  moves to it, when it is its item's only origin or joined the item by its ids rather than founding
  it. A joined origin whose ids then name nothing goes back to an item of its own, so taking back a
  Needs you link splits the copy out again; the founding origin never leaves.
- **Unchanged files are never opened.** `library_files` keeps each file's stat and a digest of
  its identity inputs (the arr record, names, its sidecars' stat). A pass writes a file only when
  either changed, and reads its header only when its bytes changed.
- **While an arr fails to list:**
  - new and changed files in the roots it manages wait, rather than being written with what its
    absence would leave;
  - files in any other folder go on as usual;
  - nothing is marked missing.

  Folder passes reuse the failed listing for a minute instead of asking the arr again on every
  change; the hourly walk always asks.
- **Header reads** (`mediaheader.Probe`) run four at a time after the walk, wanted items first,
  and wait while any playout session is live. A file whose header does not parse is marked failed
  until its bytes change; a share that stops answering ends the stage and leaves the rest pending.
  The wait never holds up a pass (decision 11): one header stage takes files at a time, outside
  the pass lock, a pass that finds it taking files hands its files over, and a read whose file a
  later pass wrote meanwhile is dropped.
- **Removals.** An unreadable root or folder makes the walk incomplete, and nothing is marked
  missing. A complete walk that would remove more than 10% or 500 of a root's files is held; the
  status lists it per folder and kind, and `POST /v1/library/roots/confirm-removal` records the
  maintainer's confirmation, which the next pass applies. Releasing a root forgets its files at once.
- **Passes.** `scan.Scanner` makes one when the app starts, hourly, when a folder is used or
  added, and on `POST /v1/library/scan` (Settings → Library's Scan now, as in Emby and Jellyfin).
  A pass is *asked* when Scan now, Refresh everything or using or adding a folder started it, or
  it is the start-up pass of a library with no files yet (its first import); an unasked pass with
  nothing to read or look up is *quiet*, and shows only "Checking your folders…".
  `POST /v1/library/refresh` (Refresh everything, asked first) clears every present file's identity
  and marks every fetched catalog row due, then starts a pass: each file is written again from its
  name and its apps, each title looked up again, and the headers already read are kept, so nothing
  stops playing. `library.scanner.enabled` (on by default) stops passes, folder passes
  and header reads. Sonarr and Radarr are its managers when connected; one with no connection
  manages nothing rather than counting as down. Header reads fall back to the same ffprobe playout
  uses, and wait while someone is watching: a channel packager whose playlist a viewer fetched
  within the last 10 s, copies included. A neighbour warm nobody joined, or a packager airing
  slate to nobody in its grace, does not hold them. A root
  inside another in-use root is walked as part of it. Walks skip `trickplay/`,
  `extrafanart/`, `.actors/`, `@eaDir/`, recycle bins, extras folders below the top of a root (a
  show at the top named "Shorts" is still a show), names that are not UTF-8 and folders holding
  an `.ignore` file, list disc images as problems, and follow a symlink only inside its root.
  A video's trickplay file beside it (Emby's `<name>.bif`, decision 61) is referenced in
  `library_trickplay`, never read or imported. It joins the file's sidecars without a stat, so
  its appearing or going changes the identity digest at no cost to the walk. Only the `.bif`
  layout is handled, because it is the one seen in a real library. The `trickplay/` folder skip
  stays until another server's layout turns up.
- **Progress.** One `scan.Tracker` records an import as three steps, each with its own count:
  1 *Looking through your folders* (the walk; its total is the top-level entries of every walked
  root, listed once at the start of the pass, and it names the folder it lists, "TV / Night Ferry /
  Season 2"), 2 *Reading each file* (header reads, growing as passes hand files over; it names the
  file), and 3 *Looking up details* (the Enricher's fetches of catalog rows due as new: never
  fetched, an outdated shape or language, or marked by Refresh everything; counted when it starts
  and grown as more become due; it names the title). Steps 2 and 3 can run together; the status
  shows the earliest one running. Each step's time left is remaining × elapsed ÷ done since its
  rate baseline, sent only once it has run 30 s and done 2%, never while paused, and the baseline
  restarts when a pause for watching ends. Folder passes show only the header reads they queue.
  `GET /v1/library/status` carries it as `scan` (`step`, `stepNumber`, `done`, `total`, `current`,
  `etaSeconds`, `paused`, `quiet`), and the admin-only `library_scan` SSE frame carries the same,
  whole and latest-wins: at most once a second while a step runs, at once when the step shown or
  its pause changes, and once when everything is idle. The empty `library_titles` frame says the
  titles changed (an item written, a header read, files marked gone), at most once every 2 s with
  a trailing frame, and the Library page, a title's page, Ready so far and Needs you refetch on
  it. Nothing in the web app polls while an import runs; the GETs stay the truth on load and
  reconnect.
  External subtitle and audio files beside a video (`<name>.en.forced.srt`, `<name>.fr.mka`,
  decision 61) join its sidecars the same way and are never opened. Their names give the
  language (the first word that names one) and the flags `forced`, `default`, `sdh`/`cc`/`hi`
  and `commentary`. They are listed as `library_streams` rows after the file's own tracks, with
  `external_ref` set to the path. A `<name>.edl` is stat'd and read (at most 64 KiB) into
  `library_markers`: each line is `start end [action]`, in seconds, with actions 0 cut, 1 mute,
  2 scene and 3 commercial. The header reader keeps the first edition's top-level chapters with
  the file's facts (`header.Version` 3), and they become `library_chapters` rows. Tracks carry
  their commentary, SDH and descriptive flags from the dispositions (decisions 45 and 46).
- **Change signals.** Each signal names folders, and `Scanner.PassFolders` passes over only those
  (`Importer.RunFolders`): it writes and removes what is under them, under the same removal guard,
  and still counts every file outside them as present. A folder deleted whole is a change to the
  one above it. Each folder waits on its own until its changes have been quiet for 5 s, or have
  waited 2 minutes, so a long copy into one folder holds back no other. Changes to files still
  being written (`.part`, `.partial`, `.tmp`, `.crdownload`, `.!qb` …), to desktop leftovers
  (`Thumbs.db`, `.DS_Store`, `._*`) and permission-only changes are dropped before they queue.
  - `scan.Watcher` watches each in-use root on a local filesystem with inotify. A kernel queue
    overflow asks for a full pass. inotify sees only changes made through this machine, so an NFS
    or FUSE root is not watched.
  - **SMB change notifications.** A root on a CIFS mount is watched through the server instead:
    one `CIFS_IOC_NOTIFY_INFO` call per root, over the whole tree, reporting names added, removed,
    renamed and written by any client. Each name's folder is passed; a report with no names (the
    server's batch overflowed) passes the whole root. A server or kernel without notifications is
    logged once and left to the hourly walk; any other failure retries after a minute and passes
    the whole root. The call cannot be interrupted, so after shutdown the one in flight ends with
    the share's next change.
  - **Arr webhooks** (`internal/library/arrhook`, `scan.ArrHooks`). At start and hourly, Loomarr
    keeps one webhook connection named "Loomarr" in each connected Sonarr and Radarr, posting
    the file events (download, upgrade, rename, file and series/movie delete) to
    `POST /v1/library/hooks/{app}` at `server.public_url`. The arr tests a connection before saving
    it, so a saved one could reach Loomarr. A failed registration is tried again after a minute,
    doubling up to the hour: at start, a proxy that routes only to a healthy container answers
    the arr's test 404. The operators hear of it only when the retry fails too. Each event's
    folders map through that arr's path map.
    The route is public: an arr holds no session, so the credential is a generated secret
    (`secret.arr_webhook`, never shown) sent as the basic-auth password. `GET /v1/library/status`
    lists each arr's state (registered, not connected, no address, failed) and its last event,
    and `POST /v1/library/hooks/{app}/test` registers again and asks the arr to send its test.
- **Arr listings are reused.** A full pass lists every connected arr (Sonarr's listing is two
  requests per series); a folder pass reuses that listing. An arr's webhook names the series (TVDB
  id) or movie (TMDB id) it changed, and the next folder pass reads just that one again, dropping
  it when the arr no longer has it. A change naming no item, a read that fails, a changed path
  map, a changed arr address, or a listing an arr failed lists the arrs again.
- **Renames keep their header.** A file the walk no longer finds and a new one with the same size
  and modification time are the same bytes renamed or moved: the new path takes the header read
  at the old one rather than opening the file again. A size and time two vanished files share
  matches neither.
- **A stuck share does not stall the scanner.** A walk that lists no folder and reads no file for 2
  minutes is given up on: the root reads as offline and nothing in it is marked missing. A call
  blocked on a dead mount cannot be cancelled, so the root is not walked again until that one ends.
  A header read gets the same 2 minutes: one that does not return ends the header stage and stalls
  its root the same way, so no further reader is sent into the mount. One file the share cannot
  give back (gone since the walk, an I/O error) stays pending for the next pass and the stage reads
  on; eight such files in a row with no read between are the share's fault, and end the stage with
  the rest still pending.
- **Notifications** (decision 32) go to operators. Each pass records every root's readability
  (`CheckLibraryRoot`), so the status strip shows what the last pass found, and a flip publishes
  `library_folder_offline` or `library_folder_back` once per change. An arr whose webhook
  registration starts failing publishes `library_changes_degraded`, at most once a day. Once a pass
  has settled, `library_needs_you` says how many titles Needs you holds, at most once a day and
  only when one is new since the last notice: `scan.NeedsNotice` marks each title it counted
  (`library_items.needs_noticed`), so a restart neither repeats a notice nor loses one.
- **Library → Items** (step 10, decisions 22 and 25) reads a small list kept in step with the
  inventory, rather than decoding every inventory observation: `library_items` holds each movie and
  series (id, kind, resolved name, sort name, year). The store rewrites an item's row in the same
  transaction as each inventory write that touches it (a snapshot, an origin marked unseen),
  through a projector the scanner installs, since only the scanner reads its origins' extension
  (0047). The row comes from the newest present scan origin; an origin written before it carried
  how its title is listed leaves the row as it was. Episodes have rows too (Items lists movies
  and series). Each resolved fact is a column with the provider that supplied it in a `_from`
  column (`rating`, `rating_from`; decision 40): original title, overview, premiere date,
  rating, community score, runtime, and an episode's numbering. `library_ids`,
  `library_genres`, `library_studios` and `library_tags` are rows in the source's order. An
  episode names its series' scan origin, and the store fills `series_id` whichever of the two
  is written second. An episode with no rating takes its show's (`rating_from = 'show'`); its
  genres and studios are its show's only (decision 42). `library_sources` has one row per present
  file, with its `edition` and the copy that plays in each edition marked (`plays`, decision 37's
  rule, shared with the title page).
  `library_streams` has one row per track, with labels computed once from the raw header values
  (decision 44): resolution by height or, for a picture wider than 16:9, by width; HDR type from
  the transfer; audio format; the channel layout where the container states none (neither
  Matroska nor MP4 does), as ffmpeg's decoder states it for the codec and channel count, checked
  against ffprobe in the header parity test; the ISO 639-1 language code and its English name.
  `library_art` has one row per artwork reference. A series has a `season-poster` row for each
  season with a poster, by `season` (0 is the specials): the folder's Kodi-named
  `season01-poster.jpg` or `season-specials-poster.jpg` first, else TMDB's poster for that season
  (`season-all-poster` is not a season's). Each `library_files` row records
  the item it was written as (`item_id`) and an episode's series (`series_id`). A title is listed
  while it has a present, importable file, and is "getting ready" while any of those files'
  headers is unread. A file row from before these columns has no item, so the next pass writes it
  again. `GET /v1/library/items` pages and searches the list (admin-only);
  `GET /v1/library/items/{id}` is one title: the newest present scan origin's facts, a movie's
  files, or a series' episodes by season.
- **Posters, backdrops and logos** (step 10b, #2567). When the importer writes a title it hands
  the resolved poster, fanart and clear logo to the image service, and the title owns each
  through a `library_item_images` row (role `poster`, `backdrop` or `logo`); that
  ref is the one record of which image it is, and a kind replaces only its own role's ref. Banner,
  landscape, clearart and discart stay references: nothing shows them. A clear logo's alpha is its
  shape, so the `logo` role has no JPEG rendition and its `<img>` fallback is WebP; its dominant
  colour weighs each pixel by its alpha, so the transparent canvas doesn't dim it. A local sidecar
  outranks TMDB's image of the same kind (decision 19). The item page carries `logoImage` and
  `backdropImage`, each absent when the title has none, and so does every guide airing (the
  guide, a channel's timeline and Home's highlights): the art of the title its provisioning key
  names, so an episode carries its show's logo and fanart beside its own still. One batched read
  per response (`LibraryTitleArt`); an image still being fetched is left off. A sidecar file is imported
  then, keyed by its content, opened only as a regular file and never through a symlink, and
  skipped when the service already holds those bytes; it is `extracted`, since the file beside the
  media is its source. A manager's https URL is adopted, and the fetch job moves the ref onto the
  bytes when they arrive; until then the API leaves the poster out. A new poster replaces the old
  ref, and a title with none lets it go. An artwork file beside the media is one of its sidecars,
  so adding or changing one rewrites the title; a folder's own art (`poster.jpg`) counts only for
  the folder's one video. A poster that fails to import leaves the title as it was, and the next
  write tries again.
- **Catalog details** (decision 20). A movie or series whose ids include a TMDB id takes its
  details from TMDB: the guide text, genres, rating, poster and fanart, ranked by precedence like
  any other source. The walk never calls TMDB. `library_catalog` caches each record by provider,
  kind and id, and the importer reads it when it writes a title. A title not cached yet is written
  without it and its key noted as wanted, and `library_items.catalog` names the row each title's
  details come from. `scan.Enricher` fetches every minute: titles never fetched first (up to 200
  a tick), then a share of the oldest, so the whole cache is fetched again within 90 days, spread
  out. Every write that changes the titles also wakes it (`Enricher.Wake`), so step 3 follows the
  write at once instead of up to a minute later, with the status reading caught up in between. A
  woken lookup fetches only the titles never fetched: the share of the oldest is sized for one
  tick a minute, so it stays with the tick. Wakes made while a lookup runs fold into one. It keeps
  to the provider's rate hint. Each row notes the shape of record it was fetched in
  (`version`, per kind, `scan.catalogVersions`). When the scanner starts reading something older
  records lack, that kind's version goes up, and its older records are fetched again with the
  titles never fetched, not at the refresh. Series are at version 5, which lists their other
  episode orders by name, kind and size (decision 53). A record that changed forgets the identity digest of
  its titles' present files (a series' episodes too) and passes over their folders, so they are
  written again with it. A title TMDB does not know is cached empty until the refresh. A failed
  fetch holds that title back for 6 hours, and with no TMDB key nothing is fetched or held back.
  A release build ships its own key (decision 51), stamped from the `TMDB_BUNDLED_KEY` release
  secret into `buildinfo` as the `tmdb.api_key` default. A household's own key wins. Settings
  lists the shipped key as `builtIn`, never as set, and never previews it. Only a dev or source
  build has no key. TMDB's notice shows on Settings → System → About whatever key is in use.
  The kill switch stops it too. An episode whose ids include its own TVDB id (from Sonarr or an
  NFO) is cached the same way, keyed `tvdb:episode:<id>` in its `library_items.catalog`: TMDB's
  view of that episode, found through TMDB's `/find`, never by its season and number when it has
  that id (decision 53). An episode with no TVDB id takes TMDB's entry for its season and number,
  keyed `tmdb:season:<series id>/<season>` (one record per season, every episode in it), but only
  when TMDB numbers the series the way the disk does: TMDB lists no other episode ordering for it
  (episode groups: DVD, absolute, story arc), and every season on disk ends at TMDB's episode
  count for that season. A pass identifies every file before writing any, so each series knows
  its seasons on disk, and each series is resolved once and shared by its episodes. A series
  record that doesn't say which orders it has is not checked. Episodes that fail the check keep
  their files' details and are marked `unnumbered`, which puts their show in Needs you
  (`LibraryOrderNeeds`), and TMDB's other orders are asked for (`tmdb:episode_order:<group id>`:
  the order's episodes numbered as it numbers them, and its season sizes, so Needs you can say
  which order fits the disk). The household's answer, kept by the show's catalog row in
  `library_episode_orders`, then decides: TMDB's own seasons (`catalog`), one of its other orders
  (`order`, each episode taking that order's entry for its season and number), or the files
  (`files`, which stops asking). The mock is `project/prototypes/library-episode-order.html`.
  The identity digest carries a write
  version; raising it
  makes every file written once more, keeping its header, so a title written before this names
  its catalog row on the next pass.
- **Needs you** (step 11a, decision 27). A movie or series is grounded when anything gives it a
  TMDB, TVDB or IMDb id: an arr, an id in its name, a sidecar, or the maintainer's link. Each
  title row keeps its scan origin (a movie's file, a series' folder) and whether it is grounded.
  `scan.Matcher` searches TMDB every minute for present titles nothing grounds and nobody
  answered, up to 20 a tick at the provider's rate hint. It searches by the name up to the release
  words (quality, source, codec) and the last year-like word after the first, or the year a
  sidecar gave. It looks at the first five results of the title's kind, fetching each one's
  details, and keeps three in `library_candidates`, best first, each by its catalog's own id
  (`namespace`, `external_id`); an answer keeps the id it was given the same way, so another
  catalog's search needs no schema change. The name counts most, then the
  year (a year apart at most), then the length (movies only: within five minutes or 8% of the
  runtime). `library_items.searched` notes what was searched, so a title is searched again only
  when its name or year changes, or once a movie's length is known. With no TMDB key nothing is
  searched or marked. A movie is matched by its search (decision 56) when exactly one result
  agrees on all three, name, year and length; two that agree, or none, wait in Needs you
  (decision 58). That match is an answer with `answered_by = search`, written as a `search`
  record ranked below every id and below the maintainer's link, which replaces it. A series is
  matched the same way (decision 57) when exactly one result agrees on the name, the year when its
  folder names one, and its episodes: every season on disk is in TMDB's layout (the `seasons` the
  detail call already carries) with at least as many episodes as the highest number found there.
  Specials and unnumbered files are not checked. It is containment, not equality, so a show still
  airing or one missing episodes fits, and a show TMDB numbers differently from the files waits
  for the maintainer. A series is searched again when its episodes on disk change. The one stored
  flag says what the files say beyond the name agrees; the API names it `lengthMatches` for a
  movie and `episodesMatches` for a series. An answer lives
  in `library_answers` by scan origin, so it outlives rewrites: a link (`POST
  /v1/library/items/{id}/link`) is a `link` record below what the files say, so its details then
  come from the catalog, and an ignore (`…/ignore`) leaves the Library and Needs you. `DELETE
  …/answer` takes either back. An answer forgets the title's files' identity digests and passes
  over their folders, as a changed catalog record does. `GET /v1/library/needs` lists each title
  with its candidates and files, `GET /v1/library/search` searches TMDB for Pick a different
  title (503 without a key), and the status counts `needsYou`. The API says which facts agree;
  the sentence is the UI's. A candidate's poster is adopted by the image service under the
  title, and the API leaves it out until its bytes are here.
- **The Library page** (step 10c, [`design/library`](../../design/library/README.md) variant D)
  is an admin nav item after Guide (`/library`, and first in the phone's More list). It shows one
  line from `GET /v1/library/status`: caught up, adding with its step, share and time left
  ("Adding your media · Step 1 of 3 · 21% · about 9 min left"), paused, checking the folders on a
  quiet pass, a held removal or an offline folder with Check again, or dead air before any folder
  is chosen. An import still running for the other folders stays on the line after a held removal
  or an offline folder ("Can't reach your Movies folder · Adding your media · Step 2 of 3 · 34%"),
  and Settings → Library keeps its bar, step, file and time left below the callout; neither says
  "All caught up" beside one. Below it are All · Movies · Shows, a search, and the poster grid, 6, 4 or 3 across by
  window width. The filter and search live in the URL (`?kind=&q=`); the grid loads 60 titles at a
  time as it scrolls, and the `library_scan` and `library_titles` frames keep the line and the
  loaded titles current, so new titles appear and "Getting ready" clears on its own (decision
  35). **Grid:** a household's library is thousands of titles, so on the web only the rows near
  the screen are mounted (`@tanstack/react-virtual`, `LibraryGrid` in `@loomarr/ui`). Each row is
  measured once drawn, the poster's 2:3 box plus its caption, and the next page of 60 is asked
  for once the last rows are mounted. The row holding keyboard focus stays mounted while it
  scrolls away. No phone or TV screen shows the Library yet; there the grid draws every row. A
  title's page
  (`/library/{id}`) shows the poster, year and length or seasons, the rating, genres and
  overview, each season's episodes in a disclosure (specials last), and Technical details. Still
  to come: Collections (step 12); On these channels, once channels name library items (step 15's
  id migration).
- **Needs you on the page** (step 11b). The Library page's strip asks about the first four
  titles that need the household ("X — is this the 1999 movie?", Yes, that's it / Pick a
  different title, or Find it with no guess); `/library/needs` holds one card each with the
  reason line, the other guesses, a TMDB search, Ignore this, and the files behind a disclosure.
  An answer locks in and leaves once seen; one that fails puts the question back. The Library nav
  badge is the status's `needsYou`, the same number as the heading.
- **A title's actions** (step 11c, decision 26). Each title row records what decided which title
  it is: the basis and provider of its highest-ranked catalog id (`match_basis`, `match_app`), and
  for an arr its page there (`match_page`, from the arr's `titleSlug`). `GET /v1/library/items/{id}`
  says `match.by`: `app` (Sonarr or Radarr), `file` (an id in its name, or an NFO), `you` (an
  answer), `search` (the Matcher's match) or `none`. Wrong title? follows it: an arr's title opens its page there ("Sonarr decides
  what this is"); a file's says to rename it or change its NFO, since an answer ranks below what
  the file says; any other opens the same search as Needs you, with Forget my answer when the
  household answered. Update details (`POST …/refresh`) fetches the title's TMDB record at once
  (503 when TMDB fails) and writes the title again, so what its arr and sidecars say now is read
  too. Check files again (`POST …/check`) puts its files' headers back to pending and passes over
  their folders, so they are opened and read again.
- **Technical details** (step 10d, design review point 3). Each scan origin keeps the provider its
  name, description and rating came from in its observation's `extension` (`{"from": …}`), so
  the page says "Description and rating from TMDB. Episode names from Sonarr." (a series takes
  the provider that named most of its episodes). It then says the title match in words
  ("Recognised from Radarr", "You picked it"), and for each movie file its path, size, and once
  its header is read the picture (4K, 1080p HD, 720p HD or standard, "extra-rich colour" for
  HDR), the sound that plays (its default audio stream), each subtitle language, and the
  measured length.
- **Editions** (step 14b-1c, decision 54). A movie file can be a labelled version of its title,
  such as a Director's Cut. Its arr's edition counts first (Radarr's, read from the release name),
  then its name's `{edition-…}`, then its NFO's `<edition>`; a catalog never has a say. The label
  is kept as given, with its words capitalised when it came in one case ("EXTENDED" →
  "Extended"); Theatrical is the default and has no label. Labels that differ only by a trailing
  "Cut", "Edition" or "Version", by case or by an apostrophe are one edition. Each file origin's
  extension notes its edition. Editions are labels, not separate titles: each edition has its own
  copy that plays, picked below, and only a second copy of the same edition is an extra. The
  `{edition-…}` tag never reaches a title's name or its search. The Library does not show
  editions yet: that needs a mock.
- **Extra copies** (step 10e, decision 37). When a movie has several files, one plays in each
  edition: the file its arr lists (each file origin's extension notes `managed`), else the
  sharpest picture, else the biggest file. Each other file is an extra copy: a warning card on the title's page ("An
  extra copy is taking 4.0 GB") with Remove extra copy (`POST …/remove-copy`, 409 for the copy
  that plays). Removing moves the file to `<media folder>/.loomarr-trash/<date>/<its path>`, the
  same share, so it is a rename and can be undone by moving it back; the trash holds a `.ignore`,
  so walks and watches skip it. Loomarr marks the file missing at once, so no removal guard holds
  it. A complete walk of the folder deletes each day's trash 30 days on; a folder there that isn't
  a date is left alone. A read-only folder answers 409, and the card says Loomarr may not be
  allowed to change it.
- **Titles that need a look** (step 10f, decision 24). A title with a file whose header failed
  (`unplayable`) or a movie with an extra copy is a problem: `ListLibraryItems{Problems: true}`
  finds them per title through the file indexes, an extra copy being a `library_sources` row no
  edition plays, and `GET /v1/library/needs` returns them as `problems` (each with `extraBytes`,
  the size of those rows) after the titles nobody can
  identify. Both count towards `needsYou`, the nav badge. Needs you words them in plain words
  ("Night Ferry has 2 episodes that can't be played", "Paper Kites has an extra copy") with
  See why, which opens the title. The title's page gives an unplayable title a warning card with
  Check again, and the shelf marks a ready title that needs a look with a dot (a phone grid has no
  room for a word), which a screen reader hears as "<name>, needs a look".
- **Your collections** (step 12, decision 28). `library_collections` keeps the household's own
  collections and `library_collection_members` their titles in order, as `library_items` ids. A
  title that leaves the library drops out of the list and stays a member, so it comes back in its
  place. `/v1/collections` (admin) lists, reads, makes, changes (name, description and titles
  whole) and deletes them; each says which channels use it: a channel's scope or any rule's
  `what` that names its id. Delete answers 409 while any do. `imported_from` marks one brought
  over from the media server at cutover (step 15). `/v1/library/collections` lists the same
  collections for a different reader: it is member-readable and returns names and sizes only,
  for the channel scope picker, where a member choosing a collection still goes through
  submit and approve. The two stay separate routes because merging them would either open the
  titles and the edits to members or close the picker to them. A channel's collection scope
  matches through the reconcile's membership index, which holds the household's collections
  under their own ids (`LibraryCollectionMemberIDs`, keyed by each title's TMDB and TVDB ids).
- **Ready-made sets** (step 12b, decision 28). A movie's TMDB record names the TMDB collection it
  belongs to; the scanner keeps that as `library_items.set_key` and asks the catalog cache for the
  set's own record (`tmdb_collection`), which lists every movie in it in release order. The
  `/v1/collections` list also returns `sets`: each set the library has a movie of, with its owned
  movies in the set's order and how many it has in all. `POST /v1/collections/sets/{id}/copy`
  makes the household's own collection from one. A set can't be changed, only copied.
- **The Collections tab and editor** (steps 12c–12d). `/library/collections` is the Library's
  fourth tab: Yours, with New collection and Edit, then Ready-made sets with Make my own copy.
  `/library/collections/$id` (and `/new`) edits the name, the description and the titles in order,
  saved together; a row moves by its grip with a pointer or the keyboard, and Add a title searches
  the library. Delete waits, with the reason, while a channel uses the collection.
- **The wizard's Library step** (step 13, decision 30). "Where's your media?" follows Connections
  on either playout path. It shows the folders found through the media server, Sonarr and Radarr
  (the Settings → Library folder cards), with Add a folder and Check again. Choosing one starts
  the import, and the step shows its progress without waiting for it. Continue needs one folder
  in use that Loomarr can open (status `ok`). Until the media server's catalog retires (step 16),
  the step is optional and Skip for now is the way past. On the Tunarr path, Tunarr's own wiring
  step follows as "Tunarr library".
  - Sonarr and Radarr are connected on this step, not on Connections. Arriving with either one
    unset, the step calls `POST /v1/setup/arrs-from-requester`. That reads the Sonarr and Radarr
    Seerr uses and saves each unset one's address and key: Seerr's own address first, its
    external one second, the first Loomarr reaches winning. If none answers, the first is saved,
    so the operator corrects only the address. A set or env-pinned connection is never changed.
  - With an arr connected and no `server.public_url`, the step asks for Loomarr's address,
    which the arrs' webhooks need.
  - The arr probe treats a 200 that isn't JSON as a login page in front of the arr, not the arr.
- **The cutover** (steps 14–15, decision 8; deleted in step 16). A parity gate paired the
  scanner's files with every file the media server listed, by path through the path map, and
  checked their identity and durations. An id map paired each media-server item id with the
  library item its file was written as (a series through its episodes), and a rewrite then
  pointed every stored id at its library item: titles, airings, channels' slots, scopes, rules and
  lineup membership, and open proposals. An id that didn't pair kept its value. The media server's
  hand-made collections became household collections (`library_collections.imported_id` names the
  one each came from). All of it ran on the release before step 16 and was then deleted with
  `library_id_map` (migration 205).

  Readers learned library item ids before the rewrite turned stored ids into them. Playout read first:
  a programme keyed by a library item id plays that item's own playing file, with no media-server
  call (`LibraryPlayingFile`); any other id resolves through the media server as before. Item
  metadata follows: the guide's descriptions, a lineup's durations and a proposal's outlook read a
  library item's overview, genres, year, rating and runtime from the tables (`LibraryItemFacts`).
  The runtime is the playing file's own duration, and an episode takes its show's genres. A
  lineup's series expands from the tables too (`LibraryEpisodes`): the episodes with a present file
  to play, in season and episode order, under the media server's listing rules (an episode that
  could not play is dropped, editorial evidence is bounded). A library series with nothing to play
  expands to nothing; it never falls back to the media server. A channel's derived format (4K,
  HDR) and a still's tone mapping read a library item's video from its playing file's header as
  the scanner read it (`LibraryPlayingVideo`), with no media server or inventory import.
- **Writers name library items** (step 15c). The availability scan, discovery's presence and the
  channel engine's rating heal ask the Library first (`LibraryTitles`, `LibraryTitleByProviderID`:
  movies and series with a present file, by their TMDB and TVDB ids), so a title that confirms or
  is found owned records its library item id. Since step 16, all three ask the Library alone. The
  ids already stored are the rewrite's.
- **Collection scopes read the household's collections** (step 15e, household-only since step
  16). The scope.collections picker and the reconcile's membership index list the household's
  collections alone, so an imported collection is known only by the household's id the rewrite
  pointed scopes at. The media server's hand-made collections were imported by the rewrite and its
  TMDB ones are the Library's ready-made sets, so it is never asked.
- **Title search reads the Library** (step 15e, Library-only since step 16). Like the collections
  picker and channel ideas, it needs no media server: only TMDB search waits on its key.
  Library-scope search, which `/v1/search` and the suggester's tools share, answers with the
  Library's movies and series by name (`SearchLibraryTitles`: an exact name, then one starting with
  the term, then the rest), under their library item ids and with their facts. A term the Library
  has no title for is no results rather than an error.
- **The provisioning backstop reads the Library** (step 15e, Library-only since step 16). Its last
  look before a title is given up as unavailable confirms one the Library holds, under its library
  item id.
- **Channel ideas read the Library** (step 15e, Library-only since step 16). Home's channel ideas
  draw from the Library's movies and series with their year and genres (`LibraryTitles`,
  `LibraryItemFacts`).
- **The filler library source reads a folder** (step 15d, decision 36). A `library` filler source
  names one of the household library's folders and is copied from, never drained: each clip file
  under it is copied into the filler watch folder for intake to hash and file, skipping a clip the
  catalog already holds (#1605), so a re-scan of an unchanged folder copies nothing. A row that
  still names a media-server library is rewritten on the next filler sync to its folder: the
  library's folders as the media server reports them, through the path map, with a row for each
  extra folder. A name with no mapped folder is logged once and does no work.
- **Playout reads only the Library's files** (step 16). A programme, its still, its first-play
  measurement and its channel's codec vote all read the library item's playing file
  (`LibraryPlayingFile`). An item whose file is gone airs the offline card. The media server's
  stream URL, its per-item path lookup and the `library_item_paths` cache are deleted (migration
  194). `library.path_map` now only helps folder discovery.
- **A series expands only from the Library** (step 16). A lineup's series reads the Library's
  episodes (`LibraryEpisodes`) and nothing else: an id that is no library series expands to nothing,
  with no media-server call. An episode needs a non-blank library item id, a positive duration and
  valid season and episode numbers, or it is dropped. Its editorial evidence (rating in `(0,10]`,
  an overview up to 2,048 code points, up to 16 tags) is repaired tolerantly: an invalid field
  becomes unavailable without discarding the episode. The read is three indexed queries: the
  household library's largest show (948 episodes) answers in under 10ms, so the media server's
  episode cache (`series_episodes`, its `channel-maintenance` refresh and `episodes.max_age`) is
  deleted (migration 195). The engine's five-second memo still collapses the repeats within one
  layout.
- **Item metadata reads only the Library** (step 16). The guide's descriptions, a lineup's
  durations and a proposal's outlook read `LibraryItemFacts` and nothing else; an id that is no
  library item has none. The guide's descriptions no longer wait on a media server being
  connected.
- **Titles read only the Library** (step 16). Title search, discovery's and movie collections'
  in-library marking, approval additions, the proposal outlook, the rating heal and the backstop's
  last look all answer from `SearchLibraryTitles` and `LibraryTitleByProviderID`; the media server
  is never asked. Approval additions and the proposal outlook no longer need one connected. The
  availability scan (`LibraryTitles`), channel ideas and the collection picker and index read only
  the Library and the household's collections too.
- **Stream facts read only the Library** (step 16). A channel's derived format, a still's tone
  mapping and audio selection read the playing file's header as the scanner read it
  (`LibraryPlayingVideo`, `LibraryAudioTracks`). The media-server importer that filled the
  inventory from an item's media information, with its 24-hour re-import, is deleted: an id that
  is no library item is unknown, which is SDR and the first audio track. A local file the Library
  has not read is probed once and the measurement kept under its stat revision, as before.
- **The semantic eval reads the Library** (step 16). `make eval` and `make eval-cert` ground
  titles and certify schedule evidence against a Loomarr database (`LOOMARR_LIBRARY_DB`,
  `sqlite://<path>`, opened without migrating and only read) through the same reads production
  uses: `catalog.LibrarySearch`, `catalog.LibraryOwnership`, `LibraryItemFacts` and
  `channels.LibraryEpisodeResolver`. A schedule evidence snapshot therefore names library item
  ids, so one frozen against the media server's ids drifts and must be frozen again.
- **The media-server client keeps no catalog reads** (step 16). `library.Client` lost its search,
  presence lookup, bulk scan, episode and metadata reads; what it still asks the media server is
  sign-in and users, folder discovery, filler and the live tuner. The demo stand-in serves only
  those. Tests drive the catalog through `catalogfixture.ScriptedLibrary`
  or the Library's own tables, and the media-server double no longer scripts titles.

## Search

Loomarr builds no search index (decision [0003](decisions/0003-federated-search.md)). `GET
/v1/search?q=&scope=library|tmdb|all` fans out to the Library's titles (by name) and TMDB's
`/search/multi` and returns unified `Candidate` results with an `in_library` flag. Clips use `LIKE`.

- **Scopes follow live configuration.** `all` searches whichever corpora are configured now and
  returns 501 only when neither is. Saving or clearing a connection changes the next request.
- **Results are a bounded blend.** After identity deduplication, a quarter of the page (at least one
  row) is reserved for outside-library candidates; the rest prefers playable library items, and an
  undersubscribed side yields to the other. Upstream relevance order is kept within each side. This
  controls what the model is shown, not what is approved.
- **Structured discovery** (no `q`) takes media type, genres, TMDB keywords, year range, original
  language, origin country, runtime and vote bounds, and person or network names. Keywords resolve
  through `/search/keyword`; people through `/search/person` (one exact match); a network through TMDB's
  daily network-id export. Malformed, unresolved or ambiguous qualifiers fail the call instead of
  broadening it. Mixed movie and series discovery splits the TMDB pool between types.
- `GET /v1/movie-collections` expands up to 24 movie keys into their TMDB collection rosters for review;
  it never builds a lineup by itself.
- Channel, proposal and help filtering stay client-side at household scale.

## Tests that pin this

Formerly `design.md` §19.

- **Library conformance:** Emby and Jellyfin flavors with a mock transport, each sending its correct
  auth header. Deleting a scheduled item from the mock library makes the sweep flag drift and
  substitute.
- **Search:** `/v1/search` fans out to the mock media server, mock TMDB and the clip store;
  `in_library` flags are correct; a member can search, but adding a missing title still routes
  through submit and approve; scope filters are honoured.
