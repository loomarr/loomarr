# Filler certification sources and OpenRouter routing snapshot

**Compiled 2026-08-25.** This note complements
[`filler-admission-confidence.md`](filler-admission-confidence.md): it narrows the acquisition pool to
sources whose rights evidence can be recorded per item and freezes the hosted candidates before any
paid bakeoff. No inference request was submitted and no OpenRouter credit was spent. This is an
engineering provenance policy, not legal advice.

## Verdict

Build the first representative corpus from four tiers, in this order:

1. clips already held by the maintainer, where Loomarr can record the maintainer's authority and the
   original file hash;
2. Internet Archive items with affirmative, machine-readable item-level rights metadata and an
   independent adjudication, especially the affirmatively marked portion of the Prelinger collection;
3. Wikimedia Commons files whose file page, source provenance, and machine-readable rights agree
   after independent review;
4. Library of Congress and federal-agency material with an item-level public-domain statement.

Do **not** treat membership in Internet Archive, Prelinger, a Wikimedia Commons category, the
National Screening Room, NARA, or a NASA site as a blanket license. Each repository contains
mixed-rights material or warns that third-party, privacy, publicity, trademark, donor, or territorial
restrictions can remain
([Internet Archive rights fields](https://archive.org/developers/metadata-schema/index.html#licenseurl),
[Library of Congress National Screening Room rights](https://www.loc.gov/collections/national-screening-room/about-this-collection/rights-and-access/),
[NARA permissions](https://www.archives.gov/research/motion-pictures/permissions),
[NASA media guidelines](https://www.nasa.gov/nasa-brand-center/images-and-media/)). A missing or
ambiguous rights field is therefore a corpus hold, not an implied public-domain decision.

For OpenRouter, preserve all eight issue #555 candidates for the measured bakeoff, but run only a
bounded, serial certification lane with a concrete model ID, one allowed provider, provider
fallbacks disabled, required parameters enforced, and the resolved canonical model/provider copied
into the report. The public catalog changed enough in one day to invalidate two prices in the prior
note: this snapshot now shows Qwen3.8 at a catalog rate of $0.425/$2.55 and Gemma 4 at $0.07/$0.34 per
million prompt/output tokens
([OpenRouter models API](https://openrouter.ai/api/v1/models),
[Qwen endpoints](https://openrouter.ai/api/v1/models/qwen/qwen3.8-27b/endpoints),
[Gemma endpoints](https://openrouter.ai/api/v1/models/google/gemma-4-26b-a4b-it/endpoints)). Prices
and providers must be snapshotted again into every paid run rather than copied into configuration.

## 1. Corpus sources and exact constraints

### Internet Archive and Prelinger

Internet Archive is the best discovery surface for period commercials, promos, industrial films,
station material, and adjacent negative examples, but the collection itself is not rights evidence.
A live Advanced Search query on 2026-08-25 returned 10,461 movie items in `collection:prelinger`, of
which only 1,914 had any `licenseurl`; the query results also include item-specific prose such as a
copyright notice or failed renewal search in `rights`
([all Prelinger movie count](https://archive.org/advancedsearch.php?q=collection%3Aprelinger%20AND%20mediatype%3Amovies&fl%5B%5D=identifier&rows=0&output=json),
[Prelinger items with `licenseurl`](https://archive.org/advancedsearch.php?q=collection%3Aprelinger%20AND%20mediatype%3Amovies%20AND%20licenseurl%3A%2A&fl%5B%5D=identifier%2Ctitle%2Clicenseurl%2Crights%2Cpossible-copyright-status&rows=10&output=json)).
Internet Archive defines `licenseurl` as an uploader-supplied URL for a recognized license, `rights`
as uploader-supplied rights prose, and `possible-copyright-status` as uploader-supplied copyright
information; none is an Archive warranty
([metadata definitions](https://archive.org/developers/metadata-schema/index.html#licenseurl)).

Consequently, corpus import should:

- require an affirmative allowlisted `licenseurl` or separately adjudicated rights statement;
- retain the exact rights value and the URL of the metadata response used to make the decision;
- exclude `BY-NC` from a generally distributable product corpus, and hold `ND` material whenever
  segmenting, frame extraction, transcription, or redistribution could exceed unchanged use;
- preserve attribution and share-alike obligations for `BY` or `BY-SA`, while preferring CC0,
  Public Domain Mark, or an explicit public-domain statement for checked-in fixtures;
- never promote an uploader's uncertain renewal-search prose to `public_domain` without an
  independent adjudication record.

Creative Commons defines `BY` as requiring attribution, `SA` as requiring adaptations to use the
same or a compatible license, `NC` as noncommercial-only, and `ND` as unadapted-form-only; it also
distinguishes CC0's rights waiver from the Public Domain Mark's status label
([official CC license summary](https://creativecommons.org/cc-licenses/),
[official public-domain tools summary](https://creativecommons.org/public-domain/)).

Those are Loomarr's conservative import rules inferred from the source metadata semantics, not
Internet Archive's legal interpretation. The Archive item identifier is stable, and stable download
URLs have the form `https://archive.org/download/<identifier>/<filename>`; redirected storage-host
URLs are explicitly not permalinks
([item structure and archival URLs](https://archive.org/developers/items.html#archival-urls)). The
metadata API returns original/derived status, format, byte size, MD5 and SHA-1 per file, so Loomarr
can select an original media file and independently add SHA-256 for immutable corpus identity
([Metadata API read response](https://archive.org/developers/md-read.html)).

Automated acquisition must identify itself with a descriptive User-Agent, cache metadata, delay
bulk requests, honor `429` and `Retry-After`, and limit concurrency; Internet Archive explicitly
requires identification and recommends those controls
([automated-access policy](https://archive.org/developers/bots.html)). This source should therefore
be a manifest builder with a resumable download cache, not an unbounded scraper.

### Library of Congress

Use two narrowly rights-cleared Library of Congress pools:

- **Selections from the National Film Registry:** the Library says it is unaware of U.S. copyright
  in this collection and that items not subject to copyright are free to use and reuse, while still
  requiring item-level assessment for privacy, publicity, trademark, licensing, and foreign rights
  ([collection rights statement](https://www.loc.gov/collections/selections-from-the-national-film-registry/about-this-collection/rights-and-access)).
- **Citizen DJ's National Screening Room subset:** the Library project says this subset was selected
  from U.S.-government-created films, is public domain, and may be copied, modified, distributed,
  and performed without restriction; attribution is recommended
  ([Citizen DJ rights and access](https://citizen-dj.labs.loc.gov/loc-national-screening-room/use/)).

These collections are more useful for public-service announcements, station/educational material,
government films, bumpers, and non-advertisement controls than for modern branded-commercial
coverage. The broader National Screening Room is unsuitable as a blanket source: the Library says
most works appear unrestricted but calls out rare permission-only copyrighted films and makes the
user responsible for the final rights assessment
([National Screening Room rights](https://www.loc.gov/collections/national-screening-room/about-this-collection/rights-and-access/)).

The public JSON API needs no key, supports collection and film/video search plus item/resource
responses, and exposes downloadable resources
([API overview](https://www.loc.gov/apis/json-and-yaml/),
[API endpoints](https://www.loc.gov/apis/json-and-yaml/requests/endpoints/)). It currently permits
20 JSON/YAML requests per minute, recommends at most 1,000 results per page, blocks for an hour when
the rate is exceeded, and warns that result metadata is incomplete until the item/resource record is
fetched
([working within limits](https://www.loc.gov/apis/json-and-yaml/working-within-limits/)). Importers
must therefore throttle below 20 requests per minute, cache item JSON, and record `access_restricted`,
the item URL, resource URL, rights text, suggested credit, and a local content hash.

### NARA and NASA: useful, but conditional

NARA is a discovery source for public-service, military, newsreel, safety, and government identity
material. U.S.-government works made by federal employees in their official duties are not eligible
for U.S. copyright, but NARA expressly says not all Special Media holdings are public domain, does
not confirm status, and may hold donor, contract, publicity, performance, or third-party rights
([NARA permissions](https://www.archives.gov/research/motion-pictures/permissions)). Admit a NARA
item only when its catalog provenance identifies the creating federal agency and the reviewed item
record has no conflicting restriction; otherwise use it only as a discovery lead. NARA requests a
`Courtesy: National Archives and Records Administration` credit for audiovisual use
([same permissions statement](https://www.archives.gov/research/motion-pictures/permissions)).

NASA can contribute strong identification, promo-like, educational, and public-service slices.
NASA says its video and other media generally are not copyrighted in the United States and permits
factual, non-endorsement use, but third-party material, music/footage, identifiable people, logos,
and promotional use can add restrictions
([NASA media guidelines](https://www.nasa.gov/nasa-brand-center/images-and-media/)). Record the
canonical NASA page, agency credit, third-party copyright marking, recognizable-person flag, logo
flag, and intended non-endorsement use for every candidate; exclude any item whose embedded music or
footage cannot be cleared.

### Wikimedia Commons: useful second pool, not a rights oracle

Wikimedia Commons is worth adding because it has directly categorized commercials, public-service
announcements, station identification, and trailers, plus openly licensed negative controls. Its
official licensing policy accepts media that is explicitly freely licensed or public domain in both
the United States and the source country, and it does not accept fair-use, noncommercial-only, or
no-derivatives-only files
([Commons licensing policy](https://commons.wikimedia.org/wiki/Commons:Licensing)). However, the
Foundation owns almost none of the content, provides no warranty for its copyright status or license
terms, and tells reusers to verify each file and account for trademark, personality, moral, privacy,
and other non-copyright rights
([Commons reuse guidance](https://commons.wikimedia.org/wiki/Commons:Reusing_content_outside_Wikimedia)).
That makes Commons a provenance-rich candidate pool, not an automatic admission signal.

#### Category snapshot

The following read-only snapshot was taken on 2026-08-25 without downloading media. For each named
root, the measurement traversed the root and its immediate subcategories with `categorymembers`,
followed API continuation, deduplicated by page ID within that root, and counted only files whose
`imageinfo` reported `mediatype=VIDEO` or a `video/*` MIME type. MediaWiki documents `cmtype=file`,
`cmtype=subcat`, 500-item pagination, and `cmcontinue`; `imageinfo` supplies MIME and media type
([categorymembers API](https://www.mediawiki.org/wiki/API:Categorymembers),
[continuation](https://www.mediawiki.org/wiki/API:Continue),
[imageinfo API](https://www.mediawiki.org/wiki/API:Imageinfo)).

| Intended slice | Commons category root | Categories visited | Approximate video files |
| --- | --- | ---: | ---: |
| broad promotion/advertising discovery | [`Advertising videos`](https://commons.wikimedia.org/wiki/Category:Advertising_videos) | 5 | 71 |
| television commercials | [`Television advertisements`](https://commons.wikimedia.org/wiki/Category:Television_advertisements) | 11 | 55 |
| public-service announcements | [`Public service announcements`](https://commons.wikimedia.org/wiki/Category:Public_service_announcements) | 6 | 23 |
| station IDs | [`Station identification`](https://commons.wikimedia.org/wiki/Category:Station_identification) | 1 | 9 |
| trailers | [`Film trailer videos`](https://commons.wikimedia.org/wiki/Category:Film_trailer_videos) | 4 | 355 |
| non-filler documentary controls | [`Documentary films videos`](https://commons.wikimedia.org/wiki/Category:Documentary_films_videos) | 9 | 293 |
| non-filler speech controls | [`Videos of speeches`](https://commons.wikimedia.org/wiki/Category:Videos_of_speeches) | 3 | 158 |
| non-filler educational controls | [`Educational videos`](https://commons.wikimedia.org/wiki/Category:Educational_videos) | 1 | 4 |

These are discovery counts, not rights-cleared cases or semantic labels. Category membership is
community-maintained, the graphs overlap, deeper descendants are deliberately excluded, and the
counts will change. In particular, `Advertising videos` contains trailer and company-related
subcategories, so its 71 files cannot be treated as a disjoint “promo” class. The result is still
large enough to justify a Commons importer: the dedicated roots expose roughly 55 commercial, 23
PSA, 9 station-ID, and 355 trailer candidates, while the three control roots expose up to 455
documentary, speech, and educational video candidates before cross-root deduplication.

#### Metadata and license caveats

Use the Commons Action API at `https://commons.wikimedia.org/w/api.php`. Category traversal should
feed a batched `imageinfo` request for `timestamp`, `user`, `url`, `size`, `sha1`, `mime`,
`mediatype`, and a filtered `extmetadata` projection. The CommonsMetadata extension exposes
`LicenseShortName`, `LicenseUrl`, `UsageTerms`, `Copyrighted`, `Attribution`,
`AttributionRequired`, `Artist`, `Credit`, `Permission`, `Restrictions`, `DeletionReason`, and
categories; it explicitly says that multi-license values are currently unreliable and that
`Restrictions` can carry trademark or personality-right flags
([CommonsMetadata fields](https://www.mediawiki.org/wiki/Extension:CommonsMetadata#Returned_data)).
The underlying `extmetadata` values can contain HTML and are built from description-page free text;
MediaWiki calls the property expensive and recommends requesting only a few results and only needed
fields
([imageinfo `extmetadata`](https://www.mediawiki.org/wiki/API:Imageinfo),
[machine-readable-data limitations](https://commons.wikimedia.org/wiki/Commons:Machine-readable_data)).

Also retrieve the file's Wikibase MediaInfo entity, whose ID is `M` plus the Commons page ID.
Structured rights use `P6216` (copyright status), `P275` (copyright license), and `P7482` (source of
file), with creator normally represented by `P170`
([MediaInfo entity model](https://www.mediawiki.org/wiki/Extension:WikibaseMediaInfo#MediaInfo_Entity),
[Commons structured-data model](https://commons.wikimedia.org/wiki/Commons:Structured_data/Modeling),
[copyright model](https://commons.wikimedia.org/wiki/Commons:Structured_data/Modeling/Copyright)).
MediaInfo is supplemental and can be a virtual entity with no stored captions or statements, so its
absence is not affirmative rights evidence
([WikibaseMediaInfo](https://www.mediawiki.org/wiki/Extension:WikibaseMediaInfo#MediaInfo_Entity)).
The copyright model also warns that a digital file and creative works contained in it can have
different rights, explicitly naming films with music or fragments of other films as complex cases
([copyright-model caveat](https://commons.wikimedia.org/wiki/Commons:Structured_data/Modeling/Copyright)).

A live example shows why both machine layers and human review are required. The 1954 Kool-Aid
commercial's `extmetadata` reports `Public domain`, no `LicenseUrl`, and
`Restrictions=trademarked`, while its MediaInfo entity has a caption but no structured `P275`,
`P6216`, or `P7482` rights claims
([file page](https://commons.wikimedia.org/wiki/File:1954_Kool-Aid_Commercial._Debut_of_Pitcher_Man.webm),
[live `imageinfo` response](https://commons.wikimedia.org/w/api.php?action=query&format=json&formatversion=2&prop=imageinfo&titles=File%3A1954%20Kool-Aid%20Commercial.%20Debut%20of%20Pitcher%20Man.webm&iiprop=timestamp%7Cuser%7Curl%7Csize%7Csha1%7Cmime%7Cmediatype%7Cextmetadata&iimetadataversion=latest&iiextmetadatafilter=LicenseShortName%7CLicenseUrl%7CUsageTerms%7CCopyrighted%7CAttribution%7CCredit%7CArtist%7CRestrictions),
[live MediaInfo response](https://commons.wikimedia.org/w/api.php?action=wbgetentities&format=json&ids=M147457667&props=labels%7Cclaims)).
That file may become a candidate only after the public-domain rationale, original source, embedded
audio, and trademark handling are independently adjudicated.

#### Exact Commons import contract

1. Traverse only declared category roots to a declared depth, follow continuation, retain category
   paths, deduplicate page IDs, and filter to API-reported video media. Category membership is never
   a truth label.
2. Batch `imageinfo`; store Commons page ID/title, file-description URL, upload timestamp/user,
   byte size, MIME/media type, Commons SHA-1, and the raw filtered `extmetadata` response hash.
3. Store the file-page revision ID and timestamp plus the MediaInfo entity revision and all `P275`,
   `P6216`, `P7482`, and `P170` statements, qualifiers, ranks, and references. A missing statement,
   deprecated rank, multi-license parse, or disagreement between description-page and MediaInfo
   rights holds the file for adjudication.
4. Require an allowlisted license or public-domain rationale, a traceable original source, creator,
   required attribution, and independent reviewer decision. Verify the original source's rights;
   Commons' copy is not independent corroboration.
5. Reject or hold deletion-nominated files, unclear permissions, untraceable source transfers,
   unresolved embedded music/film rights, and any `Restrictions` value without an explicit handling
   decision. Preserve trademarks, personality/privacy rights, jurisdiction, and attribution as
   separate policy flags rather than collapsing them into copyright.
6. Preserve the exact license URI and terms selected for multi-licensed files. For derivatives,
   record the applicable attribution, change notice, and share-alike obligation; prefer CC0 or
   independently established public domain for redistributable fixtures.
7. Before media acquisition, freeze the metadata evidence and predicted byte/request budget. After
   an approved download, independently compute SHA-256 and hash every derived segment/frame/audio/
   transcript artifact; do not rely on the mutable file title or Commons SHA-1 alone.

Wikimedia requires a meaningful User-Agent with contact information. Its API guidance recommends
serial rather than parallel reads, batching titles or using generators, caching, respecting all
throttling responses, and exponential backoff; the Foundation forbids attempts to evade limits
([User-Agent policy](https://foundation.wikimedia.org/wiki/Policy:Wikimedia_Foundation_User-Agent_Policy),
[API etiquette](https://www.mediawiki.org/wiki/API:Etiquette),
[Foundation API policy](https://foundation.wikimedia.org/wiki/Policy:Wikimedia_Foundation_API_Usage_Guidelines)).
Noninteractive import should send `maxlag=5` and honor `Retry-After`; MediaWiki recommends that value
for Wikimedia bots
([maxlag guidance](https://www.mediawiki.org/wiki/Manual:Maxlag_parameter)). Loomarr should keep the
same one-request concurrency ceiling as the certification runner and cache category and metadata
pages by revision.

### Minimum provenance record

Every admitted corpus segment should retain:

| Field | Why it is required |
| --- | --- |
| source authority, collection, stable item ID and item URL | reconstruct the discovery path |
| metadata retrieval time, raw metadata hash and evidence URL | prove what the source asserted at import time |
| exact `licenseurl`, verbatim rights statement, adjudicator, decision and decision time | separate source assertion from Loomarr's legal/curatorial judgment |
| creator, contributor, publication date, required credit and restriction flags | preserve attribution and non-copyright obligations |
| source filename/URL, original-or-derivative status, format, byte size and source checksums | reconstruct the selected representation |
| downloaded SHA-256, segment offsets, derivative/frame/transcript hashes | prevent silent media or evidence drift |
| source/similarity cluster and split | prevent near-duplicate leakage across development and holdout |

This record is an engineering inference from the repositories' item-level rights warnings and
metadata facilities. Media that cannot be redistributed can remain outside Git; the checked-in
manifest can still lock hashes, provenance, labels, and split membership.

## 2. OpenRouter snapshot for issue #555

The following values were read from OpenRouter's public model and endpoint APIs on 2026-08-25.
Prices are USD per million prompt/output tokens and do not include every possible image, audio,
reasoning, cache, or request charge. Endpoint prices vary by provider and service tier, so the table
uses the catalog-level price and separately records endpoint count/provider families
([models API field contract](https://openrouter.ai/docs/api/api-reference/models/list-all-models-and-their-properties)).

| Requested model ID | Input modalities | Catalog $/M in/out | Live endpoints and provider families | Bakeoff role |
| --- | --- | ---: | --- | --- |
| [`google/gemini-3.7-flash`](https://openrouter.ai/api/v1/models/google/gemini-3.7-flash/endpoints) | text, image, video, file, audio | 0.375 / 1.875 | 6; Google, Google AI Studio | primary direct-video and frame candidate |
| [`qwen/qwen3.8-27b`](https://openrouter.ai/api/v1/models/qwen/qwen3.8-27b/endpoints) | text, image, video | 0.425 / 2.55 | 10; Chutes, Phala, CoreWeave, AkashML, Alibaba, Cloudflare, Reka, Venice, Parasail, Io Net | hosted open-weight direct-video/frame candidate |
| [`google/gemma-4-26b-a4b-it`](https://openrouter.ai/api/v1/models/google/gemma-4-26b-a4b-it/endpoints) | image, text, video | 0.07 / 0.34 | 9; Darkbloom, DeepInfra, Cloudflare, NextBit, SiliconFlow, Novita, Venice, Parasail, Google | low-cost direct-video/frame candidate |
| [`openai/gpt-4.1-mini`](https://openrouter.ai/api/v1/models/openai/gpt-4.1-mini/endpoints) | image, text, file | 0.40 / 1.60 | 3; Azure, OpenAI | frame/text incumbent; no direct video |
| [`openai/gpt-5-mini`](https://openrouter.ai/api/v1/models/openai/gpt-5-mini/endpoints) | text, image, file | 0.25 / 2.00 | 4; OpenAI, Azure | sampled frame/text challenger; no direct video |
| [`anthropic/claude-sonnet-5`](https://openrouter.ai/api/v1/models/anthropic/claude-sonnet-5/endpoints) | text, image, file | 2.00 / 10.00 | 9; Anthropic, AWS/Bedrock, Azure, Google | sampled premium frame/text ceiling; no direct video |
| [`google/gemini-3.1-flash-lite`](https://openrouter.ai/api/v1/models/google/gemini-3.1-flash-lite/endpoints) | text, image, video, file, audio | 0.25 / 1.50 | 7; Google, Google AI Studio | sampled low-cost direct-video/frame challenger |
| [`qwen/qwen2.5-vl-72b-instruct`](https://openrouter.ai/api/v1/models/qwen/qwen2.5-vl-72b-instruct/endpoints) | text, image | 0.80 / 1.00 | 2; Nebius, Parasail | sampled OCR/frame regression reference; no direct video |

All eight advertise `response_format`/structured output at the model level. Provider support is
still endpoint-specific, so certification must set `provider.require_parameters: true`; OpenRouter
documents that this excludes providers that cannot honor all requested parameters
([structured outputs](https://openrouter.ai/docs/guides/features/structured-outputs),
[provider selection](https://openrouter.ai/docs/guides/routing/provider-selection)). The endpoint
APIs also expose provider tag, quantization, context, supported parameters, and exact endpoint price;
copy those fields into the run manifest before the first request.

### Reproducible routing contract

For each certification cell:

- send one concrete `model` ID, never a `latest` alias or a multi-model `models` array;
- constrain `provider.only` (or a one-entry `provider.order`) to one preselected provider;
- set `provider.allow_fallbacks: false` and `provider.require_parameters: true`;
- set `provider.data_collection: "deny"`, and require `provider.zdr: true` only when the endpoint
  snapshot proves that a compatible endpoint remains;
- use strict JSON Schema output and record the requested ID, catalog `canonical_slug`, response
  model, provider name/tag, endpoint pricing, modality, derivative hash, token usage, charged cost,
  latency, and failure class;
- keep concurrency at one and enforce request, media-byte, token, and dollar ceilings before the
  first paid call.

OpenRouter's default is to load-balance eligible providers and allow provider fallback; `only`,
`order`, `allow_fallbacks`, `require_parameters`, `data_collection`, and `zdr` are the documented
controls that remove that ambiguity
([provider selection](https://openrouter.ai/docs/guides/routing/provider-selection)). Model fallback
arrays can also change which model is billed and returned, so they do not belong in certification
([model fallbacks](https://openrouter.ai/docs/guides/routing/model-fallbacks)).

Direct video uses `video_url` content at `/api/v1/chat/completions` and supports MP4, MPEG, MOV, and
WebM. Public URLs or base64 data URLs are accepted, but transport varies by provider: Google AI
Studio accepts only YouTube URLs for Gemini, while Vertex requires base64 rather than video URLs
([OpenRouter video inputs](https://openrouter.ai/docs/guides/overview/multimodal/videos)). Because
Loomarr's certification media is local and provenance-locked, use the same bounded base64 derivative
for every compatible provider; do not let a URL-versus-base64 difference contaminate the model
comparison.

## 3. Immediate certification sequence

1. Inventory maintainer-owned clips and the adjudicable Archive, Commons, and Library pools without
   downloading bulk media; reject candidates missing affirmative per-item rights evidence.
2. Independently review and adjudicate rights, filler role, taxonomy, reject class, and policy flags;
   cluster before assigning development/holdout splits.
3. Freeze 300--500 cases if the lawful pool supports it, retaining external media by hash when it
   cannot be redistributed.
4. Refresh the OpenRouter model and endpoint JSON, choose one provider per cell, calculate the
   maximum possible spend from the locked request/token ceilings, and stop for operator confirmation.
5. Only then run the serial paid bakeoff and subsequent fictional-media-server shadow lane.

This order avoids paying to benchmark an unrepresentative or legally ambiguous corpus and prevents
provider routing changes from masquerading as model-quality differences.

## 4. 2026-08-25 rights-authority audit

### Internet Archive license fields are leads, not independent approval

**Decision:** Loomarr cannot independently rights-approve a vintage commercial merely because its
Internet Archive `licenseurl` is PDM, the legacy `licenses/publicdomain` URL, CC0, CC BY, or CC
BY-SA. Internet Archive defines `licenseurl`, `rights`, and `possible-copyright-status` as fields
defined and editable by the uploader
([Archive metadata schema](https://archive.org/developers/metadata-schema/index.html#licenseurl)).
Its own rights guidance says it does not guarantee an item's copyright status or the rights
information on item and collection pages, describes the CC choice as a license *assigned by the
uploader*, and puts non-infringement review on the reuser
([Internet Archive Rights](https://archivesupport.zendesk.com/hc/en-us/articles/360014759692-Rights)).
The Archive is therefore a host and metadata transport for this purpose, not the authority that
verified the uploader's chain of title.

The labels have materially different effects, but none supplies the missing identity and authority
proof:

- PDM is an informational label, not a legal tool. Creative Commons says anyone may apply it only
  after researching and confirming that the work is already free of copyright worldwide; it should
  not be used for uncertain or jurisdiction-specific status, including status based only on failed
  U.S. formalities
  ([CC public-domain tools](https://creativecommons.org/public-domain/),
  [PDM guidance](https://creativecommons.org/public-domain/pdm/)). An uploader-selected PDM URL
  records the assertion but not the research behind it.
- The legacy `http://creativecommons.org/licenses/publicdomain/` label corresponds to Creative
  Commons' U.S.-specific Public Domain Dedication and Certification. Creative Commons retired it in
  2010 because it mixed certification and dedication, and no longer recommends applying it
  ([retired-tools table](https://creativecommons.org/retiredlicenses/),
  [legacy deed](https://creativecommons.org/publicdomain/certification/1.0/us/deed.en)). It is not a
  current worldwide clearance mechanism.
- CC0 is a waiver/dedication by an affirmer only to the extent that person owns the copyright and
  related rights. Creative Commons instructs users to apply CC0 only to their own work or when they
  have legal authority, and warns that trademark, privacy, publicity, and other people's rights can
  remain
  ([CC0 usage boundary](https://creativecommons.org/public-domain/),
  [CC0 legal code](https://creativecommons.org/publicdomain/zero/1.0/legalcode.en)).
- CC BY and BY-SA grant only rights the licensor has authority to grant. Creative Commons tells
  licensors to secure all necessary rights and separately excludes patent and trademark rights;
  privacy, publicity, moral, performance, broadcast, sound-recording, and embedded-party rights can
  remain, while BY also requires attribution and change/license notices
  ([CC BY 4.0 legal code](https://creativecommons.org/licenses/by/4.0/legalcode.en)). Older BY/BY-SA
  versions in the inventory must be preserved and evaluated under their exact linked version, not
  normalized to 4.0.

The bounded 2026-08-25 inventory selected 331 items from the official
[`classic_tv_commercials` licensed-item query](https://archive.org/advancedsearch.php?q=collection%3Aclassic_tv_commercials%20AND%20mediatype%3Amovies%20AND%20licenseurl%3A%2A&fl%5B%5D=identifier&fl%5B%5D=licenseurl&rows=1000&sort%5B%5D=identifier%20asc&output=json):
165 PDM, 121 legacy public-domain, 24 CC0, and 21 BY/BY-SA items; 266 are at most 90 seconds. Those
numbers measure technical candidates and uploader assertions, not cleared works. Only one selected
item had separate `rights` prose and none had `possible-copyright-status`; that item combines PDM
with a Fair Use Notice, an unresolved contradiction visible in the
[`crying_indian_psa_hd` metadata](https://archive.org/metadata/crying_indian_psa_hd). This is direct
evidence that presence of an allowlisted URL cannot be the admission predicate.

Accordingly, all 331 start as `rights_unverified`. A case can advance only when Loomarr records an
independent basis such as a primary rightsholder's license/waiver, documented federal authorship,
or an institutional item-level rights determination with traceable reasoning. The current
`1968EasterSealsWithCarolBurnett` probe must be re-adjudicated if its approval relied only on the
Archive PDM field. Conflicting rights prose, fair-use rationales, unclear uploader authority,
unresolved music/performance/footage, or a brand/personality restriction remains a hold or reject;
duration and semantic suitability do not cure provenance.

### Rights-safer pools that can reach hundreds of short cases

| Pool | Scale and machine interface | Admission boundary |
| --- | --- | --- |
| Library of Congress Citizen DJ government-film subset | The Library identifies the subset as U.S.-government-created and public domain. Citizen DJ exposes 201 audio segments and 4,096 audio one-shots derived from 18 source films; those 4,297 samples are not video cases. Source item pages are reachable through the no-key LOC JSON API ([Citizen DJ rights and inventory](https://citizen-dj.labs.loc.gov/loc-national-screening-room/use/), [LOC API](https://www.loc.gov/apis/json-and-yaml/)). | Strong seed for deriving bounded video controls, PSAs, and promo-like segments from the 18 cleared source films, but insufficient alone for a representative 300-case video corpus. Keep every derivative of one source film in one split. Do not generalize the decision to the mixed-rights National Screening Room, because LOC normally tells users to determine rights themselves ([LOC legal notice](https://www.loc.gov/legal/)). |
| NARA Catalog | The key-gated Catalog API exposes archival descriptions and digital objects and supports field search and bulk export; NARA separately explains that official federal-employee works are uncopyrightable in the United States ([Catalog API and terms](https://www.archives.gov/research/catalog/help/api), [motion-picture permissions](https://www.archives.gov/research/motion-pictures/permissions)). | Admit only records that identify federal creation and have no conflicting restriction. NARA warns that many Special Media holdings contain third-party, contract, donor, publicity, performance, or foreign rights. Cache immutable evidence outside the API if permitted: current API terms say not to cache returned content, require a key and NARA attribution notice, and direct true bulk use to the AWS open-data export ([Catalog API terms](https://www.archives.gov/research/catalog/help/api)). |
| NASA Image and Video Library | The official search API supports `media_type=video`, paging, NASA IDs, centers, creators, dates, and asset manifests ([NASA API documentation](https://images.nasa.gov/docs/images.nasa.gov_api_docs.pdf)). NASA describes extensive video galleries and says NASA media generally lacks U.S. copyright ([NASA media guidelines](https://www.nasa.gov/nasa-brand-center/images-and-media/)). | Useful at hundred-case scale for IDs, bumpers, educational/PSA-like positives, and controls, but only after rejecting third-party-marked media and resolving embedded music/footage, recognizable people, logos, and promotional/non-endorsement constraints. Follow NASA's separate AI-application disclosure and branding restrictions when frames are used in model evaluation ([NASA media and AI guidance](https://www.nasa.gov/nasa-brand-center/images-and-media/)). |
| Creator-controlled CC video | Vimeo's API exposes the exact CC license, owner/user, duration, copyright-restriction state, and whether download is enabled; Vimeo requires uploaders to possess the rights and permissions to upload and share ([video response fields](https://developer.vimeo.com/api/reference/response/video), [Vimeo terms](https://vimeo.com/legal/)). | This is safer only for a directly verified creator account or a written creator grant, not arbitrary CC search results. Import only CC0/BY/BY-SA, require downloadable media, verify creator identity and authority over music/performers/stock, and preserve attribution/version. Direct file links require authenticated scopes and an eligible Vimeo plan, so the API is not an anonymous bulk-download corpus ([file-link requirements](https://developer.vimeo.com/api/files/video-links)). |

Smithsonian Open Access is strong institutional evidence for assets actually marked CC0, and its
public API is available through `api.data.gov`, but its official FAQ says video and sound are among
the formats not yet fully incorporated into Open Access. It should be a supplemental, item-verified
source rather than the plan for hundreds of video cases
([Smithsonian Open Access FAQ](https://www.si.edu/openaccess/faq),
[Smithsonian terms](https://www.si.edu/termsofuse)).

The practical corpus strategy is therefore to use LOC Citizen DJ as a small rights-safe seed; add
NARA and NASA only under their item-level flags; and use directly verified creator-owned CC video for
commercial/promo diversity. Archive and Commons remain high-value discovery pools for authentic
vintage advertisements, but their uploader labels never bypass independent rights adjudication.
There is not yet a qualified primary source for a representative 300-case video corpus. Before a new
source receives an importer, it must demonstrate useful coverage of commercials, promos, bumpers,
station IDs, trailers, and PSAs; rights clarity and inventory scale alone are not sufficient.
