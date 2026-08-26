# Filler certification corpus sources

**Status:** source-selection decision for issues #549 and #555
**Decision date:** 2026-08-25

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
| Commercials | 35 | 20 item-cleared LOC/Prelinger; 15 commissioned modern masters |
| Promos | 35 | 15 NASA/LOC; 20 commissioned or directly licensed |
| Bumpers | 25 | Commissioned or directly licensed |
| Station IDs | 25 | 20 commissioned or directly licensed; up to 5 confirmed Commons files |
| Trailers | 35 | 10 Blender, 10 NASA, 10 confirmed Commons, 5 licensed independent works |
| PSAs | 35 | 20 CDC/federal; 15 LOC/Prelinger |
| Programme excerpts | 20 | Rights-cleared negative controls |
| Compilations | 15 | Rights-cleared negative or ambiguous controls |
| Fragments | 15 | Deliberate bounded cuts from approved masters |
| Degraded or corrupt media | 15 | Deterministic derivatives from approved masters |
| Non-filler institutional video | 15 | Rights-cleared negative controls |
| Adversarial/instruction-bearing media | 10 | Commissioned or deterministic derivatives |
| Conflicting evidence | 10 | Curated packets over approved masters |
| Sensitive/policy/rights conflicts | 10 | Curated held or reject cases; never playback authority |

The first metadata-only inventory is bounded to 120 candidates: 35 Prelinger, 25 Library of
Congress, 25 NASA, 15 CDC, 10 Blender open-movie trailers, and 10 Wikimedia Commons files. Attrition
is expected. In parallel, acquire at least 75 accepted modern masters across commercials, promos,
bumpers, and station IDs. The pilot is useful only as a rights-review queue; it cannot certify.

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

### Blender open movies

Use first-party pages for exact open-movie works and their CC BY terms. These provide modern,
redistributable trailer material with a clear owner-to-work licence chain, subject to the stated
credit.

- [Blender Studio films](https://studio.blender.org/films/)
- [Big Buck Bunny copyright](https://peach.blender.org/about/)
- [Sintel copyright](https://durian.blender.org/about/)

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

1. Freeze the 120-row metadata inventory with raw-response hashes and hard request/byte ceilings.
2. Complete rights review; expect and report attrition without backfilling from an unreviewed source.
3. Acquire and hash only approved media into the external corpus store.
4. Build similarity clusters before assigning development and holdout splits.
5. Run two blind semantic label batches and third-party adjudication where they disagree.
6. Lock the 300-case manifest, then run the bounded provider bakeoff on identical evidence packets.
