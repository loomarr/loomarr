# Operational signals for subjective movie queries

Observed: 2026-09-16, America/New_York.  Scope: issue #1299 and the exposed
**Development Corpus** only.  This note does not change the evaluation schema,
make a title claim, authorize an inference run, or establish certification.

## Decision

Do **not** replace `attentionalDemand` with another one-number title axis.  The
existing v6 packet has three family-distinct model reviews but leaves that axis
unresolved for every title.  That result is useful evidence of ambiguity, not
missing consensus to be manufactured.

The term conflates at least three different things:

1. a work's narrative continuity structure;
2. properties of a particular delivered representation (audio, captions,
   descriptions, cut rate); and
3. a particular viewer's language, sensory access, device, surrounding task,
   and tolerance for interruption.

Only (1) is plausibly title-intrinsic.  Even there, the best available
instrument is an audience-response measure: Busselle and Bilandzic's original
*Measuring Narrative Engagement* article defines a scale for engagement, not a
catalogue field for a film.  Its publisher record identifies it as a 2009
research article in *Media Psychology*. [Taylor & Francis,
accessed 2026-09-16](https://www.tandfonline.com/doi/full/10.1080/15213260903287259)

Accordingly, retain the four resolved directional mood axes unchanged; do not
turn `arousal` into a surrogate for attention.  For a future development packet,
adopt at most the narrowly named **narrative-continuity dependence** axis below,
and only on evidence that meets its gate.  Keep language and audiovisual access
as an evaluation-run compatibility record, not a title score.

## Signal map

| Candidate | What it actually describes | Title-intrinsic? | Development recommendation |
| --- | --- | --- | --- |
| Narrative-continuity dependence | Likely loss of plot state if viewing is interrupted | Partly; its assessment is still interpretive | Conditional 0--3 ordinal, below. |
| Narrative complexity | A theory/reception construct, often confused with comprehension or engagement | No single stable title value in this packet | Do not score separately; it would duplicate or overclaim continuity dependence. |
| Pacing / arousal | Editing tempo versus experienced stimulation | Tempo may be representation-measurable; arousal is an existing subjective axis | Retain resolved `arousal`; do not add a "slow" or pacing score. |
| Dialogue / language dependence | Comprehension with this viewer's audible language and selected text/audio tracks | No | Compatibility vector only; never a title ordinal. |
| Visual dependence | Information unavailable from the audio alone, and availability of a description track | No; also representation-specific | Compatibility/accessibility record only; never infer it from a synopsis. |
| Background-friendliness | Suitability while a viewer performs another task | No | Reject as a title label. |

The distinction is not cosmetic.  The HTML media specification distinguishes
subtitles (dialogue for a listener who does not understand it), captions
(dialogue plus relevant non-speech sound), and descriptions, and lets a source
offer tracks with language tags. [WHATWG HTML, accessed
2026-09-16](https://html.spec.whatwg.org/dev/media.html#the-track-element)
The Library of Congress likewise catalogs spoken language, caption language,
and accessible-audio language separately. [MARC 21 field 041, accessed
2026-09-16](https://www.loc.gov/marc/bibliographic/bd041.html)  Those are facts
about a representation and access provision, not evidence that one person can
follow a film in the background.

W3C's accessibility guidance makes the boundary still clearer: description
supplies visual information needed to understand media when it is not already
in the audio; where all visual information is already conveyed in audio, extra
description is unnecessary. [WCAG 2.2 Understanding SC 1.2.8, accessed
2026-09-16](https://www.w3.org/WAI/WCAG22/Understanding/media-alternative-prerecorded)
That supports recording whether a *specific supplied source* has access tracks.
It does not permit an absent track, a catalogue synopsis, or model agreement to
prove that a title intrinsically has low visual dependence.

There is a defensible future measurement lane, but it is not present in the
current title-fact packet.  In experiments on event perception, situational
changes predict perceived event boundaries, and prior narrative context can
improve comprehension of the same target clip. [Zacks *et al.*, accessed
2026-09-16](https://pmc.ncbi.nlm.nih.gov/articles/PMC8710938/)
[Loschky *et al.*, accessed
2026-09-16](https://pmc.ncbi.nlm.nih.gov/articles/PMC4659561/)  That supports
recording asset-derived transition evidence as an *input* to a continuity
review—not asserting that scene count itself is attention demand.  MovieNet,
for example, documents manually annotated scene boundaries and links movie
segments to synopses, scripts, and subtitles; it is a potentially auditable
feature source where its access restrictions and exact-title matching permit.
[MovieNet project, accessed 2026-09-16](https://movienet.github.io/)

## Conditional title axis: narrative-continuity dependence

This is an operational development heuristic, not a psychometric diagnosis, a
viewer prediction, or a universal property of a title.  It answers only:

> From the supplied evidence, how much story state would a viewer probably need
> to retain to reconnect after an ordinary interruption?

Use exactly one integer only after the evidence gate succeeds:

| Value | Definition | Minimum affirmative evidence |
| --- | --- | --- |
| 0 | No evidenced ongoing causal thread: the packet supports a self-contained, observational, anthology, or performance structure. | Two independently attributable structural facts, including one direct work source. |
| 1 | A stable premise or recurring characters matter, but the packet supports locally understandable segments or explicit reorientation/recap. | A direct work source plus a second independent structural source that states the local/episodic framing. |
| 2 | The packet evidences a continuing causal thread in which missed scenes plausibly obscure motivation, relationships, or present circumstances. | Two sources that identify the continuing thread and its state change; a one-sentence synopsis is insufficient. |
| 3 | The packet evidences multiple interdependent threads, non-linear/revelatory structure, or rapid state changes for which an interruption plausibly removes necessary relations. | Two independent detailed structural sources, one of which explicitly supports the relevant interdependence, chronology, or reveal mechanics. |

`Unknown` is a first-class outcome, not value 0.  A rating, genre, runtime,
release year, popularity, a keyword, a trailer, or an LLM rationale cannot meet
any row.  Nor may a score be inferred from a request such as “slow” or “easy to
leave on.”  These are all deterministic catalogue facts or request text, not
evidence of the proposed construct.

### Evidence and uncertainty rules

For each scored title, freeze a packet containing the title Key, representation
version where relevant, source URL, access date, short paraphrase, evidence ID,
and the exact ordinal definition version.  At least one source must be a
rights-holder, distributor, festival/programme note, screenplay, or other
work-specific primary editorial material; the second must be independently
attributable.  Do not silently reuse mutable provider prose as both sources.

Two blinded reviewers must independently assign the ordinal against only that
packet.  They receive no request text, expected polarity, Library status,
existing model score, or one another's rationale.  Preserve both submissions.

- Any disagreement of two or more values, a disagreement crossing a query
  threshold, an absent required source, or an evidence-to-definition mismatch
  yields `unknown` pending adjudication.
- An adjudicator may choose one existing value or `unknown`, and must cite the
  packet evidence and rubric version.  It may not average scores or introduce a
  new uncited fact.
- If the published evidence describes only premise, tone, audience rating, or
  marketing language, record `unknown`; do not use model consensus as a tie
  breaker.
- A candidate marked `unknown` must be excluded from a required/forbidden
  continuity rule, rather than treated as a positive or negative.

These rules deliberately mirror the existing development protocol's
preservation of individual reviews and its uncertainty-first treatment.  They
are stricter here because the public evidence packet currently contains no
approved structural-source lane.

## Non-title compatibility record

For an Intent that genuinely mentions language, captions, audio-only listening,
or looking away, attach this record to an **evaluation run**, keyed by the exact
Media Source rather than the Title:

```text
viewerLanguage: BCP-47 tag or unknown
audioTracks: [{language, role: main|dub|description}]
textTracks:  [{language, kind: subtitles|captions}]
deviceCanSelectTracks: true|false|unknown
viewerWillWatchScreen: yes|no|unknown
```

This is intentionally a vector, not an ordinal.  A French audio track can be
fully accessible to one viewer and unusable to another; captions can help a
viewer who understands neither the audio nor every device's rendering; and a
description track is a source/delivery feature.  HTML requires a valid BCP 47
language tag for subtitle tracks, while the `default` choice is conditional on
user preferences. [WHATWG HTML, accessed
2026-09-16](https://html.spec.whatwg.org/dev/media.html#attr-track-srclang)
The record therefore prevents a false title assertion and makes a missing track
or unknown device capability fail closed for language/access predicates.

It is not available from the current public title-fact packet.  Until a
versioned source-level manifest exists, such Intent phrases remain explanatory
or synthetic controls only; they must not contribute required or forbidden
title Keys.

## Future representation measurements (not current title scores)

If Loomarr later has a rights-authorized, versioned Media Source and separately
approved analysis pipeline, keep each measurement observable and provenance
bearing.  Do not turn it into a mood score by name alone:

| Measurement | Record | Permitted interpretation | Prohibited shortcut |
| --- | --- | --- | --- |
| Event/transition evidence | Scene boundaries and documented time/place/causal shifts per minute, extractor/version, confidence | A reviewer may use it as one cited input to continuity dependence. | “More cuts means more attention required.” |
| Formal tempo | Median and distribution of shot duration; camera-motion share; asset hash and detector version | Describes this representation's edit tempo. | “Fast is arousing” or “slow is background-safe.” |
| Spoken-text density | Timed subtitle/script tokens or subtitle events per minute, language, coverage and source rights | Describes available text on this representation. | “More dialogue makes it inaccessible” or “original language predicts this viewer.” |
| Access-track availability | Main/dub/description audio and subtitle/caption tracks, language, player selection result | Determines whether a requested viewer/device configuration is supported. | “No description track proves high visual dependence.” |

The need to keep tempo and arousal separate has empirical support: the
LIRIS-ACCEDE project provides induced valence/arousal annotations for excerpts,
whereas MovieNet documents cinematic/shot data.  They are different data types,
not interchangeable scales. [LIRIS-ACCEDE, accessed
2026-09-16](https://liris-accede.ec-lyon.fr/)
[MovieNet, accessed 2026-09-16](https://movienet.github.io/)

## Rejected substitutes

**Background-friendly.** This is a viewer-and-context preference, not a stable
work property.  It changes with the task being performed, familiarity with the
work, language, audio route, captions, screen size, and accessibility tools.
There is no responsible title-only ordinal in the public packet.
An experiment using an intact versus out-of-order film and a concurrent
prospective-memory task found a divided-attention consequence for its specific
stimulus and task; it does not yield a movie-ranking instrument. [Narrative
transportation experiment, accessed
2026-09-16](https://pmc.ncbi.nlm.nih.gov/articles/PMC4675523/)  Use that as a
reason to validate any future policy with real viewer outcomes, not to label
titles in advance.

**Dialogue/language dependence.** Original language and dialogue quantity are
not a measure of a viewer's comprehension.  Treating language metadata as such
would collapse a deterministic catalogue fact into a subjective evaluation
label and disadvantage multilingual, dubbed, captioned, or described playback.

**Visual dependence.** The accessibility standards establish that some visual
information can require description, but they do not publish a per-title
four-level rating.  Track availability is neither a proxy for story structure
nor proof of its absence.  Do not invent this axis from plot summaries.

**Pacing as attention.** Shot cadence can be measured from a supplied asset,
but that technical measurement is not experienced arousal or interruption cost.
The corpus already has a separately reviewed `arousal` axis.  A second score
would double count it, while a runtime/genre/editorial label would only restate
catalogue metadata.

**Narrative complexity.** The narrative-engagement literature separates such
audience experiences as attention and narrative understanding; it does not
license treating an engagement questionnaire as an objective catalogue tag.
Use the narrower continuity heuristic only when its evidence gate can be met.
Otherwise leave it unknown. [Busselle & Bilandzic, 2009, accessed
2026-09-16](https://doi.org/10.1080/15213260903287259)

## Calibration design

Do not calibrate on a homogeneous set of prestige dramas, English-language
films, or one era.  Before any continuity-dependent development query is
promoted, build a small, source-reviewed calibration matrix spanning:

| Stratum | Deliberate contrasts |
| --- | --- |
| Form | animation, documentary, concert/performance, anthology/episodic work, conventional feature narrative, and multi-thread/chronological-experiment narrative where public structural evidence exists |
| Era and region | at least three release eras and multiple production/language regions; do not make an English-language source the default evidence lane |
| Runtime and segmentation | short feature, long feature, and works with explicit chapter/episode framing; runtime alone is never the label |
| Delivery/access | original-audio-only, dubbed, captions/subtitles, and described-audio exemplars, held as compatibility cases rather than title-score examples |
| Evidence strength | clear two-source cases, deliberately ambiguous premise-only cases, and conflicting-source cases; the last two must exercise `unknown` |
| Query polarity | threshold positives, near-boundary controls, and deterministic matches that remain continuity-unknown so a model cannot use genre/era shortcuts |

The calibration outcome is agreement, uncertainty rate, and error analysis by
stratum—not a claim that the reviewers discovered the title's true attention
requirement.  Freeze the source packet and reviewer outputs by digest, then
keep it separate from certification and any paid/provider qualification.

## Limits

Model consensus is not truth.  It is especially weak evidence here because all
models can share unstated cultural assumptions and can agree on a label that no
packet fact supports.  Even a two-reviewer or adjudicated development value is
an exposed, revisable evaluation aid.  It is not a recommendation to viewers,
a claim about accessibility, an acquisition decision, a production policy, a
provider qualification, or certification evidence.

No public source inspected supplies a generally accepted per-film
“background-safe,” dialogue-dependence, visual-dependence, or attentional-demand
catalogue field.  The responsible default for the current corpus is therefore
to leave `attentionalDemand` unused and unresolved, retain the already resolved
mood dimensions, and add only source-gated continuity evidence in a future,
explicitly development-only revision.

## Sources inspected

- Busselle, R. W. and Bilandzic, H., “Measuring Narrative Engagement,” *Media
  Psychology* 12(4), 321–347 (2009), DOI
  [10.1080/15213260903287259](https://doi.org/10.1080/15213260903287259);
  publisher page accessed 2026-09-16.
- [WHATWG HTML Living Standard: `track` element](https://html.spec.whatwg.org/dev/media.html#the-track-element),
  accessed 2026-09-16.
- [W3C WCAG 2.2 Understanding SC 1.2.8](https://www.w3.org/WAI/WCAG22/Understanding/media-alternative-prerecorded),
  accessed 2026-09-16.
- [Library of Congress MARC 21 Bibliographic Format, field 041](https://www.loc.gov/marc/bibliographic/bd041.html),
  accessed 2026-09-16.
- [Zacks *et al.*, event boundaries and narrative prediction](https://pmc.ncbi.nlm.nih.gov/articles/PMC8710938/),
  accessed 2026-09-16.
- [Loschky *et al.*, prior context and film comprehension](https://pmc.ncbi.nlm.nih.gov/articles/PMC4659561/),
  accessed 2026-09-16.
- [MovieNet project and documented annotations](https://movienet.github.io/),
  accessed 2026-09-16.
- [LIRIS-ACCEDE affective-video dataset](https://liris-accede.ec-lyon.fr/),
  accessed 2026-09-16.
- [Primary divided-attention/narrative-transportation experiment](https://pmc.ncbi.nlm.nih.gov/articles/PMC4675523/),
  accessed 2026-09-16.
