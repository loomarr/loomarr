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

The earlier 300-case target is retired: it cannot satisfy the confidence bounds the scorer claims.
The locked corpus now requires at least 1,426 cases: a separate 300-case development set plus a
1,126-case holdout with 446 eligible positives, 446 deterministic-invalid controls, 147
semantic-invalid controls, and 87 ambiguous cases with answerable review questions. Those
denominators use the scorer's one-sided 95% Wilson method and tolerate one observed error at the
99% admit/deterministic-reject, 97% semantic-reject, and 95% review-answerability gates.

Source discovery is not rights approval. Every case still requires its item-level rights record,
immutable media and metadata hashes, independent semantic labels, and split-cluster controls.

## Target mix

| Eligible holdout role | Minimum independent cases |
| --- | ---: |
| Commercials | 82 |
| Promos | 82 |
| Bumpers | 59 |
| Station IDs | 59 |
| Trailers | 82 |
| PSAs | 82 |

No holdout cluster, campaign, or source master may contribute more than one case. No creator may supply more than
10% of a role, and no source may supply more than 25% of eligible holdout cases. Deterministic
derivatives remain useful controls, but every derivative family is one cluster and cannot inflate an
independent denominator. Synthetic material cannot stand in for authentic positives.

The source-yield pilot remains discovery evidence only. The
[content-addressed review summary](../evidence/filler-pilot-rights-review-2026-08-26.json) qualified CDC and
did not qualify Prelinger, LOC, NASA, or Commons under the common five-of-ten rights-and-relevance
gate. That result is not acquisition authority and is far short of the authentic, diverse pool this
contract requires. Directly licensed creator/broadcaster masters are therefore the critical path;
the former 100-case direct cohort is useful input, not a complete positive denominator.

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
2. Scale CDC only; do not build adapters for failed lanes merely because their APIs are convenient.
3. Commission or directly license enough independent creator/broadcaster masters to satisfy every
   role, creator, campaign, source, and split constraint above.
4. Complete item-level rights review, then acquire and hash approved-only media into the external
   corpus store.
5. Build source and similarity clusters before assigning development and holdout splits.
6. Run two blind semantic label batches and third-party adjudication where they disagree.
7. Generate separate opaque-alias packets for both reviewers, mechanically unblind their submissions,
   lock the schema-v5 manifest, then run the bounded provider bakeoff on identical evidence packets.
