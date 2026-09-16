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

## History editorial-era source review

Observed 2026-09-16, America/New_York. This is a bounded review of the U.S.
History Channel/HISTORY request behind EPO-01 and DAT-04/05, not a recovery of
the channel's complete 1990s schedule. The sources below distinguish the
network's catalog from a generic ``history'' subject query and distinguish an
editorial-era qualifier from an episode-airing filter.

| Reference | Directly supported fact | Fixture-safe use | Limit / non-claim |
| --- | --- | --- | --- |
| [HISTORY's Modern Marvels series page](https://www.history.com/shows/modern-marvels) | The present A+E-owned HISTORY catalog carries *Modern Marvels* and describes it as a series about the history of technology. | The named-series thematic anchor for a request that explicitly includes *Modern Marvels*. | Its current catalog presence, season count and streaming presentation do not establish a 1990s premiere, a historical schedule, or the membership of another program. |
| [HISTORY Season 2](https://www.history.com/shows/modern-marvels/season-2) | The catalog dates technology/history episodes including *Las Vegas* (January 6), *The Tennessee Valley Authority* (January 7), *Golden Gate Bridge* (February 3), *Panama Canal* (February 25), *Eiffel Tower* (March 10), *Tunnels* (April 27), *The Computer* (November 24) and *Captured Light* (December 15), all in 1996. | Positive historical-date evidence for the one *Modern Marvels* anchor. It supports a technology, infrastructure and invention-oriented semantic slice. | The page does not label these original broadcasts or name the original network; treat each displayed date as an episode-date fact, not a whole-series premiere or a complete season chronology. |
| [HISTORY Season 3](https://www.history.com/shows/modern-marvels/season-3) | It dates *Satellites* August 17, 1997; *Radio* August 24, 1997; *Radar* September 21, 1997; *Polio Vaccine* October 5, 1997; and further episodes through February 14, 1998. | Corroborates that the same anchor's dated catalog spans late 1997 into early 1998. | It does not independently prove what else the channel carried, or that a user asking for the 1990s wants only episodes with 1990s air dates. |
| [HISTORY Season 4](https://www.history.com/shows/modern-marvels/season-4) and [Season 5](https://www.history.com/shows/modern-marvels/season-5) | They date *Extreme Sports Gadgets* November 3, 1998; *Weather Prediction* December 7, 1998; *Airships* January 18, 1999; and, in Season 5, *City Parks* March 9, *Spy Technology* March 15, *Dynamite* June 21 and *New York Bridges* August 2, 1999. | Corroborates a 1998--99 continuation of the anchor's technology/history format. | The current catalog has incomplete and questionable date fields; it cannot establish all airings or a network-wide editorial roster. |
| [Library of Congress moving-image finding aid: *Modern Marvels: Baseball Parks*](https://wwws.loc.gov/rr/mopic/findaid/BASEBALL.pdf) | The institutional catalog identifies *MODERN MARVELS. BASEBALL PARKS* as a 1999 Arts and Entertainment Network work and names CBS News and History Channel. | Independent catalog corroboration for one 1999 *Modern Marvels* work and its History Channel association. | It is one collection record, not a schedule, lineup, original-airing assertion, or evidence for a different program. |
| [A&E chronology preserved by the Library of Congress](https://www.loc.gov/static/programs/national-film-preservation-board/documents/tva%26e.pdf) | A contemporaneous A&E chronology records that The History Channel launched with one million subscribers, later completed its first year with 7.8 million U.S. subscribers, and launched in Great Britain and Ireland in November. | Context that a distinct History Channel existed in the relevant period; keep the request in the network role rather than treating it as a generic subject label. | The extracted chronology does not put a self-contained calendar-year heading next to every entry. It must not supply an asserted U.S. launch date, a U.S./UK programming equivalence, or individual-program membership. |
| [HISTORY's Ancient Aliens Season 1](https://www.history.com/shows/ancient-aliens/season-1) and [HISTORY's *The Curse of Oak Island* Season 1](https://www.history.com/shows/the-curse-of-oak-island/season-1.) | HISTORY dates an *Ancient Aliens* episode April 20, 2010 and an *Oak Island* episode January 5, 2014. | Useful **post-1990s HISTORY** negative candidates when their Catalog Keys are independently resolved: they test that current/later channel association is not silently treated as 1990s editorial membership. | Their dates do not prove a channel-wide format transition, that either program is unsuitable for every History request, or any program's exact original premiere/network beyond the displayed episode fact. |

Current TMDB identity pages resolve *Modern Marvels* to
[`series:tmdb:6145`](https://www.themoviedb.org/tv/6145-modern-marvels),
*Ancient Aliens* to
[`series:tmdb:32608`](https://www.themoviedb.org/tv/32608-ancient-aliens), and
*The Curse of Oak Island* to
[`series:tmdb:60603`](https://www.themoviedb.org/tv/60603-the-curse-of-oak-island).
Those pages establish provider identity and current title metadata only. They do
not establish historical network membership, an editorial transition, or the
episode dates used above.

### Conclusions for a bounded representative roster

Direct source claims support **one positive program anchor only**: *Modern
Marvels*. Its official episode catalog supplies a deliberately small,
representative 1996--99 evidence slice (for example, *The Tennessee Valley
Authority*, *Radar*, *Weather Prediction*, and *New York Bridges*), but those
are episodes of one series, not four independently evidenced programs. The LOC
record independently corroborates the 1999 *Baseball Parks* work's History
Channel association. This is sufficient for a bounded ``1990s History Channel,
with Modern Marvels'' family where Modern Marvels is required and a small set of
independently resolved episode facts may exercise an explicitly requested
episode-date axis.

That conclusion is an engineering inference from the cited facts, not a source
claim that *Modern Marvels* represented all History Channel programming. A
representative roster may therefore be called **partial and single-anchor**, not
complete or network-defining. Do not add a second positive program merely
because it is historical, currently appears on HISTORY, appears in an external
TV database, or has a title containing ``History.'' No additional 1990s
History-program membership reached this review's primary-source threshold.

For independent negative contrasts, use a later HISTORY title with an explicit
post-1999 displayed episode date (*Ancient Aliens* or *The Curse of Oak Island*)
only after its Catalog Key is independently resolved. Treat a generic
``history documentary'' request as a role-collision control rather than as a
network request; and treat *History of Tall Buildings* on the Season 4 page as
an episode title, not an independently eligible series. These are controls for
semantic role and scope, not assertions that generic-history works are absent
from the network.

### Conflicts, gaps and explicit non-claims

- Season 2 still shows *Gothic Cathedrals* as premiering November 11, 2231.
  That impossible field remains a negative data-quality control. Also, the
  directly retrieved Season 5 page says *City Parks* aired March 9, 1999 while a
  search-index rendering had said March 3; cite the direct page if the item is
  ever used, but do not make it a boundary fact.
- The present first-party pages associate the series with HISTORY, yet do not
  state whether each displayed 1990s date is the original U.S. History Channel
  transmission. The LOC record is stronger for the named 1999 work, but does
  not repair that gap for every episode. A contemporaneous schedule, A+E press
  release, or preserved first-party program page is needed before asserting
  original-network predicates.
- The official Season 1 route was not available in this review. Do **not** turn
  the channel's 1995 launch context into a *Modern Marvels* 1995 premiere, and
  do not replace the pilot's synthetic 1993 fact.
- This review does not establish ratings, popularity, intended audience, a
  documentary-only channel, a shift caused by later programs, current licensing
  availability, a complete episode list, or a complete 1990s channel lineup.
  It does not make current HISTORY branding interchangeable with historical U.S.,
  UK, Canadian or other regional schedules.
- Before adding another positive series or episode, independently resolve it to
  a provider ID and record the source URL, observation date, concise reviewed
  excerpt, uncertainty and artifact digest. Keep source membership, Catalog
  identity, explicit episode dates and simulated Library facts separate.

## Movie lookup source review (#1290)

Observed 2026-09-16, America/New_York. This bounded development review covers
movie release epochs, person roles, country/language, audience evidence and a
small directional mood slice. It is not a canonical-movie list or a subjective
quality ranking.

### Authority roles

- A linked TMDB page resolves the current provider identity and therefore the
  `movie:tmdb:<id>` Key. Its title, year, keywords, popularity and community
  fields do not override a historical or national-film authority.
- AFI Catalog, Unifrance, KOFIC, Studio Ghibli and first-party studio pages may
  support the release, credit, genre, country, language or synopsis fields they
  state. Each field keeps its source and territory; it is not inferred from a
  translated title, setting or neighboring credit.
- A classification board supports only its own jurisdiction's classification
  and content advice. In particular, BBFC `PG` is not a U.S. MPA `PG` fact.
- Library ownership, ratings and availability in a fixture are simulated test
  controls. They are never learned from these public pages. Subtitle/audio
  availability is a current Media Observation, not a title-language fact.

### Reviewed candidate identities and usable facts

| Candidate and provider identity | Independently usable source facts | Boundary for fixture use |
| --- | --- | --- |
| *Casablanca* [`movie:tmdb:289`](https://www.themoviedb.org/movie/289-casablanca) | [AFI](https://catalog.afi.com/Film/27175-CASABLANCA) records a 26 November 1942 New York opening, Romance, director Michael Curtiz, United States and English. | TMDB currently displays 1943 while AFI distinguishes the 1942 opening from the January 1943 national release. It is safe for a 1940s case, but an exact-year case must name and bind its release territory. |
| *Singin' in the Rain* [`movie:tmdb:872`](https://www.themoviedb.org/movie/872-singin-in-the-rain) | [AFI](https://catalog.afi.com/Film/50652-SINGIN-INTHERAIN) records 11 April 1952, Musical comedy and directors Gene Kelly and Stanley Donen. | This supports release-era, genre and director facts, not an objective cheerful-mood label. |
| *2001: A Space Odyssey* [`movie:tmdb:62`](https://www.themoviedb.org/movie/62-2001-a-space-odyssey) | [AFI](https://catalog.afi.com/Catalog/moviedetails/23399) records 3 April 1968, Science fiction, director Stanley Kubrick, United Kingdom/United States and English. | Mixed country must remain mixed; setting in space supplies no origin fact. |
| *Jaws* [`movie:tmdb:578`](https://www.themoviedb.org/movie/578-jaws) | [AFI](https://catalog.afi.com/Film/55193-JAWS) records 20 June 1975, director Steven Spielberg, United States, English and an MPAA `PG` field; its synopsis and history describe the threatening/suspense context. | The catalogued rating is not a substitute for a rating-board record. Synopsis/history may be input to human mood review, not a source-certified fear score. |
| *E.T. the Extra-Terrestrial* [`movie:tmdb:601`](https://www.themoviedb.org/movie/601-e-t-the-extra-terrestrial?language=en-US) | [AFI](https://catalog.afi.com/Catalog/moviedetails/67140) records 11 June 1982, Fantasy/Science fiction, director Steven Spielberg, United States, English and an MPAA `PG` field. | Use release, genre and director deterministically; do not infer mood or current audience suitability from genre or an old catalogued rating. |
| *Back to the Future* [`movie:tmdb:105`](https://www.themoviedb.org/movie/105-back-to-the-future) | [AFI](https://catalog.afi.com/Film/55763-) records 3 July 1985, Comedy/Science fiction and director Robert Zemeckis; Steven Spielberg is credited as executive producer. | This is the required role negative for “directed by Spielberg”: producer association is not direction. |
| *Jurassic Park* [`movie:tmdb:329`](https://www.themoviedb.org/movie/329-jurassic-park) | [AFI](https://catalog.afi.com/Film/67200-JURASSIC-PARK) records 11 June 1993, Adventure/Science fiction, director Steven Spielberg and an MPAA `PG-13` field. | Use the release, genre and director facts. Treat the catalogued classification as non-board evidence. |
| *Forrest Gump* [`movie:tmdb:13`](https://www.themoviedb.org/movie/13-forrest-gump) | [AFI](https://catalog.afi.com/Film/55201-FORREST-GUMP) records 6 July 1994, Tom Hanks in the cast, director Robert Zemeckis, United States, English and an MPAA `PG-13` field. | This supports a 1990s Tom Hanks acting example, not a Spielberg or mood example. |
| *Toy Story* [`movie:tmdb:862`](https://www.themoviedb.org/movie/862-toy-story?language=en-US) | [AFI's title record](https://catalog.afi.com/Film/55210-TOY-STORY) records 22 November 1995, Comedy/Animation, Tom Hanks as the voice of Woody, United States, English and an MPAA `G` field. | AFI labels this title outside its 1893–1993 feature-film catalog. Preserve the voice-credit distinction and do not use the page as rating-board authority. |
| *Amélie* [`movie:tmdb:194`](https://www.themoviedb.org/movie/194-le-fabuleux-destin-d-amelie-poulain) | [Unifrance](https://en.unifrance.org/movie/20864/amelie-from-montmartre) records a French release on 25 April 2001, director Jean-Pierre Jeunet, French production language and a majority-French France/Germany production. | “Majority French” is not France-only, and French language does not establish subtitle availability. |
| *Spirited Away* [`movie:tmdb:129`](https://www.themoviedb.org/movie/129/) | [Studio Ghibli](https://www.ghibli.jp/works/chihiro/) records the 2001 work, Hayao Miyazaki, approximately 125 minutes and a 20 July 2001 release. | The reviewed page does not independently establish an English mood, country or original-language field. Its Japanese webpage language is not the film's language evidence. |
| *Paddington 2* [`movie:tmdb:346648`](https://www.themoviedb.org/movie/346648-paddington-2) | [BBFC](https://www.bbfc.co.uk/release/paddington-2-q29sbgvjdglvbjpwwc0zotc2nzq) records 2017, English, Adventure/Comedy/Children, UK `PG`, mild threat and reassuring outcomes. [BFI](https://whatson.bfi.org.uk/online/default.asp?BOparam%3A%3AWScontent%3A%3AloadArticle%3A%3Apermalink=paddington-2) records the France/UK/Luxembourg production and supplies editorial evidence about family fun and kindness. | The BBFC classification is jurisdiction-specific. Its content advice and BFI prose are evidence inputs for review, not an objective universal “comfort” label. |
| *Parasite* [`movie:tmdb:496243`](https://www.themoviedb.org/movie/496243) | [KOFIC's film record](https://www.koreanfilm.or.kr/eng/films/index/filmsView.jsp?category=ALL&mode=INDEX_FILMS_LIST&movieCd=20183782&pageIndex=1&pageRowSize=10&searchKeyword=parasite) records South Korea, 30 May 2019, director Bong Joon-ho, Drama and domestic rating 15. [KOFIC editorial coverage](https://www.koreanfilm.or.kr/mobile3/news/featuresView.jsp?seq=474) calls it a darkly satirical thriller with comic qualities. | Country is not original language, Korean `15` is not a U.S./UK classification, and editorial prose remains rubric input rather than a deterministic mood fact. |

### Deterministic fixture recommendation

Use this as one digest-bound Development Corpus source family, with source facts
separate from simulated Library presence. It spans the 1940s through the 2010s:
one reviewed title in each of the 1940s--1970s, two in the 1980s, three in the
1990s, two in the 2000s and two in the 2010s. That is enough to exercise DAT-01
and INT-01/03/04, including disjoint decades, “before 1990,” inclusive 1990--99,
genre-plus-decade intersections and off-era negatives. INT-02 is not yet causal:
the reviewed pool has no 1990--92 positive, so add a sourced title in that interval
before claiming both adjacent ranges are preserved. Relative dates such as “last
five years” must bind an observation date and remain a separate case.

For PPL-02, use *Jaws*, *E.T.* and *Jurassic Park* as Spielberg-directed
positives and *Back to the Future* as the executive-producer-but-not-director
negative. *Forrest Gump* and *Toy Story* can test Tom Hanks acting versus voice
credit only when the Intent states which roles count. Country/language cases may
contrast explicit US/English records, French-language mixed-origin *Amélie*,
English mixed-origin *Paddington 2* and South Korean *Parasite*. Do not score
*Spirited Away* or *Parasite* on original language until that field has a direct
source, and never equate subtitle availability with language.

Do not promote AUD-02/03 from this pool yet. BBFC supplies one UK `PG`; the AFI
pages report several historical MPAA fields but are not the rating board. A
same-jurisdiction, board-sourced G/PG/PG-13 comparison is still required. Until
then ratings may exercise synthetic plumbing only and cannot be called reviewed
audience truth.

### Directional mood slice and rubric `movie-mood-ordinal-v1`

Mood is a human-reviewed judgment over cited evidence, never provider metadata.
Two independent reviewers receive only the versioned title facts, synopsis and
classification/content-advice or institutional editorial prose above—no poster,
popularity, user reviews, mutable TMDB vibe/keyword field, model rationale or
expected answer. Each records a source pointer, short paraphrased evidence note
and one ordinal on every axis:

| Axis | 0 | 1 | 2 | 3 |
| --- | --- | --- | --- | --- |
| Valence | bleak/distressing | mostly downbeat or sharply mixed | broadly positive/hopeful | strongly uplifting/reassuring |
| Arousal | quiet | mostly relaxed | sustained stimulation/tension | intense/urgent |
| Threat/fear | negligible | mild/brief with safe resolution | sustained threat/suspense | frightening or severe |
| Comedic warmth | absent/cold | occasional warmth or humor | recurring warm humor | warmth/comedy is central |
| Attentional demand | background-safe | easy, light attention | regular plot attention | dense or continuously demanding |

Store both reviews unchanged with rubric version and reviewer identifiers. A
difference of two or more, or any difference that crosses a request threshold,
requires adjudication against the same evidence; preserve both originals and the
adjudicator's rationale. If the evidence still does not settle the threshold,
mark the candidate uncertain and omit it from required/forbidden grading rather
than forcing consensus.

The initial slice should stay deliberately small. For “comforting, warm and
low-threat,” require the independently reviewed *Paddington 2* anchor and forbid
*Jaws* and *Parasite* as directional contrasts. For “tense/dark,” admit *Jaws*
and *Parasite* to the acceptable set and forbid *Paddington 2*. These are
directional expectations over this reviewed pool, not universal claims about
the films. Deterministic constraints—release interval, genre, credit role,
country/language and audience policy—run first. Grade mood by acceptable-set
precision/coverage and threshold direction; do not demand one exact lineup or
award a genre label automatic mood credit. Intersections may then combine
decade+genre, director+decade, country/language+genre, or audience+tone only when
each deterministic axis has its own authority.

### Explicit nonclaims and remaining gaps

- Story setting is not origin country; translated title is not original
  language; release year is not production year; cast, direction and production
  credits are not interchangeable.
- Mixed-nationality works remain mixed. A current provider year does not erase a
  territory-specific premiere, and no release date proves current availability.
- BBFC, Korean and U.S. classifications are not one comparable scale. Genre is
  not mood, and a synopsis can inform but cannot certify sentiment.
- No source here proves subtitles, household ownership, current streaming,
  popularity, “best” status, or the completeness of any decade/genre/person set.
- This review recommends exposed development fixtures only. It is not provider
  qualification, a sealed holdout, a two-reviewer result, or permission to copy
  cooperative model picks into expected answers.

## Remaining review queue before further case authoring

1. Map original American Full House and later TGIF revival titles to exact provider Keys without
   replacing synthetic identities in frozen manifests. The six Phase One identities are now in the
   cumulative v2 development corpus; the three History control identities are in v3.
2. Record source URL, observation date, supported role/epoch, excerpt digest and
   uncertainty for each new fixture fact. Keep retrieved page data untrusted.
3. Obtain an independently reviewed historical-network source for each additional
   History-era member; the current executable family remains explicitly partial and single-anchor.
4. Author each positive expectation and its negative contrast independently of
   the cooperative model trace, then exercise the production Suggester/Runner.
5. Keep all exposed pilot and registry siblings in development. Author new,
   unexposed families separately for the later sealed holdout.

The [100-family registry](../plans/suggestion-query-family-registry.md) defines
the authoring backlog. Its rows are proposed development families, not passed
tests, provider qualification or historical lineup completeness.
