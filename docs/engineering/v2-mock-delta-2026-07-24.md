# v2 mock delta — research findings and evidence

> ## ▶ Looking for what to build? Read [`v2-build-plan.md`](archive/v2-build-plan.md)
>
> **This document is the evidence base, not the plan.** It records how each conclusion was reached,
> in the order it was discovered, with file:line grounding for every claim. Read it when you need to
> know *why* a decision was taken — especially the four that reverse documented design (D-I, D-E2′,
> D-F, D-K) — or when re-verifying a finding against a moved repo.
>
> Where this document and the build plan disagree about *what to do*, **the build plan wins**; it is
> maintained, this one is a dated record. Where they disagree about *a fact*, this one carries the
> citation — check it.

**Status:** research complete. Both mocks read in full (markup **and** the 190 KB state block).
All decisions taken 2026-07-24 and recorded in §5a.
**Verified against:** `55fc691` (HEAD), 2026-07-24.
**Inputs:** `Loomarr Prototype v2.dc.html` (502,509 bytes, complete), `Loomarr Mobile v2.dc.html`
(Claude Design project `dc543738-…`), the project's own `github.md` sync log, plus the maintainer's
Program Plan grounded in `9871fef`.

**Reading order.** §1 is the summary. §2–§2i are the four investigation passes in chronological
order — later passes correct earlier ones, and where they do, the correction is stated inline rather
than by silent edit. §5 is the defect register re-verified at HEAD. §5a is the decision record.
§8's phase list has been **superseded by `v2-build-plan.md`** and is retained only for its
pre-users sequencing rationale.

### The four passes, and what each found

| Pass | Scope | Headline finding |
| --- | --- | --- |
| 1 — structural | Section inventory, both mocks | The mocks were built **from this repo** (§2a); much apparent delta is the mock reflecting shipped code |
| 2 — interiors A | `CHANNEL DETAIL` (44 KB), `SETTINGS` (74 KB) | ⛔ The Security tab proposes an SSO subsystem **§11 forbids** (§2d.1); ~20 registry keys don't exist (§2d.2) |
| 3 — interiors B | `GUIDE`, `WIZARD`, `SUGGEST`, `APPROVALS`, `BOARD`, `DASHBOARD`, `FILLER` | ⛔ Internal playout **reverses `design.md`'s first principle** (§2e); the grid needs a new endpoint (§2f); `approveProposal` takes no body (§2h.1) |
| 4 — state block | The 190 KB `text/x-dc` script | ⚠ The JS **reimplements shipped algorithms incorrectly** (§2i.1); Coverage's *"same ladder"* claim is **false** (§2i.2) |

Depth-of-read and severity-of-finding were uncorrelated: the structural pass found five surfaces; the
fourth pass found a false attribution that would have shipped a known-bad meter.

---

## 1. The headline

The v2 mocks change the app's information architecture at the top level: a net-new **Dashboard**
becomes the landing surface, **Channels becomes Guide** and turns into a real TV time-grid, **Users
becomes People**, **Board becomes My requests**, and **Settings absorbs a System tab** containing
Playout, Database, Backup, About and Tasks.

**But much of the mock is not new work.** The mocks were built *from* this repo (§2a) and a lot of
what looks like delta is the mock reflecting already-shipped code — including ~25 verbatim-matching
copy strings. On one surface (channel detail) the mock is actually **a commit behind** the repo.

**Net-new surfaces, after the interior pass (§2d):**

> **Guide (time-grid) · Dashboard · transcode telemetry · restart control · the IA renames ·
> ~20 new settings keys (playout / backup / SSO groups) · a `playout_token` secret ·
> Filler starter-pack + coverage/"Find clips" · per-title refine rationale ·
> the All-settings search surface · cross-tab save bar.**

The first-pass list stopped at the first five. The interior pass roughly doubled it — the settings
registry work in particular is backend surface, not UI polish, and one item (**SSO**) is a §11 safety
conflict rather than a build item at all (§2d.1).

Equally important: the mocks **answer several questions the Program Plan lists as open** (§2 of that
doc — decisions #3, #8, #10, #11) and declare one conflict against the code outright (§2b). They are
a design *proposal* on those points, not a settled input. See §6.

---

## 2. Provenance — RESOLVED

**The full desktop mock was obtained out-of-band on 2026-07-24** (maintainer export,
`~/Downloads/Shared file archive`) and is installed at
`design/loomarr-prototype-desktop-v2.dc.html`. **502,509 bytes — the MCP fetch had delivered
262,144, so 48% of the file was previously unread.** Both mocks are now complete and authoritative:

1. Desktop v2 — 500,882 bytes of markup, 22 sections, closing `</x-dc>` present.
2. `Loomarr Prototype v2.dc.html` and `… v2 copy.dc.html` are **byte-identical** (`a1d9eba2…`). The
   "copy" is a duplicate; there is no second desktop variant.
3. Mobile v2 — 53,208 bytes, verified identical to the earlier fetch (`82c75f84…`); never truncated.

*(`Shared file archive(1)` is a second identical export of the same archive — no new content.)*

### What the previously-unread tail resolved

Every hedge that depended on the truncation is now settled, and **the cautious reading was correct
on the one that mattered**:

| Open question | Resolution |
| --- | --- |
| Did v2 drop **Help**? | **No — `HELP` exists** (10,975 bytes), a full two-pane docs reader. The "v2 dropped Help" inference the earlier draft warned against would have been wrong. |
| Does a **Security** tab exist? | **Yes** — `stSecurity`, a "How people sign in" card with SSO modes. Ratifies #4 on desktop evidence, not just mobile. |
| Does an **All settings** search surface exist? | **Yes** — `stAll`, search over `Key / Value / Group / Provenance` with an `ADV` chip. Ratifies #5. |
| `MEMBER INTRO` / `USER DRAWER` / `TOAST`? | **All present** (2,414 / 6,349 / toast markup). |
| Is there an **ACCOUNT** screen? | **Yes** (6,041 bytes) — full change-password form + session list. |

**Section sizes (full file):** LOGIN 4,874 · WIZARD 39,327 · APP 6,123 · TRANSMISSION (P-4) **47** ·
DASHBOARD 14,709 · GUIDE 26,219 · CHANNEL DETAIL 44,006 · BOARD 4,766 · SUGGEST 15,356 ·
APPROVALS 14,475 · FILLER 2,019 · COVERAGE 1,841 · CATALOG 7,590 · DISCOVER 5,864 · SOURCES 7,485 ·
PEOPLE 11,994 · **SETTINGS 74,162** · ACCOUNT 6,041 · HELP 10,975 · MEMBER INTRO 2,414 ·
USER DRAWER 6,349 · TOAST *(+ trailing runtime script)*.

`TRANSMISSION (P-4) + faults (P-7)` is **47 bytes — a bare marker with no markup**, confirmed
against the complete file. It is a placeholder, not a screen.

---

## 2a. The mocks were built FROM this repo — most of §4 is not new work

**Discovered after the first pass, and it materially shrinks the delta.** The design project contains
`github.md`, a sync log (`repo: loomarr/loomarr`, `branch: main`, last sync
**`2026-07-24T21:51:03Z`** — at or ahead of `55fc691`). It records the mocks being rebuilt *from*
shipped source: *"Rebuilt channel detail as the real 5-tab surface from `channels/$id.tsx`"*,
*"Adopted the repo's channel vocabulary from `channel-health.ts`"*, and carries a screen→source map.

So the v2 mocks are substantially a **reflection of shipped code**, not a greenfield proposal. Every
claim in that log was verified against HEAD:

| Sync-log claim | Verdict at `55fc691` |
| --- | --- |
| Programming folds Refine + "What plays" / "How it's ordered" / "When it changes" + cycle preview | **EXISTS** — `-channel-programming.tsx:71,81,86,91,109`, literal headings |
| Filler draft/apply sandbox (criteria chips, always/never, Unsaved dot, Apply/Discard) | **EXISTS** — `channel-filler.tsx:20,45,59,63,72,84` |
| Advanced = relaxation-ladder sentences + Tunarr link | **EXISTS** — `-channel-advanced.tsx:9,41,65,68` |
| Danger zone = pause/resume + two-step delete + "Also remove it from Tunarr" | **EXISTS** — `channel-danger-zone.tsx:54,58,89,94` |
| Per-channel ⋮ row menu (Edit / Pause-Resume / Delete) | **PARTIAL** — menu ships (`channel-row-menu.tsx`); **no Edit item** (the row itself is a `<Link>`, `index.tsx:118`) |
| Webhook-handshake + Connect-Live-TV steps removed | **EXISTS (already absent)** — `steps.ts:10-26`; the mock removes nothing |
| TV power-on entrance (SMPTE bars, wordmark) + staggered nav reveal | **PARTIAL** — `color-bars.tsx` + `brand-lockup.tsx` ship; **no animation** |
| Guide: calendar date picker, start-time, −/+ zoom | **DOES-NOT-EXIST** |
| `/channels` card grid → Tunarr-style TV Guide (ruler, rail, duration-scaled programs, now-line, 2H/4H/6H zoom) | **DOES-NOT-EXIST** — shipped `index.tsx:106-151` is a vertical `<ChannelCard>` list; repo-wide grep for `time-grid\|now-line\|ruler` returns zero components |
| Dashboard | **DOES-NOT-EXIST** — no `dashboard` symbol in the frontend |
| Transcode telemetry | **DOES-NOT-EXIST** — `transcod\|encoder` matches only `json.NewEncoder` |
| Restart / reload-settings control | **DOES-NOT-EXIST** in code (see §2c) |

**Correction to §4:** the channel-detail delta is smaller than first reported. Shipped is a **4-tab**
surface (`info`, `programming`, `filler`, `danger`) with Advanced as a collapsed disclosure inside
`info` — precisely commit `55fc691`. The sync log describes a *5-tab* surface with Advanced separate,
so on this one surface **the mock is a commit behind the repo**, not ahead of it.

**The real net-new surface list is therefore short:** Guide (time-grid), Dashboard, transcode
telemetry, restart control — plus the IA renames. Everything else in §4 is the mock catching up.

## 2b. A conflict the mock declares against itself

The sync log states outright:

> *"Every wizard step is now required (product decision — diverges from `steps.ts`, which marks Users
> `optional: true`)."*

Confirmed: `web/apps/web/src/wizard/steps/steps.ts:25` is
`{ id: "users", title: "Users", optional: true }`, and `steps.ts:71` uses `!s.optional` to choose the
resume step. The mock knows it disagrees with the code and asserts a product decision to override it.

Per §3 precedence this is **not the mock's call** — required-vs-optional onboarding steps are
behavior, so `design.md` decides. Add to §6 as an explicit decision.

## 2c. The restart control extends doctrine rather than contradicting it

Worth stating precisely, because the naive reading is that a restart button violates hot-apply.
`docs/config-design.md:72`: *"`RestartRequired` exists as a flag for honesty but applies only to the
**bootstrap set**, which the UI never edits."* The mock's own copy matches the doctrine verbatim —
*"Most settings apply without a restart — Loomarr reloads them in place. A restart is only for the
few that are read at boot."*

So the Dashboard restart control is the **surfacing of defect S5's unimplemented flag**, not a
contradiction. But it *does* extend the doc: the clause "which the UI never edits" stops being true
the moment the wizard edits `DATABASE_URL` at step 1. That is decision **#6** (bootstrap-file tier),
and it is the hinge the Database/System work hangs on.

## 2d. Interior pass — what the structural skim missed

The first two passes inventoried *sections*. This pass read the interiors of the two largest —
`CHANNEL DETAIL` (44 KB) and `SETTINGS` (74 KB) — against the shipped implementation. It found
substantially more than the section inventory implied, and one **safety conflict**.

### 2d.1 ⛔ The Security tab proposes an SSO subsystem that §11 forbids

**This is the most important finding in the document.** The mock's Security tab specifies a full
identity-provider integration:

- `ssoModes` — 3 radio cards, copy: *"Loomarr's own sign-in always works — an identity provider is
  added alongside it, never instead of it, so a broken provider can't lock you out."*
- `ssoProviders` (4 chips), `ssoFields` (issuer / client id / client secret / scopes-or-redirect)
- A **"Create people on first sign-in"** toggle
- **"Admin group"**, placeholder `loomarr-admins`, help: *"A person in this group is an admin in
  Loomarr. Everyone else lands as a member with the default quota."*
- A test flow rendering **"What the provider told us"** (a claims dump) + a **"Show break-glass URL"**

**Verified against `docs/design.md` §11: there are ZERO mentions of SSO, OIDC, OAuth, or an identity
provider anywhere in the design doc.** And §11's model is the *opposite* of what the mock proposes:

> §11:9 — *"A login attempt for a username with **no matching row is rejected**… **There is no lazy
> self-provision.**"*
> §11:20 — explicit import *"is the **only** way a media-server user gains access."*
> §11:21 — sync *"**never adds** new users — sync reconciles the allowlist, import defines it."*

**"Create people on first sign-in" is exactly the lazy-provisioning hatch §11 was reworked to
close.** The auth rework closed two such hatches (login and sync); this re-opens the login one and
adds group-derived role assignment on top, which also bypasses the *"Loomarr doesn't infer role from
the media server"* stance the mock's own People tab states.

Per AGENTS.md prime directive 3, the authorization model (§7/§11) is **not negotiable, including in
tests and seed data**. So this is not a build item:

> **⛔ Do not implement the SSO surface as drawn.** It requires an explicit §11 amendment first, and
> that amendment has to answer how an auto-created account is compatible with an allowlist that is
> the source of truth. **New decision D-F below.**

The rest of the Security tab is fine and mostly exists: the **Sessions** group (`session.ttl`,
`cookie.secure`, `user.sync_every`) **EXISTS**, and **Instance secrets** matches `secrets.go` exactly
— `session_secret` (never displayable) + `api_token` (viewable), including the masked `…a1b2` preview
form. The mock adds a **type-to-confirm** regen gate (*"Type {{ regenKey }} to confirm"*) that
`secrets.go` doesn't model — a genuine, small, safety-positive addition.

### 2d.2 The mock invents ~20 settings keys that do not exist

Verified: `grep -cE '"auth\.|"playout\.|"backup\.' internal/settings/declared.go` → **0**. Existing
groups are MediaServer, Tunarr, Requester, TMDB, AI, Channels, Filler, UsersSecurity, Advanced —
**no Playout, no Backup, no SSO**. Secrets are **two**, not three.

| Area | Keys the mock implies | Status |
| --- | --- | --- |
| Playout | `playout.backend`, `playout.encoder`, `playout.resolution`, `playout.bitrate`, `playout.max_channels`, `playout.public_url`, `playout.transport` (weakest inference) | none exist |
| Secrets | `playout_token` — generated, regenerable, *"Signs every segment request"* | not in `secrets.go` |
| Backup | `backup.schedule` (*"nightly at 03:30"*), `backup.retain` (*"keeps 7"*), `backup.dir` | none exist |
| Auth/SSO | `auth.mode`, `auth.sso.provider`, `auth.sso.issuer_url`, `auth.sso.client_id`, `auth.sso.client_secret`, `auth.sso.scopes`, `auth.sso.auto_create`, `auth.sso.admin_group` | none exist — **and see 2d.1** |
| DB migration | 6 target-PostgreSQL connection fields (host/port/db/user/password/sslmode) | may be transient, not persisted |
| Metadata | a per-setting `self_healed` flag; `Setting.Optional` for the `OPTIONAL` chip | not modeled |
| AI | a **third** `llm.provider` chip (registry enum has 2: `ollama`/`openai`) | new enum value |

**`server.public_url` is a trap.** It exists, but `declared.go:117` documents it as *"only needed for
uploaded channel icons"* and files it under `GroupTunarr`. The mock makes it **segment-critical**
(*"Get it wrong and channels appear in the guide but never play"*). Same key, materially different
contract and blast radius — this is Program Plan fact **T3**, now confirmed from the mock side.

Per AGENTS.md, config that isn't in §15 must be **added to §15 first**. So the playout/backup groups
are doc-first work, not UI work.

### 2d.3 Save-bar shape differs from the shipped model

The mock's dirty bar is **global and sticky at the screen level**, outside the scroll container, and
therefore **spans tabs** — *"Save changes"* / *"Discard"*, with each tab's attention dot likely
marking dirty-or-failing. Shipped code comments specify *"an explicit save bar **per page**"*, and
`config-design.md:118` describes the per-page Sonarr-style bar. Cross-tab dirty state is a different
state model (it must survive tab switches, and Save must commit across tabs).

Four surfaces deliberately opt out of the global bar and commit inline: playout-token regen, the DB
stepper, the task-schedule modal (*"Save schedule"*), and secret regen. That exception list is a
design contract worth preserving.

### 2d.4 Channel detail — the refine diff is a mutual subset, not a gap

I previously framed C3 as "the repo is behind the mock." That was wrong in both directions:

- **The mock's refine diff is lineup-only** — rows of `sign` / `name` / **`why`**. It surfaces
  **zero** policy deltas (no Era, Audience ceiling, Ordering, Seasonal, *or* Separation).
- **The repo's diff is policy-aware** — `refine-review.tsx:36-44` renders a four-field policy delta
  group with pinned/kept handling, plus grouped title changes (`Keeping · N`, `Adding · N`,
  `Adding (needs downloading) · N`, `Removing · N`).
- **Neither has per-row rationale.** The mock's `why` column has no repo equivalent.

So: the repo is **ahead on policy**, **behind on explanation**, and **C3′ (add `Separation`) stands**
— the mock provides no cover for it either way. Adding a per-title `why` is a **new capability**
requiring the suggester to emit per-title rationale.

### 2d.5 Channel detail — other interior findings

| Finding | Status |
| --- | --- |
| **Channel icon picker** on the info panel ("Suggest from TMDB" / "Paste a URL" / "Upload") | `ChannelIconField` **is built** at `components/loomarr/channel-icon-field/` but **never imported by `$id.tsx`** — a built-but-unmounted component. Cheapest real win in the mock. |
| Filler **"Starter pack"** — creation-seeded clips, `SEEDED AT CREATION` badge, per-clip keep/exclude, `GAP` row, *"Start from scratch"* | **NOT-IN-REPO**, entirely new flow |
| Filler **coverage banner + "Find clips"** CTA when thin | **NOT-IN-REPO** — this is register defect **F4** (gap flagging) drawn concretely |
| Filler **4-step pipeline explainer** strip | **NOT-IN-REPO** |
| Cycle preview: **wall-clock time column + "AT THIS MOMENT" cursor** | **DIFFERENT** — repo rows are ordinal (1,2,3…) with S×E + break dividers; mock promises real airtimes. Repo's `datetime-local` + 5 preset buttons is a **superset** of the mock's single `AT` dropdown. |
| Advanced as a **tab** (mock) vs collapsed **"Diagnostics"** disclosure (repo, `$id.tsx:263-270`) | **Deliberate divergence** — commit `55fc691` chose the disclosure and dropped `?section=advanced` on purpose. §12 wins; keep the disclosure. |
| `autoCurate` toggle, `strategy` control, `group` control | **Absent from the mock too** — so defects **C5**/**C6** get no UI guidance from v2. They need a design decision, not a port. |
| Verbatim-matching copy across ~25 strings (audience ceiling, era, no-repeat windows, danger zone, delete confirm, lineup empty state) | **EXISTS verbatim** — confirms the mock was generated from the repo (§2a) |

## 2e. ⛔ STOP POINT — Internal playout reverses `design.md`'s first principle

**Found in the second interior pass (GUIDE + WIZARD). This is a bigger conflict than §2d.1, and it
invalidates the premise of three already-ratified decisions.** Per AGENTS.md ("Ask the maintainer"),
a gate that requires weakening a prime directive is a design conversation, not a workaround.

### What the design doc actually says

| Line | Statement |
| --- | --- |
| `design.md:39` | **"Not a transcoder/streamer.** Tunarr (and Emby/Jellyfin) do playback, transcoding, EPG, and HDHomeRun/M3U output. `loomarr` decides *what plays and in what order* and hands that to Tunarr." |
| `design.md:530` | "Tunarr then plays clips from that list into the flex gaps… Tunarr inserts filler at **program boundaries**… **treat mid-roll as out of scope**" |
| `design.md:727`, `:795` | "**Loomarr still never probes media**"; "**`ffprobe` is deliberately NOT bundled**" |
| `design.md:942` | Mid-roll insertion is listed under **§20 Open questions** — i.e. explicitly *not* decided |

A repo-wide grep for Loomarr serving streams / owning an encoder / handling segments returns
**nothing**. Internal playout is not an extension of the design; it is a **reversal of its opening
architectural claim**.

### What the mock asks for, against that

The wizard's **Playout** step (net-new, step 3) contains a *"Transcode check"* that **encodes a real
15-second test pattern** (*"this is the real pipeline, not a guess"*), enumerates hardware encoders
(`tcOptions` — vaapi/qsv/nvenc/libx264) with measured per-encoder verdicts, and streams ffmpeg
stderr. That is a transcoder's self-test, and it requires exactly the media probing §16 twice says
Loomarr does not do.

### Corrections to my own earlier analysis (⚠1 was wrong on the facts)

1. **The image variants are not `runtime` vs `filler`-as-a-sidecar.** `design.md:795` is explicit:
   **`loomarr:latest`** (31 MB, no media tooling) and **`loomarr:filler`** (549 MB — *same Go
   binary*, plus pinned yt-dlp + ffmpeg + deno, non-distroless because those are glibc-linked).
   "Two tags, one binary."
2. **Folding them costs 31 MB → 549 MB**, not the ~250 MB I estimated. An **18× increase**.
3. **`design.md:505` already litigated and reversed the sidecar**, choosing the opt-in image tag
   precisely to keep media tooling out of the default image. D-E2 as decided reverses *that*
   reversal — so it is the third time this decision has been taken, and the rationale recorded at
   `:505` argues against it.

### The decisions this destabilizes

| Decision | Premise | Status |
| --- | --- | --- |
| **#10 Internal-first playout** | Loomarr can serve streams | contradicts `design.md:39` |
| **#11 Both transports (HLS + MPEG-TS)** | Loomarr serves segments | same |
| **#13 Ad-break detection** (decode-to-detect) | Loomarr probes media | contradicts `:727`/`:795` |
| **D-A full Dashboard w/ transcode telemetry** | Loomarr owns an encoder | same |
| **D-E2 fold `filler` into one image** | needed for the above | 18× size cost; reverses `:505` |
| **#12 guide advertises mid-roll = No** | — | **consistent** (mid-roll is out of scope anyway) |

### D-I — DECIDED 2026-07-24: amend the design; Loomarr becomes a streamer

**The maintainer has taken the reversal deliberately, with the facts above on the record.** Loomarr
grows from "decides what plays" into "decides what plays **and serves it**". This is the largest
single change in the entire v2 program, and it reframes Track T from a parallel track with optional
stopping points into **the spine of the roadmap**.

**Doc amendments required — all doc-first, before any playout code:**

| Section | Amendment |
| --- | --- |
| **§1** | Strike/rewrite *"Not a transcoder/streamer"* (`:39`). Loomarr's identity now includes playout. State what Tunarr remains for (or that it becomes optional). |
| **§10** | Mid-roll moves from **out of scope** (`:530`) to in scope — owning the encoder is exactly what removes Tunarr's program-boundary-only constraint. Also strike the §20 open question at `:942`. |
| **§14** | `ffmpeg` becomes a **core runtime dependency**, not an ingest-only vendored binary. **`ffprobe` is re-added** — `:727`/`:795`'s "never probes media" no longer holds once Loomarr assigns its own durations. |
| **§16** | Single 549 MB image (D-E2); the two-tag model at `:795` and the sidecar rationale at `:505` are both superseded. Non-distroless base; preserve `USER nonroot:nonroot` explicitly. |
| **§15** | The `playout.*` group (D-G), plus the `playout_token` secret. |
| **§6/§9** | Loomarr's own M3U + XMLTV output — today `StaleLoomarrListings` identifies Loomarr's provider **by its Tunarr-shaped path** (fact **T2**), which silently breaks on retarget. |
| **§7.1** | Segment-serving routes — and they are **token-authenticated, not session-authenticated** (a TV can't hold a session cookie). This is a genuinely new auth surface; §11's model must say so. |

**Decisions this re-validates:** #10 (Internal-first), #11 (both transports), #13 (decode-to-detect
ad breaks), D-A (full Dashboard with transcode telemetry), D-E2 (one image). All four now rest on a
premise the design will state rather than contradict.

**#12 stands unchanged** (guide does not advertise mid-roll breaks) — still the right call even once
mid-roll is in scope.

**Two risks worth naming now, not at implementation time:**

1. **Segment auth is a new §11 surface.** `playout_token` authenticates a *device*, not a person, and
   it bypasses the session/allowlist model entirely. That is legitimate and necessary — but it is a
   second authorization path, and §11 currently has exactly one. Design it in the §11 amendment
   alongside D-F's SSO path, not separately.
2. **`ffprobe` returns, reversing a documented size decision** (~99 MB, `:727`). Fine at 549 MB, but
   the *reason* it was excluded ("Loomarr never probes media") is exactly what changes — so the
   amendment must say why, or the next reader will re-litigate it.

## 2f. The Guide grid needs a new endpoint — V14 is not a frontend phase

The second-largest finding. The API's **only** guide shape is
`NowNextEntry{title, startMs, stopMs, gap, tmdbId}` (`channels.go:203`), served two ways:

- `GET /v1/channels/now-next` — **`now` + `next` only** (`guideadapter.go:24` reduces a timeline to a pair)
- `GET /v1/channels/{id}/upcoming?limit≤24` — one channel, and it **drops every gap**
  (`if e.IsGap() { continue }`, `guideadapter.go:79`)

**Data gaps the grid needs and the backend does not expose:**

1. **A windowed multi-channel schedule** — `GET /v1/guide?from=&to=`. Nothing returns >2 entries per
   channel, or more than one channel at a time.
2. **Arbitrary `from`/`to`, past and future.** Both handlers hardcode `time.Now()`; there is no query
   param. Required for the date picker and the mock's `AS AIRED` mode.
3. **Gaps must survive** — the grid's pods and pending slots *are* the gaps `Upcoming` filters out.
4. **A `kind` discriminator** (`program` | `pending` | `pod`). **`gap bool` cannot express "pending
   slot"** — so the grid's two most visually distinct block types (cyan dashed *"pending slot —
   backfills in place"* vs amber filler pod) collapse into one boolean today.
5. **Per-airing pod composition** — clip name, kind, duration, **quality**, **era**, `matchLevel`,
   and the `why` rationale, for *a specific break at a specific time*. Today only the channel-wide
   pool exists (`GET …/pods`), and `PodEntryDTO` has **no quality and no era**, both of which the
   hover card renders.
6. **Program metadata on guide entries** — full title, year, rating, genre, runtime, **episode
   title** (`S01E01 "Night of the Sentinels, Part 1"`), and an acquisition-provenance string
   (*"in library · 76 episodes"* / *"acquiring · Sonarr grabbed it"* / *"requested · 48h deadline"*).
   None is on `NowNextEntry`; `CycleSlotDTO` carries season/episode *numbers* only, no episode title,
   and no wall-clock times.
7. **Guide timezone** — hardcoded `America/New_York` in the mock; no timezone field anywhere in
   `internal/api`.
8. **Guide retention window** — the mock asserts *"guide retained 7d back"* and greys out days beyond
   it. Not exposed.

Not a gap: `ChannelDTO.Logo` exists; the monogram/color fallback is FE-derivable.

**Consequence:** V14 (bundled rename + grid) needs a **backend phase in front of it**. Added as
**V13b** below. Date picker, hour stepper, zoom and START are safely client-only.

## 2g. WIZARD — smaller findings

- **The mock's rail has no Skip.** No `onSkip`, no `skipped` token, no `OPTIONAL` chip on rail steps.
  Shipped has *"Skip for now"* + `SKIPPABLE = {library, users}`. **D-C (Users stays optional) is
  therefore a deliberate departure from the mock** — and the shipped comment gives the reason: a
  non-skippable step gated on an unsatisfiable check is a dead end, because the rail isn't clickable.
  The mock's rail is clickable only at sub-item level, so it doesn't rescue that case either.
- **"Saved as you go" is framing, not autosave.** The rail says it, but Connections renders a dirty
  Save/Discard bar — i.e. configure → validate → save → advance. That matches the *current* rule
  (`config-design.md:118`, `:128`; `design.md:640`) rather than the superseded one.
- **Confirmed defect S12:** `design.md:940` still lists *"§13's wizard deliberately validates rather
  than stores"* under §20 Open questions. Dead bullet; strike it.
- **`bk.summary`** (a per-connection one-line status) has no field in `SetupCheck`
  (`{name, ok, hint, docHref}`) — derive client-side or add it.
- **`skipped` is React-state only** in the shipped wizard; a refresh loses it. If skip should survive
  resume, `setup/state` needs a skipped-steps field.
- The mock's Connections step otherwise **matches shipped exactly** — same 5 block ids, same
  `REQUIRED_CHECKS = {media_server, tunarr}`, same Save/Discard. Cosmetic diff only: mock chip says
  `SET VIA ENV`, shipped says "set via environment".

## 2h. Third interior pass — SUGGEST · APPROVALS · BOARD · DASHBOARD · FILLER

The remaining large sections. Two findings here are structural.

### 2h.1 Edit-before-approve — the largest single gap (D-K)

**Verified:** `approveProposal(ctx, in *proposalIDInput)` takes a **path id and no body**
(`suggestions.go:181`) and approves the stored proposal verbatim. The mock's Approvals queue lets an
admin **drop titles (`✕`), add their own via search, and attach a note** — *"Drop titles with ✕, add
your own, and approve the version you want. The edit is recorded with the approval."* / *"Note to
{requester} (optional) — what you changed and why"*.

**Why this is safe to build (and how):** the gate's safety property is *"nothing unapproved ever
acquires"*, enforced by `suggest.Approve` being the **single shared implementation** — the code
comment says so explicitly, *"so the two can never disagree about what approving means."* Editing
doesn't weaken that: an admin still approves, through the same chokepoint. What changes is that the
approved artifact differs from the submitted one — a **provenance** problem, not an authorization
one. Hence the mock's *"CHANGED BY {approvedBy} · {modSummary}"* disclosure and the note.

**DECIDED (D-K): build it, one chokepoint.** `POST /approve` gains a body (dropped keys, added
titles, note); `suggest.Approve` **remains the sole implementation** and receives an edited proposal
rather than gaining a second path. Persist `modSummary` + `note` + `approvedBy` and surface them to
the requester.

### 2h.2 Deny-reason: the API works, the UI never uses it

`denyInput.Body.Reason` exists (`suggestions.go:232-237`) and persists to `Proposal.DenyReason`. But
**all three call sites send `data: {}`** — `suggest.tsx:65`, `approval-queue.tsx:67`,
`channel-suggest-panel.tsx:78` — so `DenyReason` is *always empty in practice*. A member never learns
why. **New defect: `A1`** (capability-without-UI, the §12 surface-map rule again).

### 2h.3 Board shows no proposals at all

`board.tsx` never queries `/v1/proposals` — it fans `GET /v1/titles?state=…` across five states.
Consequence: **a member cannot see their own submitted or denied requests anywhere.** The mock's
"My requests" is a two-tier surface (request cards → title table) whose entire first tier has no data
source. `approvedBy` is stored but rendered nowhere in the web app. **New defect: `A2`.**

### 2h.4 Suggest — data that exists but is never rendered

`ProposalDTO` carries `channelName`, `eraBalance`, `overall`, `policy`, `officialRating`, `genres`,
`overview`; the review renders only Theme fit + Ready now. `mustInclude`/`mustExclude` exist on
`suggest.Intent` with **no UI**. `onEditItem` is a prop on `proposal-review` that **no route ever
passes** — a dead affordance. **New defect: `A3`** (cheap wins: surface what already exists).

### 2h.5 Dashboard — nothing exists, and the services panel has a primitive

No dashboard route, no `src/dashboard/`, no matches for Transcoding / Service control / Recent
activity. Two notes for the build:

- **"probed every 30s · same checks Settings runs"** — the probe primitive **exists**
  (`POST /v1/setup/test`). The Dashboard's Services panel is an *aggregating wrapper + 30s poll* over
  it, not a new probe. Reuse it, or the two will disagree.
- The mock's stat-tile labels, service names and `restartFacts` text are **bindings with no literal
  fallback** — that content lives in the prototype's JS state, not the markup. Those exact strings
  must be read from the `<script>` block before building, or invented deliberately.

### 2h.6 Filler — a new persisted entity, and the coverage meter's key constraint

The mock is **3 tabs + 1 persistent banner** (Catalog / Discover / Sources, with Coverage always
visible above), not five peer surfaces.

- **`sources` is a new table** — kind (library / watched-folder / remote), target, status, per-source
  count, individually removable and re-fetchable. Today `clipfetch` takes an **ephemeral `[]Source`**
  from a pasted URL blob; migrations top out at `00012_channel_icons.sql`.
- **Clip metadata is absent**: no thumbnail, quality, or usage columns. `ClipDTO.Source` is a bare
  string, stored and never rendered.
- **Coverage** attributes itself *"from the same ladder reconcile uses"* — so it **must** consume
  `internal/filler/ladder.go`, exactly like the pod-preview/reconcile shared-assembler rule (§10).
  Reimplementing the tiering would let the meter and reality disagree, which is the bug the shared
  assembler exists to prevent.
- **Discover names its own endpoint in the mock**: `GET /v1/filler/discover`, searching Archive.org,
  with license badges and playable previews. Preview serving is media-serving Loomarr doesn't do
  today — though after **D-I** it will own that capability anyway.

**DECIDED: full filler build-out**, consistent with #8 (F3b in v1).

## 2i. Fourth pass — the prototype's JS state block (190 KB)

The last unread region: a `<script type="text/x-dc">` block at offset 309,743 (190,020 bytes, 2,449
lines, 323 declarations) holding the data behind every `{{ binding }}`. It contains both the literal
copy the markup couldn't show **and reimplemented algorithms**.

### 2i.1 ⚠ The JS is a demo fixture, NOT a spec — and it silently reimplements shipped logic

**This is the most important framing in this section.** The prototype re-implements filler ladder,
pod assembly, seeding, coverage and channel-health in JavaScript so the mock renders plausibly. Those
reimplementations **diverge from the shipped Go**. That is expected and harmless *as a prototype* —
it becomes dangerous only if someone builds from it.

Verified against the repo (the repo is correct in every case below):

| Behavior | Repo (authoritative) | Mock JS |
| --- | --- | --- |
| **Ladder tiers** | relaxes **era** holding audience: exact year → same decade → any era (`ladder.go:26`) | relaxes **audience** holding era: exact → general\|family → any audience. **Inverted.** |
| **`EraStrict`** | deletes the widened rung | the `era:'strict'` setting exists but `podBreak` **never reads it** — inert |
| **Category variety** | implemented — `place(false)` then `place(true)`, "never two clips of the same Category consecutively" (`ladder.go:120-133`) | **absent entirely**; `category` is carried but never consulted |
| **Pins** | placed **first, unshuffled** — *"a pin is an explicit choice, so honor its order"* (`pod.go:179`) | merged into the ad pool and **shuffled** — yet the hover copy claims *"Pins first"* |
| **Exclusions** | threaded into `used` before pin placement, so exclude beats pin (`pod.go:111`) | a separate pre-filter; same outcome, but no `used` set, so the rule is inexpressible |
| **Seed** | **per-channel** — FNV-1a over the channel id (`reconcile.go:319`); the filler list is *"a per-channel POOL, not a per-gap sequence"* | **per-gap** — `chNum*97 + start` via an LCG. And the displayed `seed ch42-815` token is computed *separately* from the seed actually used |
| **No-repeat window** | `used` threaded **across pods in a window** | per-pod only; no cross-pod state |
| **Gap reserve** | `GapMs - 12000` (documented ~two 6s bumpers) | `budget - 8` (magic number, different units) |
| **`PodMax`** | counts pins; bumpers appended outside the cap | counts ads only → podMax 4 yields 6 entries |
| **Determinism** | `sortByID` before any random pick, so catalog order can't leak | draws from filter-order arrays — output depends on input order |
| **channel-health** | keys on **`pendingCount`** (`channel-health.ts:15`) | `ch.filled < ch.total` — **the exact bug the repo fixed** (C1) and documented as wrong |

**Rule for the build, stated plainly:** where the mock's JS and the repo disagree about behavior, **the
repo wins and the mock is a rendering artifact**. This is `design/README.md`'s structure-vs-look rule
applied one level deeper — the prototype isn't authoritative for *algorithms* either.

*(Housekeeping found en route: `ladder.go:57` carries a stale `TODO(learning)` docblock describing an
unimplemented fill loop, directly above a complete implementation that satisfies every bullet in it.
Delete or rewrite the comment — it reads as "unfinished" to anyone grepping. **New defect `A4`.**)*

### 2i.2 ⛔ "From the same ladder reconcile uses" is false in the mock

The Coverage meter's attribution line — *"from the same ladder reconcile uses"* — is **not true of the
mock's own code**. Coverage recomputes its buckets inline with a different predicate set, and the file
contains **five mutually inconsistent era/audience predicates**: `podBreak`'s rungs, the coverage
block (era hardcoded to 1990–1994 regardless of channel), Discover's `eraOk`, the approve-time seed
pack (OR where the ladder uses AND), and the mock's audience-widening rule.

Worse, the percentages are **shares of catalog size** while `covDiagnosis` renders them as
*"Breaks resolve exactly N% of the time"* — a composition ratio presented as a resolution rate. Those
are different numbers under any assembly model.

**This makes V29's gate non-negotiable:** Coverage must consume `internal/filler/ladder.go` and a test
must assert the meter and pod assembly agree. The mock demonstrates precisely the bug the claim
denies — it is the §10 shared-assembler lesson (pod preview vs reconcile) repeating itself.

### 2i.3 Literal content the markup could not provide

Now recovered — these are build inputs for V16/V31/V32 and the Settings phases.

- **navItems** — admin: `Dashboard ◧` · `Guide ▶` · `Queue ✓` · `Filler ▦` · `People ◎` · `Settings ⚙`
  · `Help ?`. **Member (4, a different list): `Guide` · `Request a channel ✦` · `My requests ☰` ·
  `Help`.** Badge only on `queue` when pending > 0. Rail `224px` / `64px` collapsed.
- **dashStats (4)** — `On air` → channels; `Needs you` (*"requests waiting on approval"* / *"nothing
  waiting"*) → queue; `Acquiring` (*"titles in flight · slots pod-filled"*) → queue in-flight tab;
  `Filler` (*"clips in the shared catalog"*) → filler.
- **dashServices** — one literal row `Loomarr core` / `v0.9.3 · sqlite · schema v1.42`, then a row per
  `s.checks` entry. **States map to the existing probe vocabulary**: pass→`healthy`,
  fail→`unreachable`, running→`probing…`, pending→`not tested`. Confirms V31's "reuse
  `POST /v1/setup/test`" gate.
- **restartFacts (4)** — `KEEPS` *"Channels stay on air — Tunarr streams them, not Loomarr."* ·
  `KEEPS` *"Sessions survive; nobody has to sign in again."* · `PAUSES` *"Reconciles, scans and
  acquisitions stop for a few seconds, then resume where they left off."* · `LOSES` *"Unsaved edits in
  an open form on any device."*
  ⚠ **The first fact is invalidated by D-I** — once Loomarr streams, a restart *does* interrupt
  playback. Rewrite it in V16/V13, don't port it.
- **restartPolicyLine** — detects a supervisor: *"restart: unless-stopped detected…"* / *"no restart
  policy detected — you will have to start the container yourself"*.
- **restartRequiredCopy** — *"You changed a boot-time setting (DATABASE_URL). Loomarr is still running
  the old value until it restarts."* (S5's flag, made concrete.)
- **tunerFiles (2)** — `M3U` *"the channel list — register it as an M3U tuner"* and `XMLTV` *"the
  guide — register it as the listings provider"*; internal URLs are
  `/playout/tuner.m3u?token=plo_…` and `/playout/guide.xml?token=plo_…`. **This is the concrete
  `playout_token` + route shape for V4/V6.**
- **aboutRows — 7, not 6** (my earlier count was wrong): Version · Commit · Built · Go runtime ·
  Uptime · Database · Licence.
- **ssoModes (3)** — `off` *"Loomarr's own sign-in"*; `sso` **RECOMMENDED** *"…Authelia, Authentik,
  Tinyauth or any OIDC provider. Local sign-in stays available."*; `sso-only` **CAREFUL** *"Refuse
  local passwords entirely. The owner account keeps a break-glass URL…"*. **Providers: Authelia ·
  Authentik (proxy) · Generic OIDC · Tinyauth.** Note `sso-only` is compatible with D-F (no
  auto-create) — it restricts *credentials*, not provisioning.
- **poBackends (2)** — `Loomarr (internal)`: *"Required for mid-roll breaks. Needs ffmpeg in the
  image."*; `Tunarr`: *"Right for hardware that can't transcode. Never going away."* Plus
  **`poScopeCopy`**: *"Changing this affects new channels only — the channels already on the other
  backend keep playing exactly as they are. Switch one from its own page."* — i.e. **playout backend
  is per-channel, not just global.** That is a schema implication for V4.
- **`poPresets` and `tcOptions` do not exist** in the state block — the encoder option list and
  quality presets were never authored. Only defaults survive: `poQuality: 'Match source up to 1080p'`,
  `poAudio: 'Stereo'`, `poCeilingVal: '6'`. **V21 must invent these deliberately.**
- **dbSteps** — `Connect · Preflight · Backup · Migrate · Verify · Restart`; lede *"Six steps,
  reversible until the switch-over. Your SQLite file is never deleted."*
- **dbChoices** — SQLite **RECOMMENDED** *"A single file on disk… right for a homelab · run exactly
  one instance"*; PostgreSQL *"…Worth it only if you already run Postgres…"*.

### 2i.4 Two consequences for the phase list

1. **The member nav is a different list, not a filtered one** (4 items, including `Request a channel`
   and `My requests` which don't appear for admins). V14's rename must model two navs, and it
   reinforces **A2** — "My requests" is a first-class member surface, not a variant of Board.
2. **Playout backend is per-channel** (`poScopeCopy`), so `playout.backend` needs a per-channel
   override alongside the global default — folded into V4's gate.

## 3. Standing precedence — unchanged, and it matters more now

`design/README.md` already states this, and the v2 mocks do not supersede it:

> The prototypes are **NOT authoritative for structure / information architecture.** Where a
> prototype's structure disagrees with `docs/design.md` §12, **§12 wins.**

That rule was written because the v1 console mock and the shipped app silently disagreed. The v2
mocks propose a *larger* structural change than v1 did, so the rule binds harder, not softer. The
mocks win on **look**; §12 wins on **what the page is**. Anything in §4 below that changes IA is a
**doc-first change to §12**, per AGENTS.md prime directive 1 — not a mock-driven refactor.

---

## 4. The delta

### 4.1 Navigation shell

| | Shipped (`app-shell.tsx:22-28`) | v2 desktop | v2 mobile |
| --- | --- | --- | --- |
| Items | 7 | 8 (`hint-placeholder-count="8"`) | 7 in sheet, 4 tabs (admin) / 3 (member) |
| Names | Channels · Board · Suggest · Filler · Users · Settings · Help | Dashboard · Guide · My requests · Suggest · Approvals · Filler · People · Settings | Dashboard · Guide · Queue · People · Settings · Help · Your account |
| Collapse | none — fixed `w-56` | `toggleNav`, animated `navW`, labels hide, badges degrade to a dot | rail → 252px sheet over a scrim |
| Extras | — | tuner-status strip: status dot + **2 copyable file chips** (M3U + XMLTV), ⌘K search button | role switch (`View as admin` / `View as member`) |

Net-new nouns: **Dashboard**, **Guide**, **People**, **Queue**. Renames touch routes, help copy, the
§12 surface map, e2e snapshots and every story baseline. This is the single largest ripple in the doc.

### 4.2 Desktop screens *(complete — §2's truncation was resolved; sizes re-measured against the full 502 KB file)*

| Section | Size | Status vs shipped |
| --- | --- | --- |
| LOGIN | 4.9 KB | evolved — "Only imported accounts can sign in." |
| WIZARD | 39.3 KB | **restructured** — 6 steps, Database is **step 1** |
| DASHBOARD | 40.9 KB | **net-new** |
| GUIDE | — | **net-new shape** — true time-grid |
| CHANNEL DETAIL | 44.0 KB | evolved |
| BOARD | 4.8 KB | reframed as "My requests" |
| SUGGEST | 15.4 KB | evolved (centered + split hero, as v1) |
| APPROVALS | 14.5 KB | evolved |
| FILLER + COVERAGE (F2) + CATALOG + DISCOVER (F3b) + SOURCES (F3a) | 24.8 KB | **expanded to 5 sub-surfaces** |
| PEOPLE | 12.0 KB | renamed + expanded |
| SETTINGS | 52.6 KB | **largest section**; adds System tab |
| *(tail)* | unknown | **unread** |

`TRANSMISSION (P-4) + faults (P-7)` is a **bare comment with no markup** — a placeholder. Its only
trace is Settings copy: *"raise it and watch Transmission."* Treat as unbuilt.

### 4.3 The four surfaces that carry the most new behavior

**Dashboard** (admin-only; members get a lockout reading *"That page is for admins"*). Panels:
restart-needed banner → 4 stat tiles → Transcoding (per-stream progress, mode chip, speed, buffer;
idle copy *"Loomarr only starts an encoder when someone tunes in, so an idle transcoder is the
healthy state."*) → Services (*"probed every 30s · same checks Settings runs"*) → Service control
(Reload / Restart with a typed confirm listing consequences) → Recent activity.

This surface implies **P-4 Transmission**, **live transcode telemetry**, and a **restart/reload
control** — none of which exist today. It is the largest net-new backend ask in the mock.

**Guide.** Rows = channels, columns = time; sticky tick header, absolute-positioned program blocks,
filler pods as sub-segmented strips, a red now-line, date picker with month popover, timezone, hour
stepper, zoom. Hover cards for program and `FILLER POD` (match score + why-line + per-clip quality).
Empty state *"Dead air"*. Legend: *"guide from Tunarr · updates itself on reconcile"*.

**Settings → System.** Sub-tabs (`hint-placeholder-count="4"`, but **five** implemented — Playout,
Database, Backup, About, Tasks). Database carries a full 6-stage migration stepper
(connect → preflight → backup → migrate → verify → restart) with the gate copy
*"A backup is required, not suggested — it's the only thing that makes this reversible."* and
row-count `MATCH` parity before switchover. This maps almost 1:1 onto Program Plan verification
item #4.

**People.** Import tab + **Local tab** with *"Create local account"* and
*"Loomarr stores this password itself — the only kind of account it can reset."*; per-row role,
quota, auto-approve, sessions drawer with `Revoke` / `Revoke all` and `Reset password…`.
This is the mock **answering decision #7 (local account management) as IN**.

### 4.4 Mobile

v2 mobile is a near-complete app, not the v1 three-screen slice. v1's `Channels / Board / Approvals`
labels **all disappear**. Deliberate desktop-only exclusions, each with in-mock copy:

- No compose/Suggest surface — *"Creating and editing channels lives on desktop — mobile is for watching, approving and quick checks."*
- Approve **yes**, edit-before-approve no — *"Editing a request before approving is desktop-only."*
- Restart renders as an inert `DESKTOP` pill; secret regeneration absent; add-person absent.
- **Account screen has full change-password** (current/new/confirm + *"Password changed — other sessions revoked, this one kept."*).

Mobile Settings shows six tabs — `Connections, AI, Defaults, Tasks, Security, All settings` — but
only three render bodies. Desktop's six are `Connections, AI, Defaults, System, Security, All
settings` (`stSecurity` / `stAll` confirmed in the full file, §2), so mobile substitutes `Tasks` for
`System` — surfacing the sub-tab directly rather than nesting it. Both confirm decisions #4 and #5.

This collides with defect **S11**: `app-shell.tsx:39` is still an unconditional `w-56` with no
responsive prefix. Mobile is claimed in `frontend-design.md` and absent in code; v2 now specifies it
in detail.

---

## 5. Defect register — re-verified at HEAD

The Program Plan's register is grounded in `9871fef`. Re-verifying all 26 entries at `55fc691`:
**5 already fixed, 1 partially fixed, 1 narrowed.** Roughly 20% staleness in 5 commits.

### Already fixed — remove from the plan

| # | Evidence at HEAD |
| --- | --- |
| **C1** | `channel-health.ts:15-16` keys on `pendingCount` with a comment naming the break-gap failure; regression test at `channel-health.test.ts:33-36`. |
| **C2** | `channels/$id.tsx:149-153` counts `programCount + pendingCount`, not slots. |
| **C4** | `internal/schedule/policy.go:49-55` documents `Source (llm\|operator)`; matches `policy_merge.go:95-97`. *(Register cites `internal/programmer/policy.go` — wrong path.)* |
| **C7** | `channel-lineup-editor.tsx:86-98` renders editable `seasonInput` + `onSeasonChange`. |
| **C9** | All four `pods/preview` refs in `design.md` (292, 293, 597, 617) read `POST`. |

Note **P2 of Wave 0 is done**: `pendingCount` and `breakCount` already exist on the DTO and drive health.

### Narrowed — C3 is now a one-field residual

Backend is fixed: `binder.go:149` delegates to `MergeFromProposal`, which skips any path in
`OperatorSet` (`policy_merge.go:33-52`; test named *"the exact data-loss bug"*). The frontend does
render a diff (`refine-review.tsx:29-46`) — but over **four** fields: Era, Audience ceiling,
Ordering, Seasonal. `MergeFromProposal` also refreshes **`Separation`** (`policy_merge.go:46-47`),
which is **never diffed**. A separation change still lands silently.

> **C3′ (new, small):** add `Separation` to `policyDeltas`. This is a handful of lines and closes the
> last of C3. It does not need the P3 phase the plan allocates.

### Still true — confirmed at HEAD

`C5` (strategy/group PATCHable, no UI) · `C6` (`AutoCurate` live in `app.go:375-380`, zero frontend
hits) · `C8` (`channels/index.tsx:26-27` states outright there is no hand-made create path) ·
`C10` (barrel is `web/apps/web/src/filler/index.ts:2`, **not** the path the register cites) ·
`S1` (`config.go:28` — `DATABASE_URL` has no `envDefault`) · `S2` · `S3` (`usersroutes.go:81` +
`user-row.tsx:58-66` still explain a password action that doesn't exist) · `S5` · `S6` · `S11` ·
`H1` (`docs/help/integrations.md:62-64` still says set `WEBHOOK_SECRET` and POST to `/hooks/arr`) ·
`H2` · `H3` · `H5` · `H6`.

**S7 partial:** endpoint live at `help.go:32-36`; only frontend reference is a test mock
(`reachability.test.tsx:129`). No user-facing version display.

**Path corrections for the register:** ChannelPolicy is `internal/schedule/policy.go`, not
`internal/programmer/`. The filler barrel is `web/apps/web/src/filler/index.ts`, not
`components/loomarr/filler/`.

---

## 5a. DECISIONS — ratified 2026-07-24

Answered by the maintainer. These are settled; §6 below is retained as the record of what was asked
and why. Where a decision went against the recommendation, the recommendation is kept alongside it so
the trade-off stays visible rather than being quietly overwritten.

| # | Decision | Outcome |
| --- | --- | --- |
| #3 | `/users` → `/people`, `Tasks` → `System` | **RATIFIED** |
| #4 | "Users & security" → "Security" | **RATIFIED** |
| #5 | "Advanced" → searchable "All settings"; merge into "Defaults" | **RATIFIED** |
| — | Channels → **Guide**, + **Dashboard** as landing | **RATIFIED** |
| #6 | Bootstrap-file config tier (`env > file > default`) | **ADOPTED** — doc-first amendment to `config-design.md:72` required in the same PR |
| #7 | Local account management in v1 | **IN** — closes S2 + S3 |
| #8 | Filler F3b live remote discovery | **v1** *(recommendation was v1.1 — accepted larger scope)* |
| #10 | Playout backend default | **Internal-first** *(recommendation was staged/Tunarr-first — see ⚠ below)* |
| #11 | Playout transport | **Both** HLS + MPEG-TS |
| #12 | Guide advertises mid-roll breaks | **No** |
| #13 | Ad-break detection scope | **Mid-roll-enabled channels only** |
| #14 | `/help` landing page | **RESOLVED by the full mock (§2): no landing page.** See below |
| D-A | Dashboard | **ALL IN**, including live transcode telemetry *(recommendation was to split — see ⚠ below)* |
| D-B | Wizard: Database at step 1 | **YES** (follows #6) |
| D-C | All wizard steps required | **NO** — Users stays `optional: true` (`steps.ts:25` unchanged) |
| D-D | Guide rename vs time-grid | **BUNDLED** — one PR *(recommendation was to separate — see ⚠ below)* |
| C#1 | Refine ownership | **No decision needed** — already fixed in code (§5) |
| C#2 | Rules semantics | **No decision needed** — already fixed in code (§5) |
| D-E | Image variants | **ONE APP** — no slim/full split, ever. See ⚠1 |
| D-E2 | ffmpeg in core (forced by D-E + #10) | **FOLD `filler` INTO THE ONE IMAGE** — core leaves distroless; §14/§16 amendment required. See ⚠1 |
| D-F | SSO / identity provider (§2d.1) | **SSO, BUT NO AUTO-CREATE** — allowlist invariant preserved. See below |
| D-G | ~20 new registry keys (§2d.2) | **ONE §15 AMENDMENT PR UP FRONT**, before any UI |
| D-H | `ChannelIconField` built but unmounted (§2d.5) | **MOUNT IT NOW** — standalone PR |
| **D-I** | **Internal playout reverses §1 (§2e)** | **AMEND THE DESIGN — Loomarr becomes a streamer.** The largest change in the program; Track T becomes the spine |
| D-E2′ | Image model, revisited on correct facts (§2e) | **FOLD ANYWAY — one 549 MB image**, regardless of D-I. Supersedes `design.md:795` two-tag model and the `:505` sidecar rationale |
| D-J | Guide grid data (§2f) | **NEW BACKEND PHASE REQUIRED** — `GET /v1/guide?from=&to=` with `kind`, gaps preserved, per-airing pod composition. V14 is not frontend-only |

### D-F — SSO as a third credential path, not a provisioning path

**Decided: build the identity-provider integration, but an SSO login still requires a pre-existing
allowlist row.** SSO becomes a **third credential path onto §11's owned identity**, exactly parallel
to how imported media-server accounts work today: the provider proves *who you are*; the `users` table
decides *whether you may enter and what you may do*.

This preserves every §11 invariant verbatim — *"a login attempt for a username with no matching row is
rejected"*, *"there is no lazy self-provision"*, and *"explicit import is the only way"* — while
adding a credential mechanism §11 simply hadn't contemplated.

**Cut from the mock as drawn:**

- ⛔ **"Create people on first sign-in"** (`auth.sso.auto_create`) — the lazy-provision hatch. Do not
  build; do not add the key.
- ⛔ **"Admin group"** (`auth.sso.admin_group`, placeholder `loomarr-admins`) — group-derived role
  assignment. Roles stay Loomarr-owned, consistent with the mock's *own* People-tab stance that
  *"Loomarr doesn't infer it from the media server."*

**Keep:** `auth.mode` (Loomarr sign-in always works alongside — never instead of, per the mock's own
copy), the provider/issuer/client-id/client-secret/scopes fields, the test flow, the *"What the
provider told us"* claims dump (valuable precisely for debugging *why* a claim didn't match an
allowlisted row), and **"Show break-glass URL"** (ties to the existing `api_token`).

**Required work:** a §11 amendment adding SSO as a credential path, stating explicitly that it does
**not** provision. §19's negative auth tests extend with: *an SSO identity with no allowlist row is
rejected* — the direct analogue of the existing media-server case. Per AGENTS.md, the negative cases
are part of the gate.

### D-G — one §15 amendment before any UI

`playout.*` + `backup.*` (+ `playout_token`) land in a single doc-first PR to §15 and
`internal/settings/declared.go`, giving Track T and the Backup UI a registry to build against.
`auth.sso.*` lands with D-F's §11 amendment, **minus** `auto_create` and `admin_group`.

Two items to resolve inside that PR rather than assume:

- **`server.public_url` vs `playout.public_url`.** The existing key is documented *"only needed for
  uploaded channel icons"* under `GroupTunarr`; the mock makes it segment-critical. Decide whether to
  **re-scope the existing key** (and rewrite its Doc + group) or **add a playout-specific one**.
  Re-scoping is likely right — two public-URL settings is a footgun — but it changes an existing key's
  contract, so it is a §15 decision, not an implementation detail. This is Program Plan fact **T3**.
- **The 6 DB-migration connection fields** are probably *transient wizard state*, not registry keys.
  Confirm before adding them.

**D-E (maintainer ruling, 2026-07-24): there is one Loomarr image, not a slim/full pair.**
Consequences: (i) the mock's wizard image-gate copy — *"This image has no ffmpeg… Run
`loomarr:latest` instead of `loomarr:slim`"* — describes a split that will not exist and **must be
cut from the wizard**; (ii) the existing `runtime` (distroless, no ffmpeg) vs `filler` (debian +
ffmpeg) split now needs resolving against Internal-first playout — see ⚠1.

### #14 resolved — and it closes three Help defects

The mock answers this without a decision being needed. **There is no `/help` landing page**: the
reader opens straight into a document (`s.helpPage || 'troubleshooting'` — Troubleshooting is the
default). So the *redirect* survives, but wrapped in a real reading surface:

- **Left rail, 230px** — a search input, placeholder **"Search help…"**, over a browse tree grouped
  as **"Start here"** (Quickstart · How Loomarr thinks · Asking for a channel) · **"Channels"**
  (Channels and lineups · Programming rules · Filler and breaks) · **"Running Loomarr"**
  (Integrations · Playout and transcoding · Database and backups · People and access) ·
  **"When something's wrong"** (Troubleshooting · Common questions).
- **Right rail, 184px sticky** — a per-page table of contents headed **"On this page"**, gated on
  more than two `h2`s.
- **Typed content blocks** — `h2, p, list, code, callout, quote, table, image`; code blocks get a
  lang chip + copy button; callouts are kinded **Note / Warning / Tip**. Footer: **"Next:"** link.

This directly closes register defects:

- **H3** (search matches titles only) — the mock's search is **full-text over block bodies**,
  deriving each hit's nearest preceding `h2` as its result heading, with a 96-char snippet. Empty
  state: *"Nothing matched "…". These are the pages that usually help:"* + three shortcuts.
- **H4** (nothing documents editing a channel) — the browse tree adds **"Channels and lineups"**,
  **"Programming rules"** and **"Filler and breaks"**, i.e. the app's central noun finally gets docs.
- **H5** (14px body copy, no max-width) — the two-rail layout bounds the measure structurally.

Caveat: only two documents are actually authored in the mock (`troubleshooting`, `quickstart`), so
the nav tree is aspirational — the other ten are labels without bodies. Treat the tree as the
**content plan**, and note it names `playout` and `database` pages, which per the Program Plan's
own rule are written *in the PRs that ship those features*.

One piece of orphaned markup: a vestigial `helpGridVisible` card grid sits **outside** the `rHelp`
block, listing the *old* v1 doc set (Quickstart · Integrations · Concepts · Member guide · Filler
guide · Troubleshooting). Dead in the mock; do not port it.

### ⚠ Three consequences to plan around

These follow from decisions that went beyond the recommendation. None is a reason to revisit — they
are scope facts the sequencing must absorb.

1. **Internal-first playout breaks the distroless-core boundary.** *(Revised after the maintainer's
   "one app, no slim/full variants" — that ruling kills the mock's image gate, but exposes a harder
   constraint underneath.)*

   The mock's gate copy (*"Run `loomarr:latest` instead of `loomarr:slim`"*) describes a split that
   **does not exist and will not be built**. Cut it from the wizard. But the repo already has a
   different two-image split, and it is the real obstacle:

   | Stage | Base | ffmpeg |
   | --- | --- | --- |
   | `runtime` — the app, published as `loomarr:latest` | `gcr.io/distroless/static-debian12:nonroot` | **none** |
   | `filler` — the §10 ingest sidecar | `debian:stable-slim` | vendored (yt-dlp/ffmpeg/deno) |

   `Dockerfile:115` states the intent outright: ffmpeg is vendored **solely** so yt-dlp can merge
   streams — *"Loomarr never probes media"* — and `Dockerfile:58` frames filler as a deliberately
   separate image. This matches the standing rule that ingest tooling never enters core.

   **Internal playout needs ffmpeg in the core runtime, which distroless/static cannot carry.** So
   "one app" forces a §14/§16 decision that is not an implementation detail:
   - **(a)** move `runtime` off `distroless/static` onto a base that carries ffmpeg — one image, but
     core loses the distroless posture (no shell, minimal CVE surface) §16 chose deliberately; or
   - **(b)** fold the `filler` stage into the single published image and have Internal playout use
     that ffmpeg — one image, larger, ingest+playout tooling unified; or
   - **(c)** keep Tunarr as the playout backend, where Tunarr owns the encoder and core stays thin.

   **DECIDED 2026-07-24: (b) — fold `filler` into the single published image.** One `loomarr` image
   carrying the binary + ffmpeg + yt-dlp + deno; playout and ingest share the vendored ffmpeg.

   **What this costs, stated plainly so §16 can be amended honestly:**
   - **Core leaves `distroless/static`.** It gains a shell and a package manager's worth of CVE
     surface. §16 chose distroless deliberately; that choice is now reversed and the doc must say so
     rather than quietly drop it.
   - **`USER nonroot:nonroot` must be preserved explicitly.** distroless gave it for free
     (`Dockerfile:165`); a debian base does not. Losing it would be a silent privilege regression.
   - **`HEALTHCHECK` can change shape.** `Dockerfile:162-164` routes around distroless's missing
     shell and notes *"a `loomarr healthcheck` subcommand can replace this later"* — with a shell
     present that constraint lifts, but the current compose HTTP check keeps working either way.
   - **Image size grows roughly an order of magnitude** (~30 MB → ~250 MB+), driven by ffmpeg.
   - **Licensing is already handled** — the bundled ffmpeg is a BtbN **GPL** build and
     `THIRD_PARTY_NOTICES.md` carries the source offer (`Dockerfile:137`). Folding it into the
     primary image makes that offer *more* prominent, not newly required. Re-read it during the PR:
     the notice's wording assumes ffmpeg ships in a filler-specific image.
   - **`ffprobe` is still deliberately excluded** (`Dockerfile:115` — ~99 MB, and Tunarr assigns
     duration during its `local`-source scan). **Internal playout may reintroduce the need**: owning
     the encoder likely means needing to probe media Loomarr previously never inspected. Decide
     during Track T, and if it comes back, record why — the current comment will otherwise read as
     contradicted.

   Combined with #11 (both transports), Track T moves from "parallel track with stopping points at
   T3/T7" to **critical path**: with Internal as the default, first-run setup cannot complete until
   Internal playout works.
2. **Full Dashboard means a new data source, not a new panel.** Loomarr delegates playback to Tunarr
   and never sees an encoder today — `transcod|encoder` matches only `json.NewEncoder`. Live
   transcode telemetry (per-stream progress, mode, speed, buffer) only has a source once Internal
   playout owns the encoder, so **D-A is now coupled to Track T** and cannot land before it. The
   cheap half (restart/reload control + service probes, closing S5) can still ship early and should.
3. **Bundling the Guide rename with the time-grid means the rename doesn't ship until the grid
   works.** The grid is the largest genuinely-new build in the mock (ruler, rail, duration-scaled
   blocks, filler pods, pending slots, now-line, zoom — none of it exists). Every PR landing on
   `/channels` in the meantime is a PR that gets renamed later. Keep the bundled PR tight, or the
   rename debt compounds.

## 6. What the mocks decide that the maintainer hasn't *(historical — answered in §5a)*

The mocks render several Program Plan §2 decisions as already-settled. Each needs an explicit
ratification or rejection, because building the mock as drawn *is* choosing.

| Plan decision | What the mock assumes | Needs |
| --- | --- | --- |
| #3 rename `/users` → `/people`; `Tasks` → `System` | **Both done**, and System holds 5 sub-tabs | ratify |
| #4 "Users & security" → "Security" | Mobile names a `Security` tab | ratify |
| #5 "Advanced" → searchable "All settings" | Mobile names an `All settings` tab | ratify |
| #7 local account management in v1? | **In** — People has a Local tab, "Create local account", "Reset password…" | scope call |
| #8 F3b live remote discovery v1 or v1.1? | **In** — `DISCOVER (F3b)` is a built sub-surface | scope call |
| #10 staged playout default | Wizard step 3 offers internal vs Tunarr with an **image gate** (*"This image has no ffmpeg… Run `loomarr:latest` instead of `loomarr:slim`"*) | ratify |
| #11 HLS / MPEG-TS / both | Not resolved in the read portion | still open |
| #14 `/help` real landing page | Mobile has a Help screen with page picker + ToC; desktop unread | resolve after §2 tail |

**Four further decisions the mocks force that the plan does not list:**

- **D-A. Dashboard is a new product surface,** requiring live transcode telemetry, service probes and
  a restart/reload control. That is a backend workstream of its own, adjacent to Track T. In or out?
  Note the telemetry half is the larger ask: Loomarr delegates playback to Tunarr and **never sees an
  encoder today** (`transcod|encoder` matches only `json.NewEncoder`). Live transcode stats mean a new
  data source, not a new panel.
- **D-B. Wizard reordering puts Database at step 1** (*"The one choice that's awkward to change later,
  so it comes first."*). Sound, and it depends on the bootstrap-file tier (decision #6).
- **D-C. Are all wizard steps required?** The mock says yes and declares the divergence (§2b);
  `steps.ts:25` says Users is `optional: true`. Behavior ⇒ `design.md` decides, not the mock.
- **D-D. Does the Guide replace the channel list, or join it?** This is the largest genuinely-new
  build in the mock (§2a) — a time-grid with ruler, rail, duration-scaled blocks, filler pods,
  pending slots, now-line and zoom, none of which exists. It is plausibly a phase on its own. If the
  IA rename lands first (§8), the shipped card list can keep working under the name *Guide* while the
  grid is built behind it — decoupling the cheap rename from the expensive surface.

---

## 7. Companion plans are missing from the repo

The Program Plan routes all per-phase detail to five companions. **None are in the repo.** Only
`docs/engineering/channels-refinement-2026-07-24.md` (32 lines) exists.

Missing: `channels-plan-amendments.md`, `filler-evaluation-and-plan.md`,
`build-plan-settings-users-database.md`, `design-brief-settings-users-database.md`,
`help-build-plan.md`, `help-design-brief.md`, `playout-build-plan.md`, `playout-design-brief.md`.

Until they land, the Program Plan is a table of contents pointing at empty chapters. Either import
them or fold their content in — but do not start a phase whose detail doc doesn't exist.

---

## 8. Sequencing — pre-users

**Premise (2026-07-24): the app has no users. It is in active development.**

This inverts the ordinary defect-first ordering, so it is stated explicitly rather than assumed.

**What it changes.** The register's `L` severity means "live user-facing failure" — an argument for
fixing those first *because every day they are broken is a day of real harm*. With no users that
premise is void. `S1` and `H1` remain real defects; they are no longer urgent ones.

It also removes the main argument against the expensive change. The IA rename
(Channels→Guide, Users→People, +Dashboard) is costly because it touches routes, help copy, the §12
surface map, e2e snapshots and every story baseline. But **half that cost is a users cost** —
migration paths, deprecation windows, broken bookmarks, support burden, compatibility shims. None of
those exist here. `/users` → `/people` is a rename plus baseline regeneration, not a product event.

So the dominant remaining cost of a structural change is **rework of work already done**. That flips
the rule: do the IA change **before** building more surfaces on top of the old IA. Every PR that
ships against `Channels`/`Users` is a PR that gets paid for twice.

**What it does not change.** Four things hold regardless:

1. ~~The truncation~~ — **resolved 2026-07-24** (§2). The full desktop mock is installed; absence
   claims are now safe to make.
2. **The decisions in §6** — no-users does not answer them, it only makes them cheaper to act on.
   *(Answered 2026-07-24 — see §5a.)*
3. **The missing companions** (§7) — still empty chapters.
4. **The precedence rule** (§3) — §12 still wins on structure. "No users" is not licence to let the
   mock silently redefine the IA; it is the reason the doc-first update is *cheap*.

One caveat: no-users means no *external* users. The maintainer is still a user, and the manual smoke
on the live homelab remains half the definition of done (AGENTS.md) — it is where the last two rounds
of real bugs surfaced.

### Prerequisites — both now cleared

- ~~Recover the full desktop mock~~ — **DONE** 2026-07-24 (§2).
- ~~Answer the decisions~~ — **DONE** 2026-07-24 (§5a); the interior pass added D-F/D-G/D-H (§2d).

### Gated phases — ⚠ SUPERSEDED by [`v2-build-plan.md`](archive/v2-build-plan.md)

> This table is the working copy the phases were derived in. **The build plan is now the maintained
> version** — it groups the same 39 phases by track and readiness rather than by discovery order.
> Retained here because the gates were written alongside the evidence that motivated them; if the two
> tables ever disagree, the build plan wins and this one is stale.

AGENTS.md requires **one phase per PR** with a **hard gate** recorded in `PROGRESS.md`. What follows
is that phase list. Each row is one PR. `make check` · `make fe` · `make e2e` green is assumed
throughout and not restated; the **Gate** column is the *additional* proof specific to that phase.
Nothing here starts before its dependency lands.

| # | Phase | Depends on | Gate (the additional proof) |
| --- | --- | --- | --- |
| **V0** | `S1` — `DATABASE_URL` `envDefault` | — | A test asserting zero-env boot resolves a store path; `docker run` with no `-e` reaches the wizard |
| **V1** | `D-H` — mount `ChannelIconField` on the info panel | — | Story + visual baseline for the info panel with the icon field; the §12 surface-map row for channel icons cites the mount |
| **V2** | `C3′` + `C10` + **`A4`** — Separation in the refine diff; drop the dead `channel-pods` export; delete the stale `TODO(learning)` docblock at `ladder.go:57` | — | A refine test where **only** `separation` changes and the diff renders it; story-coverage gate green with `channel-pods` gone; no `TODO(learning)` above an implemented function |
| **V2b** | **`D-I` — the design amendment, docs only.** §1 (identity), §10 (mid-roll in scope), §14 (ffmpeg + ffprobe core), §16 (one image), §6/§9 (own M3U/XMLTV), §7.1+§11 (token-auth segments) | — | **No code.** Every section in D-I's table amended; `:39`, `:505`, `:530`, `:795`, `:942` each either rewritten or explicitly struck; the §11 amendment covers segment token-auth *and* D-F's SSO path together |
| **V3** | `D-E2′` — single 549 MB image, doc-first | V2b | §16 amended in-PR; image builds; `USER nonroot:nonroot` asserted; `ffmpeg -version` **and `ffprobe -version`** succeed; `THIRD_PARTY_NOTICES.md` re-read for the folded layout |
| **V4** | `D-G` — §15 amendment: `playout.*` + `backup.*` + `playout_token` | V2b, V3 | §15 updated **before** `declared.go`; `make config-docs` diff empty; registry round-trip tests; `server.public_url` re-scope-or-add decided **in this PR**; log-grep redaction test for `playout_token`; **`playout.backend` supports a per-channel override** (§2i.3 `poScopeCopy`), not just a global default |
| **V5** | `#6`/`D-B` — bootstrap-file config tier (`env > file > default`) | V4 | Resolution-precedence tests across all three tiers; `config-design.md` amended (the *"UI never edits the bootstrap set"* clause) |
| **V6** | Track T: Internal playout to first frame (T1–T3) | V3, V4, V5 | A native channel in the media-server guide **playing a test card**; both transports (#11) covered; `StaleLoomarrListings` still cleans up after retargeting (fact **T2** — it identifies Loomarr's provider by its *Tunarr-shaped path* today); segment requests **reject an absent/wrong `playout_token`** |
| **V7** | `#7` — local accounts: create + change password (closes **S2**, **S3**) | — | The bootstrap admin changes their own password; §19 negatives (member 403s, session death on disable) extended; S3's row labels become true |
| **V8** | `D-F` — SSO as a credential path, **no auto-create** | V4, V7 | §11 amended stating SSO does **not** provision; **an SSO identity with no allowlist row is rejected** (the §19 negative); no `auto_create`/`admin_group` key exists in the registry |
| **V9** | Settings IA — 6 tabs + System sub-tabs + cross-tab save bar | V4 | Dirty state survives tab switches and Save commits across tabs; the four inline-commit exceptions (§2d.3) verified as exceptions |
| **V10** | All-settings search surface | V9 | Search matches key **and** group **and** value; `ADV` chip reflects `Setting.Advanced`; empty-state copy present |
| **V11** | System → Database migration stepper | V5, V9 | An established SQLite install migrates to PostgreSQL with **row-count parity**, and rolls back by reverting one config line; backup gate cannot be skipped |
| **V12** | System → Backup UI (closes S6) + About (closes S7) | V4, V9 | Backup downloads; retention honored; version renders from `GET /v1/system/version` |
| **V13** | Restart/reload control + service probes (closes S5) | V9 | `RestartRequired` implemented and surfaced; reload re-probes without downtime; restart confirm lists consequences before acting |
| **V13b** | **`D-J` — the guide endpoint** (§2f): `GET /v1/guide?from=&to=`, multi-channel, **gaps preserved**, `kind` discriminator (`program`\|`pending`\|`pod`), per-airing pod composition (+`quality`, +`era`, `matchLevel`, `why`), program metadata (episode title, year, rating, genre, runtime, acquisition provenance), guide timezone + retention window | V6 | A window spanning past **and** future returns per-channel timelines; a pending slot and a filler pod are **distinguishable** (today `gap bool` cannot); `Upcoming`'s gap-filtering is not reintroduced; retention boundary enforced |
| **V14** | IA rename + Guide time-grid (**bundled**, per D-D) | V13b | §12 updated **first**; `/guide` + `/people` routes; **two distinct navs** (admin 7 items; member 4, incl. `Request a channel` + `My requests` — §2i.3), not one filtered list; grid renders duration-scaled programs, filler pods, pending slots, now-line, zoom; hover cards render every field V13b exposes; e2e + visual baselines regenerated |
| **V15** | Help rebuild — two-rail reader, full-text search (closes **H1**–**H6**) | V14 | `grep -ri "hooks/arr\|WEBHOOK_SECRET" docs/` returns **nothing** (H1, H2); search hits body text (H3); channel-editing docs exist (H4); measure bounded (H5); links use `tune`, not `suggest` (H6) |
| **V16** | Dashboard incl. transcode telemetry (D-A) | V6, V13 | Live per-stream telemetry from the Internal encoder; member sees the lockout, not a 403 wall; **`restartFacts[0]` is rewritten** — *"Channels stay on air — Tunarr streams them, not Loomarr"* is false after D-I (§2i.3) |
| **V17a** | `F1` — read the sidecars: parse `--write-info-json`, stop passing the provenance enum as the LLM's source description | — | A tagging test asserting the prompt carries real metadata, **not** the literal `"tunarr-local"`; sidecar fields reach the tagger |
| **V17b** | `F2` — clip previews (thumbnail/poster on `ClipCard`) | V17a | `ClipCard` renders a preview; visual baseline; no remote URLs in stories (inline `data:` URIs only) |
| **V17c** | `F3` — a quality dimension beyond `INGEST_PREFER_ORIGINAL` | V4, V17a | Quality is a first-class field on a clip, surfaced in the catalog and usable in selection |
| **V17d** | Filler: starter pack + coverage/"Find clips" (closes **F4**) | V17b | Seeded clips reviewable with keep/exclude; a thin channel surfaces the gap and routes to acquisition |
| **V18** | Mobile responsive (closes S11) | V9, V14 | AppShell collapses; the mobile v2 screens render at 375px; desktop-only actions render as disabled affordances, not dead ends |
| **V19** | Per-title refine rationale (`why`) | — | The suggester emits per-title rationale; the diff renders it |
| **V20** | Wizard: **Database** step (net-new, becomes step 1 per D-B) | V5, V11 | Choice persists across the restart-and-resume; `setup/state` survives a DSN switch (it currently lives *in* the database being migrated); the SQLite path is unchanged and default |
| **V21** | Wizard: **Playout** step + transcode check (net-new) | V6, V20 | Encodes a real 15s test pattern and reports per-encoder verdicts; progress streams over `/v1/events`; **the `IMAGE GATE` copy is absent** (one image — D-E2′); ffmpeg stderr tail renders on failure. **Note: `tcOptions` + `poPresets` were never authored in the mock (§2i.3)** — the encoder list and quality presets are invented in this PR, deliberately |
| **V22** | Wizard reconciliation: `bk.summary`, persisted `skipped`, strike the dead §20 bullet (**S12**) | V20, V21 | `skipped` survives a refresh (today React-state only); `design.md:940` struck; per-connection summary line renders |
| **V23** | **`A1`** — deny-reason UI (the API field already exists, unused) | — | Denying requires or offers a reason; it reaches `Proposal.DenyReason`; the requester sees it. No call site sends `data: {}` |
| **V24** | **`A3`** — surface existing-but-hidden proposal data | — | `channelName`, `eraBalance`, `overall` render; `mustInclude`/`mustExclude` get inputs; the dead `onEditItem` prop is either wired (V25) or removed |
| **V25** | **`D-K`** — edit-before-approve, one chokepoint | V4, V24 | `POST /approve` takes a body; **`suggest.Approve` remains the sole implementation** (a test asserts no second acquisition path); `modSummary`+`note`+`approvedBy` persist; §19 negatives still hold (member 403s on approve) |
| **V26** | **`A2`** — "My requests": per-user proposal list + admin-edit provenance | V25 | A member sees their own submitted/denied/edited requests; *"CHANGED BY … "* renders; the denial line shows; `GET /v1/proposals` supports a per-user scope |
| **V27** | Approvals queue as its own surface: tabs (pending/in-flight/history), bulk approve, audit rows | V25, V26 | Tab counts correct; bulk approve goes through the same chokepoint; history rows carry timestamps (`approvedAt` — not currently in the DTO) |
| **V28** | Filler `sources` entity + clip metadata columns | V4, V17a | Migration `00013`; sources CRUD + per-source `Fetch now`; clip thumbnail/quality/usage populated; `ClipDTO.Source` becomes a real reference |
| **V29** | Filler **Coverage** meter (F2 banner) | V28 | **Consumes `internal/filler/ladder.go`** — a test asserts the meter and pod assembly agree; thin coverage routes to Discover |
| **V30** | Filler **preview serving** (catalog + discover) | V6, V28 | Clip previews stream; no remote URLs in stories (inline `data:` URIs only, per the visual-story rule) |
| **V31** | Dashboard **Services** panel — aggregating wrapper + 30s poll | V13 | **Reuses `POST /v1/setup/test`** (a test asserts one probe implementation); "Fix →" routes to the failing block |
| **V32** | Dashboard **Recent activity** feed | V31 | A persisted activity feed (not just live SSE); survives restart |
| **V33** | `#8` — F3b live remote discovery: `GET /v1/filler/discover` (the mock names the route) + the Sources registry | V17c, V28, V30 | The Archive.org contract is a **pinned testkit fixture**; discovery never runs in unit tests; license badges render; batch-select downloads into the drop folder |

**Not scheduled, deliberately — three capability-without-UI defects the mock does not solve:**

| # | Defect | Why it has no phase |
| --- | --- | --- |
| `C5` | `strategy` + `group` are PATCHable with no UI, while `policy.ordering` offers "Inherit channel default" — inheriting from an invisible field | **The v2 mock shows no control for either** (§2d.5). Note the inheritance copy is actively misleading today, so this is worse than a plain gap. |
| `C6` | `policy.autoCurate` is live (`app.go:375-380`) with **zero** frontend references | **The mock shows no toggle.** |
| `C8` | `POST /v1/channels` supports hand-made channels; `channels/index.tsx:26-27` states outright there is no such surface | The mock's only create path is the AI composer, matching today. **Possibly not a defect at all** — §12's "origination-vs-evolution" model calls hand-made channels *"express doors into the same object, not a separate create screen"*, so this may be working as designed. |

All three need a **design decision before a phase**, and inventing one here would be exactly the guess
this document exists to avoid. Each is a live violation of the §12 surface-map rule ("every channel
capability has exactly one home"), so they should be *decided*, not indefinitely deferred — the
honest options are "add the UI", "remove the capability", or "document it as API-only".

**Ordering notes.**

*(Dependency graph verified mechanically: 39 phases, acyclic, no dangling references, every register
defect mapped. The blocking counts below are transitive-closure measurements, not estimates.)*

- **Nine phases are free** — no dependencies, no pending decisions: **V0, V1, V2, V2b, V7, V17a, V19,
  V23, V24**. Any of them can start today.
- **V2b is the critical-path head by a wide margin: 28 of 39 phases sit transitively behind it.**
  It is **docs only** — the D-I design amendment. That makes writing it the single highest-leverage
  task in the program, and it needs no code. V3 blocks 27, V4 blocks 26, V5 blocks 12.
- **The spine is V2b → V3 → V4 → V5 → V6.** After D-I, Track T is no longer a parallel track with
  optional stopping points — it gates the guide, the Dashboard, and two wizard steps.
- **V7 (local accounts) is fully independent** of that spine and can run in parallel throughout.
- **V17a is independent and fixes a live correctness bug** (every clip's LLM prompt currently reads
  *"Source description: tunarr-local"*). Good parallel work.
- **V14 is the rename-debt clock.** Every PR touching `/channels` before it lands gets renamed
  later — but D-D bundled it with the grid, and the grid now needs V13b, which needs V6. So the debt
  runs longer than the bundling decision assumed. If that debt becomes painful, the escape hatch is
  to revisit D-D and split the rename out; the plan does not assume you will.
- **V19 and V17a–e** carry no dependency on the spine and can fill gaps whenever the critical path
  is blocked on review.

**Correct the Program Plan's register** (§5) whenever it is next edited: drop the 5 fixed entries,
narrow C3 to C3′, fix the two stale paths, re-date to `55fc691`.
