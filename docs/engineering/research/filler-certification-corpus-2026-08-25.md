# Filler certification corpus sources

**Status:** source-selection decision for issues #549 and #555

**Decision date:** 2026-08-25

**Reconciled:** 2026-08-26 against the completed source-yield pilots and
[`filler-certification-source-qualification-2026-08-26.md`](filler-certification-source-qualification-2026-08-26.md)

## Decision

No single public collection is representative enough to certify Loomarr's filler-admission policy.
The corpus will combine item-reviewed public material with commissioned or directly licensed modern
masters. DVIDS is excluded: its rights posture and API are convenient, but military public-affairs
video does not cover the commercials, promos, bumpers, station IDs, trailers, and PSAs the product
must distinguish.

The locked corpus target is 300 cases: 190 eligible filler cases and 110 invalid or ambiguous
controls. Source discovery is not rights approval. Every admitted case still requires the item-level
rights record, immutable media and metadata hashes, independent semantic labels, and split-cluster
controls required by design §10.

## Target mix

| Slice | Cases | Acquisition plan |
| --- | ---: | --- |
| Commercials | 35 | 15 item-cleared LOC/Prelinger; 20 commissioned modern masters |
| Promos | 35 | 15 NASA/LOC; 20 commissioned or directly licensed |
| Bumpers | 25 | Commissioned or directly licensed |
| Station IDs | 25 | Commissioned or directly licensed; confirmed Commons files may replace, not expand, this target |
| Trailers | 35 | 30 item-cleared NASA/LOC/Commons or static open works; 5 directly licensed independent works |
| PSAs | 35 | 30 item-cleared CDC/federal/LOC/Prelinger; 5 commissioned or directly licensed |
| Programme excerpts | 20 | Rights-cleared negative controls |
| Compilations | 15 | Rights-cleared negative or ambiguous controls |
| Fragments | 15 | Deliberate bounded cuts from approved masters |
| Degraded or corrupt media | 15 | Deterministic derivatives from approved masters |
| Non-filler institutional video | 15 | Rights-cleared negative controls |
| Adversarial/instruction-bearing media | 10 | Commissioned or deterministic derivatives |
| Conflicting evidence | 10 | Curated packets over approved masters |
| Sensitive/policy/rights conflicts | 10 | Curated held or reject cases; never playback authority |

The source-yield gate is the locked 50-candidate pilot: ten each from Prelinger, Library of Congress,
NASA, CDC, and Wikimedia Commons. An independent reviewer must approve both rights and product
relevance for at least five rows in a lane before it can scale into the bounded 155-candidate public
inventory. In parallel, commission or directly license at least 100 accepted modern positives: 20
commercials, 20 promos, 25 bumpers, 25 station IDs, 5 trailers, and 5 PSAs. Neither discovery nor the
pilot is certification or download authority.

## Source lanes

### Prelinger Archives on Internet Archive

Discover through Internet Archive Advanced Search with `collection:prelinger`. Prelinger says it
reviews the copyright status of films it makes available, while Internet Archive expressly declines
to provide a rights warranty. A collection field, uploader, downloadability, or `licenseurl` is
therefore discovery evidence only; the item page and its claimed licence must be reviewed and bound
to the exact representation.

- [Prelinger Archives reuse guidance](https://www.panix.com/~footage/prelarch.html)
- [Internet Archive terms](https://archive.org/about/terms.php)
- [Internet Archive metadata API](https://archive.org/developers/md-read.html)

### Library of Congress National Screening Room

Use the collection's JSON response (`fo=json`) at no more than 20 requests per minute. Candidate
commercials, advertising films, public-service material, and trailers remain subject to the rights
and access statement on each item. Library possession is not a blanket public-domain decision.

- [National Screening Room](https://www.loc.gov/collections/national-screening-room/)
- [Library of Congress JSON/YAML API](https://www.loc.gov/apis/json-and-yaml/)
- [Library of Congress rights guidance](https://www.loc.gov/homepage/legal.html)

### NASA

Use the NASA Images API search endpoint with `media_type=video`. NASA material is generally usable
under the agency media guidelines, but NASA identifiers, depicted people, music, and third-party
material require item-level review. This lane is useful for trailers and promos, not commercials or
station identity.

- [NASA Images API](https://images.nasa.gov/docs/images.nasa.gov_api_docs.pdf)
- [NASA media usage guidelines](https://www.nasa.gov/nasa-brand-center/images-and-media/)

### CDC

Use current first-party short PSA material. Most federal CDC-authored material is public domain, but
contractor and third-party elements remain exceptions; attribution and non-endorsement requirements
must be recorded per asset.

- [CDC public-domain and copyright guidance](https://www.cdc.gov/other/agencymaterials.html)
- [CDC media resources](https://www.cdc.gov/digital-media-tools/)

### Wikimedia Commons

Use the MediaWiki API for category discovery and `imageinfo`/`extmetadata`, then review the exact file
page and its source chain. Category membership and structured licence metadata are not chain-of-title
proof. Commons is a small supplementary lane, not the corpus backbone.

- [MediaWiki API image information](https://www.mediawiki.org/wiki/API:Imageinfo)
- [Commons licensing policy](https://commons.wikimedia.org/wiki/Commons:Licensing)

## Rights evidence threshold

An asset may proceed to download and labeling only with one of:

1. A federal item record plus the responsible agency policy and an item-level third-party check.
2. A first-party owner licence for the exact work that permits the required commercial copying,
   modification, redistribution, and provider evaluation.
3. A signed owner agreement bound to the asset checksum.
4. An item-specific public-domain analysis recorded by the rights reviewer.

Uploader assertions, API licence fields, collection membership, and download availability remain
candidate signals. They never authorize acquisition, model upload, redistribution, or admission.

## Execution order

1. Independently review every row in the locked five-lane, 50-candidate source-yield pilot.
2. Scale only lanes with at least five rights-approved, product-relevant pilot rows into the bounded
   155-candidate public inventory.
3. Commission or directly license the 100-case modern positive cohort in parallel.
4. Complete item-level rights review, then acquire and hash approved-only media into the external
   corpus store.
5. Build source and similarity clusters before assigning development and holdout splits.
6. Run two blind semantic label batches and third-party adjudication where they disagree.
7. Lock the 300-case manifest, then run the bounded provider bakeoff on identical evidence packets.
