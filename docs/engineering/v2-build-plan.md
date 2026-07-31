# v2 build plan

**Status:** ready to execute. All decisions taken 2026-07-24.
**Verified against:** `55fc691`. Re-verify before acting — the repo moves.
**Research & evidence:** `v2-mock-delta-2026-07-24.md`. Every claim here is grounded there with
file:line. This document is the *what to do*; that one is the *why*.

---

## 1. What this program is

The v2 mocks (`design/loomarr-prototype-{desktop,mobile}-v2.dc.html`) reshape Loomarr's IA and add
several new surfaces. Both mocks were read in full — markup **and** the 190 KB state block.

**The single largest change is not visual.** Decision **D-I** amends `design.md`'s opening claim:
Loomarr stops being *"not a transcoder/streamer"* and grows into one. That reframes Track T from a
parallel track into the spine of the roadmap, and it is why the docs-only phase **V2b blocks 28 of
the 39 phases below**.

Net-new surfaces: **Guide time-grid · Dashboard · transcode telemetry · restart control · IA renames
· ~20 registry keys (playout/backup/SSO) · a `playout_token` secret · filler sources + coverage +
discover · edit-before-approve · All-settings search · cross-tab save bar.**

---

## 2. Standing rules

1. **One phase per PR, doc-first.** The losing document is corrected in the same PR as the code.
   `design.md` wins on behavior; companions win on their domain.
2. **Gates are hard.** Never weaken a test to go green.
3. **Never weaken safety:** the approval gate (§7/§11), grounding (§8), the audience ceiling
   (fail-closed), forward-only migrations (§16).
4. **The prototype is not authoritative for structure or algorithms.** `design.md` §12 wins on IA;
   the shipped Go wins on behavior. The mock's JS reimplements the filler ladder, pod assembly,
   seeding and channel-health **incorrectly** — see research §2i.1. Never port logic from it.
5. **Surface map:** a PR adding a channel capability updates `design.md` §12 in the same PR.
6. **Help grep:** a PR retiring or renaming a capability greps `docs/help/` in the same PR.
7. **A backend phase that adds user-facing capability MUST name its UI phase.** "The endpoint
   exists" is not the gate — **reachability is**. A phase may legitimately ship backend-only, but
   then it says so in its own row and points at the phase that finishes the job, and it does not
   claim to close a user-facing defect.

   *Learned the hard way, in this program:* V7 shipped `POST /v1/auth/password` with 19 tests and
   **no Account screen**, and its PR claimed to close S2 — while a user still could not change their
   password by clicking anything. That is the exact defect class the first eight phases existed to
   close (a complete feature with one missing link), recreated while closing it. V7b/V7c/V25b were
   added afterwards; this rule is so the next one is caught before merge, not after.
8. **No new dependencies** (§14). Generated files are regenerated, never hand-edited.

---

## 3. Decisions (all ratified 2026-07-24)

| # | Decision | Outcome |
| --- | --- | --- |
| #3/#4/#5 | IA renames | **RATIFIED** — `/people`, `System`, `Security`, `All settings`, `Defaults` |
| — | Channels → **Guide**, + **Dashboard** landing | **RATIFIED** |
| #6 | Bootstrap-file config tier (`env > file > default`) | **ADOPTED** — amends `config-design.md:72` |
| #7 | Local account management | **IN v1** — closes S3; **S2 needs the UI too** (V7 backend + V7b screen) |
| #8 | Filler F3b live remote discovery | **v1** |
| #10 | Playout backend default | **Internal-first** |
| #11 | Playout transport | **Both** HLS + MPEG-TS |
| #12 | Guide advertises mid-roll breaks | **No** |
| #13 | Ad-break detection scope | **Mid-roll-enabled channels only** |
| #14 | `/help` landing page | **No landing page** — opens into Troubleshooting, wrapped in a two-rail reader |
| D-A | Dashboard | **ALL IN**, including live transcode telemetry |
| D-B | Wizard: Database at step 1 | **YES** (follows #6) |
| D-C | All wizard steps required | **NO** — Users stays `optional: true` |
| D-D | Guide rename vs time-grid | **BUNDLED** — one PR (V14) |
| D-E | Image variants | **ONE APP** — no slim/full split |
| D-E2′ | ffmpeg in core | **FOLD — one 549 MB image**; supersedes the `design.md:795` two-tag model |
| **D-I** | **Internal playout vs §1** | **AMEND THE DESIGN — Loomarr becomes a streamer** |
| D-F | SSO | **YES, but NO auto-create** — allowlist invariant preserved |
| D-G | ~20 new registry keys | **ONE §15 amendment up front** (V4) |
| D-H | `ChannelIconField` unmounted | **MOUNT IT** (V1) |
| D-J | Guide grid data | **NEW BACKEND PHASE** (V13b) — V14 is not frontend-only |
| D-K | Edit-before-approve | **BUILD IT, one chokepoint** — `suggest.Approve` stays sole |

**Cut deliberately:** the wizard's `IMAGE GATE` copy (no slim/full); SSO `auto_create` and
`admin_group` (they re-open the lazy-provision hatch §11 closed); the vestigial `helpGridVisible`
card grid.

---

## 4. The phases

42 phases, one PR each. `make check` · `make fe` · `make e2e` green is assumed throughout; the
**Gate** column is the *additional* proof. Dependency graph verified mechanically: acyclic, no
dangling references, every register defect mapped.

**Every v2 mock surface has a phase.** Guide → V14 (grid) + **V13b** (the endpoint it needs);
Dashboard → V16; People → V14 + V7c; Settings → V9/V10/V11/V12/V13/V8; Account → **V7b**;
Help → V15; Approvals → V27 + **V25b**; My requests → V26; Filler → V17b–d/V28–30/V33;
Wizard → V20/V21/V22; Mobile → V18.

### Free to start now — no dependencies, no pending decisions

| # | Phase | Gate |
| --- | --- | --- |
| **V0** | `S1` — `DATABASE_URL` `envDefault` | Zero-env boot resolves a store path; `docker run` with no `-e` reaches the wizard |
| **V1** | `D-H` — mount `ChannelIconField` on the info panel | Story + visual baseline; §12 surface-map row cites the mount |
| **V2** | `C3′` + `C10` + `A4` — Separation in the refine diff; drop the dead `channel-pods` export; delete the stale `TODO(learning)` at `ladder.go:57` | A refine test where **only** `separation` changes renders it; story-coverage green; no TODO above an implemented function |
| **V2b** | **`D-I` — the design amendment, DOCS ONLY** | See §5 below. **Blocks 28 phases.** |
| **V7** | `#7` — local accounts **backend** (closes **S3**; **S2 only at the API layer** — see V7b) | The bootstrap admin changes their own password *via the API*; §19 negatives extended |
| **V7b** | **Account screen** — change password, session list, revoke, sign out everywhere. **Closes S2 for real.** | A user changes their password by clicking; a reachability assertion proves the screen mounts; the copy says **all** sessions end, not "this one kept" (the code revokes every session — see `password.go`) |
| **V7c** | **People: create local account + reset password** — the admin half of V7's surface | An admin mints a local account and resets a forgotten password from the UI; a member sees neither affordance |
| **V17a** | `F1` — read the sidecars; stop passing the provenance enum as the LLM's source description | A tagging test asserts the prompt carries real metadata, **not** the literal `"tunarr-local"` |
| **V19** | Per-title refine rationale (`why`) | **No backend work needed** — `ProposalItem.Rationale` already exists and the LLM already populates it; `diffLineup` dropped it building `DiffRow`. The diff renders it on ADDED rows only |
| **V23** | `A1` — deny-reason UI (API field exists, unused) | Denying offers a reason; it reaches `Proposal.DenyReason`; no call site sends `data: {}` |
| **V24** | `A3` — surface existing-but-hidden proposal data | `channelName`, `eraBalance`, `overall` render; `mustInclude`/`mustExclude` get inputs |

### The spine — V2b → V3 → V4 → V5 → V6

| # | Phase | Deps | Gate |
| --- | --- | --- | --- |
| **V3** | `D-E2′` — single 549 MB image, doc-first | V2b | §16 amended in-PR; image builds; `USER nonroot:nonroot` asserted; `ffmpeg -version` **and `ffprobe -version`** succeed; `THIRD_PARTY_NOTICES.md` re-read |
| **V4** | `D-G` — §15 amendment: `playout.*` + `backup.*` + `playout_token` | V2b, V3 | §15 before `declared.go`; `make config-docs` diff empty; `server.public_url` re-scope-or-add decided here; log-grep redaction for `playout_token`; **`playout.backend` supports a per-channel override** |
| **V5** | `#6`/`D-B` — bootstrap-file config tier | V4 | Precedence tests across all three tiers; `config-design.md:72` amended |
| **V6a** | **T2 — the guide-path trap.** `StaleLoomarrListings` identified Loomarr's listings provider by its **Tunarr-shaped path**, so retargeting to internal playout would silently stop recognising it and orphan the provider a migration needs to remove — symptom: a deleted channel that keeps streaming. Now matches EITHER backend's shape | — | A Tunarr-shaped provider is stale once we retarget to internal playout, and vice versa; a token in the URL does not confuse the match; a provider Loomarr never wrote is never touched. Verified failing when the suffix is swapped rather than added |
| **V6** | Track T: Internal playout to first frame. **Verifiable HERE — the dev env has a live Emby (`100.75.125.45:8096`) and Tunarr (`:8000`), plus `make smoke-livetv`. Do not defer the gate as "needs the homelab".** Only the maintainer's own GPU encoders and TV-as-a-client are genuinely out of reach. **Read [`playout-prior-art.md`](playout-prior-art.md) first** — Tunarr (the system we're replacing, so its wire contract is the one Emby already accepts), ErsatzTV (an independent solution to the same problem), and viewra. **Tunarr and ErsatzTV disagree on the central mechanism**; the doc states the trade-off and recommends Tunarr's HTTP ffconcat loop for the first frame | V3, V4, V5 | A native channel in the media-server guide **playing a test card**; both transports; `StaleLoomarrListings` still cleans up after retargeting (**T2**); segment requests reject a wrong/absent `playout_token`. **Plus, from the prior art:** viewers are **reference-counted** (a second viewer must not evict the first); **client disconnect** is detected via `Request.Context().Done()`, not an idle sweep (a live encoder never exits — a leak burns a core forever); the **tuner endpoint is continuous MPEG-TS** with no `Content-Length`, no ranges, periodic `Flush()`; ffmpeg is killed by **process group**; hardware failures classify on **exit code, not substrings**; segment URIs **carry the token** (relative URIs in an ffmpeg playlist do not inherit it) |
| **V6b** | **XMLTV listings** — `GET /playout/guide.xml`, the second half of §9.1's "what internal playout serves". V6 shipped the M3U tuner and the streams; the guide was promised by the design doc and never scheduled, so a channel appears in the media server and plays with an **empty EPG**. Same wall-clock source as playout itself (`AiringAt` over `CyclePreview`), so what the guide advertises is what actually plays | V6 | A channel added to Emby's Live TV shows **real programme titles and times** in the guide, not "no information"; `channel@id` matches the M3U's `tvg-id` exactly (a mismatch is silent — the channel plays with no listings); `programme` entries span a configurable window and are XMLTV-DTD-valid; **breaks are NOT advertised** (§10: a break rendering as its own EPG entry is confusing, and empty breaks already caused exactly that); requests without a valid `playout_token` 404 like every other playout route |

### Settings & system

| # | Phase | Deps | Gate |
| --- | --- | --- | --- |
| **V9** | Settings IA — 6 tabs + System sub-tabs + cross-tab save bar | V4 | Dirty state survives tab switches; the four inline-commit exceptions verified |
| **V10** | All-settings search surface | V9 | Search matches key **and** group **and** value; `ADV` chip reflects `Setting.Advanced` |
| **V11** | System → Database migration stepper | V5, V9 | SQLite → PostgreSQL with **row-count parity**, rollback by reverting one config line; backup gate cannot be skipped |
| **V12** | System → Backup UI (**S6**) + About (**S7**) | V4, V9 | Backup downloads; retention honored; version renders from `GET /v1/system/version` |
| **V13** | Restart/reload control + service probes (**S5**) | V9 | `RestartRequired` implemented and surfaced; reload re-probes without downtime; restart confirm lists consequences |
| **V8** | `D-F` — SSO as a credential path, **no auto-create** | V4, V7 | §11 amended stating SSO does **not** provision; **an SSO identity with no allowlist row is rejected**; no `auto_create`/`admin_group` key exists |

### Guide & IA

| # | Phase | Deps | Gate |
| --- | --- | --- | --- |
| **V13b** | `D-J` — the guide endpoint: `GET /v1/guide?from=&to=`, multi-channel, **gaps preserved**, `kind` discriminator, per-airing pod composition, program metadata, timezone + retention | V6 | A window spanning past **and** future returns per-channel timelines; a pending slot and a filler pod are **distinguishable** (today `gap bool` cannot); `Upcoming`'s gap-filtering not reintroduced |
| **V14** | IA rename + Guide time-grid (**bundled**, D-D) | V13b | §12 updated **first**; `/guide` + `/people`; **two distinct navs** (admin 7; member 4 incl. `Request a channel` + `My requests`), not one filtered list; grid renders duration-scaled programs, pods, pending slots, now-line, zoom; baselines regenerated |
| **V15** | Help rebuild — two-rail reader, full-text search (closes ~~**H1**, **H2**~~, **H3**, **H4**, **H5**, **H6**) | V14 | ⚠ **H1/H2 are already closed, and the gate as written can never pass** (corrected 2026-07-26): `grep -ri "hooks/arr\|WEBHOOK_SECRET" docs/` returns 3 hits — `design.md` and the two v2 plan docs — but all three describe the retirement *historically*, which is the point of writing it down. The gate must scope to the **shipped** surface: `grep -ri "hooks/arr\|WEBHOOK_SECRET" docs/help/` returns nothing (verified), and `make retired-verify` is the standing guard (CLAUDE.md's retired-identifier rule, which exists *because* this exact webhook kept being documented as a live setup step). Remaining for this phase: search hits body text, not just titles (H3); channel-editing docs exist (H4); measure bounded by the two-rail layout (H5); links use `tune`, not the AI `suggest` colour (H6) |
| **V18** | Mobile responsive (**S11**) | V9, V14 | AppShell collapses; mobile v2 screens render at **390px**; desktop-only actions render as disabled affordances, not dead ends. ⚠ *Corrected 2026-07-26: this said 375px, but the visual harness's mobile project is pinned to 390×844 (`playwright.shared.ts:28`) and the committed `*-mobile-linux.png` baselines are that width. A gate naming a width the suite never renders cannot be checked as written — change the harness or the gate, deliberately, not by accident.* |

### Approvals & requests

| # | Phase | Deps | Gate |
| --- | --- | --- | --- |
| **V25** | `D-K` — edit-before-approve **backend**, one chokepoint | V4, V24 | `POST /approve` takes a body; **`suggest.Approve` remains the sole implementation** (a test asserts no second acquisition path); `modSummary`+`note`+`approvedBy` persist; member still 403s |
| **V25b** | **Edit-before-approve UI** — drop titles with `✕`, add via search, note to the requester | V25 | An admin edits a proposal and approves the edited version from the queue; the note reaches `modSummary`; a reachability assertion proves the panel mounts |
| **V26** | `A2` — "My requests": per-user proposal list + admin-edit provenance | V25 | A member sees their own submitted/denied/edited requests; *"CHANGED BY …"* renders; the denial line shows |
| **V27** | Approvals queue as its own surface: tabs, bulk approve, audit rows | V25, V26 | Tab counts correct; bulk approve goes through the same chokepoint; history rows carry `approvedAt` |

### Filler

⚠ **This table was CORRECTED against the code at `662e181` (2026-07-31)** before the remaining
phases were started. Four dependency declarations were wrong and one gate was unbuildable as
written; §6.2 records each correction and the evidence. The rows below are the corrected ones.

| # | Phase | Deps | Gate |
| --- | --- | --- | --- |
| **V17b** | `F2` — clip previews on `ClipCard` | V17a, **V30** | Preview renders; visual baseline; **stories use inline `data:` URIs, production fetches the V30 route** |
| **V17c** | `F3` — a quality dimension beyond `INGEST_PREFER_ORIGINAL` | V4, V17a | An **opt-in minimum-quality floor, default off**; with the floor off, selection is byte-identical to today (a test pins this). ⚠ **Amends a published contract** — see §6.2 |
| **V17d** | Starter pack + "Find clips" (**F4**) | V17b, **V29b** | Seeded clips reviewable with keep/exclude; a thin channel surfaces the gap. ⚠ Coverage is **V29b's**, not this phase's |
| **V28** | Sources **read-model** + clip metadata columns | V4, V17a | Migration `00017`; `GET /v1/filler/sources` derives the rows and `Fetch now` triggers the existing sync; `thumbnail` populated at scan; a play is recorded **from playout**, not from assembly |
| **V29a** | **Export a coverage entry point from `internal/filler`** | V28 | `ladder.go`'s pools become reachable without duplicating them; unit-tested against the same fixtures as pod assembly. **Go only — no endpoint, no UI** |
| **V29b-api** | `GET /v1/channels/{id}/filler/coverage` over V29a's export | V29a | The route returns the rungs V29a computes for a channel's own selection; **API lane**, because it edits `api/openapi.yaml` |
| **V29b** | Coverage meter (F2 banner) | V29b-api | **Consumes V29a's export through the V29b-api route** — a test asserts the meter and pod assembly agree, in the shape of `preview_test.go:28`. See §6 |
| **V30** | Filler preview serving | V6, V28 | Thumbnail bytes are reachable over HTTP; **the route is NOT named `preview`** — see §6.2 |
| **V33** | `#8` — F3b discovery: `GET /v1/filler/discover` (archive.org **and** YouTube) + **the persisted Sources registry** (V28 ships the read-model; V33 owns the table) | V28, **`internal/clipfetch`** | The Archive.org **and yt-dlp** contracts are **pinned testkit fixtures**; discovery never runs in unit tests. ⚠ **No licence badges — gate amended, see §6.3** |
| **V34** | **Compilation splitting + per-clip metadata** — one downloaded compilation becomes many tagged clips | V33 | See §6.4. Measured on 6 real compilations before being written; **not** a V33 deliverable |

### Dashboard & wizard

| # | Phase | Deps | Gate |
| --- | --- | --- | --- |
| **V16** | Dashboard incl. transcode telemetry (D-A) | V6, V13 | Live per-stream telemetry; member sees the lockout, not a 403 wall; **`restartFacts[0]` rewritten** — *"Tunarr streams them, not Loomarr"* is false after D-I |
| **V31** | Dashboard Services panel — aggregating wrapper + 30s poll | V13 | **Reuses `POST /v1/setup/test`** (a test asserts one probe implementation); "Fix →" routes to the failing block |
| **V32** | Dashboard Recent activity feed | V31 | A persisted feed (not just live SSE); survives restart |
| **V20** | Wizard **Database** step (step 1 per D-B) | V5, V11 | Choice persists across restart-and-resume; `setup/state` survives a DSN switch (it currently lives *in* the database being migrated) |
| **V21** | Wizard **Playout** step + transcode check | V6, V20 | Encodes a real 15s test pattern; progress streams over `/v1/events`; **`IMAGE GATE` copy absent**. Note `tcOptions`/`poPresets` were never authored in the mock — invented here, deliberately |
| **V22** | Wizard reconciliation: `bk.summary`, persisted `skipped`, strike the dead §20 bullet (**S12**) | V20, V21 | `skipped` survives a refresh; the §20 **"DB-backed settings UI"** bullet is struck. ⚠ *Corrected 2026-07-26: this cited `design.md:940`, which is now a row in §14's stack-decision table — §20 begins at `design.md:1333` and the doc has shifted repeatedly since. The bullet is identified by its TEXT rather than a line number so this cannot rot again; it is already marked "Resolved and superseded" in place, so the remaining work is removing it, not deciding it.* |

### Not scheduled — need a design decision first

**Two of the three are now closed** (2026-07-26), not by this plan but by the surface audit
(`surface-audit-2026-07-26.md`, PRs #88/#89) — which reached the same capabilities from the
"what has no door?" direction. Re-verify before trusting any row here; that is what found these.

- ~~`C5` — strategy/group have no UI, while `policy.ordering` offers "Inherit channel default",
  inheriting from an invisible field.~~ **CLOSED.** `strategy` now has a control beside Ordering
  (`channel-policy-fields.tsx`), which is exactly the value that phrase refers to. `group` remains
  editable via the identity fields.
- ~~`C6` — `autoCurate` live with zero frontend refs.~~ **CLOSED.** Built in Programming → When it
  changes. Note *why* it stayed orphaned so long: **the opt-in IS the object's presence**
  (`*AutoCurate`, nil = off), so there is no boolean for a generic field editor to bind to —
  nothing could construct it. §12 also claimed a home ("Settings → lifecycle") for a tab that was
  never built, so the reachability question answered *yes*.
- ~~`C8` — no hand-made channel create surface.~~ **CLOSED 2026-07-26 — API-ONLY BY DECISION.**
  The three honest options were *add the UI*, *remove the capability*, or *document it as
  API-only*; the third was chosen and is now recorded in §12's surface map, so the row reads
  "API-ONLY BY DECISION", not "ORPHANED". Removing it was off the table: §7 documents three
  origination seeds and the single-series path is implemented and tested, while §12:345 calls
  the seeds "express doors into the same object". The UI keeps exactly ONE origination door
  (describe → approve, now in the Guide header) and says so. The consequence stands and is
  recorded: `strategy` is a **required** field of `POST /v1/channels`, so it is unsettable at
  creation — consistent with there being no form, and a §7 default would be needed before any
  UI could offer one.

  ⚠ The claim that "the v2 mock shows no UI for C8" was right in substance and wrong in its
  evidence: the mock DOES contain the string "New channel", above a `<textarea>` and a
  `Suggest` button — it is the describe→approve path with a heading, not a create form. Reading
  the string without reading the markup produces the opposite conclusion.

---

## 4a. ✅ CLOSED — `design.md` no longer leads the code on playout

**V2b merged design amendments describing a system that did not exist yet.** §9.1 said Loomarr
serves HLS + MPEG-TS segments, publishes `/playout/tuner.m3u` and `/playout/guide.xml`, and
authenticates devices by `playout_token`. §1 listed playout as a goal. None of it was
implemented, and V4's registry keys had zero consumers.

That was the right sequencing — V2b unblocked 28 phases and doc-first is the rule — but it
opened a window where the doc was **intent, not description**. Recorded because that window is
exactly how `design.md`'s §20 accumulated dead bullets: a claim nobody tracked until it read as
history.

**Closed by V6 + V6b (both merged 2026-07-25), verified on the maintainer's own Emby.** Every
§9.1 claim now has a live consumer: a real Emby tuner pulls `/playout/tuner.m3u`, real library
films play with the full transcode on the GPU, breaks cut to real commercials, and
`/playout/guide.xml` serves real listings (7 channels / 252 programmes on the dev stack).

**⚠ The lesson, kept because it will recur.** The XMLTV guide went unnoticed through V6 because
that phase's gate said *"a channel playing a test card"* — which the M3U tuner alone satisfies.
A doc claim only gets tracked if some phase's gate actually asserts it. When a phase implements
part of a multi-part design promise, its gate should name the parts it does **not** cover, or
the remainder becomes exactly the dead bullet this section exists to prevent.

*Also worth separating, because the word "guide" means two different things:* V13b's
`GET /v1/guide` is JSON for **Loomarr's own** time-grid UI (V14). It does **not** put listings
in a media server's EPG — that is V6b's XMLTV file, a different format for a different consumer.
V6b shipped `playout.BroadcastsBetween`, which already answers "what airs when, per channel,
over a window", so V13b is now mostly a JSON projection of an existing walk.

## 5. V2b in detail — the highest-leverage phase

Docs only. No code. **28 of 39 phases sit behind it.**

| Section | Amendment |
| --- | --- |
| **§1** | Strike/rewrite *"Not a transcoder/streamer"* (`:39`). State what Tunarr remains for. |
| **§10** | Mid-roll moves from out-of-scope (`:530`) to in scope; strike the §20 open question (`:942`). |
| **§14** | `ffmpeg` becomes a core runtime dependency; **`ffprobe` is re-added** (`:727`/`:795`'s "never probes media" no longer holds). |
| **§16** | Single 549 MB image; the two-tag model (`:795`) and the sidecar rationale (`:505`) are both superseded. |
| **§15** | The `playout.*` group + the `playout_token` secret. |
| **§6/§9** | Loomarr's own M3U + XMLTV output — `StaleLoomarrListings` currently identifies Loomarr's provider by its **Tunarr-shaped path** (**T2**). |
| **§7.1/§11** | Segment routes are **token-authenticated, not session-authenticated** (a TV holds no session cookie). Design this alongside D-F's SSO path, not separately — §11 currently has exactly one authorization path. |

**Write the rationale, not just the change.** Four decisions in this program reverse documented
design (D-I, D-E2′, D-F, D-K). `design.md:505` shows what happens otherwise: the sidecar question was
settled once with a recorded rationale, and is now being reversed again. Without the *why*, the next
reader re-litigates.

---

## 6. Two gates that are not negotiable

**V29 — Coverage must consume the real ladder.** The mock's Coverage meter is attributed *"from the
same ladder reconcile uses"* and that claim is **false in the mock**: it recomputes its buckets
inline, and the prototype contains five mutually inconsistent era/audience predicates. It also renders
catalog-composition shares as *"Breaks resolve exactly N% of the time"*. Building it as drawn ships a
meter that disagrees with reality — the exact bug §10's shared assembler exists to prevent.

⚠ This is why V29 is **two** phases (§6.2). `ladder.go` exports nothing today, so "consume the real
ladder" had no API behind it — and a phase whose gate cannot be satisfied is a phase that gets
satisfied some other way. V29a exports the entry point; V29b is the only thing allowed to draw the
meter, and only through that export.

**V25 — one approval chokepoint.** `suggest.Approve` is the single shared implementation *"so the two
can never disagree about what approving means"*. Edit-before-approve passes it an edited proposal; it
does **not** get a second path. The edit is a provenance change, not an authorization change.

---

## 6.1 Filler: what V28 corrected, and the two overlaps still open

V28's row was written before `00013`–`00016` landed and had gone stale in three ways. Recorded
here rather than silently edited, because each correction is a claim about what is true:

- **Migration number.** The row said `00013`; that number was taken by `clips_path_identity`.
  V28 is `00017`.
- **`quality` was already shipped** by `00014_clips_quality`, whose comment is explicit that
  quality is **display-only** and must never affect pod selection — *"a well-meaning 'prefer HD'
  would quietly starve the era-accurate 4:3 commercials the whole feature exists to play."*
- **`sources` is a READ-MODEL, not a table** (maintainer decision). Filler discovery is driven by
  `filler.dir` plus the media-server library scan; a `sources` table would be a second source of
  truth needing a precedence rule against the setting. `GET /v1/filler/sources` derives the mock's
  three rows from the config that already exists plus live per-source clip counts. **V33 owns the
  persisted registry**, when remote sources genuinely need rows.

  ⚠ **V33 DOES NOT NEED THAT PRECEDENCE RULE (maintainer decision, 2026-07-31): its table holds
  REMOTE SOURCES ONLY, and they nest under the read-model's `remote` row.** The folder and library
  rows stay derived from config, so `filler.dir` remains the only thing that says where the folder
  is, and a row and a setting never describe the same source.

  ⚠ **An earlier version of this note said the opposite** — "the TABLE WINS, and `filler.dir` SEEDS
  it" — and the migration carried a `seeded_from` column to make the resulting inertness visible.
  That was decided before reading `fillersources.go`, which turned out to return **three FIXED rows
  describing CONFIGURATION** rather than a list of things that exist. The two models do not merge:
  a table of rows cannot express *"you could set up a library but have not"*, which is exactly what
  `configured:false` is for, and forcing them together would have shown one source twice. The
  seeding model and its column were dropped before either shipped.
- **`usage` had no honest write point.** Pod assembly takes a `used` map but `adapter.go` passes a
  fresh empty one per call — it is per-pod de-duplication with no memory — and pods re-assemble on
  every 10m reconcile sweep, so counting at assembly would inflate without bound and count
  *scheduled*, not *aired*. A play is therefore recorded from **playout**, where a filler `Airing`
  actually starts. ⚠ Consequence to state plainly: only internal playout can report this, so
  **Tunarr-backed channels read zero** — the UI must say "not counted" there rather than "0 plays",
  or it reports a wrong number on half of installs.

**Two overlaps are still unresolved** (flagged, deliberately not decided here):

| Overlap | The problem |
| --- | --- |
| **V17b vs V30** | V17b is "clip previews on `ClipCard`" (frontend); V30 is "filler preview serving" (backend). You cannot render a preview without something serving it, so either V17b is blocked on V30 and the deps are wrong, or V17b is thumbnails-only. V28's `thumbnail` column is the thing that makes the thumbnails-only reading buildable. |
| **V17d vs V29** | V17d is "coverage / Find clips"; V29 is "the coverage meter". §6 makes V29 non-negotiable — the meter MUST consume `internal/filler/ladder.go`. If V17d ships a coverage UI first it either duplicates that or ships the mock's lying version. |

→ **Both are now decided. See §6.2.**

**V17c's gate is amended** (maintainer decision): quality becomes selectable via an **opt-in
minimum-quality floor**, default off. That preserves `00014`'s era-accuracy default — an install
that sets nothing behaves exactly as today — while letting an operator exclude 240p rips. The knob
is a §15 conversation when V17c is built, not a V28 deliverable.

## 6.2 The remaining Filler phases, corrected against the code (2026-07-31)

Before starting the six unbuilt Filler phases, each was verified against `662e181` rather than
trusted. §6.1 exists because V28's row was stale in three ways; this is the same check applied to
what is left, and it found four wrong dependency declarations and one unbuildable gate.

**V29 is unbuildable as written, and is now two phases.** Its gate says the meter "**consumes
`internal/filler/ladder.go`**" — but every symbol in that file is unexported (`pool`,
`candidatePools`, `fillCommercials`, `filterEra`, `filterDecade`, `filterAudience`, `filterKinds`,
`filterCategories`, `durationEligible`, …). There is no API to consume. The only exported
ladder-derived artifact is `Pod.MatchLevel` (`pod.go:40`), which is a per-pod outcome, not a
catalog-wide coverage answer. So **V29a** exports a coverage entry point over the same pools and
**V29b** builds the banner on it. This also corrects the label: V29 is not the frontend-only phase
"(F2 banner)" implies.

⚠ The export must be *over the same pools*, not a reimplementation. A second copy of the ladder
that agrees today and drifts next quarter is precisely the "lying meter" §6 forbids — the
agreement test is the gate, and `preview_test.go:28` (`TestPreviewMatchesWhatReconcileAttaches`,
which pins preview against reconcile *by construction*) is the shape to copy.

**V17b depends on V30.** §6.1 left this open as "either the deps are wrong, or V17b is
thumbnails-only". The code answers it: `Clip.Thumbnail` is populated (`thumbnail.go`,
`store/clips.go:58`) and crosses the wire (`api/filler.go:99`), but the value is a path relative to
`.loomarr-thumbs` inside `FILLER_DIR` and **no route turns it into bytes**. Thumbnails-only is
buildable in every respect except the one that matters. V17b's `V17a`-only dep was wrong.

⚠ The gate's "inline `data:` URIs only, never remote URLs" was read as a production constraint and
is not one — it is about **stories**, which must stay offline and deterministic (§5.2). Production
fetches the V30 route. The gate now says which is which, because as written it forbade the only
implementation that works.

**V17d depends on V29b, and loses its coverage half.** V17d's declared dep was `V17b` alone while
its scope included "coverage", which would let a coverage UI ship with no ladder-backed source —
the exact failure §6 exists to prevent. Coverage is V29b's; V17d keeps the starter pack and "Find
clips".

**V30 must not be called `preview`.** "Preview" already means something else here and is shipped
twice over: `PodAdapter.Preview` (`adapter.go:123`) and two channel-scoped pod-preview routes
(`openapi.yaml:4189`, `:4222`) assemble *the pool a channel would get* — a JSON listing, not media.
A third meaning on a route name is how an endpoint gets called by the wrong handler a year later.

**V33 loses two deps and gains one that was never recorded.** Its declared `V17c` and `V30` deps
are unjustified: discovery returns candidate remote sources and shares no code with a quality floor
or with byte serving. What it actually builds on is `internal/clipfetch`, which already implements
the full Archive walk with HTTP+FS injected (`archive.go:32`) against a pinned fixture captured
live (`internal/testkit/fixtures/archive/`, 2026-07-13).

⚠ **V33 IS BLOCKED, and the blocker is fixture coverage — found 2026-07-31 while building it.**
An earlier pass at this section recorded the gate's hardest clause ("the Archive.org contract is a
pinned testkit fixture; discovery never runs in unit tests") as *already satisfied* by that
V17a-era work. It is not. That capture was taken for the DOWNLOAD walk and pins only what the walk
consumes:

| V33 needs | Pinned today |
| --- | --- |
| `licenseurl` — the gate's "license badges render" | ✗ `metadata_item.json` holds only `mediatype`, `title`, `description` |
| the collection search response (`advancedsearch.php`) | ✗ stubbed INLINE at `archive_test.go:66`, not a fixture |

Archive's metadata endpoint does return `licenseurl` in reality, but our capture does not contain
it — and a fixture is **pinned truth from a live capture** (CLAUDE.md). Inventing the field would
make every downstream test agree with a shape nobody verified, which is precisely the failure
fixtures exist to prevent. Live contact is maintainer-supervised, so this is a **stop point, not a
workaround**.

**To unblock:** one supervised capture pass against the live Archive JSON API — `GET /metadata/<id>`
for an item that carries a license, and `GET /advancedsearch.php?q=collection:<id>&fl[]=identifier&output=json`
for a real collection — saved beside the existing fixture with a source-version comment, exactly as
Phase 0 did. V33's scope after that is unchanged: the route, the persisted registry table, and the
badges (no `license` field exists on `ClipDTO` or `clips` today).

**V17c amends a published contract, and that is the phase's first commit.** Three places make two
different promises about the same field:

| Where | Says |
| --- | --- |
| `00014_clips_quality.sql:2` | quality "**NEVER** affects pod selection" |
| `api/openapi.yaml:526` → generated TS | "display-only, **never** affects pod selection" |
| `internal/filler/clip.go:83` | "display-only **by default** … V17c adds an OPT-IN floor" |

The migration and the API description are unconditional; the domain model already anticipates the
floor. Since the openapi string ships to clients, this is a **contract change, not a comment
edit** — `design.md`, the migration comment, and the description are amended in the same PR, before
the floor is built (CLAUDE.md doc-first). ⚠ The warning being overridden stays true and stays
written down: *"a well-meaning 'prefer HD' would quietly starve the era-accurate 4:3 commercials
the whole feature exists to play."* That is why the floor is **opt-in with the default off**, and
why the gate requires a test proving selection is byte-identical when it is unset.

### Lane assignment

The phases do not parallelize freely: V30, V17b and V33 each add an endpoint, so all three edit
`api/openapi.yaml` and regenerate the orval client — the conflict CLAUDE.md's worktree rule names.
Two lanes, and the split is by *generated output*, not by size:

| Lane | Phases | Owns |
| --- | --- | --- |
| **API** (sequential) | V30 → **V29b-api** → V17b → V17c → V33 | `api/openapi.yaml`, the orval client, filler migrations |
| **Coverage** | V29a → V29b → V17d | `internal/filler` exports, the meter, the starter pack |

⚠ **V29b was split across the lanes, and the reason is worth keeping.** The meter needs coverage
over HTTP, so V29b as one phase would have put the Coverage lane into `api/openapi.yaml` — the
exact conflict the split exists to prevent. The route half (**V29b-api**) therefore belongs to the
API lane and the meter stays in Coverage, which means **the Coverage lane is BLOCKED on V29b-api
merging**. That ordering is why V29b-api is built before V17b even though V17b comes first in the
phase list: the sequential lane should unblock the parallel one before doing its own work.

The alternative considered and rejected: folding coverage into `GET /v1/filler`'s response, which
needs no new route at all. Rejected because it grows the catalog listing's DTO for a different
question — "what is in my catalog" and "what would a break resolve to" are separate reads, and one
of them is per-channel.

⚠ **V29a is cherry-picked into the API lane**, not re-implemented: V29b-api compiles against
`filler.Coverage`, which the Coverage lane owns. Both branches therefore carry the same commit and
git resolves it on merge. This is the one place the lanes are not disjoint, and it is a read
dependency in one direction only — if the API lane ever needs to CHANGE that export, it belongs in
the Coverage lane instead.

⚠ The lanes are not fully disjoint: V29a exports from `internal/filler`, which V17c also edits for
the floor. They touch different functions (a coverage read vs. a selection filter) and no shared
generated file, so they merge cleanly — but if either lane changes `ladder.go`'s *pool
construction*, the other rebases rather than resolving by hand.

---

## 6.3 V33's licence-badge gate, amended by measurement (2026-07-31)

V33's gate said **"license badges render"**. It no longer does, and the reason is a number rather
than a preference.

**What was measured, live, during the fixture capture:**

| Source | Licence available? | Evidence |
| --- | --- | --- |
| archive.org | ~8% of items declare `licenseurl` | 667 of 8362 in `classic_tv_commercials` |
| YouTube (yt-dlp) | **never** | `license: null` on all 5 search rows **and** on a full non-flat extraction of one of them |

The YouTube result is the decisive one, and it was checked twice on purpose: `--flat-playlist`
returning null could have been an artefact of the cheap listing mode, so a full
`--dump-json --skip-download` was run against a single video. Still null. yt-dlp cannot report a
YouTube licence at all.

**So a badge could never say anything useful on the surface where the decision happens.** Every
YouTube row would read "unknown", and on archive.org 92% would too. A chip that is empty almost
always does not inform a choice — it *implies a per-item check that never happened*, which is a
worse claim than silence. Removed from Find clips entirely (maintainer decision).

⚠ **This does not mean the clips are unlicensed.** It means Loomarr cannot tell, and must not
imply it knows. The operator decides what they may reuse; the app's job is to not fabricate
reassurance.

⚠ **The licence is still CAPTURED and STORED** — `clips.license`, `filler_sources.license`, and
both sidecar readers all landed in #152 and stay. It is a record of what a source declared, and
it costs nothing to keep. What changed is that nothing RENDERS it: `ClipDTO` was never given the
field, so this is a decision not to add one rather than a removal.

---

## 6.4 V34 — compilation splitting and per-clip metadata (proposed 2026-07-31)

**The problem V33 leaves open.** Discovery finds sources; ingest downloads them. But a large share
of what it finds is a **compilation** — one file holding twenty or more commercials back to back.
Ingested whole it is a single 15-minute "clip" the pod assembler can never place: breaks are
30-90 seconds, and `durationEligible` rejects it. The catalog gains a row and no usable filler.

Splitting it produces the opposite problem: twenty clips named `compilation_seg07`, with no era,
audience or category, which the ladder also cannot place. **Splitting and metadata are one phase
because either alone produces unplaceable clips.**

### What was measured before this was written

Six chapterless compilations (~1h45m) from YouTube, scored on "segments of plausible ad length":

| Detector | Result | Notes |
| --- | --- | --- |
| `blackdetect` @ `pix_th=0.20` | **81–100%** per video | The tuning matters: at `0.10` two videos scored 60-67%, because they fade to dark grey rather than black |
| `blackdetect` + `silencedetect` union | 69–95% | Better on some, worse on others — not a uniform win |
| `scdet` / PySceneDetect `ContentDetector` | **unusable** | 454 and 318 boundaries on one video: they detect camera cuts INSIDE an advert. Wrong granularity, not wrong tuning |
| PySceneDetect `ThresholdDetector` | 82% | Same fade-to-black signal as ffmpeg, no better |
| Comskip | rejected without testing | Its strongest signals — logo, closed captions, aspect-ratio change — are all absent from a YouTube compilation, and `punish_no_logo=1` scores no-logo content *toward* "commercial", so it misfires on all-advert content |

⚠ **Detection quality is a property of the SOURCE, not of the threshold.** Two videos produced
clean `30 30 30 30` runs; two produced 129s and 188s blocks with boundaries genuinely absent. No
setting fixes the second kind, which is why the phase cannot be "split everything automatically".

### ⚠ The ceiling, and the only thing that breaks through it

One 149-second block defeated every A/V detector. Transcribing it showed **three complete
adverts** — a Swiffer spot ending and an Aqua Globes spot beginning, cutting straight into each
other with no black frame and no silence. **Those boundaries exist only in language.**

Given the transcript, a local LLM found the cut at 27.4s exactly. That is the case for the
transcript step: it is not a nicety for metadata, it is the only signal that sees this class of
boundary at all.

Two failure modes were hit and diagnosed, and both belong in the gate:

- **A poor transcript produces confident nonsense.** Whisper `tiny` dropped ~60s of audio (4 gaps
  over 5s, worst 28s) and the LLM then returned phone-number digits as product names. Whisper
  `small` had **zero gaps** on the same audio. The transcript is the bottleneck; the model size is
  not a tuning preference.
- **The LLM invents boundaries in a single long advert.** A 121s infomercial for one product was
  split into three at suspiciously round 30/61/92s marks. Adding *"if the whole transcript is ONE
  advert, return exactly one entry"* fixed it — and on a genuine multi-advert block the model then
  labelled an uncertain boundary `"unknown"` rather than naming it. **A test must cover the
  single-advert case**, or this ships a splitter that manufactures clips.

### Shape

1. **Triage.** `yt-dlp --dump-json` exposes chapters without downloading; a pre-chaptered source
   splits for free. ⚠ Rare in practice — 6 of 8 sampled had none — so this is an optimisation, not
   the mechanism. ⚠ `--write-auto-subs` was tested as a free-metadata shortcut and **does not
   work**: zero of three test videos had captions of any kind.
2. **Coarse split.** `blackdetect` + `silencedetect`, parsed in Go. Segments under ~3s dropped.
3. **Rescue.** Any segment far longer than a plausible advert goes to transcript + LLM for
   boundaries the A/V pass could not see.
4. **Metadata.** The transcript feeds the EXISTING `filler.Classify` unchanged — it already takes
   `(name, sourceText)` and already knows `cereal`, `toys`, `cars`. Verified on real transcripts:
   *"Rice Krispie's treats are so easy to make"* classifies correctly today.

   ⚠ **A §8 GROUNDING HOLE, measured: 2 of 10 clips got an INVENTED era.** Running the real
   `tagSystemPrompt` over real transcripts, two clips came back `1980` and `1970` with **no year
   anywhere in the transcript** — the model inferred a decade from tone. `validateTags` accepts
   any year in 1930-2035, so both would be persisted as fact.

   This is pre-existing (V17a's sidecar path can hit it too) but transcripts make it far more
   likely, because ad copy is full of period-sounding language and contains a literal year only
   rarely. §8 forbids exactly this: a tag nothing grounds. The phase must either require the year
   to appear in the source text before accepting `era`, or record era as a *suggestion* the
   operator confirms — not silently trust it. Encouragingly, the model answered `era: 0` honestly
   on the other 8, so the prompt is mostly working; it is the validator that has no way to tell an
   inferred year from a read one.
5. **Dedup.** The same advert recurs across compilations. **Measured, and the margin is wide:** a
   dHash over frames sampled at 1/3fps gives a mean per-frame Hamming distance of **1.1 for a
   re-encoded, downscaled duplicate vs 27.6-32.2 for different adverts** — a 25x separation, so
   any threshold in the teens works. It is ~30 lines of pure Go over `ffmpeg -pix_fmt gray`
   output: no library, no cgo, nothing vendored. ffmpeg's `signature` filter also works (verified:
   "whole video matching" on a duplicate, silent on a different clip) but needs a custom coarse
   index to avoid O(n²) and reports only at `-loglevel verbose`, which is an easy trap.
6. **Review.** ⚠ **Not optional.** Detection is 69-100% depending on the source, so the operator
   confirms the proposed cuts before they enter the catalog. Auto-accepting a 69% result puts
   3-minute "commercials" into ad breaks.

### Dependencies

Steps 1-3, 5 need **nothing new** — ffmpeg and yt-dlp are already bundled and already exec'd.
Step 4 uses the LLM provider that already exists. **Whisper is the one genuine addition**, and it
is a §14 conversation: `whisper.cpp` ships a self-contained `whisper-cli` binary matching the
existing vendored-binary pattern exactly (no cgo, no service), with `base.en-q5_1` at 60MB. The
Python package was what these measurements used; the binary is the shippable form.

⚠ **Not attempted, and why:** TransNetV2 via ONNX would need cgo plus a ~20MB native library plus
validating a third-party export nobody has certified — and it is a *shot* boundary model, which
§6.4's `scdet` result shows is the wrong granularity for this problem. The expensive option is
also the wrong one.

- **Pre-users premise:** the app has no external users, so defect-first ordering does not apply.
  Structural change is cheap now and expensive after more surfaces build on the old IA. The maintainer
  is still a user — the manual homelab smoke remains half the DoD.
- **Nine phases are free.** Start V0 for a ten-minute win, or V2b for maximum unblocking.
- ~~**V14 is the rename-debt clock.**~~ **PAID OFF, 2026-07-26.** `/channels` and `/suggest` are
  deleted and `/guide` is the channels surface; the admin nav is the mock's seven and the member's
  is three. The fold was blocked for several phases on "the grid has no origination affordance yet"
  — recorded in four places — and the mock had always carried one (`✦ Add a channel` in the header
  of a Guide screen headed "Channels"). Nothing had ported it. **C8** is answered in the same pass:
  API-ONLY BY DECISION, recorded in §12's surface map rather than left orphaned. See PROGRESS.md.
- **V7 and V17a are fully independent** of the spine and good parallel work.
- **Blocking counts** (transitive): V2b 28 · V3 27 · V4 26 · V5 12 · V9 11.

---

## 8. Known gaps in this plan

Stated so they are not mistaken for oversights:

1. **No estimates.** 39 phases is sequenced, not sized.
2. **The five companion plans referenced by the older Program Plan do not exist in the repo** — only
   `channels-refinement-2026-07-24.md`. No V-phase depends on them.
3. **`tcOptions` / `poPresets` were never authored** in the mock; V21 invents the encoder list and
   quality presets deliberately.
4. **Four decisions reverse documented design.** Deliberate and recorded — but see §5.
