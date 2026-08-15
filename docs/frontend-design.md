# Loomarr Frontend — Architecture & Design System ("Test Card")

**Status:** Implemented · companion to [`design.md`](design.md)
**Precedence:** the main design doc is authoritative for *behavior* (endpoints, flows, auth, phases). This doc is authoritative for *how the frontend looks and is built*. Conflicts → main doc wins on what, this doc wins on how; fix the loser in the same PR.
**Language policy (main doc §14) applies:** everything here is build-time tooling and libraries that compile to static assets embedded in the Go binary.

---

## 1. Design concept: Test Card

A *test card* is the color-bars calibration image broadcasters transmitted to prove the picture was true — the original pixel-perfect contract between a station and every screen tuned to it. That is exactly this frontend's contract: a broadcast-console aesthetic whose correctness is *enforced by machines* (the Playwright visual suite is our transmitted test card).

**Aesthetic direction:** a modern, dark broadcast console — calm surfaces, precise data, mono-set channel numbers — with retro-TV warmth used as *seasoning, not sauce*. Loomarr's product soul is era-matched: Saturday-morning cartoons with period cereal ads. The UI should feel like the master control room that makes that possible: professional first, nostalgic in the margins.

**Rules that keep it tasteful:**
- CRT/scanline/static flourishes appear **only** on idle surfaces — onboarding, empty states, the login screen. Never on data, tables, or forms.
- All flourishes are disabled under `prefers-reduced-motion` **and** in visual-test mode (determinism, §5).
- Nostalgia lives mostly in **microcopy** ("Dead air — create your first channel", "You're on the air") and in the `onair`/channel-number idiom, not in texture overlays.

---

## 2. Design tokens

Tokens are the single source of truth. **No raw color/size literals outside the token layer** — a hex code in a component is a review-blocking defect.

### 2.1 Color — the Test Card palette

Dark-first; **v1 ships dark-only**, but every color is a semantic token so a light theme is a token-set later, not a refactor. Accents are derived from the SMPTE bars, adjusted for WCAG AA on dark surfaces (raw SMPTE values are not accessible).

**Static scale (neutrals — "the set"):**

| Token | Hex | Use |
| --- | --- | --- |
| `static-950` (bg) | `#0B0C0E` | App background |
| `static-900` (surface) | `#131519` | Cards, panels |
| `static-800` (surface-2) | `#1B1E24` | Nested/hover surfaces |
| `static-700` (border) | `#2A2E37` | Hairlines, dividers |
| `static-400` (muted) | `#8B93A3` | Secondary text, placeholders |
| `static-100` (text) | `#E7EAF0` | Body text |
| `static-0` | `#FFFFFF` | High-emphasis text |

**Broadcast accents (from the bars):**

| Token | Hex | Meaning / use | Contrast notes |
| --- | --- | --- | --- |
| `signal` (amber) | `#FFB020` | **Brand & primary actions**, focus ring, active nav | 10.7:1 on bg — AA everywhere; 8.8:1 on its tint |
| `onair` (red) | `#E5484D` | Live/actively-streaming indicators; destructive actions | 5.0:1 on bg; **on its tint use `onair-300`** |
| `onair-300` | `#E85A5F` | Badge/pill text on the onair tint | 5.0:1 on tint (computed with margin) |
| `suggest` (magenta) | `#D6409F` | **The AI color**: intent input focus, generation progress, proposal accents | 4.75:1 on bg; **on its tint use `suggest-300`** |
| `suggest-300` | `#DC5BAC` | Badge/pill text on the suggest tint | 5.0:1 on tint |
| `tune` (cyan) | `#4CC9E8` | Links, informational states, in-progress "tuning" | 10.1:1 on bg; 7.9:1 on tint |
| `lock` (green) | `#3DD68C` | Success, checklist pass, "signal locked" | 10.4:1 on bg; 8.2:1 on tint |
| `caution` (yellow) | `#F5D90A` | Warnings, drift flags, conflicts | 10.8:1 on its tint; dark text on solid fills |

**Tints are alpha washes, not fixed hexes** (adopted from the Claude Design prototype): a tint is `color-mix(in srgb, <accent> N%, transparent)` layered over the surface, with standard steps N ∈ {8, 12, 15, 30, 40}. One formula replaces six tint tokens and yields a consistent ramp per accent.

⚠ **The steps split into two jobs, and only one is contrast-gated.** Steps 8/12/15 are TEXT BACKGROUNDS — a badge or pill with accent copy on it — and the badge rule below governs them; the CI check validates each accent's `on` stop against the **15%** composite. Steps 30/40 are **fills only**: a pod segment, a progress bar, a block in the guide grid. Accent text does not clear AA on them (the `Design/Palette` page renders every step and shows exactly where the ramps fail), so putting a label on a `-30`/`-40` fill is out of contract. This was an unstated assumption until the palette page made the whole ramp visible — the rule had always been written as though 15% were the only step that existed.

**The badge/tint rule (learned the hard way, twice):** 11px badge text *is small text under WCAG* — the 4.5:1 bar applies regardless of how label-like it feels. On the composited 15% tints the base stops fail (onair 4.02:1, suggest 3.86:1); the `-300` stops pass with margin (`#E85A5F` → 4.54, `#DC5BAC` → 4.65). Every `accent-on-tint` pairing is machine-verified against the *composited* tint color; the token generator (§2.5) recomputes these ratios in CI, so a palette or alpha edit that breaks a pairing fails the build.

Additional statics from the prototype: **`signal-400` `#FFC14D`** (hover/active amber, 11.3:1 on card) and **`static-500` `#5A6170`** — 2.94:1 on cards, therefore restricted to **disabled states and decorative glyphs only**; any text carrying information uses `static-400` muted or better.

Semantic aliases map onto these (`primary`→signal, `destructive`→onair, `success`→lock, `warning`→caution, `info`→tune) so shadcn primitives restyle without edits. The lifecycle states (§ main doc §4) map: `wanted`/`requested` static-400 · `downloading` tune · `available` lock · `unavailable` static-400 strikethrough · drift caution.

Stored as OKLCH in the source of truth (Tailwind v4/shadcn convention); hex above is for human reference.

### 2.2 Typography

- **Geist** (UI) + **Geist Mono** (data) — one family pair, OFL-licensed, **self-hosted from the embed** (satisfies the main doc's offline rule *and* makes visual tests deterministic — no CDN font swaps).
- Mono is a design signature, not a garnish: **channel numbers, EPG times, state badges, external ids, durations** are always mono. If it came from a machine, it's set in mono.
- Scale: 12 / 13 / 14 (body) / 16 / 20 / 24 / 32, line-height 1.5 body · 1.2 headings. Channel-number display style: mono 24–32, tabular numerals.

### 2.3 Space, radius, elevation

- 4px grid (`space-1`=4 … `space-8`=32). Density: comfortable default; tables use compact row height (40px).
- Radius: `sm` 4 (inputs, badges) · `md` 8 (cards, buttons) · `lg` 12 (dialogs, hero surfaces).
- Elevation in dark UIs is **borders first, shadows second**: surface + `static-700` hairline; one soft shadow token reserved for overlays (dialogs, popovers, ⌘K).
- **Non-text contrast (WCAG 1.4.11):** the `static-700` hairline is 1.3–1.4:1 — fine for *decorative* dividers and card outlines (the surface fill does the identifying), but **no control may be identified by that hairline alone.** Controls get a fill difference + the focus ring; where a border is genuinely the boundary (e.g. an unfocused outline input), use **`border-control` `#61646B`** — computed ≥3:1 on both card and page.

### 2.4 Motion

- Tokens: `fast` 120ms · `base` 200ms, ease-out. Nothing animates longer than 300ms except the suggester's generation shimmer.
- `prefers-reduced-motion` is honored globally (single CSS gate) and force-enabled in visual-test mode.
- Signature moments (used sparingly): checklist items "lock in" (static→clear, 200ms) during onboarding; a channel card's `onair` dot fades in when its first reconcile completes.

### 2.5 Token pipeline (the mobile bridge starts here)

`web/packages/tokens` holds the TS source of truth → generates three artifacts in CI:
1. `theme.css` — Tailwind v4 `@theme` variables for the web app,
2. a **shared Tailwind preset** — consumed by web now, by NativeWind (Expo) later,
3. `tokens.json` — for any future non-Tailwind native consumer.

CI fails if generated artifacts drift from source (`make fe-tokens` regenerates; diff must be empty). This is the same committed-artifact discipline as `api/openapi.yaml`.

---

## 3. Component library — three layers

**Layer 0 — tokens** (§2).
**Layer 1 — primitives:** shadcn/ui (new-york style, Tailwind v4), copy-in per its philosophy. Restyled **only** via tokens/CSS variables — never fork primitive logic. **Base UI** (`@base-ui/react`) underneath gives focus management and a11y for free; shadcn ships a Base UI variant of every component the app uses, so the copy-in path is unchanged. ⚠ "For free" has one documented exception: Base UI's Tooltip is **visual-only by design** (no `role="tooltip"`, no `aria-describedby`), so a tooltip whose content is information rather than a restatement of the trigger's label must declare its own description — see `FieldHelp` and design §14.
**Layer 2 — Loomarr components:** the actual product library, in `apps/web/src/components/loomarr/`. **Pages compose Layer-2 components; Tailwind utility soup is confined to Layers 1–2.** Every Layer-2 component: CVA variants, typed props from the orval client where applicable, a co-located Storybook story (§5) enumerating all states, and renders RFC 7807 errors through the shared `ErrorState`/field-error patterns — never raw JSON.

### Signature components (the vocabulary of the app)

| Component | Purpose | States to register |
| --- | --- | --- |
| `AppShell` | Nav rail (Channels, Board, Suggest, Filler, Users, Settings, Help) + ⌘K + user menu | member / admin / mobile-web collapsed |
| `StateBadge` | Provisioning lifecycle chip (mono) | wanted · requested · downloading · available · unavailable · drift |
| `OnAirIndicator` | The red dot with a pulse (pulse ≤ reduced-motion) | off · live · reconciling |
| `ChannelCard` | Channel health at a glance: number (mono), name, now/next, managed badge | healthy · pending-slots · drift · error · creating |
| `NowNextStrip` | "Now: X · Next: Y" line for a channel | playing · flex/pod gap · empty |
| `IntentInput` | The hero — NL intent with `suggest` magenta focus ring + template chips | empty · focused · template-filled · submitting |
| `GenerationProgress` | SSE-driven suggester steps | searching · reasoning · scoring · done · failed |
| `ProposalReview` | Lineup + acquisitions with rationale, confidence, alternates; edit-via-search | draft · submitted · approved · denied · partially-edited |
| `PodTimeline` | A break visualized: bumper→ads→bumper with era/audience chips | matched · fallback-widened · bumper-card-only |
| `ClipCard` | Filler clip w/ kind/era/audience/category chips | tagged · untagged · ai-suggested-tags |
| `ChecklistItem` | Wizard/Settings check row | pending · running · pass · fail(+hint+doc-link) |
| `ApprovalQueueItem` | Admin queue row with one-click approve/deny | pending · approving · denied |
| `SearchCommand` | ⌘K palette over `/v1/search` scopes + channels + help | idle · results(in-library flag) · empty |
| `EmptyState` / `ErrorState` | Mandatory for every list / RFC7807 renderer | per-surface copy variants |

---

## 4. Architecture & mobile-readiness

### 4.1 Workspace layout (pnpm, inside `web/`)

```text
web/
  packages/tokens/      # §2.5 — source of truth + generators
  packages/api/         # orval output: types + TanStack Query hooks (platform-agnostic)
  packages/core/        # SSE bus, zod schemas, formatters, shared data contracts
  packages/fixtures/    # deterministic "test card" story/test data (web + future mobile)
  apps/web/             # Vite + React 18 + TanStack Router + Tailwind v4 + shadcn
  apps/web/.storybook/  # Storybook 10 (react-vite): main, preview, vitest wiring
  apps/mobile/          # FUTURE: Expo + NativeWind + RN Reusables + @storybook/react-native
```

### 4.2 The sharing decision (explicit)

**Web-first now; when mobile happens, share logic and tokens — never component implementations.**

| Shared across platforms | Per-platform |
| --- | --- |
| Design tokens + Tailwind preset (§2.5) | Component implementations (shadcn/Base UI web ↔ React Native Reusables native) |
| `packages/api` (orval types + query hooks — TanStack Query runs on RN) | Navigation (TanStack Router ↔ Expo Router) |
| `packages/core` (zod validation, SSE handling, domain logic, formatters, **shared data contracts**) | Gesture/touch interactions, portals (`PortalHost` on native) |
| CVA variant definitions & component *contracts* (names, props, states) | Styling details where RN lacks cascade (each `Text` styled directly) |
| **Storybook story *contracts*** (CSF states) + `packages/fixtures` "test card" args | Story *implementations* (`*.stories.tsx`: web shadcn ↔ RN Reusables) |
| Icon vocabulary (lucide ↔ lucide-react-native, same names) | Visual-test baselines |

**Rejected alternatives, on the record:** `react-native-web` (would forfeit shadcn/Base UI and the decided web stack to render RN primitives on the web); universal kits like Tamagui/gluestack (different styling philosophy, heavier lock-in — our bridge is the token/preset layer, which NativeWind consumes natively). The future mobile app is Expo + NativeWind + React Native Reusables — the shadcn-philosophy port built on rn-primitives — consuming the same preset, tokens, and `packages/{api,core,fixtures}`. Its component workshop is **`@storybook/react-native`** (v10, on-device via the `withStorybook` Expo wrapper), authored in the same CSF format and reusing the same `packages/fixtures` args — so a component's *states* are defined once and rendered by each platform's own implementation. Consistent with the `react-native-web` rejection above, the mobile Storybook runs on-device, not by rendering RN primitives in a browser.

### 4.3 Forms & state

- **TanStack Form + zod**, the zod schema passed straight to the form as a **Standard Schema** validator — no resolver adapter. Zod schemas live in `packages/core` so mobile reuses validation verbatim, and `@tanstack/form-core` lets it reuse the form logic too. (Not shadcn's `<Form>`: that wrapper is react-hook-form-bound and Loomarr hand-composes `Label`+`Input`, so there was nothing to inherit.)
- **No global state library.** TanStack Query owns server state (SSE-invalidated, main doc §12); local UI state is React state. Introducing zustand/jotai requires updating this doc first.

### 4.4 Route payloads are bounded

File-route code splitting only helps when a route's imports stay behind that route boundary. Production
code imports the nearest component or domain barrel (`@/components/ui/button`,
`@/components/loomarr/dashboard`, `@/filler/filler-page`), never an app-wide barrel such as
`@/components/ui`, `@/components/loomarr`, `@/channels`, `@/filler`, or `@/settings`. The same rule
applies to workspace packages: runtime code imports a tag/model subpath from `@loomarr/api` and a
single module from `@loomarr/core`, while their package roots remain the complete public catalogs for
tests and tooling. Using a catalog barrel in the running app lets the bundler merge unrelated screens
back into one eagerly preloaded chunk.

The production build enforces both sides of this contract: no JavaScript chunk may exceed 500 KiB, and
the entry script plus its module preloads may not exceed 1 MiB uncompressed. The first bound keeps one
heavy screen from becoming a parse cliff; the second catches a nominally split build that still downloads
the whole app before authentication or route selection.

The embedded production server negotiates gzip for HTML, JavaScript, CSS, and other compressible static
formats. It prepares those representations once when its handler is constructed, not on every request;
the route budget above remains uncompressed so compression cannot hide parse and evaluation regressions.
The document also starts the initial `/v1/auth/me` read while that entry graph is downloading. The shared
API transport adopts that exact response once, preserving the route guard's error handling without putting
the session round trip after JavaScript evaluation or issuing a duplicate request.

---

## 5. Component workshop + pixel-perfect testing (Storybook + Playwright)

The component library lives in **Storybook** — the workshop for building and reviewing components in isolation — and the visual suite is the transmitted test card: if the picture drifts, the build fails. **Stories are the contract; Playwright is the camera.**

**Why Storybook over a hand-rolled `/__gallery`:** stories are the industry-standard component contract (CSF), they double as the dev workshop (controls, autodocs, the a11y panel), and they carry to the future mobile app (§4.2) via `@storybook/react-native`. The mechanics below preserve **every guarantee** of the earlier registry plan — offline, deterministic, committed baselines, 100%-coverage-enforced. **Chromatic is rejected on the record:** it is a hosted SaaS visual-diff service that would send our UI off-box and break the offline/self-hosted rule (§2.2, main doc §16); visual regression stays self-hosted Playwright against the offline `storybook-static` build.

### 5.1 Stories are the contract
- **Co-located CSF stories** — every Layer-1 primitive and every Layer-2 component has a `*.stories.tsx` beside it (folder-per-component), enumerating **every registered state**. Storybook 10 (`@storybook/react-vite`) indexes them; the built `storybook-static/` is the offline gallery. **A component without a story fails the build** — a coverage test enumerates the component barrels against the story index (the successor to "unregistered components are a lint error").

  ⚠ **Layer 1 is in scope, and was not always.** The original rule said "100% of Layer-2", on the premise that primitives are vendored shadcn nobody touches. That premise does not survive §2.5: primitives are restyled **through the token layer**, so a palette or alpha edit changes all nine of them — and with no primitive stories, neither the gallery nor the visual suite would show it. The tokens are precisely the thing most likely to change and least likely to be noticed, which makes the primitives the *last* place to skip coverage. A primitive's story enumerates its variants (every `Button` intent × size, every `Badge` tone), because those variants are what the token edit moves.

### 5.1a The library is browsable, not just complete

Coverage says every component *has* a story. It says nothing about whether a person can **find** one — and past ~30 components a flat alphabetical list stops being a gallery and becomes a lookup table you have to already know the answer to.

- **Stories are grouped by what a component is for**, not by where its file sits: `Design/*` (the system itself — palette, tokens, type, spacing), `Primitives/*` (Layer 1), then Layer 2 split by domain (`Channels/*`, `Guide/*`, `Filler/*`, `Setup/*`, `Shell/*`, `Feedback/*`). The folder layout stays flat and co-located; only the story `title` carries the grouping, so nothing moves on disk.
- **The design system documents itself in the workshop.** `Design/Palette` renders every accent with its hex, its tint ramp, and its **measured** contrast ratios — read from the generated token artifacts, never retyped, so the page cannot drift from what the build enforces (§2.5). `Design/Tokens` lists the full set; `Design/Typography` and `Design/Spacing` show the scales. The rules in §2 stop being prose only a maintainer has read and become the first thing anyone opening Storybook sees.
- **The `Design/*` pages are ordinary CSF stories, not MDX.** MDX would need `@storybook/addon-docs` (a new dependency, §14) to render prose that sits *beside* the values; a story renders the values themselves — it type-checks against `tokens.json`, gets a visual baseline like every other story, and cannot drift from the generated artifacts. Explanation rides along as rendered copy in the page. Executable documentation over described documentation, and no new dependency to justify.

### 5.1c Layer 2 must not re-implement Layer 1

§4.1 says primitives are restyled only through tokens and Layer 2 composes them. Nothing enforced the second half, and an audit found Layer 2 quietly growing its own primitives:

| Duplicated pattern | Where | Should be |
| --- | --- | --- |
| Small tinted chip, mono/uppercase | `Badge` (L1) **and** `StateBadge` (L2), each with its own `cva` and its own tint classes | `StateBadge` composes `Badge` |
| `font-mono text-[10px] uppercase tracking-…` caption | **21 occurrences across 11 components** | a `Caption` primitive |
| Status dot (`size-2 rounded-full`, pulse when live) | `GenerationProgress`, `GuideGrid`, `OnAirIndicator` | a `StatusDot` primitive |
| 15 Layer-2 components import **no** primitive at all | — | audited case by case; a leaf like `TvStatic` legitimately owns its markup, a card-shaped one does not |

**Why this matters more than tidiness.** The badge/tint contrast rule (§2.1) — the one learned the hard way, twice — is now encoded in `Badge` *and* `StateBadge` independently. Fix a stop in one and the other stays wrong, and **the contrast gate cannot tell**: it verifies token *pairings*, not which components consume them. Every duplicated pattern is a second place a design rule has to be remembered rather than inherited.

**The rule:** a visual pattern appearing in three or more Layer-2 components is a Layer-1 primitive, and Layer 2 composes it. Extraction is not a refactor to schedule later — the duplicate is the bug, because it is where the next contrast regression will land.

### 5.1b Story args come from `packages/fixtures`

§4.2 lists **"Storybook story *contracts* (CSF states) + `packages/fixtures` args"** as SHARED across web and the future mobile app: a component's states are defined once and each platform renders them. A story that hand-rolls DOMAIN data is not portable — it is a web-only literal the native Storybook cannot reuse.

**The rule:** any arg representing domain data (a channel, a clip, a proposal, a guide row, a title) comes from `@loomarr/fixtures`. Presentational args stay inline — a `size="lg"`, an `open` boolean, a `state` enum, a name-and-number pair are not domain data, and pushing them into a shared package would bloat it with values no other platform benefits from.

This is also what keeps the visual suite honest: fixtures are deterministic by construction (§4.2 — no `Date.now`, no randomness), while an inline literal invites a live clock straight into a snapshot.

**Audit result: the library already complies.** Eight stories import fixtures and the rest pass presentational args only — no story constructs a DTO inline. An earlier draft of this section reported "38 of 46 stories hand-roll their args", which was a miscount: it subtracted the fixture importers from the total and assumed the remainder were all domain data. Recorded because the wrong number nearly bought a 38-file migration that the codebase did not need.
- **`make fe-visual`** builds `storybook-static` on the host, then runs Playwright **inside the pinned official Playwright Docker image** (the reference rasterizer, §5.2) over every story at **two viewports** (1280×800 desktop, 390×844 mobile-web) with `maxDiffPixelRatio: 0.001`. The committed baselines are the `*-linux.png` that image produces; **the only sanctioned update path is `make fe-visual-update`** (same image), and baseline diffs are reviewed as images in the PR. The container reuses the host's JS-only `node_modules` and the browsers baked into the image — no in-container install, so a dev's native binaries are untouched.
- **`make storybook`** runs the dev workshop; **`make storybook-build`** produces the static gallery.
- Page-level snapshots cover key screens (each wizard step, Channels, Suggest workspace, Settings) with a shared `mask()` helper for dynamic regions.

### 5.2 Determinism kit (what makes pixel-perfect honest)
- **`make fe-visual` and CI both run in the official Playwright Docker image** — one rasterizer, one font stack — against the static `storybook-static` build (no dev server, no HMR). macOS/GPU rendering is *not* the reference and is expected to drift; the image is. Launch flags pin the rest: `--disable-gpu` (software GL), `--force-color-profile=srgb`, `--disable-lcd-text` (grayscale, non-subpixel text AA).
- Self-hosted Geist (§2.2), loaded in `.storybook/preview`; each test waits for the story to render, then awaits `document.fonts.ready`.
- Visual-test mode: Playwright forces `prefers-reduced-motion: reduce`, and each test injects `animation: none` before the shot — reduced-motion only *fast-forwards* (`duration: 0.001ms`), which leaves an **infinite** spinner frozen at a random frame; `animation: none` freezes it at its initial state. Snapshots target the **component element** (`#storybook-root`), not the centered page, whose fractional margins shift text AA run-to-run.
- A rare residual sub-pixel AA jitter is de-flaked by **test retries** — a real visual diff (or an a11y violation) reproduces and still fails every attempt, so retries never mask a regression.
- **Injected fixed clock** (all times render from a frozen date) and shared `packages/fixtures` data — the same "test card" fixtures the web and future mobile stories use; no `Date.now`/random.

### 5.3 Accessibility gate
- **a11y is enforced from both sides:** `@storybook/addon-a11y` (axe-core) surfaces violations live in the workshop as you author, and the CI gate runs **`@axe-core/playwright`** over every story in `storybook-static` — the *same* Playwright pass as the visual suite, so pixels and axe share one browser layer. Zero serious/critical violations, WCAG AA contrast — this exact class of failure is what the gate exists to catch.
- **Contrast is enforced twice:** the token generator (§2.5) recomputes every published fg/bg pairing at build time and fails CI on regression; axe verifies the rendered result.
- **Live regions:** SSE-driven state changes that matter to a person — a channel flipping ON AIR, a checklist item locking, a proposal completing — are announced via a single polite `aria-live` announcer (never one per component; a chorus of live regions is its own accessibility bug).
- **Badges:** stylized uppercase mono text pairs with a sentence-case `aria-label` ("On air", "Backfilling 4 of 7") so screen readers speak words, not letter-spaced shouting.
- **Forced colors:** one story smoke pass under `forced-colors: active` (Windows High Contrast) — layouts must survive; the CRT flourishes must vanish.

These suites join **phase 13's gate** in the main doc's build plan.

---

## 6. UX standards ("sensible elements")

### Onboarding — welcoming, on-theme
- Voice: warm broadcast metaphor without cosplay. "Let's get you on the air." Checklist items *lock in* (`lock` green) as each signal is acquired; the webhook handshake shows a live "listening…" state that flips on receipt.
- **Failures never blame.** Every red check = plain-language cause + the exact fix + a deep link into the embedded docs (backend contract, main doc §13). No stack traces in the wizard, ever.
- Resume-safe: the wizard reflects `GET /v1/setup/status` truth, so a browser refresh loses nothing.
- The finale is the payoff: the guided first channel ends with its `ChannelCard` flipping to **ON AIR** — the product's promise, demonstrated in the first ten minutes.

### Config that makes sense
- Settings groups: **Connections · Channels & Playback · AI · Users · Advanced**.
- Per-key provenance: env-pinned fields render locked with a "set via environment" chip; everything else is editable with configure → validate → save (main doc §13/§15). No inputs that look editable but aren't.
- The re-runnable connection checklist is embedded at the top of Connections — Settings *is* the troubleshooting console.
- Destructive actions live in an isolated danger zone with typed confirmation (`onair` styling).

### Empty, error, loading
- **Every list has an empty state with exactly one next action.** "Dead air — create your first channel." · "No clips yet — drop files in the filler folder or point MeTube at a playlist." · "Queue's clear — nothing awaiting approval."
- Errors: RFC 7807 `title` in a toast (sonner) for mutations, inline field errors via TanStack Form for forms; retry offered only where the operation is idempotent.
- Loading: **skeletons, not spinners**, for anything list-shaped; the word "Tuning…" is reserved for suggester generation. SSE keeps surfaces live so loading states are rare after first paint.

### Responsive posture
- Desktop-first admin surfaces, fully functional ≥768px. Mobile web is a first-class *read-and-approve* experience (Board, approval queue, channel status); creation flows are optimized for desktop. The true mobile app is the future Expo target (§4.2) — mobile web is not asked to fake it.

---

## 7. Deliverables & integration with the build plan

- **Phase 1** (main doc; also add the `web/packages` layout to its repo-layout block): `web/` workspace skeleton + `packages/tokens` with generators + self-hosted fonts + the `fe-tokens` make target.
- **Phase 13**: everything else here. **Gate additions:** story coverage = 100% of **Layer-1 primitives AND Layer-2 components** (each has a co-located `*.stories.tsx`); visual baselines committed for all stories at both viewports; axe clean (`addon-a11y` `test: 'error'`); `fe-visual` green in the Playwright Docker image.
- **Library-health gate** (§5.1a, §5.1b), enforced by the same coverage test rather than by review habit:
  - every Layer-1 primitive has a story enumerating its variants;
  - every story `title` carries a group prefix — a bare `Loomarr/<Name>` fails, so the sidebar cannot silently flatten again as the library grows;
  - the `Design/*` pages exist and read their values from the **generated** token artifacts (a hand-typed hex in a swatch is the drift §2.5 exists to prevent);
  - a story whose args contain domain data imports them from `@loomarr/fixtures`.

  **Written as a gate on purpose.** Every finding in §5.1a/§5.1c was a rule the library outgrew, not a rule anyone broke: 46 components under one flat namespace, nine primitives with no story coverage, `StateBadge` forking `Badge`, 21 hand-rolled captions across three off-scale sizes. None of that was visible to a gate, so none of it surfaced until someone went looking — and the one finding that turned out to be a miscount (§5.1b) is itself the argument for gates over audits: a number nobody can re-derive is a number that will be wrong. A convention that only lives in prose decays at exactly the rate the library grows.
- **Makefile additions:** `fe-tokens`, `storybook`, `storybook-build`, `fe-visual`, `fe-visual-update` (join the AGENTS.md command contract).
- This doc is a **seed doc**: incorporate as `docs/frontend-design.md` during phase 14; the palette table also feeds the docs site's own styling.
- **Visual reference (authoritative for look):** the Claude Design prototypes ship in-repo at `design/loomarr-prototype-desktop.dc.html` and `design/loomarr-prototype-mobile.dc.html` — recreate them pixel-perfectly per the handoff README (match visual output, not internal structure). Two reconciliation deltas apply on top of the prototypes: badge text on tints uses the `-300` stops (the prototypes predate the contrast calibration), and `static-500` text is demoted to disabled/decorative. Gallery baselines (§5) are judged against the prototypes-plus-deltas.
