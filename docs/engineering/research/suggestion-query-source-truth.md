# Source truth for semantic-query expansion

Observed: 2026-09-15, America/New_York. Tracking:
[#1278](https://github.com/loomarr/loomarr/issues/1278). A bounded read-only
Terra/Medium investigation gathered primary sources; the delivery owner checked
the pages and refined the evidence limits below. No paid inference, household
integrations, runtimes or fixtures were changed.

## Supported facts and limits

| Reference | Supported claim | What it does not prove |
| --- | --- | --- |
| [Walt Disney Archives / D23](https://d23.com/were-going-to-walt-disney-world-sabrina-the-teenage-witch/) | TGIF began in 1989; Sabrina the Teenage Witch joined in 1996. The article also names Full House, Boy Meets World, Family Matters and Step by Step. | A complete roster, every season's membership, or every title's exact Friday slot. |
| [ABC's fall 2018 TGIF announcement](https://abc.com/news/010c5f8c-9176-401e-b0ed-4bf00fc5c41d/category/1138628) | ABC announced a new TGIF era featuring Fresh Off the Boat, Speechless and Child Support on October 5, 2018. | Those titles' membership in the original 1990s block. A reused block name is not one timeless set. |
| [Paley's original Sabrina broadcast record](https://www.paleycenter.org/collection/item?item=T%3A51561&p=73&q=all) | One archived episode is dated November 7, 1997 on ABC. The record distinguishes the ABC 1996–2000 run from The WB 2000–2003. | Slot-by-slot TGIF membership or permission to treat the whole series as ABC-era programming. |
| [HISTORY's Modern Marvels page](https://www.history.com/shows/modern-marvels) | The official catalog associates Modern Marvels with HISTORY and describes its technology-history subject. | Original-series premiere, original broadcast network for every episode, or a complete 1990s network roster. |
| [HISTORY's Modern Marvels Season 2](https://www.history.com/shows/modern-marvels/season-2) | It records 1996 air dates for Las Vegas, Golden Gate Bridge, Panama Canal and The Computer. | That the first broadcast network was HISTORY for every listed episode; a current catalog is not complete historical network evidence. |
| [Disney UK's MCU phase retrospective](https://www.disney.co.uk/movies/avengers-infinity-war/marvel-cinematic-universe) | Its Phase 1 section lists Iron Man, The Incredible Hulk, Iron Man 2, Thor, Captain America: The First Avenger and Avengers Assemble. Phase 2 separately lists Iron Man 3 and Thor: The Dark World. | That regional title strings alone resolve the correct Catalog Key, or that current streaming availability establishes phase membership. |
| [Marvel's Avengers Guidebook](https://www.marvel.com/comics/issue/57704/guidebook_to_the_marvel_cinematic_universe-_marvels_the_avengers_2016_1) | The official indexed description places The Avengers at the end of Phase One. | An independently resolved six-film Catalog allowlist. The owner could read the indexed description; direct Marvel page access was restricted. |

No frozen pilot or certification facts have been upgraded from synthetic to
historically verified by this note. Fixture-ready **claims** still need grounded
provider IDs, a versioned fact artifact, digest binding and independent outcome
review before becoming executable expectations.

## Questionable or incomplete evidence

HISTORY's Season 2 page also presents Gothic Cathedrals with a premiere date of
November 11, 2231. Treat that as questionable metadata, not a valid historical
boundary or a value to silently repair. First-party ownership does not make every
field trustworthy. Missing dates remain unknown.
[Observed episode page](https://www.history.com/shows/modern-marvels/season-2)

The inspected official pages do not settle Modern Marvels' original premiere or
complete original-network timeline. Do not replace the pilot's synthetic 1993
fact with a guessed 1995 fact, and do not assert that every early episode first
aired on HISTORY. A contemporaneous A+E/network press or broadcast archive is
still needed for those predicates.

The research worker also inspected a current Disney+ Sabrina listing whose date
range differs from Paley's archived run. The owner did not independently verify
that streaming listing; it is not adopted as an oracle. Current catalog packaging
cannot overwrite a dated broadcast record. More corroboration is required before
episode/network boundaries become hard expectations.

The Marvel collected Guidebook page was blocked by HTTP 403 during the owner's
check. Its inferred phase roster is therefore not the authority for this note.
Disney UK's explicitly separated phase lists supply the supported roster above.
The UK name Avengers Assemble must be resolved to the same film identity as the
appropriate regional Avengers title, never counted as a second movie.

## Fixture review consequences

These are engineering implications of the evidence, not additional historical facts:

- For an explicitly original/1990s ABC TGIF request, Sabrina is a supported
  positive; a title from the 2018 revival is a useful era negative. Do not assume
  an unqualified TGIF request explicitly asks for episode-airing dates.
- Do not infer every season's ABC/block membership from title-level inclusion.
  Series inclusion, editorial epoch, premiere and episode airing are distinct.
- Modern Marvels is a useful technology-history exemplar for the user's request.
  A History-like editorial epoch must not automatically impose a pre-2000 episode
  cutoff. Separate explicit episode-date wording still requires its own date axis.
- Treat questionable source dates as an uncertainty/control family. Never build
  a supposedly complete History roster from one current series page.
- Use Disney's Phase 1 list for reviewed MCU member claims and its Phase 2 list
  for later-film contrasts. Resolve catalog IDs independently; avoid a generic
  Marvel keyword lookup as membership evidence.
- Keep historical association, present streaming licensing and household Library
  availability as different facts. None can substitute for the others.

## First executable identity review

The first expansion batch independently resolves four positive series Keys:
Family Matters [2685](https://www.themoviedb.org/tv/2685-family-matters?language=he-IL),
Step by Step [2617](https://www.themoviedb.org/tv/2617-step-by-step),
Boy Meets World [1777](https://www.themoviedb.org/tv/1777-boy-meets-world/images/backdrops?language=en-US)
and Sabrina [605](https://www.themoviedb.org/tv/605-sabrina-the-teenage-witch).
The final source explicitly identifies the 1996 sitcom; punctuation differences
are normalized without changing identity. These are current provider identities,
not proof that every season or episode was broadcast in TGIF.

The negative Full House [31183](https://www.themoviedb.org/tv/31183-full-house)
is the 2009 Philippine adaptation, not the American sitcom. Attempts to read the
original American Full House provider page were restricted; its Key is therefore
not promoted into this batch's independently verified positive pool. This is a
partial roster, not a claim that Full House was absent from TGIF. Frozen pilot
IDs are explicitly synthetic and remain untouched.

Reviewed membership and identity records, observation date 2026-09-15, authored
paraphrased excerpts and uncertainty are recorded in the new digest-bound
`query-expansion-v1.json` manifest and its source artifact. Simulated Library
presence/ratings are labeled separately. There is no complete page capture or
historical season oracle. See the [executable batch evidence](../plans/suggestion-query-evaluation.md#first-executable-expansion-batch).

## Second executable identity review

Observed 2026-09-16. Disney UK's phase retrospective remains the membership
authority: Phase 1 names *Iron Man*, *The Incredible Hulk*, *Iron Man 2*,
*Thor*, *Captain America: The First Avenger* and the UK title *Avengers
Assemble*; its Phase 2 section separately begins with *Iron Man 3* and *Thor:
The Dark World*. Current TMDB identity pages resolve those titles to movie Keys
[1726](https://www.themoviedb.org/movie/1726-iron-man),
[1724](https://www.themoviedb.org/movie/1724-the-incredible-hulk),
[10138](https://www.themoviedb.org/movie/10138-iron-man-2),
[10195](https://www.themoviedb.org/movie/10195-thor),
[1771](https://www.themoviedb.org/movie/1771-captain-america-the-first-avenger)
and [24428](https://www.themoviedb.org/movie/24428-the-avengers). The last
identity is one film: the source's regional *Avengers Assemble* string does not
create a second Catalog Key beside *The Avengers*.

Phase 2 controls use *Iron Man 3*
[68721](https://www.themoviedb.org/movie/68721-iron-man-3) and *Thor: The Dark
World* [76338](https://www.themoviedb.org/movie/76338-thor-the-dark-world).
Separate title-collision controls use the documentary *The Avengers: A Visual
Journey* [448368](https://www.themoviedb.org/movie/448368-the-avengers-a-visual-journey)
and *Almighty Thor* [63736](https://www.themoviedb.org/movie/63736-almighty-thor).
Provider pages establish identity and current title/year metadata only; Disney's
phase list establishes membership. The v2 fixture's Library ownership and
ratings are simulated and explicitly labeled as such.

## Remaining review queue before further case authoring

1. Map original American Full House and later TGIF revival titles to exact provider Keys without
   replacing synthetic identities in frozen manifests. The six Phase One identities are now in the
   cumulative v2 development corpus.
2. Record source URL, observation date, supported role/epoch, excerpt digest and
   uncertainty for each new fixture fact. Keep retrieved page data untrusted.
3. Obtain an independently reviewed historical-network source for each additional
   History-era member; mark incomplete rosters incomplete.
4. Author each positive expectation and its negative contrast independently of
   the cooperative model trace, then exercise the production Suggester/Runner.
5. Keep all exposed pilot and registry siblings in development. Author new,
   unexposed families separately for the later sealed holdout.

The [100-family registry](../plans/suggestion-query-family-registry.md) defines
the authoring backlog. Its rows are proposed development families, not passed
tests, provider qualification or historical lineup completeness.
