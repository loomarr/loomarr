# Filler evidence retrieval: provider and authority review

**Reviewed 2026-09-19.** This note records the retrieval boundary for contextual filler
enrichment. It is a dated provider/terms snapshot, not legal advice and not a promise that a
provider's terms will remain unchanged.

## Decision

Loomarr will own retrieval. A configured language model may interpret a bounded evidence packet,
but it does not receive a general web-search tool and does not choose which hosts Loomarr contacts.

The implementation uses direct, attributable public knowledge APIs and exact source metadata.
It sends only metadata that already came from a public remote source; local filenames and private
library metadata are not web-search queries. Results remain **context suggestions**, never verified
clip facts, admission authority, or scheduling inputs. The UI must say “Likely” and link to the
underlying pages.

The default search is a bounded fan-out rather than one literal-title lookup. Loomarr derives a
small deterministic subject query from the public title, searches English Wikipedia for general
campaign context, searches Archive.org's public metadata index for related historical items, then
de-duplicates and caps the combined evidence before model interpretation. Partial provider failure
does not discard attributable evidence returned by another adapter. No adapter may turn a result URL
into a second arbitrary fetch.

Commercial search remains an adapter seam, not a dependency of the domain model. Before enabling
one, its current terms must permit Loomarr's intended retention and display of evidence. Search
snippets are discovery aids; the original page is the preferred durable citation.

## Provider findings

### Wikimedia

MediaWiki exposes search, page metadata, and plain-text extracts through a documented API. Wikimedia
requires a meaningful User-Agent and asks automated clients to limit concurrency, cache responses,
and respect `maxlag`. Its text is reusable under CC BY-SA with attribution. This makes it a suitable
first adapter for low-volume product/campaign discovery, provided Loomarr retains the page URL and
does not present an encyclopedia statement as proof that the archived bytes are the exact advert
described.

- [MediaWiki API etiquette](https://www.mediawiki.org/wiki/API:Etiquette)
- [MediaWiki search API](https://www.mediawiki.org/wiki/API:Search)
- [Wikimedia Foundation Terms of Use](https://foundation.wikimedia.org/wiki/Policy:Terms_of_Use)

### Internet Archive

Internet Archive exposes public item metadata and metadata search through documented JSON APIs. The
search adapter uses only the fixed `archive.org/advancedsearch.php` endpoint, strips query-language
operators from public title text, requests a small fixed field set, and cites canonical
`archive.org/details/{identifier}` pages. A related item is corroborating context, not proof that two
uploads contain the same advert; interpretation must retain that uncertainty.

- [Internet Archive item-search APIs](https://archive.org/developers/index.html)
- [Internet Archive Metadata API](https://archive.org/developers/metadata.html)

### YouTube search

The official YouTube Data API supports relevance search, but it requires a Google Cloud project/API
key and enforces project quota. Loomarr will not hide that setup behind the ordinary filler flow or
scrape YouTube's consumer search page. The exact title, description, uploader and canonical page
captured by yt-dlp during acquisition remain available as public item evidence. A future advanced
adapter may use an explicitly configured YouTube Data API credential without changing the research
module's external interface.

- [YouTube Data API overview](https://developers.google.com/youtube/v3/getting-started)
- [YouTube `search.list`](https://developers.google.com/youtube/v3/docs/search/list)

### Brave Search API

Brave offers an independent-index API, documents rate-limit headers, and currently prices web search
at $5 per 1,000 requests with monthly credits. However, its Search API Terms prohibit storing,
caching, or creating a database from Search Results except transient operational storage unless a
separate agreement permits it. That is a poor fit for Loomarr's durable evidence and citation model.
Brave may be reconsidered as transient discovery only after an original-page fetch and a fresh terms
review; Loomarr must not persist the public-plan result payload as evidence.

- [Brave Search API](https://brave.com/search/api/)
- [Brave Search API terms](https://api-dashboard.search.brave.com/documentation/resources/terms-of-service)
- [Brave rate limiting](https://api-dashboard.search.brave.com/documentation/guides/rate-limiting)
- [Brave Search API privacy policy](https://api-dashboard.search.brave.com/privacy-policy)

### Exa

Exa's public terms restrict downloading, copying, publishing, or distributing information obtained
through the service except temporary browser caching or where expressly permitted. The public terms
therefore do not support making its response the default durable evidence record. Exa advertises
enterprise zero-data-retention arrangements, but that is a separate commercial decision.

- [Exa pricing](https://exa.ai/pricing)
- [Exa Terms of Service](https://exa.ai/assets/Exa_Labs_Terms_of_Service.pdf)
- [Exa privacy policy](https://exa.ai/privacy-policy)

### Tavily

Tavily currently provides a free monthly allowance and a search API returning processed snippets.
Its public terms distinguish output from the service but do not clearly grant the durable retention
and redistribution rights Loomarr needs for a local evidence ledger. Its terms also allow broad use
of customer input to improve services. Do not make Tavily the default authority without written
clarification or appropriate commercial terms.

- [Tavily pricing](https://help.tavily.com/articles/8816424538-pricing)
- [Tavily Search API](https://help.tavily.com/articles/4840311948-tavily-search-api)
- [Tavily terms](https://www.tavily.com/terms)

### OpenRouter web search

OpenRouter's server-side web-search tool can supply citations and domain filters, but it combines
retrieval and model interpretation behind one provider boundary. A live diagnostic also showed that
one route ignored the requested domain restriction and promoted campaign-level context too strongly.
Loomarr will keep using OpenRouter as a model transport when configured, while keeping retrieval and
evidence validation in Loomarr.

- [OpenRouter web search](https://openrouter.ai/docs/features/web-search)
- [OpenRouter tool calling](https://openrouter.ai/docs/features/tool-calling)

## Traffic and safety policy

Loomarr does not attempt to look human or bypass anti-bot controls. Retrieval adapters must:

- use documented APIs, an identifying User-Agent, HTTPS, and provider rate-limit signals;
- cap results, response bytes, redirects, request time, concurrency, and clips per pass;
- retry a failed structured provider only on a later scheduled pass, with bounded backoff and no
  CAPTCHA bypass; reserve optional general-web fallback durably at most once per Clip input revision,
  including failed requests, so provider failure cannot create an unattended cost loop;
- bind completed work to normalized plan and adapter versions so an unchanged Clip is not retrieved
  again; a future shared query cache may remove duplicate lookups across distinct Clips;
- contact only adapter-owned hosts; arbitrary model- or user-returned URLs never become fetch targets;
- send public remote-source title/description only, never a local path or private library text;
- preserve attribution and distinguish campaign context from exact-item evidence.

## Product inference

For a title such as “Tootsie Pop Classic Commercial,” the subject query can identify the long-running
campaign while Archive metadata can surface independently titled historical uploads. That is useful
context, but neither proves that the exact archived cut is the 1970 debut. Loomarr may therefore show
“Likely 1970s · United States” with citations and uncertainty. It may not write `era=1970` or
`country=US` into the verified axes until exact item/content evidence supports those facts or the
operator confirms them.

## Structured authorities and geography-aware catalogs (2026-09-20)

### Wikidata

Wikidata documents the fixed entity endpoint `https://www.wikidata.org/wiki/Special:EntityData/{QID}.json`
and public Query Service at `https://query.wikidata.org/sparql`. Entity lookup is deterministic once
a Q-ID is known; a bounded SPARQL query can project label, description, and the entity URI without
fetching an arbitrary result page. Use a fixed query shape with language filters, `LIMIT`, and a
short timeout. Wikidata requires a meaningful `User-Agent`, gzip/deflate, low concurrency, and
stopping on 429 while honoring `Retry-After`; MediaWiki requests should use `maxlag`. This is a
subject/entity authority, not an advertisement catalog or media license. The entity URI is the
canonical public URL and should be retained with attribution and retrieval time.

- [Wikidata data access and best practices](https://www.wikidata.org/wiki/Wikidata:Data_access/en)
- [Wikidata Query Service](https://www.wikidata.org/wiki/Wikidata:SPARQL_query_service)
- [MediaWiki API etiquette](https://www.mediawiki.org/wiki/API:Etiquette)

### Library of Congress (US)

The LoC JSON API uses fixed host `www.loc.gov` and requires no API key. The documented shape is
`GET https://www.loc.gov/search/?q={terms}&fo=json&at=results&c={small-cap}`. Results provide title,
summary bibliographic fields, and an `id`/`url` canonical LoC page. A result's own path may be
requested with `?fo=json&at=item,resources`; arbitrary external URLs must not be followed. LoC
recommends no more than 1,000 items/page and caps deep paging at 100,000 results. Its documented
JSON rate is 20 requests/minute; excess traffic can be blocked for an hour, and 429/CAPTCHA can
occur under load. No identifying header is mandated in the cited API docs, but Loomarr should send
its descriptive User-Agent and honor backoff. It is a useful US-first context/catalog source, not
a complete national bibliography.

- [LoC endpoints and search query](https://www.loc.gov/apis/json-and-yaml/requests/endpoints/)
- [LoC result fields and canonical URLs](https://www.loc.gov/apis/json-and-yaml/responses/search-results/)
- [LoC rate and page limits](https://www.loc.gov/apis/json-and-yaml/working-within-limits/)

### Geography-specific candidates

Australia has an official, versioned Trove API v3 at `https://api.trove.nla.gov.au/v3`, with an
OpenAPI specification. Trove is explicitly intended for non-human traffic and can return bounded
record metadata and public record URLs. It is not a no-configuration default: API-key, quota, and
current terms must be pinned and reviewed before enabling an AU adapter. This note does not claim an
unverified quota or authorize acquiring a key.

- [Trove API v3 announcement and OpenAPI link](https://trove.nla.gov.au/sites/default/files/2023-02/Introducing%20Trove%20API%20v3.pdf)
- [Trove API for non-human traffic](https://trove.nla.gov.au/blocked.htm?partId=nla.obj-96148230)

For GB and CA, this review found no equally clear, stable, unauthenticated national-catalog JSON
search contract suitable for selection solely from installation country. Do not guess at British
Library or Library and Archives Canada endpoints, and do not create an unbounded provider registry.
Open Library is global, but its official guidance limits it to low-volume human-facing discovery,
requires an identifying User-Agent plus email (1 request/sec default, 3/sec identified), and says
it is not a backend for third-party services; it is not a geography-selected v1 provider.

- [Open Library API scope and rate guidance](https://openlibrary.org/developers/api)

### v1 recommendation

Ship Wikidata and LoC as fixed structured providers, selected by adapter configuration rather than
model/user hosts. Keep AU Trove behind an explicit API-key setting and later terms/quota review.
For GB and CA, return no country-specific catalog result in v1. Normalize every provider result to
`{title, snippet, canonical_url, provider, retrieved_at}` with bounded count/bytes and no follow-up
fetch of `canonical_url`.
