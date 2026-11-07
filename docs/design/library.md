# Library and inventory

Formerly `design.md` §5 (media inventory, cached series episodes), §6 (Emby and Jellyfin) and §7.2
(search). How Loomarr reads the household media server and what it keeps of it.

## The media server contract

Emby and Jellyfin share one adapter; `library.flavor` selects the auth header, never the logic.

- **Lookup:** `GET /Items?Recursive=true&AnyProviderIdEquals=<tmdb.<id>|tvdb.<id>>&IncludeItemTypes=<Movie|Series>&Limit=1`.
  Present means `Items` is non-empty. A series counts as in-library when the show exists. Provider
  name casing differs across server versions: check it first when a known title comes back empty.
- **Auth is a header, never the `api_key` query parameter** (it leaks into logs): Emby
  `X-Emby-Token`, Jellyfin `Authorization: MediaBrowser Token="…"`.
- **Bulk scans** drive availability ([acquisition](acquisition.md#state-machine)): `RecentlyAdded(since)`
  sorts by `DateCreated` with `MinDateLastSaved`, and `AllItems()` is the same query without it. Both
  return provider ids, so a scan item produces the same `provision.Key` as a Title.
- **Collections** (BoxSets) back `scope.collections`: `Collections()` lists them, and
  `CollectionMembers(id)` returns members with full provider ids through `ParentId`. `ChildCount` is not
  returned by Emby and is optional. Real libraries hold hundreds of collections, most of them
  tool-generated one-per-franchise groupings, so any picker must rank hand-made ones first.
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
  read through `inventory.AnalysisReader`. Playout never asks the media server at airtime: an
  unmeasured source gets one stream-facts probe on first play.
- `ResolveSource` returns identity, revision, safe observations and a protected locator. Inventory
  owns what a source is, not the credentials for opening it.

## Header facts

`internal/mediaheader` is the only code that parses container bytes (decision
[0046](decisions/0046-loomarr-scans-its-own-media-files.md)). It reads a file's duration, streams
(codec, size, frame rate, pixel format, colour and HDR, field order, languages, default and forced
flags, channels, sample rate) and chapters from the Matroska/WebM header or the MP4/MOV `moov`,
plus the keyframe index from Matroska Cues or the MP4 sample tables, which `mediameasure` uses.

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

`internal/metadataprovider` is the registry of systems that already know what a media file is
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
  when the folder holds only that video, or `tvshow.nfo` in a series folder. It reads typed `uniqueid`
  elements, guide text, `mpaa` and numbering. The default `uniqueid` names its catalog. Reads are capped
  at 1 MiB, refuse symlinks and any DOCTYPE, accept UTF-8, Latin-1 and Windows-1252, and drop malformed
  ids.
- **TMDB** (`metadataprovider/tmdb`) is a catalog over the shared `internal/tmdb` client. A movie or
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
  its bytes, and only regular files: the image service imports one, keyed by its content, the first
  time something shows it. Season art is not read; Loomarr has no season items.
- **Arr requests never follow a redirect**, so the API key cannot reach another host.

**Precedence** lives in one place, `metadataprovider/precedence`, and ranks bases and catalogs, never
providers. A field takes the highest-ranked source that has a value for it. Numbering and the file list
move whole.

| Family | Order |
| --- | --- |
| Technical | measured, header, manager |
| Identity and numbering | manager, id in a name, sidecar, confirmed link |
| Descriptive (text, genres, keywords, community rating, collection) | a service (catalog or manager) whose text comes from TMDB for movies and series, or TVDB for episodes; then other services; then sidecars, which never outrank a service |
| Certification | the strictest grounded rating on the US ladder |
| Artwork | sidecar, catalog, manager, per artwork kind |

Certification uses the audience ladder that enforcement uses. Ratings in another country's system and
unrated values ground nothing, and a resolver for a country other than the US is refused. An episode
that no grounded source rates inherits its series' strictest rating. One that a source rates keeps
that rating, whether it is stricter or not.

## Media folders

`internal/library/scan` keeps the folders Loomarr reads media from (decision
[0046](decisions/0046-loomarr-scans-its-own-media-files.md); #2151 decision 6), shown on
**Settings → Library**.

- **Discovered, then confirmed.** One pass asks the media server (`/Library/VirtualFolders`, video
  libraries only) and Sonarr and Radarr (`/api/v3/rootfolder`, whatever requester is chosen) for
  their folders. Each folder maps to a local folder: the same path when Loomarr can open it, else
  the `library.path_map` translation, else the one mount whose folder carries the same trailing
  names. A root is always absolute; an ambiguous or missing folder stays unmapped. A discovered
  folder waits for the maintainer's **Use this folder**; one typed by hand is in use at once and
  must open.
- **One path map.** `library_root_paths` holds every authority's name for every folder. A row the
  maintainer edits under "Folders have different names in other apps" is never rewritten by
  discovery; clearing it hands it back.
- **A failing source forgets nothing.** Only an authority that answered drops the folders it no
  longer reports, and only a folder that is neither in use, typed by hand nor named by any mapping
  disappears. Every folder is re-checked on each pass, within 5 s, so an offline NAS shows as
  "Loomarr can't open this folder" rather than hanging the page.
- `GET /v1/library/status` reports `needs_folders`, `folder_offline` or `ok`, with counts.

## Cached series episodes

Expanding a series into episodes costs one media-server call per show, which used to dominate guide
latency. `series_episodes` caches one row per show (episodes plus `fetched_at`); the library stays the
source of truth.

- **Read:** a miss or a stale row falls back to the live call and writes the result back.
- **Refresh:** the `channel-maintenance` job re-enumerates aged rows, only for shows referenced by
  lineups. It is deliberately not hung off `library-scan`, which only visits in-flight acquisitions and
  would never refresh a show that is already available.
- **Playable contract:** every cached episode needs a non-blank library item id, a positive duration and
  valid season and episode numbers. A write that breaks it is rejected whole; a cache row that breaks
  it fails the read, and duplicate identity members are an error, not last-one-wins.
- **Editorial evidence** (rating in `(0,10]`, an overview up to 2,048 code points, up to 16 tags) is
  repaired tolerantly: an invalid or ambiguous field becomes unavailable without discarding the episode.

## Search

Loomarr builds no search index (decision [0003](decisions/0003-federated-search.md)). `GET
/v1/search?q=&scope=library|tmdb|all` fans out to the media server's `SearchTerm` and TMDB's
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
