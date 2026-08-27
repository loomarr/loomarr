# Filler certification source qualification

**Compiled 2026-08-26.** This is the source-selection and rights-evidence decision for issues
[#549](https://github.com/loomarr/loomarr/issues/549) and
[#555](https://github.com/loomarr/loomarr/issues/555). It uses first-party repository policies,
official APIs, and owner-published licences. Counts are dated discovery snapshots, not promises that
the material is semantically correct or legally admissible. No media was downloaded and no model
inference was purchased during this review. This is an engineering provenance policy, not legal
advice.

**Superseded for corpus size and pilot outcome.** The source observations below remain useful, but
the 300-case planning arithmetic predates the independent rights-yield review and executable
statistical contract. The current quota and outcome are recorded in
[`filler-certification-corpus-2026-08-25.md`](filler-certification-corpus-2026-08-25.md).

## Decision

No public source can produce a representative Loomarr certification corpus by itself. The current
holdout needs 446 eligible positives across the six product roles plus 593 invalid and 87 ambiguous
controls, all under independent-cluster and concentration caps. The independent pilot qualified CDC
only; LOC, Prelinger, NASA, and Wikimedia Commons did not meet the common rights-and-relevance gate.
The Blender pilot was already rejected after it failed the ten-distinct-live-candidate precondition.

Public sources therefore remain supplementary. Current consumer advertising, entertainment promos,
real channel bumpers, station IDs, and a sufficiently diverse creator/campaign distribution require
authentic creator/broadcaster agreements; archive proxies and Loomarr-generated imitations are not
representative substitutes.

DVIDS remains excluded. Its metadata and public-domain notices do not repair its product-relevance
failure: military public-affairs video is not an adequate stand-in for entertainment filler.

## Historical sourcing hypothesis

The tables in this section explain what the pre-pilot 300-case plan expected each source to provide.
They are retained as research provenance, not as current certification quotas.

### Positive cases

| Role | Total | Public-source target | Commissioned/direct target | Source plan |
| --- | ---: | ---: | ---: | --- |
| commercial | 35 | 15 | 20 | item-cleared Prelinger/LOC period ads plus modern creator-owned ads |
| promo | 35 | 15 | 20 | NASA/LOC and a small confirmed Commons pool plus entertainment/channel promos |
| bumper | 25 | 0 | 25 | broadcaster/creator masters in real 5/10/15-second forms |
| station ID | 25 | 0 | 25 | broadcaster/creator masters; confirmed Commons files may replace, not expand, this target |
| trailer | 35 | 30 | 5 | item-cleared NASA/LOC/Commons or static open works, plus directly licensed independent works |
| PSA | 35 | 30 | 5 | CDC and item-cleared federal/LOC/Prelinger material plus modern direct masters |
| **Total** | **190** | **90** | **100** | |

The public-source numbers are acquisition targets, not quotas. A lane that fails rights review is
allowed to underfill; the gap moves to direct licensing rather than to a weaker rights rule.

### Invalid and ambiguous controls

| Slice | Cases | Construction |
| --- | ---: | --- |
| programme excerpts | 20 | independently selected spans from rights-approved open works |
| compilations and mixed roles | 15 | authentic approved reels or deterministic assemblies |
| fragments and partial end cards | 15 | bounded cuts from approved masters |
| degraded or corrupt media | 15 | deterministic, recorded transformations |
| non-filler institutional/educational video | 15 | rights-approved LOC/NASA/CDC material |
| adversarial visible, spoken, or metadata instructions | 10 | Loomarr-authored transformations |
| conflicting evidence, including #545 | 10 | contradictory but reproducible evidence packets |
| sensitive, policy-prohibited, or rights-conflicted | 10 | explicit reject/hold cases; never playback authority |
| **Total** | **110** | |

Every derivative remains in its source master's cluster. No film, campaign, advertiser, creator,
near-duplicate family, or derivative family may cross development and holdout splits. Aim for at
least 120 independent positive source clusters, and cap one repository at 25% of positive cases and
one creator/campaign at 10% of any role. These are corpus-diversity controls, not rights rules.

## Source lanes assessed

### 1. Prelinger Archives on Internet Archive: historical ads, promos, and PSAs

**Discovery and media contract.** Discover with Internet Archive Advanced Search using
`collection:prelinger AND mediatype:movies`. Fetch each selected item through
`GET https://archive.org/metadata/<identifier>` and retain the canonical item identifier, raw
metadata response, selected original filename, original/derived flag, byte size, format, MD5/SHA-1,
and stable archival media URL `https://archive.org/download/<identifier>/<filename>`. The Archive's
[Metadata API](https://archive.org/developers/md-read.html) exposes file metadata, and its
[item documentation](https://archive.org/developers/items.html#archival-urls) distinguishes stable
archival URLs from redirected storage hosts. Automated reads must identify themselves, cache, run
serially, and honor `429`/`Retry-After` under the Archive's
[bot policy](https://archive.org/developers/bots.html).

**Current scale.** The 2026-08-26 Advanced Search snapshot returned 10,459 Prelinger movie items
and 1,913 with any `licenseurl`. Narrow field-scoped probes found 127 commercial, 63 public-service,
9 promo, 4 trailer, 4 bumper, and no station-identification candidates. A wider
`title:promo* OR subject:promotional` query returned 90 results, demonstrating that exact query text
must be stored with every count. Search results overlap and titles/subjects are not truth labels.

**Rights authority.** Prelinger says films it makes available were checked for copyright status and
that reuse follows the Creative Commons licence, if any, on the exact detail page
([Prelinger reuse guidance](https://archivesupport.zendesk.com/hc/en-us/articles/360004715031-Prelinger-Archive)).
Internet Archive separately says it does not guarantee item or collection copyright information
and that rights fields are normally supplied by uploaders
([Archive rights guidance](https://archivesupport.zendesk.com/hc/en-us/articles/360014759692-Rights),
[metadata schema](https://archive.org/developers/metadata-schema/index.html#licenseurl)). Therefore:

- collection membership and `licenseurl` are discovery evidence, never approval;
- require a reviewer to bind the exact item, metadata hash, selected media file, licence URI, rights
  prose, publication facts, embedded music/film, attribution, trademarks, and other restrictions;
- accept a first-party CC grant only when the rights holder and work can be traced; independently
  adjudicate public-domain claims and renewal-search prose; and
- do not copy Prelinger's copyrighted descriptions, synopses, or shot lists into a distributable
  corpus.

**Bounded inventory:** 40 rows: 18 commercials, 8 promos, 8 PSAs, and 6 compilation/non-filler
controls. Expect high rights attrition. The existing 331-row `classic_tv_commercials` packet remains
an authentic discovery/hold set, not certification authority.

### 2. Library of Congress: historical advertising and federal controls

**Discovery and media contract.** Use the National Screening Room collection endpoint with
`fo=json`, then fetch each item/resource response rather than treating search-result metadata as
complete. Retain the canonical `loc.gov/item` URI, LCCN when present, collection, contributors,
date, subjects/genres, `access_restricted`, resource URL, rights text, credit line, raw item/resource
hashes, and downloaded SHA-256. The [LOC JSON/YAML API](https://www.loc.gov/apis/json-and-yaml/)
needs no key. Its [published limits](https://www.loc.gov/apis/json-and-yaml/working-within-limits/)
allow 20 JSON requests per minute, recommend no more than 1,000 results per page, and warn that an
over-limit client may be blocked for an hour.

**Current scale.** Search on 2026-08-26 returned 85 hits for `commercials`, 22 for `advertising`, 29
for `public service`, five for `trailers`, and one each for `bumper` and `station identification`.
Free-text totals are query-sensitive and include false positives, so they establish only that a
bounded manual inventory is practical. Citizen DJ's National Screening Room pool is only 18 source
films; its 201 segments and 4,096 one-shots are audio derivatives, not thousands of video cases.

**Rights authority.** The broader National Screening Room says the Library is unaware of U.S.
restrictions for most works but expressly leaves final assessment to the user and warns about rare
permission-only films, foreign copyright, privacy, publicity, trademark, licensing, and donor
restrictions
([National Screening Room rights](https://www.loc.gov/collections/national-screening-room/about-this-collection/rights-and-access/)).
Citizen DJ's narrower subset says its source films were identified as U.S.-government-created,
public-domain works that may be copied, modified, distributed, and performed
([Citizen DJ rights](https://citizen-dj.labs.loc.gov/loc-national-screening-room/use/)). The narrow
subset is a strong rights seed but weak role coverage. Broader advertising items require individual
public-domain or owner-permission adjudication.

**Bounded inventory:** 25 rows: 10 advertising/sponsored films, 5 promotional or public-service
items, and 10 programme/institutional controls. Do not build a generic National Screening Room
downloader that interprets collection membership as permission.

### 3. NASA Image and Video Library: current promos and trailers

**Discovery and media contract.** Search
`GET https://images-api.nasa.gov/search?media_type=video&q=<term>`, retain each `nasa_id` and the raw
search item, then fetch the returned `collection.json` asset manifest and select a bounded original
or highest suitable MP4. Record NASA ID, center, creator/secondary creator, date, keywords,
description, raw search and collection hashes, exact asset URL/size, and downloaded SHA-256. The
official [Image and Video Library API](https://images.nasa.gov/docs/images.nasa.gov_api_docs.pdf)
documents the search and asset-manifest endpoints.

**Current scale and relevance.** The 2026-08-26 API returned 38 video results for `trailer` and 18
for `promo`. Manual title inspection found genuine mission, programme, launch, event, and social
trailers/promos among them. Exact PSA, bumper, and station-identification probes produced no genuine
role matches. A `commercial` query returned 779 results dominated by the Commercial Crew programme,
not advertisements. Search text must never become a role label. NASA is a strong institutional
promo/trailer lane, not a commercial, bumper, ID, or PSA lane.

**Rights authority.** NASA says its content generally is not copyrighted in the United States and
allows factual, non-endorsement use, but identifies third-party material, insignia/logotypes,
identifiable people, publicity/privacy, and commercial-promotional use as separate constraints
([NASA media guidelines](https://www.nasa.gov/nasa-brand-center/images-and-media/)). The agency page
plus a NASA API record is not enough when credits or the media reveal a partner, licensed music, a
protected identifier, or a recognizable person. Hold those cases until the embedded rights and
intended redistribution are cleared.

**Bounded inventory:** 25 rows: 12 genuine trailers and 13 genuine promos selected by title and
item inspection, not by query result alone.

### 4. CDC: modern PSAs, small manual lane

**Discovery and media contract.** CDC's Content Services API offers stable media IDs, source URLs,
publication/modification times, source organizations, attribution, language, campaign/tags, and
pagination at `https://tools.cdc.gov/api/v2/resources/media`
([official API reference](https://tools.cdc.gov/api/docs/info.aspx)). Not every current campaign
video is indexed or exposed as an original downloadable file through that API. For those items,
use the first-party campaign page as the stable item record, freeze its HTML and outbound media URL,
and hash the acquired file. Do not scrape arbitrary CDC YouTube embeds.

**Current scale and relevance.** The current
[`Still Going Strong` video page](https://www.cdc.gov/still-going-strong/campaign-resources/videos-and-radio.html)
offers five downloadable short videos: 15- and 30-second `Health in Numbers` spots plus 8-, 15-, and
30-second `Jingle` spots. CDC also publishes first-party disaster PSA pages such as
[`Charge Your Phone`](https://www.cdc.gov/natural-disasters/psa-toolkit/charge-your-phone.html).
This is high-value modern PSA material but not a scalable 30-case source by itself.

**Rights authority.** CDC says most agency material is public domain, but contractor, grantee,
third-party stock, state/local, and foreign material are exceptions; attribution and a clear
non-endorsement disclaimer are required
([CDC reuse rules](https://www.cdc.gov/other/agencymaterials.html)). Require the exact page to name a
CDC content source/corporate author, contain no conflicting copyright notice, and survive an
embedded footage/music/performer review.

**Bounded inventory:** 15 rows: the five current `Still Going Strong` videos and ten additional
first-party campaign PSAs with downloadable media and explicit CDC provenance. The Natural
Disasters toolkit currently exposes at least twelve distinct video-PSA pages, so this is a plausible
manual lane. Treat failure to find ten qualifying items as measured source attrition, not permission
to use an embed downloader.

### 5. Blender Foundation open movies: modern trailer seed

**Discovery and media contract.** Hand-curate from the first-party
[Blender Studio films catalog](https://studio.blender.org/films/) and each project's own download and
licence pages. Retain project/work title, Foundation ownership statement, exact licence/version,
attribution, exclusions, first-party page snapshot/hash, source asset URL, and media SHA-256. There
is no documented stable bulk trailer API suitable for an automated corpus importer; use a checked
manifest of explicit works.

**Scale and rights.** Blender currently lists 18 open-film projects, but not every project has a
separate downloadable trailer and not every film release uses the same licence. This pass confirmed
explicit first-party trailer/download lanes for Big Buck Bunny, Sintel, and Tears of Steel only;
audit the remaining projects before assuming more. Big Buck Bunny's
project page explicitly licenses published results under CC BY 3.0 and supplies a trailer
([rights](https://peach.blender.org/about/), [trailer](https://peach.blender.org/trailer-page/));
Sintel's project page likewise identifies the Foundation project and CC BY 3.0 terms
([Sintel](https://durian.blender.org/about/)). Blender Studio says its content is generally CC BY
but instructs reusers to read the item-specific licence text
([remixing guidance](https://studio.blender.org/remixing/)). For example, Agent 327's final film was
published under CC BY-ND, so a general `Blender is CC BY` assumption is unsafe.

**Bounded inventory:** start with the three confirmed trailers and admit at most five actual
first-party trailers/teasers after item-specific licence review. Do not relabel a full open movie or
an editor-created excerpt as an authentic trailer.

**Live pilot outcome (2026-08-26): retired as a lane.** Execution found live first-party trailer
representations for Big Buck Bunny and Sintel. The Tears of Steel page still names its MP4 teaser,
but that media URL returns 404; only a ZIP archive remains on the Blender download host. The source
therefore cannot supply the required ten distinct live candidates. Duplicate encodes, two edits of
one trailer, full open movies, demo reels, and unrelated gallery files do not repair that product-
relevance failure. Do not add a Blender adapter or a special short-lane format. Individually cleared
works may still enter the direct/static cohort, and the missing public-source yield moves to direct
licensing.

The retirement probe used metadata/HEAD requests only:

| First-party representation | HTTP | Reported type | Meaning |
| --- | ---: | --- | --- |
| Big Buck Bunny `trailer_480p.mov` | 200 | `video/quicktime` | live trailer |
| Sintel `sintel_trailer-480p.mp4` | 200 | `video/mp4` | live trailer |
| Sintel `Sintel_Trailer1.480p.DivX_Plus_HD.mkv` | 200 | `application/octet-stream` | second edit of the same work, not an independent project |
| Tears of Steel `tears-of-steel_teaser.mp4` | 404 | `text/html` | project page points to dead media |
| Tears of Steel `tears-of-steel_teaser.mp4.zip` | 200 | `application/zip` | archive container, not a directly represented video |

### 6. Wikimedia Commons: broad candidate pool, secondary rights evidence

**Discovery and media contract.** Traverse declared category roots through
`https://commons.wikimedia.org/w/api.php?action=query&list=categorymembers`, following continuation
at a fixed depth. Batch `imageinfo` for page ID/title, revision/upload time, user, original URL, size,
SHA-1, MIME/media type, and a filtered `extmetadata` set; separately fetch the `M<page-id>` MediaInfo
entity and retain `P275` licence, `P6216` copyright status, `P7482` source, and `P170` creator claims
with references and ranks. Official references:
[categorymembers](https://www.mediawiki.org/wiki/API:Categorymembers),
[imageinfo](https://www.mediawiki.org/wiki/API:Imageinfo),
[CommonsMetadata](https://www.mediawiki.org/wiki/Extension:CommonsMetadata), and
[MediaInfo](https://www.mediawiki.org/wiki/Extension:WikibaseMediaInfo).

Use a descriptive User-Agent, serial/batched reads, `maxlag=5`, caching, and exponential backoff as
required or recommended by Wikimedia's
[User-Agent policy](https://foundation.wikimedia.org/wiki/Policy:Wikimedia_Foundation_User-Agent_Policy)
and [API etiquette](https://www.mediawiki.org/wiki/API:Etiquette).

**Current scale.** On 2026-08-26 a direct-root, API-MIME-filtered read found 64 advertising videos,
2 television advertisements, 8 PSAs, 9 station IDs, and 320 film trailers. A root-plus-immediate-
subcategory traversal on 2026-08-25 found approximately 71 advertising videos, 55 television
advertisements, 23 PSAs, 9 IDs, and 355 trailers. The large difference shows why the traversal depth,
category paths, continuation tokens, and query time must be frozen. These counts still contain
category errors and overlaps; they support a bounded metadata inventory, not an approval forecast.
No credible dedicated bumper pool was found.

**Rights authority.** Commons permits only freely licensed or public-domain media and excludes
fair-use, noncommercial-only, and no-derivatives-only content
([Commons licensing policy](https://commons.wikimedia.org/wiki/Commons:Licensing)). But Commons owns
almost none of the files and tells downstream users to verify copyright, licence, attribution,
trademark, personality, privacy, and other rights themselves
([reuse guidance](https://commons.wikimedia.org/wiki/Commons:Reusing_content_outside_Wikimedia)).
`extmetadata` is derived partly from free-form page text; multi-licence values can be unreliable,
and file-level rights may differ from embedded film/music rights
([CommonsMetadata limitations](https://www.mediawiki.org/wiki/Extension:CommonsMetadata#Returned_data),
[copyright modeling](https://commons.wikimedia.org/wiki/Commons:Structured_data/Modeling/Copyright)).

Require agreement between the description page and MediaInfo, a traceable original source, and
direct first-party or VRT-backed permission. A YouTube-originated CC claim remains held until the
creator/owner and original licence can be independently verified. Deletion nominations, missing
source/creator, deprecated claims, unexplained multi-licensing, embedded music/film, or unresolved
`Restrictions` flags hold the case.

**Bounded inventory:** 50 rows: 15 commercial/advertising, 8 PSA, 5 station-ID, 17 trailer, and 5
promo-like candidates. Expect substantial source-chain and role-label attrition.

**Live pilot outcome (2026-08-26): awaiting independent review.** The direct
`Category:Advertising videos` root exhausted in two continued category/imageinfo requests; one
batched MediaInfo request then froze ten deterministic candidates. The run used 3 of 10 requests,
162,981 response bytes, 448,607,902 predicted media bytes, and 1.0 second. It intentionally retains
YouTube/Vimeo provenance, trademark restrictions, `License review needed` categories, and missing
P275/P6216/P7482/P170 claims as review evidence rather than treating category membership or a
licence string as approval. Seven rows also freeze the asserted HTTPS `LicenseUrl` structurally;
public-domain rows without that field retain the exact public-domain and credit assertions instead
of inventing a URL.

## First bounded inventory and go/no-go review

Freeze **155 metadata-only candidates** before implementing another reusable adapter or downloading
media:

| Lane | Rows | Primary roles |
| --- | ---: | --- |
| Prelinger | 40 | commercial, promo, PSA, compilation controls |
| Library of Congress | 25 | period advertising, PSA/promo, institutional controls |
| NASA | 25 | promo, trailer |
| CDC | 15 | PSA |
| Wikimedia Commons | 50 | commercial, PSA, ID, trailer, promo candidates |
| **Total** | **155** | |

Before scaling source adapters, send ten representative rows from each active lane through
independent rights review. Continue a lane only if it produces at least five approved,
product-relevant items without a source-specific lowering of the rights threshold. Freeze the
complete inventory only after that yield check. The public-source target is 90 approved positives;
any underfill moves to additional direct licensing beyond the 100-case direct minimum.

The metadata-only inventory must have hard item, request, response-byte, predicted-media-byte,
duration, pagination, retry, and wall-clock ceilings. It stores raw-response hashes and performs no
media fetch, model call, or licence decision. Rights review consumes that frozen inventory; only a
fully locked approval record may authorize the separately bounded downloader.

### First adapter decision

The first **new reusable adapter** should be a metadata-only Wikimedia Commons inventory adapter,
after its ten-row rights-yield pilot passes. The Archive lane already has repository tooling;
NASA is narrower and institutionally skewed; LOC has weaker blanket rights; the small CDC pool is
better represented by a reviewed static manifest; and Blender is retired as a pilot lane after its
live surface failed the ten-distinct-candidate precondition.
Commons is the only remaining API lane with meaningful candidate coverage across commercials,
station IDs, trailers, PSAs, and promo-like material.

That adapter must stop at frozen candidates. It traverses only configured category roots to a fixed
depth, follows continuation, filters API-reported video, records category paths, page/revision and
MediaInfo revisions, raw `imageinfo`/MediaInfo hashes, exact source and rights fields, predicted
bytes, and hard budgets. It does not infer a role, approve rights, or download media. If fewer than
five of the ten pilot rows survive independent source-chain and rights review, do not implement the
adapter; use direct licensing instead.

## Directly commissioned or licensed modern cohort

Commission or license **100 positive cases**: 20 commercials, 20 promos, 25 bumpers, 25 station IDs,
5 trailers, and 5 PSAs. Prefer many independent creators/small broadcasters over one production
pack, and deliberately cover modern editing, vertical and widescreen formats, different eras,
languages, audiences, sensitive categories, text density, voice-over, music, silent spots, and
short 5/10/15-second forms.

A commissioned work is not automatically owned by the commissioner. The U.S. Copyright Office
explains that commissioned audiovisual work qualifies as work made for hire only under the statutory
conditions and a signed written agreement, and that copyright transfers generally require a signed
writing
([Works Made for Hire, Circular 30](https://www.copyright.gov/circs/circ30.pdf),
[17 U.S.C. chapter 2](https://www.copyright.gov/title17/92chap2.html)). Prefer a signed assignment
plus an explicit licence as a fallback, rather than relying on the label `commissioned`.

Each agreement must identify the exact master or later bind its SHA-256 and grant the rights needed
to copy, modify, segment, extract frames/audio/transcripts, distribute, display, perform, sublicense,
and transmit the work to named or bounded third-party inference providers for commercial evaluation.
It must cover territory and duration, attribution, confidentiality/embargo, source-project delivery,
and warranties or releases for music, performers, voices, locations, stock, trademarks, and other
embedded works. The rights ledger records the signer and their authority; an email from an uploader
without ownership evidence is not enough.

## Item-level approval threshold

An approved case needs both a source assertion and an independent rights decision bound to the
frozen metadata and exact media:

1. **Federal work:** an item record naming the responsible federal agency/employee plus that
   agency's reuse policy and an item-level check for contractors, partners, third-party content,
   music, logos, people, donor restrictions, and foreign rights. Section 105 excludes U.S.
   government works from U.S. copyright, but the item still must be proved to be such a work
   ([17 U.S.C. §105](https://www.copyright.gov/title17/92chap1.html#105)).
2. **Owner-published open licence:** a first-party owner page naming the exact work and exact licence
   version, with the owner/source chain, attribution, modification and redistribution rights, and
   embedded works verified.
3. **Signed direct grant:** a signed owner or authorized-agent assignment/licence tied to the exact
   work or checksum and covering all corpus uses.
4. **Public-domain adjudication:** a reviewer-recorded item-specific analysis of authorship,
   publication, jurisdiction, term/renewal where relevant, source, and embedded rights. A Public
   Domain Mark is a status assertion, not a licence from an owner.

Every approval records source authority and collection, stable item ID/URL, metadata retrieval time
and raw hash, exact rights statement/licence URI, creator and contributors, date/jurisdiction,
required credit, restrictions, reviewer/reason/time, source media URL/file/size/checksums, downloaded
SHA-256, segment bounds, all derived evidence hashes, and source/similarity cluster. Media stays
outside Git unless its redistribution terms explicitly permit repository distribution.

## Fail-closed exclusions

Exclude or hold all of the following:

- DVIDS, because it is not product-representative even when its rights notice is clear;
- the 331-row `classic_tv_commercials` worksheet as certification truth; all rows remain discovery
  candidates until independently approved;
- generic Internet Archive or Prelinger membership, downloadability, `licenseurl`, `rights`, or
  `possible-copyright-status` without an item decision;
- broad National Screening Room, NARA, NASA, or CDC membership without item provenance and an
  embedded-rights review;
- Commons category membership, `extmetadata`, MediaInfo, or an uploader's YouTube licence without a
  traceable owner/source chain;
- CC BY-NC, CC BY-ND, or another licence that blocks commercial use or required derivatives;
- unexplained multi-licensing, missing licence URI, ambiguous Public Domain Mark, Fair Use Notice,
  deletion nomination, stale/changed metadata, or conflicting rights fields;
- Ad Council campaign media with issue-use, modification, placement, or expiry restrictions, and
  YouTube downloads that rely only on API availability or uploader assertions;
- unresolved third-party music, stock footage, film excerpts, performers, privacy/publicity,
  trademarks, logos, donor terms, territory, or attribution/share-alike obligations;
- source redirects outside an allowlisted host, a media identity that differs from the reviewed
  item, files exceeding declared ceilings, or any metadata/media hash drift after review; and
- synthetic or derived material presented as an authentic commercial, promo, bumper, ID, trailer,
  or PSA. Derivatives are valid controls only when labeled and clustered as derivatives.

## Execution order

1. Run the five-lane, 50-row rights-yield pilot: ten metadata-only candidates per qualified lane.
2. Freeze the 155-row inventory only for lanes that pass product-relevance and rights-yield review.
3. Execute the 100-case direct-license/commission brief in parallel; do not wait for archives to
   solve the modern bumper/ID gap.
4. Independently adjudicate rights and acquire/hash approved-only media under existing ceilings.
5. Build source and perceptual-similarity clusters before assigning development and holdout splits.
6. Produce two blind semantic-label batches and a third adjudication only for disagreements.
7. Generate independent opaque-alias review packets, lock the schema-v5 manifest with at least 300
   development and 1,126 holdout cases, then run #555's paid, label-blind OpenRouter bakeoff on
   identical evidence packets and proceed to fictional-media-server shadow certification.
