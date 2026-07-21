# Design-mock review — build vs. `design/` prototypes (2026-07-20)

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

## Not captured

Require state or overlays not set up in this pass; no findings recorded:
- **Channel detail** — needs a channel to exist (store was empty).
- **Member intro**, **User drawer** — overlay states.
- **⌘K command palette** — *was* checked: good parity (centered modal, "Search titles,
  channels, help…", ESC affordance, empty-prompt copy all match).

## Environment for this review

Live app: `go run ./cmd/loomarr` (SQLite in scratch) on `:8080` + `pnpm --filter
@loomarr/web dev` on `:5173` (Vite proxies `/v1` → `:8080`). Mock: `python3 -m
http.server` over `design/` (and a patched copy) rendered via `support.js`. Both dark,
1280×800. The app was bootstrapped (owning admin created) so wizard step 1 was passed.
