# Filler certification sources and OpenRouter routing snapshot

**Compiled 2026-08-25.** This note complements
[`filler-admission-confidence.md`](filler-admission-confidence.md): it narrows the acquisition pool to
sources whose rights evidence can be recorded per item and freezes the hosted candidates before any
paid bakeoff. No inference request was submitted and no OpenRouter credit was spent. This is an
engineering provenance policy, not legal advice.

## Verdict

Build the first representative corpus from three tiers, in this order:

1. clips already held by the maintainer, where Loomarr can record the maintainer's authority and the
   original file hash;
2. Internet Archive items with affirmative, machine-readable item-level rights metadata and an
   independent adjudication, especially the affirmatively marked portion of the Prelinger collection;
3. Library of Congress and federal-agency material with an item-level public-domain statement.

Do **not** treat membership in Internet Archive, Prelinger, the National Screening Room, NARA, or a
NASA site as a blanket license. Each of those repositories contains mixed-rights material or warns
that third-party, privacy, publicity, trademark, donor, or territorial restrictions can remain
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

1. Inventory maintainer-owned clips and the rights-cleared Archive/Library pools without downloading
   bulk media; reject candidates missing affirmative per-item rights evidence.
2. Independently review and adjudicate rights, filler role, taxonomy, reject class, and policy flags;
   cluster before assigning development/holdout splits.
3. Freeze 300--500 cases if the lawful pool supports it, retaining external media by hash when it
   cannot be redistributed.
4. Refresh the OpenRouter model and endpoint JSON, choose one provider per cell, calculate the
   maximum possible spend from the locked request/token ceilings, and stop for operator confirmation.
5. Only then run the serial paid bakeoff and subsequent fictional-media-server shadow lane.

This order avoids paying to benchmark an unrepresentative or legally ambiguous corpus and prevents
provider routing changes from masquerading as model-quality differences.
