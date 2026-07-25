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
| **V6** | Track T: Internal playout to first frame | V3, V4, V5 | A native channel in the media-server guide **playing a test card**; both transports; `StaleLoomarrListings` still cleans up after retargeting (**T2**); segment requests reject a wrong/absent `playout_token` |

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
| **V15** | Help rebuild — two-rail reader, full-text search (closes **H1**, **H2**, **H3**, **H4**, **H5**, **H6**) | V14 | `grep -ri "hooks/arr\|WEBHOOK_SECRET" docs/` returns **nothing** (H1, H2); search hits body text, not just titles (H3); channel-editing docs exist (H4); measure bounded by the two-rail layout (H5); links use `tune`, not the AI `suggest` colour (H6) |
| **V18** | Mobile responsive (**S11**) | V9, V14 | AppShell collapses; mobile v2 screens render at 375px; desktop-only actions render as disabled affordances, not dead ends |

### Approvals & requests

| # | Phase | Deps | Gate |
| --- | --- | --- | --- |
| **V25** | `D-K` — edit-before-approve **backend**, one chokepoint | V4, V24 | `POST /approve` takes a body; **`suggest.Approve` remains the sole implementation** (a test asserts no second acquisition path); `modSummary`+`note`+`approvedBy` persist; member still 403s |
| **V25b** | **Edit-before-approve UI** — drop titles with `✕`, add via search, note to the requester | V25 | An admin edits a proposal and approves the edited version from the queue; the note reaches `modSummary`; a reachability assertion proves the panel mounts |
| **V26** | `A2` — "My requests": per-user proposal list + admin-edit provenance | V25 | A member sees their own submitted/denied/edited requests; *"CHANGED BY …"* renders; the denial line shows |
| **V27** | Approvals queue as its own surface: tabs, bulk approve, audit rows | V25, V26 | Tab counts correct; bulk approve goes through the same chokepoint; history rows carry `approvedAt` |

### Filler

| # | Phase | Deps | Gate |
| --- | --- | --- | --- |
| **V17b** | `F2` — clip previews on `ClipCard` | V17a | Preview renders; visual baseline; **inline `data:` URIs only, never remote URLs** |
| **V17c** | `F3` — a quality dimension beyond `INGEST_PREFER_ORIGINAL` | V4, V17a | Quality is first-class on a clip, surfaced and usable in selection |
| **V17d** | Starter pack + coverage/"Find clips" (**F4**) | V17b | Seeded clips reviewable with keep/exclude; a thin channel surfaces the gap |
| **V28** | `sources` entity + clip metadata columns | V4, V17a | Migration `00013`; sources CRUD + per-source `Fetch now`; thumbnail/quality/usage populated |
| **V29** | Coverage meter (F2 banner) | V28 | **Consumes `internal/filler/ladder.go`** — a test asserts the meter and pod assembly agree. See §6. |
| **V30** | Filler preview serving | V6, V28 | Previews stream; inline `data:` URIs in stories only |
| **V33** | `#8` — F3b discovery: `GET /v1/filler/discover` + Sources registry | V17c, V28, V30 | The Archive.org contract is a **pinned testkit fixture**; discovery never runs in unit tests; license badges render |

### Dashboard & wizard

| # | Phase | Deps | Gate |
| --- | --- | --- | --- |
| **V16** | Dashboard incl. transcode telemetry (D-A) | V6, V13 | Live per-stream telemetry; member sees the lockout, not a 403 wall; **`restartFacts[0]` rewritten** — *"Tunarr streams them, not Loomarr"* is false after D-I |
| **V31** | Dashboard Services panel — aggregating wrapper + 30s poll | V13 | **Reuses `POST /v1/setup/test`** (a test asserts one probe implementation); "Fix →" routes to the failing block |
| **V32** | Dashboard Recent activity feed | V31 | A persisted feed (not just live SSE); survives restart |
| **V20** | Wizard **Database** step (step 1 per D-B) | V5, V11 | Choice persists across restart-and-resume; `setup/state` survives a DSN switch (it currently lives *in* the database being migrated) |
| **V21** | Wizard **Playout** step + transcode check | V6, V20 | Encodes a real 15s test pattern; progress streams over `/v1/events`; **`IMAGE GATE` copy absent**. Note `tcOptions`/`poPresets` were never authored in the mock — invented here, deliberately |
| **V22** | Wizard reconciliation: `bk.summary`, persisted `skipped`, strike the dead §20 bullet (**S12**) | V20, V21 | `skipped` survives a refresh; `design.md:940` struck |

### Not scheduled — need a design decision first

`C5` (strategy/group have no UI, while `policy.ordering` offers "Inherit channel default" — inheriting
from an invisible field), `C6` (`autoCurate` live with zero frontend refs), `C8` (no hand-made channel
create surface — possibly working as designed per §12's origination-vs-evolution model).

**The v2 mock shows no UI for any of them**, so there is nothing to port. Each violates §12's
surface-map rule; the honest options are *add the UI*, *remove the capability*, or *document it as
API-only*. Decide, don't defer indefinitely.

---

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

**V25 — one approval chokepoint.** `suggest.Approve` is the single shared implementation *"so the two
can never disagree about what approving means"*. Edit-before-approve passes it an edited proposal; it
does **not** get a second path. The edit is a provenance change, not an authorization change.

---

## 7. Sequencing notes

- **Pre-users premise:** the app has no external users, so defect-first ordering does not apply.
  Structural change is cheap now and expensive after more surfaces build on the old IA. The maintainer
  is still a user — the manual homelab smoke remains half the DoD.
- **Nine phases are free.** Start V0 for a ten-minute win, or V2b for maximum unblocking.
- **V14 is the rename-debt clock.** Every PR touching `/channels` before it lands gets renamed later.
  D-D bundled the rename with the grid, and the grid needs V13b → V6 — so the debt runs longer than
  the bundling decision assumed. If it becomes painful, the escape hatch is to revisit D-D and split
  the rename out.
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
