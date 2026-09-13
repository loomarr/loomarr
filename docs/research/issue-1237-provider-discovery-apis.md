# Issue #1237: first-party provider discovery APIs

Date: 2026-09-13

## Executive finding

Archive.org is suitable for a low-config, server-side typeahead source picker: its
Advanced Search endpoint searches public item metadata and returns JSON (or other
formats), and the item Metadata API resolves an exact identifier without credentials.
YouTube has no equivalent credential-free first-party name search. The official
`search.list` endpoint requires Google API credentials and quota, but Loomarr's existing
yt-dlp runtime can perform a credential-free, best-effort video search and return stable
channel identities. Loomarr can turn those hits into channel suggestions after a short
debounce, while retaining deterministic manual URL input for channels and playlists.

## Archive.org

* **Discovery/typeahead:** the official Item Search API documents
  `advancedsearch.php` as the traditional search API over item metadata, with JSON
  output and paging (for example,
  `https://archive.org/advancedsearch.php?q=subject:palm+pilot+software&output=json&rows=100&page=5`).
  It supports metadata queries and has a 10,000-result limit for sorted paging. The
  same documentation describes the cursor-based Scraping API for deeper paging.
  Source: [Internet Archive Item Search APIs](https://doc-tools.readthedocs.io/en/ia-test-gsod/item-search-apis.html).
* **Collections:** collection membership is metadata-backed: the official metadata
  documentation says assigning an item to a collection makes it discoverable by
  browsing that collection. Search can therefore constrain queries using collection
  metadata/query syntax, but a collection is not a separate credential-free provider
  directory. Source: [Internet Archive Metadata](https://internetarchive.readthedocs.io/en/stable/metadata.html).
* **Exact resolution/canonicalization:** `GET https://archive.org/metadata/{identifier}`
  returns an item’s metadata for the exact identifier. Identifiers are globally unique,
  composed of alphanumerics, `_`, and `-`, and cannot be changed once defined. The
  canonical public item URL is consequently `https://archive.org/details/{identifier}`;
  input handling should extract the identifier from `/details/` (and reject ambiguous
  or non-item URLs), then resolve via MDAPI. Source: [Item Metadata API: Read](https://doc-tools.readthedocs.io/en/ia-test-gsod/md-read.html)
  and [Archive.org Metadata](https://internetarchive.readthedocs.io/en/stable/metadata.html).
* **Credentials:** the MDAPI documentation states that most returned metadata is
  publicly available; authorization is needed for certain user JSON fields and all
  writes. Read-only public item resolution is therefore credential-free. Source:
  [Item Metadata API](https://doc-tools.readthedocs.io/en/ia-test-gsod/metadata.html).

## YouTube

* **Discovery/typeahead:** the official `search.list` method returns resources matching
  a query and can restrict `type` to `channel` or `playlist`; results include IDs and
  snippets. Source: [Search](https://developers.google.com/youtube/v3/docs/search) and
  [Search: list](https://developers.google.com/youtube/v3/docs/search/list).
* **Credentials and quota:** Google’s getting-started documentation says applications
  must obtain authorization credentials. `search.list` uses the API key/developer
  credential path for public searches and has a quota cost of 100 calls per day in the
  documented default bucket (the quota calculator describes the per-call cost and
  daily limits). This is low-config only if Loomarr supplies and manages a project key;
  it is not credential-free. Sources: [YouTube Data API getting started](https://developers.google.com/youtube/v3/getting-started),
  [Search: list](https://developers.google.com/youtube/v3/docs/search/list), and
  [Quota calculator](https://developers.google.com/youtube/v3/determine_quota_cost).
* **Known channel/playlist resolution:** with a channel ID, `channels.list` can return
  channel metadata and its uploads playlist ID; `playlists.list` can then retrieve a
  channel’s playlists. These are API calls and inherit the credential requirement.
  Source: [Implementation: videos](https://developers.google.com/youtube/v3/guides/implementation/videos)
  and [Implementation: playlists](https://developers.google.com/youtube/v3/guides/implementation/playlists).
* **Credential-free first-party options:** YouTube’s official public URL forms are
  useful for manual input validation, not search: canonical ID URLs use
  `/channel/{channelId}`; handle URLs use `/@handle`; legacy `/c/` and `/user/` URLs
  continue to redirect where applicable. The Help Center states handles are unique and
  their URL is automatically created. A validator can recognize these URL shapes and
  preserve the submitted URL, but resolving a handle/custom URL to a stable channel ID
  is not documented as a credential-free API operation. Sources: [Learn about YouTube
  handles](https://support.google.com/youtube/answer/11585688) and [About YouTube
  channel URLs](https://support.google.com/youtube/answer/6180214).
* **oEmbed/feeds boundary:** official YouTube developer documentation lists the IFrame
  Player API for playback, but does not provide a credential-free channel/playlist
  discovery endpoint. oEmbed or RSS-style URLs, where usable, can be treated only as
  best-effort metadata/manual validation and must not be represented as authoritative
  typeahead search without a documented first-party contract.

## Recommended product boundary

1. Implement Archive.org typeahead against `advancedsearch.php`, returning an item
   identifier, title, collection/mediatype metadata, and canonical `/details/{id}` URL.
2. Implement provider-neutral manual URL parsing and validation separately from
   typeahead. For Archive.org, extract and resolve the exact identifier with MDAPI.
3. For YouTube, accept and validate explicit `/watch?v=`, `/playlist?list=`,
   `/channel/`, and `/@handle` URLs. A bounded yt-dlp search may additionally suggest
   channels without configuration, provided the UI calls it best-effort and always
   retains URL input as the fallback.
4. Keep “suggestion” distinct from “resolved/verified source”: a search result is a
   candidate, while URL/identifier resolution must independently fetch and validate
   the target metadata.

## Source list

All sources above are first-party documentation from the Internet Archive, Google
Developers, or YouTube Help. No secondary sources were used.

## Credential-free YouTube alternatives (follow-up checkpoint)

### yt-dlp search

Loomarr already vendors yt-dlp, whose official README documents the `ytsearch:` search
prefix and YouTube search support. It can therefore perform a submitted search without
a YouTube Data API key, and its extractor can emit flat result metadata/IDs. Source:
[yt-dlp README](https://github.com/yt-dlp/yt-dlp/blob/master/README.md).

This is not a good per-keystroke typeahead primitive. Each search invokes YouTube
extractor/network work; yt-dlp's own issue tracker records a 10-result search taking
about 16.5 seconds in one report and scaling with result count. That issue is not a
contract or benchmark for current versions, but it demonstrates the latency shape and
why the operation should be debounced and submitted, not called for every keypress.
Source: [yt-dlp issue #1865](https://github.com/yt-dlp/yt-dlp/issues/1865).

yt-dlp's maintained extractor guidance also documents guest/account request-rate limits,
warnings that YouTube increasingly requires externally supplied PO tokens for some
formats/features, and that OAuth login no longer works with yt-dlp. It recommends
delays for rate limiting and cautions that account use can be banned. Search may work
without credentials today, but availability, result shape, latency, and bot challenges
are operationally unstable and require upgrade/observability paths. Source:
[yt-dlp extractor notes](https://github.com/yt-dlp/yt-dlp/wiki/Extractors).

### Invidious and Piped

Both projects expose search-like proxy APIs by having a self-hosted service talk to
YouTube’s undocumented/internal clients. Invidious source shows a `/youtubei/v1/search`
request and a URL-resolution endpoint, while its API routes expose search results.
Sources: [Invidious YouTube backend](https://github.com/iv-org/invidious/blob/master/src/invidious/yt_backend/youtube_api.cr)
and [Invidious channel routes](https://github.com/iv-org/invidious/blob/master/src/invidious/routes/api/v1/channels.cr).

They are not credential-free *first-party YouTube APIs*: they are independent projects
whose availability depends on YouTube behavior and on operating an instance. Invidious
explicitly warns public instances are untrustworthy, says the public list is short due
to recent YouTube issues, recommends hosting at home, and requires uptime/staleness and
stability criteria for listed instances. Source: [Invidious instance guidance](https://github.com/iv-org/documentation/blob/master/docs/instances.md).
Piped similarly documents an instance directory and says self-hosting the backend is
available; its backend is an alternative built on NewPipeExtractor. Sources:
[Piped instances](https://github.com/TeamPiped/Piped/wiki/Instances) and
[Piped backend README](https://github.com/TeamPiped/Piped-Backend/blob/master/README.md).

Using a public instance would add third-party privacy, uptime, throttling, and supply-
chain dependencies. Self-hosting adds deployment, proxy/IP reputation, update, and
monitoring cost. These can be viable optional integrations for an operator who accepts
that trade-off, but should not be the default authoritative provider path.

### YouTube terms and feeds

YouTube’s Terms prohibit accessing the service using automated means except as permitted
by the service/API terms, and the API Services Terms require compliance with the API
Terms of Service and policies. This makes HTML scraping or reverse-engineered internal
endpoints a policy-sensitive choice rather than a stable first-party contract. Sources:
[YouTube Terms of Service](https://www.youtube.com/static?template=terms) and
[YouTube API Services Terms](https://developers.google.com/youtube/terms/api-services-terms-of-service).

RSS/feed URLs are useful for a known channel’s uploads (when accepted by YouTube), but
they do not provide query-based channel/playlist discovery. Treat them as post-resolution
refresh inputs, not typeahead. Likewise, `ytsearchN:` is best classified as a debounced
submitted search: one request after the user submits or pauses, with bounded `N`, caching,
timeouts, and explicit “may be unavailable” UI. An interactive typeahead would multiply
requests while typing and magnify latency, throttling, bot-detection, and policy risk.

### Updated recommendation

Keep Archive.org as the default credential-free typeahead. Use a server-side, debounced
and bounded yt-dlp `ytsearchN:` lookup for credential-free YouTube channel suggestions,
with caching, cancellation, timeouts, rate limits, and a manual URL fallback. Search hits
are candidates, not authoritative identity resolution: Loomarr must deduplicate their
`channel_id`/`channel_url` values and validate the selected channel before registration.
Do not depend on public Invidious/Piped instances. If this best-effort path later proves
too unreliable, the official Data API remains an optional predictable integration rather
than a requirement for the normal user.

### Repository-specific live validation

Loomarr already captured `ytsearch` behavior in
[`internal/testkit/fixtures/ytdlp/FINDINGS.md`](../../internal/testkit/fixtures/ytdlp/FINDINGS.md).
On 2026-09-13, the current pinned/host version (`yt-dlp 2026.08.19`) was also run against
`ytsearch5:vintage television commercials` in flat, listing-only mode. It returned five
results in about 1.3 seconds. Every result included a channel name, `channel_id`,
`channel_url`, `uploader_id`, and `uploader_url`. That is sufficient to collapse video
matches into unique channel suggestions without downloading media or asking the operator
for credentials. It does not discover playlists by name; manual playlist URL entry
remains necessary.
