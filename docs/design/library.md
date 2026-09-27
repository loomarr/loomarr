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
