# V54 — Filler refresh 2

**Status: complete.** Phase A shipped in #247/#248; the remaining product work shipped across
#347/#348 (split boundary and confirmation), #350 (settings honesty), #352 (source hierarchy,
split preview, catalog, accessibility and docs), #354 (hosted/OpenRouter model selection), #356
(automated intake, break handling and persisted fingerprints), and #359 (intake reliability,
hold reasons, counts and composite visibility). The final certification PR closes two upgrade and
provider-routing seams found only by exercising the completed system: stale holds on legacy
confirmed composites, and filler paths bypassing a hosted provider's namespaced credential.

### Final certification — 2026-08-15

An isolated SQLite runtime was started from a clean worktree and driven through the real Tasks and
Filler UI with this deterministic corpus:

| Fixture | Expected decision | Observed |
| --- | --- | --- |
| healthy commercial + byte-identical copy | ingest once, discard duplicate | 7 files taken, 1 duplicate discarded; one human-titled clip filed |
| clip below the minimum duration | reject at the scan boundary | rejected before cataloguing |
| video-only clip | reject at the scan boundary | rejected before cataloguing |
| 24s black-screen clip | reject with measured reason | `black_content`; 99% black |
| 24s silent-audio clip | reject with measured reason | `silent_content`; 100% silent |
| compilation made from duplicate catalog content | resolve without approval | proposal exhausted by deterministic duplicate handling; terminal, non-airable composite |
| varied compilation | preserve uncertainty for review | one review item with four proposed clips |

The Incoming page contained no 32–64 character hexadecimal display names; it showed the source
filenames and the measured rejection reasons. After a process restart, the proposal and all
terminal decisions were unchanged, the catalog fingerprint cache retained its row, and another
pipeline pass reported no work (`repaired=0`, `advanced=0`, `deferred=0`). Provider routing is
certified at the HTTP boundary with local deterministic servers for text/tagging, inherited vision,
hosted timed transcription, and the setup check. No OpenRouter credential was available in the
isolated environment, so this run intentionally did not make a paid live inference call.

**Original worktree:** `../loomarr-v54-filler`, branch `v54-filler-refresh`, based on
`origin/main` `2283dfaa`. Re-verify that historical setup before resuming. The generated API client
is gitignored, so `make bootstrap` is load-bearing in a new worktree.

**Plan file naming:** deliberately descriptive and repository-local. The V51 plan lived only in one
agent product's home directory, which cost a later session a re-derivation. Active plans belong under
`docs/engineering/plans/` so every harness sees the same artifact.

---

## Context

The maintainer walked the filler surfaces on 2026-08-10 and listed ten problems. A 12-area
audit (live browser via Playwright MCP + source reading, each area independently re-verified by
a second agent) confirmed all ten and found roughly twenty-five unreported defects.

Five of those unreported defects are the **same class the last three merged PRs were about** —
a control that accepts input and quietly does less than it says (#236 "four settings stop
lying", #240 "three filler controls that accepted input and quietly did less than they said",
#241). One is a security hole. That is why severity comes before UX here.

Two framing corrections the audit produced, both of which SHRINK work and must not be
re-litigated during implementation:

- **A pod duration budget already exists and is enforced.** `ladder.go:84`
  (`budget := w.GapMs - bumperBudgetMs`) and `ladder.go:139` (reject clips that overflow it);
  pinned pools get the same at `pod.go:252`; the two-stage fill subtracts as it goes at
  `pod.go:204`. `Window.GapMs` IS the duration target. It never binds in production only
  because callers pass `poolGapMs` (~588s of effective budget), not a break length. Do not
  invent a new duration field.
- **Cross-catalog near-duplicate dHash detection already works end-to-end.**
  `(*Splitter).dedup` (`splitjob.go:250`) hashes every catalog clip, persists the hit as
  `SplitSegment.DupOf` (`split.go:68`), blocks auto-confirm (`autosplit.go:101`), is counted in
  the API (`fillerincoming.go:549`) and survives store conformance
  (`conformance_filler.go:1567,1584`). What is missing is **persistence of the hashes** — they
  are recomputed every run, i.e. one ffmpeg `GrayFrames` decode per catalog clip per split job.

---

## Decisions locked (maintainer, 2026-08-10)

| # | Decision |
| --- | --- |
| 1 | **Severity first, then UX.** Phase A is the security hole + the controls that lie. |
| 2 | **Persist dHash; defer embeddings.** No vector column. Delete §10's false claims about it. |
| 3 | **Fix `IMAGES_DIR`, make a missing dir fail loud.** Animated hover already exists — repair, don't build. |
| 4 | **`filler.break_duration` default `5m`, global + per-channel**; `pod_max` becomes a ceiling that yields to duration. |
| 5 | **Restore `Classify` in `StageSplit`** so auto-split's gate has its data. Keep the gate's safety check; add a budget knob. Auto-split stays default-ON. |
| 6 | **`poolGapMs` becomes a floor**, not a constant: `Window.GapMs = max(poolGapMs, resolved break duration)`. No ceiling on the user-facing knob. |

⚠ Decision 4 changes behaviour on upgrade. Acceptable per PROGRESS.md's standing note that
Loomarr has no production installs; record it as a deliberate change, not a bug fix.

---

## Phase A — the hole and the lies

Doc-first note: A2–A7 are code-vs-code defects, so §10 needs no change for them *except* where
noted. A1 needs a §11 sentence.

### A1 — Open redirect on the login route  ✅verified
`/login?redirect=https://evil.example` on an already-signed-in browser becomes an off-site
`window.location.replace()` via `redirect({href})` → `reloadDocument`, gated only by an
http/https scheme check. The same unvalidated value behaves differently at the two consumption
points: `login.tsx:96` navigates off-site, `login.tsx:28` throws a cross-origin
`history.pushState` SecurityError.

- Add `safeRedirectPath()` under `web/apps/web/src/auth/`, **mirroring the Go original**
  `safeReturnPath` at `internal/api/ssoroutes.go:296-309` (doc comment :283-295; call sites :168
  and :259, the latter commented "Re-validated HERE, not merely at /start"). Must: require exactly
  one leading `/`, reject `//`, reject `\` **anywhere** (not just leading — `/\/evil.test` defeats
  the narrow fix), reject any scheme or host, and `url.Parse` rather than prefix-match.
- Apply in `login.tsx` `validateSearch` (:82) so a hostile value never reaches the component,
  **and** at both consumption points (:28, :96).
- Add the missing test: nothing today asserts the param is set, that login honours it, or that
  an external target is refused. `reachability.test.tsx:328` excludes `/login`, and
  `wizard.spec.ts:115` asserts `toHaveURL(/\/login/)` — a regex, so it does not constrain the
  param either. ⚠ Reuse the existing harness: `app-router.test.tsx:66-68` already returns the
  router "so a test can assert WHERE the router settled" and `:117` defines
  `at = (router) => router.state.location.href`. Use `at()`, **not** pathname+search —
  `:113-116` records that `location.search` is a parsed object and concatenating it throws
  "Cannot convert object to primitive value", which was once blamed on the app for five failures.
- §11: add the "post-login redirect is validated where it is emitted" sentence.

⚠ **The bounce is reachable by SERVER RESTART, not only by a password change** — this
reprioritises the item from cosmetic to routine. `_authed.tsx:72-78` wraps `ensureQueryData` in a
bare `catch {}` and `meQueryOptions` sets `retry: false` (`me-query.ts:6`). **Nothing inspects the
status**, so any failure — network blip, 500, proxy hiccup — bounces a user holding a perfectly
valid cookie to `/login?redirect=…`. And this app has a first-class self-restart feature wired
into the authed layout (`RestartWatchProvider`/`RestartOverlay`, `_authed.tsx:21-24,:57-62`), so an
operator restarting from the Dashboard hits it **every time**. Fix: branch on
`ApiError.status === 401` (`mutator.ts:11-19`) before concluding "signed out". Otherwise any
"Sign in to continue to X" copy gets shown to someone whose session never ended.

⚠ Two smaller gaps in the same flow: a refused SSO round trip **discards the destination**
(`ssoroutes.go:279-281` emits `/login?sso=<reason>` and nothing else, so the deep link carried
into `/v1/auth/sso/start?next=…` is gone exactly when the user is most disoriented), and the
`/wizard` bounce discards it too (`_authed.tsx:76`, `login.tsx:93`). The latter is defensible on an
unclaimed install — decide rather than inherit it.

⚠ Do **not** add a self-session-revoke fix: there is no such path. `account.tsx:184-195` hides
Revoke for the caller's own session (`{!s.current && …}`), backed by `SessionDTO.current`
(`usersroutes.go:192`, computed via `auth.IsCurrent` at :239), with the comment that a button
labelled "Revoke" which logs you out is a trap. The password-change half of this is real; the
revoke half has nothing to fix.

### A2 — Incoming's "Add tags" / "Not right" buttons do nothing  ✅verified
`ClipTagDialog` is mounted only inside the catalog-tab branch (`filler-page.tsx:854`), and a
path is passed where a hash is expected. The buttons render, take a click, and no dialog opens.
Mount the dialog for the incoming branch and fix the identifier passed.

### A3 — **Three of the four decision buttons don't stick**  ✅verified
⚠ **Reframed by the verification pass, and it is the largest defect in this plan.** The root
cause is not the dismiss path specifically: **the review→terminal transition has no operator-side
writer at all.** The only things in the whole tree that write `filler_clip_pipeline` are
`filler.Pipeline` itself and `clearPipelineRejects` (`fillerbulk.go:216-231`, restore only). So:

- **"Use it" / approve** — `POST /v1/filler/file` calls `SetClipsHeld(held=false)` and nothing
  else (`fillerfile.go:97`). The row still reads `disposition=review`,
  `ListClipPipelines(ConveyorOnly)` still returns it (`store/clippipeline.go:133-137`), and
  `conveyorDTO` re-marks `needsDecision = (disposition == review)` (`fillerincoming.go:445`). **A
  filed clip returns to the "needs a decision" list on the next refetch and `total` never
  decrements.**
- **"Don't use it" / dismiss** — `SetClipsRemoved` writes `removed_at` only
  (`store/clips.go:884`); `GetClip` is `WHERE hash = ?` with no `removed_at` predicate
  (`clips.go:315`), so the belt's fallback loop (`fillerincoming.go:281-292`) re-resolves it.
- **"Looks right"** — does **not** file, contrary to the first report. `onConfirmEra` calls
  `confirmEra.mutate` only (`incoming-tab.tsx:107-119`), and `patchFillerClip`
  (`filler.go:508-580`) touches tags/kind and never `SetClipsHeld`. There is no second mutation
  and no server-side unhold.

Fix the transition itself — give the operator paths a writer that moves the pipeline row to a
terminal disposition — rather than excluding removed clips from the fallback loop, which repairs
only a third of it.

### A4 — Auto-split is default-on and can never fire  ✅verified
`AutoConfirmable` rejects any segment where `s.Audience == "" && s.Category == ""` with
`RejectUntagged` (`autosplit.go:120-123`), and nothing populates those fields. A default-on
feature is dead code.

**Decision 5: restore `Classify`, keep the gate.** The gate's own comment justifies the check as a
safety property — a segment with neither "can only ever be a fallback-ladder pick, which is not
something to create unattended out of a file the operator still had" — so weakening it would trade
safety for convenience, which prime directive 3 forbids. `split.go:54-64` already documents these
fields as coming from "the SAME `Classify` the tag job uses", and `Classify` still exists at
`tag.go:165`. **Not circular:** it takes `(name, sourceText)`, not a created child clip, so it can
run on a proposal.

Implementation:

1. ⚠ **`SplitStage` cannot call `Classify` today — it lacks both dependencies.** The struct holds
   only `splitter`, `store`, `autoConfirm`, `minClipDuration` (`stage_split.go:33-40`), while
   `Classify(ctx, provider llm.Provider, forest *taxonomy.Forest, name, sourceText)` needs a
   provider and a taxonomy forest. Thread them in with a `WithClassify(...)` builder, matching the
   existing `WithAutoConfirm` idiom (`stage_split.go:49-54`) — and keep the same nil-means-skip
   safety: no classifier ⇒ propose only, exactly as no `autoConfirm` does today.
2. Call it per segment in `Run`, after `splitter.Propose` and **before** `AutoConfirmable`
   (`stage_split.go:98`).
3. Add `filler.pipeline.max_classify` beside `filler.pipeline.max_whisper` (`declared.go:716`) and
   `max_vision` (`:721`). Non-negotiable: the live catalog holds proposals with **235, 222, 142 and
   133 segments**, so an unbudgeted rung fires one LLM call per segment per reel.

⚠ **The prerequisite sub-question, and the reason this may not move the needle on its own:**
`Classify`'s `sourceText` is a transcript, and `StageTranscribe` runs **after** `StageSplit`
(`StageOrder`, `pipelinestate.go:39-42`). At split time a transcript exists only on the *rescue*
path, for over-long segments that needed boundary recovery (`splitrescue.go`, which already runs
whisper and has `TranscriptText`). For every other segment `sourceText` is empty, leaving `Classify`
only the proposed name — which for "…part 7" grounds nothing, so the gate rejects anyway.

Proceeding on this assumption unless you say otherwise: **reuse the rescue path's whisper over the
compilation, sliced per segment span, bounded by the new `max_classify` budget.** That is what makes
the rung meaningful rather than decorative. If the cost is unacceptable, the honest fallback is
decision 5's runner-up — flip `filler.autosplit.enabled` to `false` and keep the review flow — not
a classifier that runs and grounds nothing.

⚠ Worth naming so it is not rediscovered: the gate wants pre-confirm certainty about data the
architecture deliberately produces post-confirm. Split **spawns** children and each child runs the
whole ladder for itself (`stage_split.go:12-14`), so those segments get transcribed and tagged
regardless — just after the cut, not before it. Restoring `Classify` buys a *second, earlier*
classification pass purely to satisfy the gate.

⚠ §10 also contradicts itself on this feature's default, and the fix must reconcile all three in
**one** pass: `design.md:2248` reads "Auto-confirm is a separate opt-in (`filler.autosplit.enabled`,
default **off**)", while `design.md:3411`, `declared.go:670-671` and `stage_split.go:16` all say
**ON** (recorded there as a V51b maintainer decision). Decision 5 keeps it ON, so :2248 is the line
that changes.

⚠ §10 also contradicts itself on this feature's default, and the fix must reconcile all three in
**one** pass: `design.md:2248` reads "Auto-confirm is a separate opt-in (`filler.autosplit.enabled`,
default **off**)", while `design.md:3411` and `declared.go:670-671` both say **`true`**. Restating
one leaves §10 prose asserting the opposite default from §15's table.

### A5 — Confirm silently quantizes every cut to a whole second  ✅verified
`toDraft` renders each boundary through `formatMmSs`, which **floors**
(`web/packages/core/src/format/format.ts:49-52`), and `toWire` parses it back with `parseMmSs`.
Round-tripping a proposal through the review screen therefore moves every boundary. Keep
sub-second precision in the draft state and only format for display.

### A6 — The 0% progress bar is unreachable-by-design  ✅verified
The `-1` "no measurement" sentinel is never produced by the backend on either the GET or SSE
path — `onProgress` discards it (`pipeline.go:611`). Every running non-stage-transcode stage
renders a **0% bar**, which is a false claim about work in progress. Four artefacts are green
over that unreachable state (`fillerincoming_test.go:241` writes `Progress:-1` straight into a
store row bypassing the runner; `clip-pipeline.test.tsx:29,:115`; `clip-pipeline.stories.tsx:12`)
— i.e. the tests assert a state production cannot reach. Produce `-1` for real, and make the
tests drive the runner rather than the store.

Also here: the DB progress write can effectively never fire for a long transcode (moving
baseline, `pipeline.go:614-617`), and §10's ">= 2s" half of that throttle is not implemented at
all. Restate the throttle in §10 as `percent >= lastWritten + 10 OR >= 2s since last write`,
where `lastWritten` is the last **persisted** value and the skip branch must not move the
baseline.

### A7 — The filmstrip promises a preview it does not give  ✅verified (browser-confirmed)
`segment-filmstrip.tsx:81` reads "every block is one detected clip — click to preview" while
`onFocus` only scrolls and rings (`split-review-editor.tsx:200-206`). Confirmed live: clicking
a block moves focus and nothing else; the page contains **zero** `<video>`, `<canvas>` and
`<img>` elements. Either deliver the preview (Phase C) or change the caption — do not ship the
caption unchanged.

### A8 — `POST /v1/filler/split` fires with no confirmation  ✅verified
The catalog card's "Split into clips" button (`clip-card.tsx:482-492`, wired at
`filler-page.tsx:802-806`) fires immediately, starting a multi-minute full-decode ffmpeg pass,
with one `<p role="status">` line as the only feedback. It is also offered on 15s commercials,
where splitting is meaningless. Gate it behind confirmation and hide it where it cannot apply.

**Phase A gate:** `make check` + `make fe` + `make openapi-verify` + `make retired-verify`, plus
a new test per item. For A2/A3/A4/A6, sabotage the fix and confirm the test goes red — a
first-try pass on this class is suspect.

---

## Phase B — Sources twirl-down (cheapest real win)

The parent/child rollup **already ships on the wire** and was specified, tested and documented
in V51c (PR #201, `62d46525`) — `git show --stat` on it touched zero files under `web/`. §10
:2411-2413 states the tab twirls down; it does not. Confirmed live: `GET /v1/filler/sources`
returns `group` + `parentId`, with `provider:archive` above its three children.

Frontend only, in `filler-sources.tsx`. No API change, no migration, no §15 knob, no §14 row —
`Disclosure` (Base UI Collapsible) is already in the tree.

1. Render a `group` row as a `Disclosure.Trigger` header with chevron, child count, and the
   `lastFetchedAt` the API already computes as a MAX over children; hide rows whose `parentId`
   is a collapsed group.
2. **Fix the denominator** (`:73`): "N of M on" counts the two derived provider nodes as
   sources, so the page shows "5 of 6 on" while the header stat says "4 of 4" — two summaries
   of one list disagreeing on screen (both confirmed live). ⚠ A fresh install shows the same
   thing, and not for the reason first reported: `00034_seed_default_sources.sql` seeds three
   archive collections plus a blank-URI youtube row, and `fillersources.go:618-620` deliberately
   skips that seeded youtube row because the derived node replaces it. So the correct
   fresh-install picture is folder + 3 archive children + Archive.org node + YouTube node = **6
   rows**, of which two are derived. Exclude derived nodes from the denominator, don't reduce the
   row count.
3. **Dormant-group rendering** (verifier's catch): every off-state is gated on
   `s.switchable && !s.enabled` (`:98`, `:143`, `:162`), and a group is `switchable:false`, so a
   provider whose every child is off still renders as if running.
4. Provider nodes inherit the leaf kind chip (`:32-37`), so the YouTube container is labelled
   PLAYLIST and a container looks identical to a collection. Give containers their own
   treatment.
5. An empty Archive.org/YouTube provider renders the `not configured` caution Badge (`:140`) —
   as a **fault**, while §10 and `store/fillersources.go:134-145` both say it is an
   **invitation**.
6. One shared `searchOpen` boolean for all rows (`sources-panel.tsx:47`) means "Search it" on
   one archive row expands every archive row's panel, sharing one query and one result set.
7. `GET /v1/filler/discover`'s `collection` param is never sent, so a collection row's search
   hits all of archive.org. ⚠ `q` and `collection` are **mutually exclusive** (422 if both) —
   not additive, as first reported.

⚠ No fixture anywhere renders a `group`/`parentId` row —
`web/packages/fixtures/src/testcard/testcard.ts:720-792` and `reachability.test.tsx:118` are
flat V37-era lists. Every unit and visual test is currently green over the flat shape, so the
fixtures must be updated or Phase B ships untested.

Docs: §10 :2409-2456 (the rendering sentence), PROGRESS.md V51c entry (:1300-1306, claims the
tab already twirls down), `docs/frontend-design.md` §5.1c (record `Disclosure` as the affordance),
and `fillersourcegroups_test.go:57-63` (its comment is a second copy of the design sentence).

---

## Phase C — Split review: modal + a real preview

**Headline: frame-accurate preview needs zero backend change.**
`GET /v1/filler/media/{hash}` serves the clip's own bytes through `http.ServeContent`, so Range
and 206 already work (`fillermedia.go:155-168`); it is `RoleMember` (`filler.go:168-175`) and
resolves hash→path via `GetClip`. `SplitProposal.clipHash` **is** that hash, and
`clipMediaURL()` already builds the URL (`web/packages/core/src/clip-thumb/clip-thumb.ts:31`).
Codec-wise the `split` rung runs after `transcode` (`pipelinestate.go:41`) and the mezzanine is
h264/yuv420p/aac in .mp4 with a forced keyframe every 2s (`transcode.go:57-63,:256`) —
natively decodable and seekable.

1. Mount a `<video>` with `preload="metadata"`, seeking to the focused segment's start. This
   turns A7's caption into the truth.
   ⚠ If you instead want server-rendered per-segment thumbnails (a filmstrip of real frames),
   note the precise gap: `filler.FFmpegArtwork` (`artwork.go:242-257`) **is** a frame-at-timestamp
   capability — `-ss <startSeconds>` before `-i`, so keyframe seek without a full decode — but
   **no HTTP route exposes it**, and `MediaTools` has no timestamp-taking frame method
   (`mediatools.go:40` `Keyframes(ctx, file, n)` spreads n frames across the whole clip). The
   video-scrub route above avoids that work entirely; the thumbnail route needs a new endpoint.
2. Fix the page's identity — `split-review-page.tsx:61` heads the screen with a raw 64-char
   content hash (confirmed live). ⚠ **This is cheaper than first reported and needs no wire
   change** (verifier's catch): `GET /v1/filler?hashes=<hash>&includeComposites=true` already
   returns the full `ClipDTO` with `name` (`filler.go:188`), `durationMs` (`:198`) and
   `thumbImage` (`:227`), and `useFillerCatalog`
   (`web/apps/web/src/filler/channel-filler/use-filler-catalog/use-filler-catalog.ts:21-46`) is
   the existing hash→ClipDTO hook. So no new DTO field, no `make openapi`. The one trap:
   the reel is `is_composite=true` (`stage_split.go:70`) and `clipWhere` excludes composites by
   default (`store/clips.go:533-535`), so `includeComposites=true` is required.
   ⚠ Still true and still a cost *if* a display field is ever added: the API leaks the domain type
   as the wire type (`getFillerSplitOutput{ Body filler.SplitProposal }`, `filler.go:959-962`;
   `confirmFillerSplitInput.Body.Segments []filler.SplitSegment`, `:983-991`).

   On the modal question itself: ⚠ **there is no precedent for overriding `DialogContent`'s
   `max-w-md`.** `grep -rn '<DialogContent'` returns exactly two hits (`job-edit-modal.tsx:59`,
   `dialog.test.tsx:25`) and neither passes a `className`. A modal here would *establish* that
   precedent, which is a different review conversation — worth deciding before building.
3. Surface the gaps between segments (confirmed live: seg1 ends 02:38, seg2 starts 02:45) —
   today unassigned regions are invisible and unclaimable.
4. Warn on segments below the `filler.min_duration` floor (`10s`) — the sample proposal contains
   3s, 4s and 5s segments that will be rejected later with no hint here.
5. The V45 copy correction never landed: `split-review-page.tsx:26-27` ("the compilation row is
   gone") and `split-review-editor.tsx:154` are wrong, and
   `confirm-filler-split`'s own OpenAPI description still tells every client the compilation row
   is removed (`filler.go:141-143` → `api/openapi.yaml:7274` → generated TS).

⚠ **Re-split duplicates airable content** (verifier's catch, worse than first reported):
`Splitter.Confirm` (`splitjob.go:301-435`) never removes or tombstones prior segments, so
confirming a second time leaves both sets in the catalog. `parent_hash` also has no writer after
insert — `store/clips.go:241` names `SetClipParent`, which does not exist. Fix both before
offering any re-split affordance.

---

## Phase D — Catalog: artwork, hierarchy, pagination

### D1 — Artwork: THREE stacked causes, browser-verified 2026-08-10

⚠ **The audit said the env fix "restores every image in the app instantly". That is FALSE, and it
was disproven by test.** Fixing `IMAGES_DIR` made an *authenticated* request work and changed
nothing in the browser: 5 image tags, **0 loaded, 5 broken**. Two defects were masking each other.
The verified chain, in order:

1. **`IMAGES_DIR` was relative** — `./data-p5/images` resolved against Air's cwd, so the store
   pointed at the v51b DB while the image dir pointed at a directory that does not exist. Already
   fixed in the v54 `.env` (both paths absolute, one `DATABASE_URL`). Evidence it was only half the
   problem: the ORIGINAL blob was present all along at `orig/d1/cc/<hash>.jpg`.
2. **Clip artwork is `VisibilityMember`** (`adoptjob.go:148`) — correct, it is library content — and
   the handler answers an anonymous request with **404, never 403** (`api/images.go:122-125`,
   deliberate: a 403 would confirm which hashes exist).
3. **`URLFor` hands the SPA an ABSOLUTE URL on `server.public_url`**, so in dev the browser asks
   `http://100.123.114.40:8080` from an app served at `localhost:5173`. That is cross-origin, the
   session cookie is scoped to localhost, **so the request is anonymous** → 404 → `text/plain` +
   `nosniff` → Chrome **ORB**-blocks it → `<picture>` has already committed → the ThumbHash is all
   that renders. **Those are the blue hashes.**

Proof, both fetched from the same page in one evaluation:

| Request | Result |
| --- | --- |
| `/v1/images/<hash>/w780.jpg` (relative, via Vite proxy) | **200 `image/jpeg`** |
| `http://100.123.114.40:8080/v1/images/<hash>/w780.jpg` | **Failed to fetch** |

**The fix: SPA-facing image URLs must be same-origin/relative.** Keep `URLFor`'s absolute base for
the images a *machine* fetches — channel logos, which Tunarr pulls unauthenticated and which are
`VisibilityPublic` (see `channelicon.go`'s own note). The two consumers need different URL shapes,
and today one field serves both.

⚠ **Why this never surfaced:** in production the backend serves the embedded SPA at the *same
origin* as the images, so the cookie is sent and everything works. It breaks in dev, and in any
deployment where `server.public_url` differs from the origin serving the SPA — a reverse proxy, a
split hostname, a LAN IP. So this is a real product defect, not merely a dev-env artifact.

⚠ **On-demand rendition rendering is NOT broken** — another audit claim to drop. `Rendition`
renders on miss behind singleflight (`service.go:328-351`); only AVIF is job-only. The absent `drv/`
directory was a *symptom* of causes 1–3, not a cause: the moment an authenticated request arrived,
`drv/` was created and the derivative written. Do not build a derivative job.

Then the defects that genuinely survive the above:

- **`images.Image.Animated` has no writer anywhere.** `Ingest` never computes it, so every
  animated hover loop is stored `animated=false`. This is why hover would not animate even with
  images loading — and the producer genuinely exists (`FFmpegArtwork`, `artwork.go:242`, writes a
  320px JPEG **and** an animated WebP, `-loop 0`, 12fps, 6s; 116 `.jpg` + 109 `.webp` on disk).
- **No derivative has ever been generated on this install** — the blob store has `orig/` and no
  `drv/` at all, and `api/images.go:129` returns `ErrNotFound`.
  ⚠ **Do NOT build an operator warning for this — one exists.** `internal/images/gc.go:179`
  `warnUnrecoverable` stats every original, counts `GCResult.MissingUnrecoverable` (`gc.go:50`),
  logs it every pass (`gc.go:111`) and raises an operator notification naming a remedy
  (`gc.go:193-198`). The sharper defect is that the signal **structurally excludes this class**:
  `ListUnrecoverableImages` is `WHERE origin IN ('upload','generated')`
  (`store/images.go:259-263`) and clip artwork is `OriginExtracted`. So the work is fixing that
  predicate/classification in an existing job, not adding a new warning.
- ⚠ **"Recoverable" is theoretical for extracted artwork — nothing recovers it.** Once adoption
  sets the hashes, `ListClipsPendingArtworkAdoption`'s predicate
  (`(thumbnail <> '' AND thumb_image_hash = '') OR (preview <> '' AND hover_image_hash = '')`,
  `store/clips.go:1085-1088`) never re-lists that clip; fetch/rehydrate only handle remote
  origins; GC won't warn (above); and the only caller of `SetClipArtworkImages` is the adoption
  job (`app/imageadapter.go:334-335`) — no route, no admin action, no CLI. So "clear the hashes
  and re-run adoption" is not an available repair today. Add the re-derive path or accept that
  extracted artwork is unrecoverable and classify it honestly.
- ⚠ **`ImageDTO.Src` is the `<picture>`'s required fallback `<img src>`, and it is JPEG**
  (`api/images.go:265` → `image.tsx:123`). Any change that returns `ErrNotFound` for "the other
  formats" must not include `Src`: if that 404s, `image.tsx:136`'s `onError` sets `failed` and
  clip-card's `fallback={null}` makes the hover layer **vanish**. Browsers take the WebP
  `<source>` first so it survives in practice — but `<picture>` commits by type, so this is
  exactly the trap that turns a missing format into a broken image rather than a fallback.
- **`images.dir` accepts a relative path with no guard** (no `filepath.Abs`/`IsAbs` in
  `internal/images` or `app/imageadapter.go`; the default is the absolute `/data/images`,
  `declared.go:323`). Make a missing or relative images dir fail loudly at boot — per decision 3.
- **`RoleThumb` reuses the backdrop ladder `{300, 780, 1280}`** (`images/ladder.go:41`) while
  clip artwork is rendered 320px wide (`filler/artwork.go:66`) and `Resize` refuses to upscale,
  so the srcset advertises 780/1280 candidates that can never exist.
- Two WebP decoders register the same `image.RegisterFormat` magic (`internal/images/codec.go:24-26`:
  `golang.org/x/image/webp` and `github.com/gen2brain/webp`) — which one `image.Decode` picks
  depends on init order. Resolve to one.

### D2 — Composite/compilation hierarchy
The lineage model is complete and conformance-tested and the frontend uses **none** of it:
`Clip.IsComposite` + `ParentHash` + `IsSegment()` (`filler/clip.go:246,255,259`) over columns
from migration `00039` in both dialects; `isComposite`/`parentHash` are on `ClipDTO`
(`api/filler.go:249,253`) and in the committed spec (`openapi.yaml:787,810`); `parentHash` is
even a query parameter. `TopLevelOnly`/`IncludeComposites` are live; the catalog sends neither
(`filler-page.tsx:168-175`).

⚠ **Hierarchy is NOT corrective for counts** — this changes the framing from the original ask.
Composites are already excluded from `PoolDTO.clips/commercials/eligible`
(`internal/app/filler.go:397`), so there is no double-counting to fix. It is a provenance and
navigation feature, which is a weaker justification and should be scoped accordingly.

⚠ **A confirmed composite is currently unreachable in the UI** — not a catalog row (params never
sent), and Confirm deletes the proposal that put it in Incoming's reels
(`splitjob.go:432` vs `fillerincoming.go:330-347`).

Docs: §10 :2532-2542 asserts "the catalog listing now asks for composites as containers and
hides the segments beneath them" — false, and it is the source of truth. Also §10 :1592 says
`parent_hash` is nullable; migration `00039` says otherwise.

### D3 — Pagination: one of five surfaces is done
`GET /v1/filler` **is** paged end-to-end (`limit` default 100 / max 500, `offset`, `total`
counted through the same `clipWhere`; FE `CATALOG_PAGE_SIZE = 60` — confirmed live as
`/v1/filler?limit=60`). V51d scoped itself there deliberately. The other four are unbounded:

- `GET /v1/filler/incoming` — confirmed live sending **no limit** and returning 78 asks + 38
  compilations. Split the audit halves out rather than paginating the envelope: bound
  `recentlyFiled` server-side the way `rejected` already is, and give each truncating list an
  honest `total`. ⚠ `recentlyFiled` is unbounded **and monotonic** (auto-filing is on by
  default, cleared only by a per-clip human decision) — the worst of the five.
- The two 100-row caps in `fillerincoming.go` are untested and invisible: no `limit`/`100`
  reference in `fillerincoming_test.go`, and no `total` on the wire, so a truncated feed reads
  as a complete one.
- `sort`/`order` exist end-to-end with the closed enum pinned by conformance, and **no UI
  reaches them** — parked by V51d alongside the composite row.

⚠ SSE amplifies the unbounded read exactly when it hurts: every terminal `filler_clip` frame
invalidates the whole `/v1/filler` prefix (`core/src/events/events.ts:129`).

⚠ **The unbounded incoming read fires on EVERY Filler tab, not just Incoming** (verifier's catch,
and it raises this item's priority): `filler-page.tsx:218` runs
`useFillerIncoming({ query: { enabled: isAdmin } })` purely to feed the tab badge (`incomingTotal`,
consumed at `:518`). TanStack dedupes it with IncomingTab's copy, so it is one request — but it
means any admin opening `/filler` (Catalog) or `/filler/sources` pays the full unbounded read with
no Incoming UI mounted, and the SSE prefix invalidation refetches it there too. Corroborated live:
`/v1/filler/incoming` was requested on both the Catalog and Sources tabs.

⚠ `PAGE_CAP = 25` (`source-search.tsx:23`) is a **hand-copied third mirror** of one number — the
server clamp `maxDiscoverRows` (`clipfetch/discover.go:62`) and `maximum:"25"` (`filler.go:707`)
are the other two. Raising the server cap silently breaks the "Showing 25 of 54 matches" message.
Note also that discover's truncation disclosure (`source-search.tsx:69,:146`) is already the
honest-truncation pattern this plan prescribes elsewhere — it is honest-but-unreachable, so do not
"fix" it by removing `total`.

### D4 — Incoming toolbars and counts
Two control rows confirmed live: "Auto-filing / Tune" and "78 clips need a decision / File all
as suggested". Merge without losing a control. The header stat says **43 waiting** while the tab
badge says **116** (= 78 + 38): `/v1/filler/watch` returns `held: 43` over a different
population than the incoming envelope. Pick one definition of "waiting" and use it in both
places.

---

## Phase E — Break length (`filler.break_duration`)

Per decision 4, and per the corrected mechanism: the enforcement plumbing is complete. The work
is four steps plus the ceiling change.

1. Declare `filler.break_duration`, `KindDuration`, default `5m`, `GroupFiller`, in
   `internal/settings/declared.go` beside `filler.breaks_per_hour` (:779) and `filler.pod_max`
   (:784). ⚠ `0s` must **not** mean "off" — `filler.breaks_per_hour = 0` already means that, and
   two knobs spelling "off" is the ambiguity §15 exists to avoid.
2. Resolve it per call and use it at `lineup.go:503` (replacing the `breakGapMs` constant), and as
   the floor input to `windowFor` (`adapter.go:184-190`) per decision 6.
3. Per-channel override mirroring `OperatorPolicy.BreaksPerHour`. ⚠ That field already ships to
   the wire (`channelPolicy.ts:22`) **with no UI** — so build its control in the same phase or
   you add a second orphan.
4. `pod_max` becomes a ceiling that yields to the duration target (a 5m break at ~30s/clip needs
   ~10 clips; `pod_max = 4` would cap it near 2m).
5. Add the test that does not exist in either direction: nothing anywhere asserts
   `pod.TotalMs <= w.GapMs`. `TestProp_Density` (`pod_property_test.go:229`) asserts clip COUNT
   vs `PodMax` and never mentions `TotalMs`.

⚠ **`filler.breaks_per_hour` is restart-only and #242 did NOT fix it** — `app.go:347` reads it
into `channels.Config` (`engine.go:122`), captured once at `New()` (`engine.go:149`). A new
duration knob must not inherit that; resolve per call, as #242 did for the pod policy.

⚠ **Keep the 30s floor but fix its stated reason before it lands in §10/§15 as a Tunarr fact.**
Tunarr rejects duration **≤ 0** ("expected number to be >0", `programmer/lineup.go:146-152`) —
the live-smoke 400 was duration 0. 30s is Loomarr's own conservative floor. The real hazard:
`flexItem` (`:169-172`) silently **clamps** anything below 30s up to 30s, so a sub-30s configured
break would not error — Tunarr channels would quietly play 30s while internal playout honoured
the configured value, i.e. the two backends disagree per channel.

### The two gap constants — corrected, and decision 6

⚠ **An earlier draft of this plan (and the audit) called `poolGapMs` vs `breakGapMs` "the actual
behavioural defect". That was wrong.** The two are deliberately different roles, documented as such:

- `breakGapMs = 120_000` (`schedule/lineup.go:473`) is the **timeline slot** — the placeholder
  duration of the `Slot{Kind: SlotFiller}` that `interleaveBreaks` inserts (`:503`). **This is what
  "how much filler is inserted" means, and it is the constant the new knob replaces.**
- `poolGapMs = 600_000` (`filler/adapter.go:82`) is the **pool budget**, and its generosity is
  intentional: "A channel's filler-list is a POOL Tunarr draws from, not a single sized break, so we
  assemble a generous pool (~one long break's worth); Tunarr picks per gap" (`:79-81`). It is
  consumed as `GapMs: poolGapMs` in `windowFor` (`:184-190`) — note this, not `adapter.go:153`.

So a pool larger than a break is correct by design, and `playoutadapter.go:721-724`'s offline card is
for the **opposite** case, which its own comment states: "The pod is shorter than the break gap.
Real: a 30s break with 20s of clips."

**Decision 6: `poolGapMs` becomes a floor.** `Window.GapMs = max(poolGapMs, resolvedBreakDuration)`
in `windowFor`. Today's pool sizing is unchanged for any break ≤ 10m (so 5m is a no-op), and it stays
correct for any value an operator sets — no arbitrary ceiling on a user-facing knob, and no break
length that silently degrades to the offline card.

Docs: §10 :2700 density bullet at :2706 already promises "target break length" and names
`fillPods`, a function absent from the tree — so this phase *fulfils* an existing doc promise
rather than fighting it. `fillPods` also appears at `design.md:3680`; correcting only :2706
leaves :3680 asserting a function that does not exist. §15 table at :3395. There is **no**
existing section owning the setting→scheduler propagation edge, so that text is new (not an
extension of `config-design.md` §3, which is resolution semantics).

---

## Phase F — dHash persistence (replaces the vector item)

Per decision 2. Justification is performance and reuse, **not** building dedup — dedup exists.

1. Add a `dhash` column, both dialects, forward-only. ⚠ Next free number is `00050`
   (`00049_drop_channel_icons.sql` is head in both) — but **check other live branches for a
   colliding `00050` first**; two long-lived branches each taking "the next free number" is a
   recorded failure here, and goose skips the second silently.
2. Populate it as a `StageDhash` rung, **not a cron job**. ⚠ The four filler jobs were retired
   in V51b and replaced by one driver plus a per-clip stage ladder
   (`StageOrder = {probe, transcode, split, language, transcribe, tag, vision, score}`,
   `pipelinestate.go:40-43`; `Stage` interface at `pipeline.go:54-66`). Anything written as a
   cron sibling of `filler-reindex` targets an architecture that no longer exists — and both
   `design.md:1727-1737` and `reindexjob.go:34-36` still describe it that way, which is doc drift
   to fix in the same PR.
3. Have `(*Splitter).dedup` read the column instead of re-decoding every catalog clip per run.
4. Expose it at ingest ("already have this advert?"), the use the split flow cannot serve.

**Also delete §10's false claims** (decision 2): :1751 asserts "§14 records them" for
sqlite-vec/pgvector/nomic-embed-text — §14 records none; :1821-1822 lists `filler.embed.enabled`
and `filler.embed.model` as "§15-declared, config-docs regenerated" — neither exists. ⚠ Record
that **sqlite-vec is impossible here**: it is a C SQLite extension and the build is
`CGO_ENABLED=0` (`Makefile:128`, `Dockerfile:50`) on `modernc.org/sqlite`, which cannot dlopen
one; pgvector would break the conformance suite (`postgres:16-alpine`, no pgvector). Leave that
finding in the doc so the question is not reopened from scratch.

Do **not** add `filler.split.autoconfirm_confidence` — the name is absent but the capability is
wired via `AutoSplitPolicy.MinConfidence` ← `filler.autofile.min_confidence` (`app.go:874`).

---

## Phase G — Accessibility sweep

Baseline is genuinely good; these are specific and mostly small.

1. `pod-timeline.tsx:85` — add an `sr-only` span per `<li>` carrying name + duration, copying
   `clip-pipeline.tsx:123`. Keep the `title`. **The only finding where a whole class of user gets
   no route to the information at all.**
2. `filler-page.tsx:688-700` — a `role="radiogroup"`/`role="radio"` grid-vs-list toggle with no
   arrow-key handling. ⚠ The verifier flags that the repo has **two** idioms here, so confirm
   which applies before rebuilding as `aria-pressed` toggles; do not treat `tune-panel.tsx:100-130`
   as settled precedent.
3. Orphan tabpanels / the `role=tab` question — the original claim that no `role="tab"` exists
   anywhere was **incomplete**; re-grep before acting.
4. The app has no global polite announcer despite `docs/frontend-design.md` §5.3 prescribing one;
   each filler surface owns its own (`incoming-panel.tsx:229`, `filler-page.tsx:838`).
5. Two independent `FieldLabel` components with identical signatures
   (`filler-criteria.tsx:17` and `channel-policy-fields.tsx`), both able to emit an orphan
   `<label>`. ⚠ **The durable guard is a disabled lint rule**: `web/biome.json` sets
   `"a11y": { "noLabelWithoutControl": "off" }` — the exact rule that catches this. Turning it on
   is worth more than the two fixes.
6. `clip-card.tsx:236,:330` and `clip-row.tsx:32` hand-roll `<input type="checkbox">` instead of
   the `Checkbox` primitive. Functionally fine (all carry `aria-label`); fold in opportunistically.

---

## Phase H — Doc reconciliation (§10)

§10 is ~1,570 lines (`design.md:1171-2739`) and is a stratified record, not a spec of today:
nine paragraphs describe a state the code left behind, and in **five** places §10 contradicts
itself because a revision was appended rather than the superseded text edited.

⚠ **Start with the fifth one — it is the root, and the verification pass found it.**
`design.md:2738` (§10 "Config") still reads: "Core: `FILLER_DIR` (**the drop-folder path Loomarr
registers as a Tunarr `local` media source** …)". That contradicts `§10:1186` in the same section
("Loomarr scans **`FILLER_DIR` itself**") and `§15:3393` ("⚠ **V38c: this is the CLIP FOLDER**").
`help/filler.md:10`, `troubleshooting.md:97-99` and `config-design.md:190` are all downstream
paraphrases of :2738 — so rewriting those three while leaving :2738 intact fixes nothing, because
`docs/design.md` is the source of truth they will be re-derived from.

⚠ When editing `docs/help/filler.md`, re-grep the line numbers: every anchor in the audit is off
by 1–3 lines ("the folder Loomarr watches" is :10 not :7; the tuning list is :45-48 not :44),
while `troubleshooting.md:95,:97` **are** exact — so an inconsistent set will land a maintainer on
blank lines. The content of the claims is correct in all cases.

⚠ One severity correction, so this does not ship overstated: `help/filler.md:40` ("Untagged
commercials still play but only match broadly") is wrong about the **mechanism** — untagged clips
are excluded outright, not matched broadly (`admitsUngroundedAudience`, `ladder.go:280-282`;
`filterAudienceWithUngrounded`, `:294-305`) — and omits that this is a deliberate safety allowlist.
Its outcome clause ("may fall back to just bumpers") is *correct* for a kids/family channel. It is
still the right top rewrite, but it does not "tell a parent the opposite of what happens".

Use the mechanism §10 already uses successfully at :2013 — strike-through plus a ⚠ REVERSED
pointer — rather than deleting: mark :1926 superseded by V38/V51a; mark :1873-1883 superseded by
V42 and **delete the −16 LUFS figure outright** (a second loudness target is the double-processing
trap :1340 forbids); mark :2020 superseded by V38c; add a forward pointer from the :1204 field
list to V45a.

Each earlier phase carries its own doc edits; this phase is what is left over.

---

## Loose ends worth filing as issues, not phases

- **The `filler` reachability check has no probe** and is permanently red on every default
  install: `connectionChecklist` registers it (`api/setup.go:66`) and skips it only when
  `filler.dir == ""` (`:108`), which cannot happen.
- **`FetchResult.StoppedBy` is discarded by its only caller** (`app/fillerjob.go:44`,
  `_, err := f.Run(ctx)`) — it is the only mechanism §10:1253 offers for "a limit that is reached
  is REPORTED, never silent".
- **`filler.starter_collection` is declared-but-unconsumed** (`declared.go:944` is its only
  occurrence in `internal/`), while `design.md:1897` says it seeds a pull on a fresh install.
- **`filler.Pipeline.Rewind` is built and wired** (`app.go:1142`) and reachable from no endpoint
  and no UI.
- **Held clips are unreachable from the Catalog tab** — `ListClips` excludes them unless
  `IncludeHeld`/`HeldOnly` is passed and the FE passes neither, so the one existing preview
  surface structurally cannot show an incoming clip.
- **Taxonomy write path is unreachable**: `POST /v1/taxonomy`, `PUT`/`DELETE /v1/taxonomy/{slug}`
  have zero FE callers, while `internal/app/fillerjob.go:88` tells operators "Runs after you edit
  the tag vocabulary yourself" — the Tasks page instructs use of a UI that does not exist.
- **`sync-filler` and `fetch-filler-source` are the same operation** (both call
  `s.filler.Sync(ctx)` and return the same shape) under two operationIds. Only `sync-filler`'s
  Summary (`filler.go:53`) and its generic 502 detail (`filler.go:614`) are stale — the disabled
  -source handling is already shared via `errSourceDisabled`, called from both.
  ⚠ **If retiring the route, retire the route — do not delete `filler.Tagger`.** `tagjob.go:220`
  delegates to `TagStage` and loops it ("ONE implementation of per-clip tagging, shared with the
  ingest pipeline's tag rung"), `TagStage` has **no** direct tests, and `tag_test.go` constructs
  `NewTagger` 18 times (enum-hallucination dropping, bad-year rejection, sidecar reads,
  transcript-as-third-signal, brand grounding). Deleting `Tagger` would delete the only test
  coverage of a live pipeline rung.
- **`ClipDTO.brand` has zero FE readers** — arguably the highest-value unwired field, since brand
  is what pod separation reasons about. The V44 grounded-advertiser ladder (text → transcript →
  vision) therefore reaches no surface at all: `clip-card` takes the whole `ClipDTO` and renders
  era/audience/category/confidence/license/playCount/language/suggestedEra/thumbImage/hoverImage/held
  — never `brand`. Same for `PodEntryDTO.brand`/`.category`/`.visibleText`
  (`channelpreview.go:45,47,51`).
- **The filler-pull audit trail is unreachable** while two comments claim otherwise
  (`fillerpulls.go:189-194`).
- **Issues #238 and #239 are open but already fixed** in the tree by PR #240 (`4dfb5d26`) — close
  them.
- **PROGRESS.md:1244 was falsified by `fc258a82`/#243**: `hashes` and discover-stats `id` now
  carry `,explode`, so "comma-separated on the wire, not repeated" is no longer true.

---

## Verification

Per phase: `make check` + `make fe` + `make openapi-verify` + `make retired-verify`, and
`make test-pg` for Phase F (it adds a column, so both dialects must pass one suite).

⚠ Do **not** run `make fe-visual` or `make e2e` locally — CI owns those gates. If a baseline
genuinely must change, `rm -rf storybook-static` first, because the target will otherwise
validate a stale build.

**Live verification is the real gate** — green tests are not a look:

1. Fix `IMAGES_DIR` (Phase D1) before judging anything visually, or every image is a placeholder.
2. Drive `:5173` in a browser, never `:8080` (the embedded SPA there is stale).
3. Per phase, confirm in the browser: A1 by attempting `?redirect=https://evil.example` while
   signed in; A2 by clicking the incoming buttons; A3 by dismissing a clip and reloading;
   A5 by round-tripping a proposal and diffing boundaries; A6 by watching a non-transcode stage
   run; B by collapsing a provider and reading both count summaries; C by seeking the video to a
   segment boundary; E by setting 5m and reading the resulting pod duration in the channel
   filler tab.
4. For every fix that turns a green test green, **sabotage the code and confirm the test goes
   red.** A first-try pass on this class of defect is suspect — several of the tests above are
   currently green over states production cannot reach.

---

## Provenance

Every claim above was produced by a 12-area audit and independently re-verified by a second
agent per area. The verification pass **overturned a substantive claim in 4 of the first 6
areas**, always in the same direction: a false negative asserting something was not built when
it was. Treat any unverified claim accordingly.

**All 12 areas verified** (24 agents, 0 errors, ~1,700 tool calls). Every correction they produced
is folded into the phases above.

The corrections that mattered most, as a warning about how to read anything not yet re-checked:

| Reported as | Actually |
| --- | --- |
| approve path "wired" | broken the same way dismiss is — **3 of 4 buttons don't stick** (A3) |
| no pod duration mechanism | exists and is enforced; fed the wrong number (Phase E) |
| no near-duplicate detection | exists end-to-end; hashes just aren't persisted (Phase F) |
| no frame-at-timestamp capability | exists (`FFmpegArtwork` `-ss`); no HTTP route exposes it (Phase C) |
| operator has no signal for missing bytes | signal exists; its predicate excludes this class (D1) |
| `DialogContent` max-w override has precedent | no surface in the tree does it (Phase C) |
| fresh install shows 2 phantom sources | it seeds 3 collections; 6 rows is correct (Phase B) |

In every single case the first pass **understated what was already built**. When implementing, treat
"X does not exist" as a claim to re-grep, not a fact — that is the one error class this audit
produced repeatedly, and it is the expensive one, because it sends you building a duplicate.

The one place the first pass *over*stated: `help/filler.md:40` was called a safety inversion that
"tells a parent the opposite of what happens". It is a mechanism error and an omission, not an
inversion (Phase H).

Browser-confirmed first-hand (not agent-reported): the login redirect URL and that it is
honoured; zero video/img on the split review page and that clicking a filmstrip block only moves
focus; the raw 64-char hash as the review page's heading; every `/v1/images/**` request failing
`ERR_BLOCKED_BY_ORB` against a 404 `text/plain` + `nosniff` body; `43 waiting` vs `116` vs
`78 + 38`; `4 of 4 sources on` vs `5 of 6 on`; `group`/`parentId` present on
`GET /v1/filler/sources`; `/v1/filler?limit=60` paged while `/v1/filler/incoming` is unbounded;
"Split into clips" offered on 15s clips; segment gaps and sub-10s segments in a live proposal.
