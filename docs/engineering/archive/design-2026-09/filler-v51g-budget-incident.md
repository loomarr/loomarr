# The split-budget incident (V51g, archived)

Archived verbatim from `docs/design.md` §10 in #779 (2026-09-27). Not current guidance: the rules are
in [`docs/design/filler-pipeline.md`](../../../design/filler-pipeline.md#budgets-are-per-clip) and decision
[0019](../../../design/decisions/0019-stage-budget-is-per-clip-rate.md).


**Found on a live catalog, not by a test.** `<local station> Commercial Breaks (1995)` — a 16m47s
recording — sat at *"Finding the ads inside"* through **twelve** consecutive pipeline passes,
failing every two minutes with `context deadline exceeded` and starting again from the beginning.
Roughly 25 minutes of GPU spent re-doing the first third of one clip, while the row animated as
though it were making progress.

**Measured, on the real file** (the numbers are the point — the first three diagnoses were wrong
without them):

| Step of the `split` rung | Cost | Fits the pass? |
| --- | --- | --- |
| `blackdetect` + `silencedetect` | **4s** (319× realtime; 44 + 53 hits) | ✅ |
| `dedup` — `GrayFrames` × 51 | **33s** (662ms/segment) | ✅ |
| cut — ffmpeg stream copy × 51 | **3s** (59ms/segment) | ✅ |
| **`classify` — one LLM turn × 51** | **≈377s** (7.4s/call, `qwen3:8b`) | ❌ **6× the whole budget** |

⚠ **The "120s pass" this table originally compared against was WRONG, and the real ceiling was
half of it (V54).** 120s is the CRON INTERVAL (`0 */2 * * * *`) — how often the job *starts*, not
how long it may run. The actual ceiling was River's `JobTimeoutDefault`, **60 seconds**, because
`river.Config.JobTimeout` was never set and `riverWorker` did not implement `Timeout()`. So every
job on every install ran under a deadline inherited from a dependency, which nothing here chose
and nothing recorded.

⚠ **It does not surface as a timeout, which is why it survived.** `exec.CommandContext` SIGKILLs
its child, so the operator sees `ffmpeg …: signal: killed` / `whisper-cli: signal: killed` — a
corrupt file or a broken binary, not a clock. Measured on the maintainer's catalog 2026-08-11: a
`blackdetect` pass reported as "killed" inside the job completed in **40s** run by hand, and the
20-minute reel it belonged to was left `Unsplittable` for want of time it should have had.

The row above still holds — 377s does not fit a 60s pass either, and the rule below is unchanged —
but the margin was 6×, not 3×, and the numbers under it were being judged against a budget twice
the real one. Jobs now declare their own ceiling (`scheduler.Job.Timeout`); media jobs take
`scheduler.LongJobTimeout`, tied to the lease horizon so a job cannot outlive its own claim.

⚠ **The rule this establishes.** The scheduler's unit of work is a CLIP: `Cost()`, the per-run
budgets (`FILLER_PIPELINE_MAX_CLIPS`, `…MAX_WHISPER`) and the retry policy are all sized per clip.
A rung whose cost scales with a clip's CONTENT breaks that, and no retry helps — attempt two is
exactly as impossible as attempt one. Cheap per-segment work is fine (the fingerprint pass is 51
segments in 33 seconds). **What a rung may not do is spend a model call per segment.**

⚠ **`classify` inside `Propose` was that, and it was strictly-worse duplicate work.** It calls the
same `Classify` the `tag` rung calls, but with `SplitSegment.Transcript` — which is EMPTY unless
`rescue` ran, and `rescue` only transcribes segments over ~120s. On this reel **none** qualified
(longest 60s), so all 51 calls classified on nothing but a generated name: `"… part 7"`, identical
across segments apart from the number. `Confirm` writes those results onto each spawned clip, but
`Tagged()` needs `Era > 0 && Audience != "" && Category != ""` — and a bare part-number grounds no
category — so the `tag` rung re-runs anyway, this time after `transcribe`, with a real transcript.
The pipeline paid twice and kept the worse answer's cost.

**The fix is removal, not rescheduling.** Split CUTS; it does not describe. Every segment is
spawned as its own clip and runs the whole ladder for itself, so the classification already happens
downstream — one clip at a time, budget-bounded, resumable, and individually visible in Incoming.
Without `classify` the rung completes in **~40s** and all 51 children are enrolled in a single
pass, after which they progress **independently and out of order**: a 16-second silent advert
reaches `score` while its sibling is still being transcribed.

⚠ **The atomic confirm STAYS.** Cutting all 51 costs 3 seconds, so streaming enrolment buys nothing
here and would cost a deliberate safety property: a proposal is editable, so a partially-applied
confirm leaves orphan cuts of a plan that no longer exists. `Confirm`'s own comment states the
invariant — the proposal is consumed only once every segment exists on disk and in the catalog.

**Two mechanical rules, independent of split and true of every rung:**

⚠ **Running out of time is not failing.** A `context deadline exceeded` is a DEFERRAL: status back
to `queued`, attempts unchanged, resume next pass. V51b already treats budget exhaustion exactly
this way; the deadline path never got the same treatment, so a timeout burned an attempt and took a
backoff it had not earned.

⚠ **Failure bookkeeping must outlive the failure.** `onFailure` computes the record, the backoff
and the `MaxAttempts` resolution, then persists them through `ctx` — *the context whose expiry
caused the failure*. The save fails and all of it is discarded; only the pre-work write
(`status=running`, `attempts++`) survives, because that one happened while the context was alive.
That is why attempts reached 12 against a `MaxAttempts` of 3, and why the row never left `running`.
**Any rung that ever times out loops forever**; `split` is simply the one that timed out first.
Failure and deferral both persist through a detached context.

**Long recordings resume instead of restarting.** The split proposal is also the detector's durable
checkpoint before it becomes operator-visible. When chapters are absent, boundary detection scans
one fixed ten-minute span per pipeline pass, persists `scannedThroughMs` plus the accumulated
black/silence gaps, and yields without spending an attempt. The next pass resumes at that exact
offset. Once the final span is scanned, the module fuses the gaps into segments, performs rescue and
dedup, removes the private checkpoint, and only then exposes the proposal in the review queue. The
ten-minute span is an implementation budget, not a setting: it is deliberately well inside the
two-minute job deadline on the measured hardware, and adding another operator control would make
reliability depend on tuning Loomarr should own.

The checkpoint rides inside `filler_split_proposals.segments_json` as a versioned document rather
than adding coordination columns to the table. Readers remain backward-compatible with the former
bare segment array. A draft has no reviewable segments and is filtered from both the Incoming reels
list and `GET /v1/filler/splits/{id}`; the pipeline and rewind path still see it. Persist after every
successful span, before starting the next: a crash or deadline can repeat at most one span, never
the whole recording. The media-tools seam takes a time span and returns black and silence gaps
separately; preserving the source across checkpoints is what lets detector agreement remain the
strongest boundary-confidence evidence. Whether production uses ffmpeg filters or a test adapter
supplies captured intervals stays inside that module.

**Every segment operation is duration-bounded.** The ffmpeg adapters seek with `-ss` and limit work
with `-t (end-start)`. They never pair an input seek with absolute `-to end`: `-to` is a position,
not a duration, and made a late 30-second segment decode/cut an ever-growing portion of the reel.
That error made fingerprinting super-linear and could produce oversized cuts. Timeline checkpointing
still stands independently—the fixed span makes each invocation bounded; the durable proposal makes
the sequence resumable.

**Catalog fingerprints are a persisted derived cache, not work a reel repeats (V54).** Duplicate
detection compares each proposed span with every other catalog clip. Decoding those whole catalog
clips again for every compilation makes the rung scale with *catalog size × reels attempted*, and a
deadline or restart throws away every completed decode. Loomarr therefore persists the successful,
non-empty dHash sequence for a catalog clip in a sibling `filler_clip_fingerprints` table and reuses
it across reels, retries and process restarts. Proposed spans remain proposal-local work: only
whole catalog clips are reusable across compilations.

The cache key is **`(clip_hash, algorithm)`**. `clip_hash` is the SHA-256 identity of the actual
bytes, so replacing or transcoding a file produces a hard miss instead of reusing evidence from the
old media. `algorithm` versions every sampling and hashing choice (`fps`, dimensions, pixel format,
hash construction and comparison alignment), so a future detector change can coexist with old rows
without a lock-step rewrite. Identity replacement deletes the old hash's entries rather than
re-keying them: the new bytes must earn new evidence. Catalog pruning removes orphan cache rows; the
table deliberately has no foreign key to the rebuildable `clips` cache, matching the pipeline-state
ownership rule above.

Cache population is incremental. After one catalog clip decodes, its fingerprint is upserted through
a short context detached from the expiring pipeline pass; a later timeout can repeat at most the
currently-decoding clip, not the entire catalog prefix. Concurrent misses may compute the same clip
and converge through the same upsert. Each reel loads the current algorithm's cache in **one read**,
not one query per catalog clip; a malformed row is omitted without discarding valid siblings. A
read/write/cache-document failure is **fail-open**: Loomarr
computes from media when it can, logs a cache write failure, and never calls a clip duplicate from
missing or corrupt cached evidence. Empty frame sequences and failed decodes are not cached, because
an undecodable file is not proof that two adverts are the same. This is derived state and adds no
operator setting or review control.

#### The rule V51g established, restated as its principle (V54)

V51g's rule was written as *"what a rung may not do is spend a model call per segment"*. That
sentence is the **letter** of a measurement taken on one provider; the **principle** it protects is
narrower and is what actually binds:

> ⚠ **A rung's cost must stay inside its pass budget, and must not scale unboundedly with a clip's
> content.** A per-segment model call is forbidden *when it cannot satisfy that*.

The distinction matters because V51g's 377s was **51 × 7.4s of SERIAL inference on one local GPU**.
That is a property of `qwen3:8b` on a single 3080 Ti, not of the algorithm: a hosted provider
answers concurrently, and the same 51 calls complete inside the pass. Restating the rule as an
absolute would forbid a shape that no longer costs what it cost when it was measured.

**What does NOT change, and is not reopened:**

- ⚠ **The text `classify` inside `Propose` stays deleted.** Its second defect was never about
  speed: `SplitSegment.Transcript` is empty at split time (`StageTranscribe` runs *after*
  `StageSplit`), so all 51 calls classified nothing but a generated name — `"… part 7"`, identical
  across segments but for the number — and grounding correctly refused to invent tags from that.
  **A faster model classifying `"part 7"` grounds exactly as much as a slow one.** The tripwire in
  `splitjob.go` stands: it must not come back to that file.
- The atomic confirm stays atomic. Deferral-not-failure stays. The per-pass segment budget is now
  a persisted checkpoint: each examined segment is marked on the proposal, a deliberate yield
  spends no retry, and the next pass resumes the unchecked tail even for ~500-segment captures.

**What a per-segment model call must satisfy to be permitted:**

1. **An explicit budget setting**, sized per pass, in the same family as `filler.pipeline.max_whisper`
   and `…max_vision`. No budget ⇒ not permitted, whatever the provider.
2. **Data the rung genuinely needs and cannot obtain later.** The bar V51g set: the classifier it
   removed produced results the `tag` rung recomputed downstream anyway — *"the pipeline paid twice
   and kept the worse answer's cost."* A rung may not buy a second, earlier, worse copy of something
   the ladder already produces.
3. **A signal that actually exists at that point in the ladder.** This is what disqualified the text
   classifier and is the test any replacement must pass.

**The worked example: grounding a segment from PIXELS, not from its name.** The auto-confirm gate
(§10 V34) refuses a segment with neither `Audience` nor `Category`, because such a clip can only
ever be a fallback-ladder pick and is not something to create unattended. `classify` used to supply
those fields; removing it left the gate with **no data source at all**, so `filler.autosplit.enabled`
has been default-ON and structurally unable to fire ever since — measured on the maintainer's
catalog as **45 compilations parked at `split`, none auto-confirmed**. A default-ON feature that
cannot fire is worse than one that is off, because the operator believes it works.

A keyframe drawn from the segment's own span satisfies all three conditions where the transcript
could not: the frames exist at split time (`MediaTools` already extracts keyframes, and already
takes a span for `Transcribe`), a category grounded from what is on screen is a real answer rather
than a re-reading of `"part 7"`, and the vision tier's grounding (`groundVisionTags`) is the same
one the `vision` rung uses — so the gate is fed by the same vocabulary that judges it.

⚠ **This does not make the gate's data authoritative for the CLIP.** Each spawned segment still runs
the whole ladder for itself and is tagged downstream with its own transcript; the split-time
grounding exists to answer the gate, not to replace `tag`. Where the two disagree, the child's own
pass wins — it is later, better-informed, and per-clip.

#### The budget is a RATE, not a ceiling (V54)

⚠ **`filler.pipeline.max_split_vision` bounded one pass but was read as a limit on the reel, and
that made the grounder above unable to finish on any real compilation.** `ground` indexed the
budget absolutely, so a reel with more segments than the budget ground its first N on every pass
and never advanced past them; the tail stayed ungrounded and `AutoConfirmable` returned
`RejectUntagged` forever. Measured on the maintainer's catalog against a default budget of **60**,
live proposals hold **82, 133, 142, 222, 235 and 303 segments** — so the budget silently meant
"reels this size can never auto-confirm", which is the same class of default-ON-but-cannot-fire
failure V54 exists to end.

Three changes make the budget behave as the per-pass cost bound it was always described as:

1. **Grounding PERSISTS on the proposal.** `SplitSegment` already carried `Category`/`Era` and
   `segments_json` is a plain JSON blob, so this is additive — no migration. One new field,
   `Looked`, records that the grounder examined a segment *whether or not it came back with
   anything*. ⚠ Inference cannot replace it: `Category != ""` conflates *never looked at* with
   *looked at and grounded nothing*, and treating those alike is exactly what makes a resumable
   budget never converge.
2. **A partly-grounded reel DEFERS** (the fourth verdict, above) rather than being judged. It
   returns to the belt with its progress recorded — *"looked at 60 of 142 cuts"* — and the next
   pass resumes at segment 61. A 303-segment reel completes in six passes.
3. **Termination is progress, not a counter.** A defer requires that the pass looked at something,
   and every look marks a segment, so the pending count strictly decreases. A pass that achieves
   nothing (vision off, no vocabulary, the provider down at the first segment) does not defer: it
   falls to the gate and the reel parks with a real reason. No new column, no timestamp, no
   retry budget.

⚠ **The write must never insert.** Grounding is a read-modify-write spanning minutes of vision
calls, so it races `Confirm`. `UpdateSplitProposalSegments` is an `UPDATE` returning `ErrNotFound`
on zero rows, deliberately NOT the `INSERT … ON CONFLICT` upsert — a grounding write landing after
a confirm would otherwise **resurrect** the proposal: a pending review for a reel already cut,
pointing at a composite whose segments are in the catalog. It also leaves `created_at` alone, since
the Incoming queue orders by it and a reel must not jump the queue for having been grounded.

⚠ **An existing proposal is RE-GROUNDED, never re-detected.** The split rung used to return
immediately when a proposal existed, which read as "leave the operator's cut list alone" and was in
fact a dead end: the row went to `review`, `ListPipelineWork` only claims `running`, and nothing
reached that reel again. Every compilation detected before the grounder existed was therefore
permanently ungroundable. Re-detection stays the operator's call (`POST /v1/filler/split`) because
a rung must not redraw a cut list a human may have open; grounding is additive and touches no
boundary.

⚠ **…and a re-detect returns the reel to the belt (V54a).** The paragraph above was only half a
remedy. `POST /v1/filler/split` does replace the proposal — `filler_split_proposals.clip_hash` is
UNIQUE and the upsert conflicts on it, so a fresh scored cut list lands in place of the stale one —
but detection writes no pipeline row. A reel parked at `split`/`review` therefore stayed parked,
now holding a proposal nothing would ever read, and the documented remedy could not work. The
operator path un-parks the row itself: `disposition='running'`, `status='queued'`, `attempts=0`,
`next_run=0` — the same four columns migration 00050 set, for the same reasons, including giving
back attempts spent losing to a gate that could not be won.

⚠ **After detection, never before.** An un-parked row is claimable, and claiming it mid-detection
would let the split rung ground the OLD segment list — or call `Propose` a second time on the same
reel — while the new list is still being written. So the un-park is the last step of a successful
detection, and a detection that fails leaves the row exactly as it found it: parked, with its
original proposal, which is the honest state.

⚠ **Scope is migration 00050's `WHERE` clause**, for the migration's reason: `stage='split'` AND
`disposition='review'` is the unreachable state. A `rejected` row keeps its own restore path
(`Soft()`) — a re-detect must not quietly overturn a refusal an operator can see and argue with —
and a row already `running` has nothing to un-park.

⚠ **Why this cannot be a migration.** 00050 performed exactly this un-park, once, and it is the
worked example of why the mechanism was wrong: it ran at 07:37 on 2026-08-12 under the binary that
preceded per-segment confirm, the old all-or-nothing gate re-parked all 17 reels within nine
minutes, and goose recorded it applied forever. A data migration cannot be re-run and cannot know
which binary it is firing under. **A state transition whose correctness depends on the running code
belongs on an operator path or a job, never in goose.**

⚠ **Side effect worth having: the gate's inputs became observable.** `GET
/v1/filler/splits/{proposalId}` returned only `endMs/index/name/startMs`, so a grounded and an
ungrounded proposal read identically and the only way to tell whether the grounder had run was to
watch for an `ffmpeg … thumbnail=n=` process. That is why V54's own shipping went unverified.
