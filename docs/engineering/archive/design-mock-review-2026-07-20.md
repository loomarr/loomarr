# Design-mock review — build vs. `design/` prototypes (2026-07-20)

> **ARCHIVED — a dated snapshot, not a to-do list.** Superseded by the v2 prototypes; see
> [`design/README.md`](../../../design/README.md).

A screen-by-screen comparison of the **running app** (Vite dev server + Go backend,
SQLite, no external deps) against the authoritative Claude Design prototypes
(`design/loomarr-prototype-desktop.dc.html`, rendered dark at 1280×800).

**Method:** each mock screen was rendered (a local copy patched to boot into
`screen:'app'` so post-login routes were reachable without walking the faux wizard) and
captured alongside the live screen. Findings are grounded in the design docs, not taste.

## The lens that matters

Per `CLAUDE.md` precedence: **the prototypes win on *look*; `docs/design.md` +
`config-design.md` win on *behavior/IA*; a prototype that contradicts the design doc gets
corrected, not followed.** The prototypes are a 2026-07-13 seed and the docs/code have
moved past them in several places. So every delta lands in one of three buckets:

1. **Real bugs** — the app is wrong; fix the app.
2. **Visual-fidelity gaps** — the prototype is authoritative (frontend-design.md §1/§2/§219
   mandate pixel-perfect recreation); build the missing treatment.
3. **Mock is stale** — the build is correct and current; matching the mock would *regress*.

Screenshots for every row are in `.cmp-review/` (gitignored) as `mock-*` / `live-*` pairs.

---

## 🟥 Bucket 1 — Real bugs (fix the app)

### B1. Board page 400s against the real backend

- **Where:** `web/apps/web/src/routes/_authed/board.tsx:24` —
  `const titles = titlesApi.useListTitles();` (no `state` argument).
- **Symptom:** live `/board` renders an **ErrorState** card ("Bad Request — state query
  param is required — Try again") instead of the stage list / "Nothing in flight" empty
  state. The mock shows the graceful empty state.
- **Root cause:** `GET /v1/titles` **requires** `state` (returns **400** without it;
  `?state=wanted` is accepted). The Board page's design assumes one call returns titles
  across *all* states (it filters `stageOf(t) === stage` client-side at
  `board.tsx:63`), but the endpoint is one-state-per-call.
- **Why no gate caught it:** `web/apps/web/src/test/channels-board.test.tsx:57` and
  `test/reachability.test.tsx:176` both mock `/v1/titles` to return `{titles: […]}` for
  **any** URL, so the param-less request the real Huma handler rejects looks fine in CI.
  This is the recurring "mock more lenient than the server" class (cf. the `GET /v1/users`
  panic `0dc957e`, the empty-env bug `be860bc`).
- **Fix shape:** fetch the relevant states (`wanted`, `requested`, `downloading`,
  `available`, `unavailable`) and merge, or add a multi-state param to the endpoint
  (design-doc change first). Regression test must use a **state-aware** mock that 400s on a
  missing `state`, so it actually reproduces.
- **Severity:** high — a shipped, nav-reachable page is unusable against a real backend.

---

## 🎨 Bucket 2 — Visual-fidelity gaps (prototype is authoritative — build these)

Confirmed against `docs/frontend-design.md`: §1 (Test Card concept + "CRT/scanline/static
flourishes appear only on idle surfaces — onboarding, empty states, the login screen"),
§2.1 (palette/accents), §2.2 (Geist), §219 ("recreate them pixel-perfectly… gallery
baselines are judged against the prototypes-plus-deltas").

### V1. The test-card identity block is missing everywhere

The **SMPTE color-bar motif** — the literal thematic anchor of the whole design — appears
nowhere in the build.

- **Login / wizard header:** mock has a **7-segment color-bar band** (see spec below)
  above a large **`LOOMARR`** wordmark + tagline **"always something on"**. Build
  (`wizard-shell.tsx`) has a `<Radio>` lucide glyph + lowercase "Loomarr" + "· first-run
  setup".
- **App sidebar brand lockup:** mock has the mini color-bar band beside `LOOMARR`; build
  has only the radio glyph + "Loomarr".
- **Empty states:** mock's "Dead air" empty state carries a color-bar strip; build has none.

**Extracted color-bar spec** (from the rendered mock):
- 7 segments, flex row, no gap, no border-radius, **14px** tall, ~28.5px each (~200px total).
- Colors = the accent tokens: `#FFB020` signal · `#F5D90A` caution · `#3DD68C` lock ·
  `#4CC9E8` tune · `#D6409F` suggest · `#E5484D` onair · `#8B93A3` static-400.

**Wordmark spec:** Geist, 32px, weight 700, letter-spacing 2.56px, color `#E7EAF0`,
centered (login) — self-hosted Geist is already in the build per §2.2.

### V2. CRT / TV-static texture missing on idle surfaces

- **Where expected:** login + wizard + empty states (§1).
- **Mock implementation:** a full-bleed `position:absolute; inset:0` div,
  `pointer-events:none`, **opacity `0.09`**, background =
  `<svg><filter><feTurbulence type="fractalNoise" baseFrequency="0.9" numOctaves="2"
  stitchTiles="stitch"/></filter><rect filter=…/></svg>` (300×300 tile, `repeat`).
- **Build:** flat `bg-background`, no texture.
- **Constraint:** must be disabled under `prefers-reduced-motion` **and** in visual-test
  mode (§1/§17 determinism), like every other flourish.

### V3. Suggest submit button uses the wrong accent

- **Mock:** the "Suggest" button is **magenta** (`suggest #D6409F`) — §2.1 assigns magenta
  as "**the AI color**: intent input focus, generation progress, proposal accents".
- **Build:** "Suggest a lineup" button is **amber** (`signal`, the primary action color).
- This is a token misuse, independent of the layout difference (mock is a centered hero;
  build is a standard page header — that framing difference is a lesser, defensible delta).

---

## ✅ Bucket 3 — Mock is stale; the build is correct (do NOT match the mock)

Matching the app to the mock here would **reintroduce superseded designs**.

| Screen | Mock (stale) | Build (correct) | Authority |
| --- | --- | --- | --- |
| **Filler** | "synced from Emby's Commercials library"; "enable the loomarr-ingest **sidecar**" | "Tunarr scans it directly — no media-server library involved" | §10 filler redesign; sidecar deleted (#28, `loomarr-filler-redesign`/`loomarr-ingest-is-go`) |
| **Users** | "Sync from Emby / everyone signs in with them" | import-only + local bootstrap admin ("Local account", "YOU", "created at first-run setup"), inline role/quota/auto-approve/sessions/disable | §11 identity rework (`loomarr-auth-rework`) |
| **Settings** | one giant scroll (connections + secrets + playback + danger) | **6-page IA** (Connections/AI/Channels&playback/Filler/Users&security/Advanced), one save bar per page | config-design §5 (authoritative on Settings IA) |
| **Help** | 6 stub cards (one cites the dead sidecar) | full searchable embedded doc reader, accurate copy | design §13; built #30/#34 |
| **Wizard** | 5-step accordion (Owner/Connect/Webhook/LiveTV/First channel) | 7-step "wizard-*is*-settings-forms" flow | design §571 + config-design §6 (explicitly *supersedes* the mock's model) |
| **Approvals** | top-level nav item + own "Approval queue" screen | folded into the Suggest page ("Awaiting approval") | design §268: "there is **no create-a-channel screen**… describe → review → approve". Acceptable variation, not a defect. |

---

## ⚠️ Bucket 4 — Process note: the visual gate has a coverage hole

`make fe-visual` is described as "the transmitted test card" (§5) — yet it is **green**
while the color-bar test-card identity (V1) is absent. That means its `/__gallery`
baselines either don't cover the brand header / login, or were captured from the React app
itself (so they enforce *whatever was built*, not the prototype). Worth confirming: the
visual suite is not currently guarding the thing it's named after.

---

## Not captured — RESOLVED 2026-07-21 (see follow-up below)

The three screens flagged here needed store state / overlays the first pass hadn't set up.
They are now captured and bucketed in the **2026-07-21 follow-up** section at the end of this
doc. Original list, for the record:

- **Channel detail** — needed a channel to exist (store was empty). → now captured.
- **Member intro**, **User drawer** — overlay states. → now resolved (one build-gap, one
  stale-mock).
- **⌘K command palette** — *was* checked: good parity (centered modal, "Search titles,
  channels, help…", ESC affordance, empty-prompt copy all match).

## Environment for this review

Live app: `go run ./cmd/loomarr` (SQLite in scratch) on `:8080` + `pnpm --filter
@loomarr/web dev` on `:5173` (Vite proxies `/v1` → `:8080`). Mock: `python3 -m
http.server` over `design/` (and a patched copy) rendered via `support.js`. Both dark,
1280×800. The app was bootstrapped (owning admin created) so wizard step 1 was passed.

---

## Follow-up (2026-07-21): the three uncaptured screens

The first pass couldn't capture Channel detail / Member intro / User drawer because they
need store state or overlays. To capture them **against a real backend** (not a mock, not a
hand-built fixture), `make seed` was implemented — a gate-respecting dev-store populator
(`cmd/seed`, see the seed note at the bottom). With it, a believable slice of state exists:
an admin + a member, titles across every provisioning stage, a live channel with a real
computed lineup, and a filler catalog. All three screens were then driven live (Playwright)
and bucketed with the same lens.

## 🟥 B2. Channel detail — relaxation chips render raw machine values  ·  FIXED

- **Where:** `web/apps/web/src/routes/_authed/channels/$id.tsx` — the "Programming policy"
  section rendered each relaxation-ladder step as `{step.kind}: {step.from} → {step.to}`.
- **Symptom (captured live):** the chips read **`episodeNoRepeat: 30h0m0s → 24h0m0s`**,
  **`seriesMinGap: 24h0m0s → 0s`**, **`blockMax: 8 → unbounded`** — a camelCase slug plus
  Go-duration `.String()` output (`30h0m0s`) leaking straight into user-facing copy. The
  code's own comment wanted a friendly `"audience: TV-Y → TV-Y7"`; the values weren't
  humanized to match.
- **Root cause / whose fix:** the API sends the raw ladder output on purpose — `Duration`
  is a locked machine contract (`internal/schedule/policy.go`; `policy_test.go` asserts the
  `"168h0m0s"` wire form). That transport is correct; the bug is a **view** displaying a
  transport string verbatim. Same call as the Board fix: humanize on the side that owns
  presentation (the FE), don't destabilize the domain serializer every other consumer relies
  on.
- **Fix:** `humanizeRelaxation()` in `@loomarr/core` (`packages/core/src/format/format.ts`)
  — a fixed label map over the four ladder kinds (`episodeNoRepeat`, `seriesMinGap`,
  `blockMax`, `era`) + a Go-duration trimmer (`30h0m0s`→`30h`, `0s`→`none`) that leaves
  counts/ranges (`8`, `1990-1999`) untouched; unknown kinds fall back to the slug so a future
  ladder step still renders. Chips now read **`Episode no-repeat: 30h → 24h`**, **`Series min
  gap: 24h → none`**, **`Block max: 8 → unbounded`**. Unit-tested (`format.test.ts`), verified
  live, Biome/tsc/vitest green.
- **Severity:** low (cosmetic) but user-facing on a shipped page; a household operator should
  never read `30h0m0s`.

## ✅ Channel detail — mock vs build (the rest)

The mock's Channel detail (desktop prototype, `route:'chdetail'`) is a **demo-scripted**
screen and diverges from the build in ways that are *not* defects:

| Aspect | Mock | Build | Verdict |
| --- | --- | --- | --- |
| Primary action | **`Reconcile now`** | **`Rebuild now`** | Build correct — house-vocab rename (copy sweep #50, `reconcile→rebuild`). |
| Programming-policy / relaxations | **absent entirely** (mock has no such concept) | present (the B2 chips) | Build is *richer* — the relaxation ladder is a real §7/§9 feature the 2026-07-13 mock predates. |
| Lineup | per-slot list w/ tvdb keys + a "simulate library drift" demo button + state badges | slot **counts** ("4 of 4 slots have a real program") + honest "Not pushed to Tunarr yet" | Different framing; build's is production-honest, mock's is a scripted story. Defensible. |
| "Today's guide" EPG strip | horizontal program timeline + amber ad-pod blocks | **absent** | **Bucket 2 (mock authoritative)** — a genuinely nice visualization the build omits. Needs a live Tunarr guide to populate, so it's real future work, not a quick add. |
| Reconcile log | timestamped history of reconciler actions | **absent** | **Bucket 2** — candidate future work (an activity log per channel). |
| Pod policy editing | per-channel chips (breaks/hr, ads/pod, era, audience) | lives in Settings, not per-channel | Build's IA choice (config-design §5). Defensible. |

Net: one real bug (B2, fixed); the build is ahead of the mock on the policy/relaxation story;
the mock is ahead on the EPG strip + reconcile log (logged as Bucket-2 future work, not
defects).

## 🎨 V4. Member intro — SPECIFIED but NOT BUILT (build gap)

- **Design says build it:** `docs/design.md` §13 **"Member first-run"** → *"**First login
  intro** — one screen with the mental model: intent → proposal → submit → admin approves →
  titles are acquired → your channel appears in the TV guide. Sets the expectation that
  channels may start filler-heavy and improve as content lands."* The mock's `memberIntro`
  overlay (with `introSteps` + a `memberIntroSeen` once-only flag) is the realization of this.
- **Build:** there is **no** intro/welcome/onboarding surface — `grep -ri
  'intro|welcome|onboard'` over `apps/web/src` finds nothing member-facing. A member's first
  login lands them straight on a page with no mental-model primer.
- **Scope note:** this is the *one* unbuilt item of §13's four "Member first-run" features.
  The other three exist: **intent-writing hints** (the Suggest form's era/tone/runtime
  placeholders), **"My proposals" status** (the Board — captured live, "5 of 7 titles have
  landed"), and **channel-template plumbing** (`IntentInput` accepts `templates`, though the
  named starter intents aren't wired yet — a lesser, separate gap).
- **Verdict: Bucket 2 (prototype + design doc authoritative).** A real, well-scoped build
  gap. The mock is right; the build is missing a documented screen.

## ✅ V-drawer. User drawer — mock is stale; inline controls supersede it (verified live)

- **Mock:** a slide-over **`userDrawer`** for editing one user (role, quota, an auto-approve
  toggle w/ `drawerAutoBg`/`drawerAutoKnob` styling, sessions, disable).
- **Build (captured live on `/users`):** **every one of those controls is present INLINE on
  the user row** — Role dropdown, Quota spinner (with "N / cap" usage), Auto-approve checkbox,
  Sessions button, Disable button. Self-row controls are correctly disabled (you can't demote
  or disable yourself). The seeded member renders as **"Media-server account"** and the admin
  as **"Local account" + a "YOU" badge** — the reworked import-only identity model
  (`loomarr-auth-rework`) rendering exactly right.
- **Verdict: Bucket 3 (mock stale; build correct).** The drawer's job is done inline; it's a
  layout evolution, not a missing feature. This matches the original review's Users row
  (which already bucketed the mock's Emby-sync users as stale). No action.

## The seed (`make seed` / `cmd/seed`) — how these captures were made honestly

`make seed` was a stub (`exit 1`); it's now `cmd/seed/main.go`. The binding constraint
(CLAUDE.md do-nots): **seed goes through the real domain, never raw rows that skip a gate.**

- `available`/`wanted` titles come **only** from `suggest.Approve(...)` — the single approval
  gate — run on a hand-built submitted proposal. Seed never `UpsertTitle`s an `available`
  record directly.
- Board stages come from walking the pure `provision.Apply` state machine
  (RequestAccepted → Grabbed → LibraryConfirmed).
- The channel's `Desired` slots + `policy.applied` chips are the **real output** of
  `schedule.ComputeDesiredAt(...)` (the reconciler's pure core over an in-memory
  `Availability`), not hand-authored literals — so the seeded relaxations are genuinely what
  the ladder computes.
- The member is passwordless-by-design (imported-style): §11 has no local-member constructor,
  so a password would misrepresent the identity model.

Running it surfaced a real bug in the seed itself (a per-call `newID` minter handed the admin
and member the same id → the member silently overwrote the admin) — caught precisely *because*
seed drives the app instead of forging rows. Login: `admin` / `loomarr`.

### Follow-ups opened here (not defects, or out of this pass's scope)

- **Bucket 2:** Channel detail could gain the mock's **EPG "Today's guide" strip** and a
  **reconcile/activity log** (both need live Tunarr guide data).
- **Build gap:** the **Member first-login intro** screen (§13) is unbuilt.
- **Minor gap:** named **channel-template starter intents** (§13) aren't wired (the
  `IntentInput` mechanism exists).
- **Dev-env:** `pnpm` isn't on PATH in non-interactive shells; `npx pnpm@<pinned>` works.
  Worth a project run-skill.

## Bonus: the wizard "set a media server flavor" bug chain (found while seeding)

Driving the seeded wizard against the live backend surfaced a **four-bug chain** behind a
single confusing symptom — the media-server Test connection reporting *"set a media server
flavor (emby | jellyfin)"* even after a flavor was picked, and later a *401* that looked
like a credential problem. Each was verified live and fixed; the FE one was NOT the Radix
Select (a natural first suspicion), which propagated its value correctly all along.

1. **Test ran before Save (FE).** `/v1/setup/test` evaluates *persisted* settings
   (config-design §6 — the wizard IS settings), but the checklist step held edits in local
   state and only saved on a separate button. Typing a flavor then Testing tested the *old*
   (empty) value. Fix: `checklist-step.tsx` `test()` now `mutateAsync`-saves dirty edits
   before running the check. Verified: the PATCH (carrying `library.flavor`) precedes the
   `/v1/setup/test` POST (regression test in `wizard-router.test.tsx`).
2. **Stale settings snapshot (BE).** The settings service refreshes its in-memory snapshot
   only on a write; an out-of-band store change (a §18 replica write, a restore, a direct
   edit) left it stale, and the Test probe reads the snapshot. Fix: `settingsAdapter.Test`
   calls a new `Service.Refresh(ctx)` (public wrapper over `reload`) before probing, so a
   Test always reflects what's persisted. Verified with a drift test: save via API → delete
   the row under the service → Test correctly reports the cleared state, not the stale one.
3. **Corrupt secret stored (BE).** `KindSecret` accepted *any* string verbatim, so a
   connection-test hint string had been persisted as `library.token`, making every probe
   `401`. Fix: a shape sanity-guard in `parseKind` (config-design §9) — trim surrounding
   whitespace, reject internal whitespace or a <4-char value. Floor is deliberately LOW
   because the guard also runs on the resolve/read path (a stored value self-heals to
   default if it no longer parses), so a high floor would retroactively invalidate a real
   short key on upgrade.
4. **Normalization dropped on write (BE, latent).** `Patch` validated via `parse()` but
   persisted the *raw* input — so a URL's stripped trailing slash and a secret's trimmed
   whitespace never reached the store (stored form ≠ resolved form). Exposed by fix #3's
   trim. Fix: `Patch` now persists `ValueString(parsed)`, the canonical value.

All four are covered by `make check` (BE unit tests) + the FE regression test; config-design
§9 gained the secret-shape and canonical-value rules (doc-first).
