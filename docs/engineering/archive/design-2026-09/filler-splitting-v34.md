# Compilation splitting (V34, archived)

Archived verbatim from `docs/design.md` §10 in #779 (2026-09-27). Not current guidance: V67 governs
where they conflict, and the current rules are in
[`docs/design/filler-structure.md`](../../../design/filler-structure.md).

V67 supersedes V34's assumption that every duration-quarantined source is a compilation and its
lossy drop tally. The V34 detector remains a shadow/proposal input while the V67 structure assessment
adds semantic source proof, complete timeline coverage, independent interval roles, and certified-slice
admission. The measured detector behavior below remains evidence; where it conflicts, V67 governs.

Discovery (V33) surfaces a source; ingest downloads it. But a large share of what discovery finds is a **compilation** — one file holding twenty or more commercials back to back. Ingested whole it is a single 15-minute "clip" the pod assembler can never place (`durationEligible` rejects anything far longer than a break); split blindly it is twenty files named `compilation_seg07` with no era, audience or category, which the ladder cannot place either. **Splitting and metadata are one phase because either alone produces unplaceable clips.** The pipeline, designed from measurement on six real compilations rather than reasoning (plan §6.4 — every number below names its method there):

1. **Triage.** A source with chapters splits for free (chapters are exposed without downloading the file). Rare in practice — 6 of 8 sampled sources had none — so this is an optimisation, not the mechanism.
2. **Coarse split.** ffmpeg's `blackdetect` + `silencedetect`, parsed in Go; segments under the **detection floor** become explicit discard intervals in the V67 plan rather than disappearing. That floor is `max(MinSegmentMs, filler.min_duration)` — the 3s sliver floor and the catalog floor, whichever binds (10s on a default install). ⚠ `max()`, never replacement: `filler.min_duration` is settable to `0s`, and a 400ms fade artefact must still be omitted there. The two numbers are **one floor on purpose** — a segment the auto-confirm gate would admit and the scan boundary would then reject (§10 V40) is a clip cut out of a compilation and thrown away, work done to produce nothing. Scene-cut detectors (`scdet`, PySceneDetect) were measured and rejected: they fire on camera cuts *inside* an advert — the wrong granularity, not a tuning problem. Detection quality is a property of the **source**, not of any threshold (69–100% across the six compilations; two had genuinely absent boundaries no setting fixes).
3. **Rescue.** A segment far longer than a plausible advert means boundaries the A/V pass could not see; it goes to **transcript (whisper) + LLM** for cut points. ⚠ The LLM must return **exactly one entry when the transcript is a single advert** — without that instruction it invented cuts at suspiciously round 30/61/92s marks inside one 121s infomercial. With no runnable whisper (`INGEST_WHISPER_PATH`, §15) an over-long segment is not guessed at: it surfaces in the review as **unsplittable**.
4. **Independent role before enrichment (V67).** Each planned interval first receives its own closed
content role; a reel-wide `commercial` kind is never inherited. Only after the role and structure are
resolved may ordinary text/vision taxonomy enrichment add product, era, and audience facts. Era follows
the grounding rule above: persisted only when the year appears in the evidence, else carried as an
unconfirmed suggestion.
5. **Deterministic discard.** The same advert recurs across compilations. A dHash over frames sampled at 1/3fps — ~30 lines of pure Go over `ffmpeg -pix_fmt gray` output, no library, no cgo — separates a re-encoded duplicate from a different advert by a measured 25× margin (mean per-frame Hamming 1.1 vs 27.6–32.2), so any threshold in the teens works. A matched segment is discarded before classification: keeping another copy cannot improve the catalog, and asking a person to make that decision is queue-shaped busywork. A segment shorter than the existing `filler.min_duration` floor is discarded at the same seam — the scan boundary would reject the resulting clip immediately, so cutting and reviewing it can produce no usable outcome. **Neither discard deletes source media:** the composite row and file remain (§10 V45), so re-splitting with improved detection is the recovery path. Detection ambiguity is never a deterministic discard: an `unsplittable` or over-long span remains with the whole reel for review.
   **Installation language is also applied per detected segment before review.** A compilation is
   not one language: each span samples its own bounded audio window using the same detector and
   single installation choice as the ordinary language rung. A confident mismatch is omitted and
   counted in the proposal receipt; wordless, unknown, unavailable-backend and failed-backend
   outcomes remain reviewable. An unresolved span carries a stable reason — `unavailable`,
   `failed`, `inconclusive`, or `paused` — plus optional technical detail, so review can distinguish
   speech recognition that is not set up from a real attempt that failed or could not identify the
   language confidently. The work is cursor-persisted and bounded by the existing whisper
   allowance, and a proposal is not reviewable until every span has a terminal outcome. Every
   finished proposal also records the installation language it used, so review can identify a
   later preference change and offer an explicit recheck. Every confident mismatch remains in the
   proposal receipt with its exact interval, detected and
   expected languages, name, and a stable mismatch reason; it is not placed back in the editable
   candidate list. When every candidate is a confident mismatch, the split resolves automatically
   with a plain pipeline receipt instead of creating an empty human-review task. Confirmed
   children inherit known and wordless results so the post-confirm rung does not spend twice, but
   only when the confirmed interval exactly matches the persisted proposal. The server ignores
   language fields supplied by the confirm client; edited or merged intervals and
   checked-but-unknown children leave the field empty so the post-confirm rung remains a
   defence-in-depth retry.
6. **Review — required unless the result is unambiguous (V43).** Because detection quality is a property of the source, an uncertain result is confirmed by a human before anything enters the catalog; auto-accepting a 69% result puts 3-minute "commercials" into 30-second breaks. Detection runs as a **job** (minutes per file) producing a **persisted split proposal** (§5) — review can happen long after detection, and a restart must not lose it — and an unconfirmed proposal writes nothing until `POST /v1/filler/splits/{id}/confirm` (§7) commits a cut list.

   ⚠ **The review PLAYS each proposed cut, in place (V54).** It did not, for as long as it existed: measured 2026-08-12 on a 52-segment reel, the screen offered a name field, two mm:ss fields, Merge, Drop and Confirm, and **no media element at all** — an operator was asked whether a cut at 04:17 was right with nothing to see or hear. V54 A7 had already deleted the mock's "click to preview" caption for being false; this is the other half.

   A proposed segment has **no bytes of its own** until confirm writes them, so the preview is a **byte-range window of the parent composite** (`GET /v1/filler/media/{clipHash}`, range-served by `http.ServeContent`). That is the operational reason V45's keep-the-parent rule matters to an operator and not only to lineage: without the retained reel there is nothing to play. The route is `RoleMember`, the page is admin-gated, and the browser authenticates with the session cookie it already holds — **no new authorization surface**.

   **The review starts with the clips Loomarr found, not detector diagnostics (V68).** Every
   proposed segment receives one representative still extracted from its exact source-bound span.
   The still is ingested through the shared image service as member-visible, proposal-owned
   artwork; the proposal document retains only the content hash and exact span binding. Confirm,
   replacement, rewind, and expiry release the proposal's image references, after which the normal
   image garbage collector owns the bytes. A failed extraction is a terminal presentation outcome
   for that proposal and renders as **Preview unavailable** rather than a broken image or an
   invented frame. Re-detection is the retry path. No browser-visible filesystem path and no new
   image store exist.

   The proportional timeline is the primary overview. It remains one ordered list, scrolls rather
   than compressing a long reel into untappable slivers, and gives every segment its still, name,
   duration, time range, useful tags, language, and any plain-language reason it needs attention.
   Hover and keyboard focus expose the same lightweight summary without starting media. Click or
   Enter selects the segment, reveals its compact row, and opens the existing exact bounded player;
   at most one player is mounted, and changing selection, collapsing it, or leaving the page tears
   that player down. Fifty segments therefore mean fifty lazy still images and **one** possible
   range stream, never fifty video elements.

   The ordinary row vocabulary is **Rename**, **Adjust timing**, **Join next**, **Remove**, and
   **Keep clips**. Boundary confidence, detector evidence, transcripts, recognition diagnostics,
   and other pipeline terms remain available under **Details** instead of competing with the
   decision. The page explains once that keeping clips creates individual filler items while
   retaining the original recording for recovery. Editing, sub-second boundary preservation,
   exact-span playback, language recheck invalidation, and the confirmation authority remain
   unchanged; this is a review presentation and evidence-lifecycle change, not a new admission
   path.

   **Proposed names describe the exact segment instead of its storage order (V69).** Chapter titles
   remain source-authored names. Otherwise the already-budgeted split vision response may propose
   one short household-readable name from text it read in that segment's bounded frames; transcript
   rescue may do the same from the exact subsegment transcript. This adds no second model request.
   A proposed identity is accepted only when its identifying phrase occurs in that exact signal,
   after case and punctuation normalisation. Generic, unsupported, malformed, overlong, wordless,
   and abstaining answers are rejected independently of valid tags or role evidence. They fall back
   to **Clip N from &lt;readable recording name&gt;**, or **Clip N from this recording** when the parent
   name is opaque. Provider failure follows the same non-blocking fallback path.

   The proposal records `source-authored`, `model-proposed`, `fallback`, or `operator-edited` plus a
   bounded evidence excerpt for a model proposal. That provenance stays inside the existing
   collapsed name-edit disclosure (and the selected-clip Details area once the planned shared
   right-side sheet owns inspection), never as another always-visible row. The server—not the confirm
   body—is authoritative: an unchanged name on the same exact span recovers
   its persisted provenance; a rename, boundary edit, or merge keeps the operator's chosen display
   name, clears machine evidence, and becomes operator-edited. The chosen name reaches the confirmed
   child. Naming is descriptive enrichment only and is deliberately absent from role, language,
   suitability, boundary-confidence, and admission inputs. Split review performs no web search and
   does not require a model to finish.

   **One selected clip uses the shared inspection sheet (V70).** The split page keeps the
   proportional filmstrip, one compact ordered clip list, reel-level explanation, and **Keep clips**
   action stable while a clip is inspected. Activating either a filmstrip item or its compact row
   opens the existing application Sheet from the right on desktop and full width on a narrow screen.
   That sheet owns the one exact bounded autoplay preview, name and timing edits, era decision,
   **Join next**, **Remove**, and progressive naming, language, transcript, recognition, and cut
   details. Previous/next controls move through a long reel without closing the sheet. Selection is
   transient local state—not a route or query parameter—and closing restores focus to the exact
   filmstrip item or row that opened it. Changing selection or closing the sheet unmounts the prior
   player, so a 50-clip reel still mounts at most one video and editing never expands or reflows the
   overview. Confirmation remains a reel-level action outside the sheet and sends the same server
   payload; this changes presentation, not split, language, naming, or admission authority.

   ⚠ **The player is clamped to `[startMs, endMs]` and reports the SEGMENT's length, never the reel's.** A 30-second cut of a 22-minute recording reads `0:04 / 0:30`. Handing the readout the reel's own numbers would present the whole recording as if it were the clip, which is precisely what makes a preview useless for judging one cut. One preview is open at a time and collapsing **unmounts** the element — otherwise every row the operator has ever clicked holds a range request open against a 20-minute file.

   ⚠ **This was "not optional, ever" until V43, and the blanket rule was over-applied.** Boundary
   confidence can safely automate the mechanical creation of well-supported child clips while
   uncertain cuts remain for review. That automation does not publish the children: each child
   still traverses conditioning, classification, configured safety/playback checks, and terminal readiness.

   **`filler-split` is a scheduled job** (on by default). It proposes splits for over-long catalog clips rather than waiting for a click, so proposals are ready when the operator looks instead of costing minutes of waiting once they do.

   **Auto-confirm is a separate switch** (`filler.autosplit.enabled`, default **ON** — maintainer decision, V51b: the gate exists to admit *confident* reels, and off-by-default meant every compilation waited for a click the design says should be unnecessary), gated on `filler.autosplit.min_confidence` — deliberately NOT reusing the auto-file threshold. The failure modes differ in kind: a mis-*tagged* clip plays in the wrong break, a mis-*cut* clip plays half an advert. One dial would force the stricter case to govern both.

   ⚠ **The gate is PER SEGMENT (V54). All-or-nothing is retired.** A segment is cut and filed when it passes every refusal *and* its boundary confidence clears `filler.autosplit.min_confidence`; the rest stay behind in a shrunken proposal. A reel of 52 becomes 47 clips and 5 cuts to review, rather than 52 cuts to review.

   ⚠ **Order matters, and it is the whole safety argument: REFUSALS FIRST, ABSOLUTELY — then the threshold.** A segment carrying `SuggestedEra > 0`, `unsplittable`, a duplicate flag, an over-long span or a sub-floor span is refused **at any score**. Confidence chooses only among segments that already pass every refusal: it can hold a qualifying segment back, never let a refused one through. `boundaryScore` cannot even see `SuggestedEra`, `Era`, `Looked`, `Category` or `Tags` — a tag fact is not in its scope, so it cannot move the number. That is what keeps `autosplit.go`'s objection true: nothing here launders a refusal into a score.

   ⚠ **This was ALL-OR-NOTHING until V54, and the old rationale is worth stating rather than deleting.** It ran: *"a badly-split reel is not uniformly slightly-wrong; it has obvious tells… confirming the good segments and surfacing the rest would split one reel's decision across two places and hand the operator fragments to judge without the picture."* That was correct **while there was no per-segment evidence**. Splitting a decision arbitrarily is indeed worse than making it once. But the rule's cost was total: one doubtful segment in 52 sent all 52 back, so the operator's work never shrank and — measured 2026-08-11 — **~50 reels sat parked with none ever auto-confirmed**. With boundary evidence per cut, keeping five back is not splitting a decision arbitrarily; it is **routing by evidence**, which is what every other rung in this pipeline already does. The filmstrip still shows the whole picture, and the parent recording is still there to play (V45), so the operator judging those five has more context than the old rule assumed, not less.

   ⚠ **A confirmed segment is not airable.** Confirm writes the rendered child and lineage, then the
   child traverses the complete pipeline while held. Boundary confidence decides whether a cut may
   be created unattended; it is not evidence that the resulting content is technically playable or
   eligible for a pod. Only the terminal-ready transaction may release it.

   ⚠ **The `filler.min_duration` floor is no longer one of those conditions, because it is enforced earlier (V54).** It used to be, and that is the reason auto-split could never fire: a real commercial compilation is *made of* sub-floor material. Measured 2026-08-11 on an 82-segment archive.org reel, **39 segments sat under the 10s floor**, the shortest 3.1s — station IDs and inter-ad bumpers. `AutoConfirmable` returns on the first failing segment, so `RejectTooShort` sank the reel before the grounding checks at the bottom of the loop were ever reached, and the V54 grounder below could not have changed the outcome no matter how well it worked. Those fragments are now dropped at **detection** (step 2 above), where a fragment the scan boundary would refuse anyway costs nothing to discard. `RejectTooShort` stays in the gate as defence-in-depth for hand-edited proposals and for those detected before V54; it is no longer a reason a freshly-detected reel sinks.

   **Boundary confidence — what routes a cut (V54).** A score of 0–100 per segment, answering *did we cut in the right place?* ⚠ **Distinct from `Clip.Confidence`**, which answers *do we know what this is?* Different question, different evidence, different field; the split gate reads the first and never writes the second. `CONTEXT.md` carries both terms precisely because "confidence" unqualified is now ambiguous.

   It is a **ceiling ladder**, in the shape §10's tagger already uses — the best evidence a boundary has sets its ceiling, and segment-level facts may only lower it:

   | Evidence for one boundary | Ceiling | Basis |
   | --- | --- | --- |
   | a chapter marker, or the reel's own start/end | 100 | declared, not inferred |
   | black **and** silence agreed on it | 90 | **measured** — 9 of 12 fades corroborated |
   | one detector only | 65 | **measured** — the other 3 of 12 |
   | the transcript rescue alone | 50 | **measured** — ±2–3s timing |
   | truncated (an overlap moved this edge) | 40 | asserted |

   **Within one boundary the best evidence wins. Across a segment's two boundaries the WORST wins** — a cut is only as trustworthy as its weaker end. Segment facts then cap it: over-long 50, `unsplittable` 20.

   ⚠ **Black is not ranked above silence.** Nothing measures that, and ranking them would invent exactly the number this design exists to avoid. Which detector fired survives in the evidence token, so the UI can still say "silence only".

   ⚠ **The duration prior is DEMOTED, and this deviates from V45's original sketch.** That sketch made "30/60s ±2s" a confidence input. Under a ceiling ladder a corroborating prior has no legal move, and capping on "not a standard slot" would flag half a good reel: measured 2026-08-11, **39 of 82 segments were sub-10s bumpers and every one was correctly cut**. Duration survives only as the over-long cap.

   ⚠ **The rescue's single-span confirmation REMOVES a cap rather than adding points** — the only legal move a corroboration has here. When the LLM returns exactly one span it is saying "this is one advert" (the measured 121s infomercial), which is the fact that defeats "over-long means a missed boundary".

   **Not scored, deliberately:** the dHash distance and `dupOf` (catalog membership is a different question); vision's `looked`/`category` (tag grounding — their exclusion is the clearest illustration of the boundary/tag split).

