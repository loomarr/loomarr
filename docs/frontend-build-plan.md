# Frontend Build Plan — Phase 13 (and its BE prerequisite)

**Status:** approved sequencing, ready to execute · owner: phase 13
**Companion to:** `loomarr-frontend-design.md` (the "Test Card" design system — authoritative for *look/build*), `docs/design.md` §12/§13 (authoritative for *behavior*), `docs/config-design.md` §5–§7 (Settings IA), `design/` prototypes + `design/README.md` (pixel-perfect visual reference).
**Precedence:** unchanged — `docs/design.md` wins on behavior; `frontend-design.md` wins on look; this doc is *process* (sequencing, gates, the FE↔BE reconciliation) and never overrides either.

This plan exists because a pre-implementation FE↔BE audit found **5 concrete seams** between what the frontend design assumes and what the backend actually exposes. Closing them *before* the FE build is the "no surprises" contract. The design itself is already mobile-aware (Expo is pre-decided in `frontend-design.md` §2.5/§4.2), so mobile is a *readiness discipline*, not a new architecture.

## Locked decisions (2026-07-17)

1. **Close the BE contract first**, as a dedicated **13.0 PR**, before any FE code — so the FE builds against a complete, fully orval-typed surface with zero FE-side workarounds.
2. **Mobile: architecturally ready now, build web only.** Stand up the pnpm monorepo + shared `packages/{tokens,api,core}` so the future Expo app is a bolt-on. The Expo app itself is a **future phase + a §14 update**, not phase 13.
3. **Add per-step suggester progress events.** The `GenerationProgress` component renders *real* `searching · reasoning · scoring · done · failed` steps, backed by new SSE events from the suggest worker (part of 13.0).

---

## 1. FE↔BE reconciliation — the 5 seams and their resolutions

| # | Seam | Evidence | Resolution (all in 13.0) |
| --- | --- | --- | --- |
| 1 | **6 core routes absent from exported OpenAPI** → orval can't type them: `POST /v1/auth/login`, `POST /v1/auth/logout`, `GET /v1/auth/me`, `POST /v1/setup/bootstrap`, `POST /v1/users/import`, `POST /v1/users/sync` | `internal/api/export.go:29` excludes some deliberately; the rest never registered into the exported doc | Bring all 6 into the exported spec so orval generates types + hooks (satisfies design §12 "no hand-written glue"). `GET /v1/events` (SSE) and `GET /v1/backup` (file download) stay intentionally hand-written — they are not JSON request/response operations. |
| 2 | **`GET /v1/setup/status` returns only `livetv` + `tunarr_library`** — 2 of the ~7 checks §13 requires | `internal/api/setup.go:47-74`; the handler comment says "other integrations' checks are added by their phases" (they were not) | Aggregate the full checklist into `setup/status`: `media_server` (reachable + token valid), `filler` (library found, if configured), `seerr` (reachable + key), `tunarr` (reachable + source matches `library.url`), `tunarr_library`, `livetv`, `llm` (reachable + **tool-calling capable**), `tmdb` (key valid). Each carries `ok` + actionable `hint` + a `docHref` deep-link anchor (§13 "failures never blame"). |
| 3 | **No read surface for webhook handshake timestamps** | store *tracks* per-app last-received (`internal/store/store.go:102,112`) but nothing exposes it | Surface per-app (`sonarr`/`radarr`) last-received timestamps in `setup/status` (or a dedicated `webhook` check with a `lastReceived` field) so the wizard's step-4 "listening… → green on receipt" has something to poll. Data already exists; this is a read path only. |
| 4 | **Suggester emits no fine-grained progress** — only implicit coarse job start/done | `internal/suggest/worker.go` publishes nothing intermediate; the bus has 3 types (`title`·`channel`·`job`) | **Add progress events** (decision 3): the worker publishes `job` events with payload `{jobId, phase}` at each stage — `searching` (grounding: library + TMDB), `reasoning` (LLM tool-calling turn), `scoring` (deterministic post-scoring), then `done` / `failed`. Stays within the existing `type:"job"` frame — richer payload, no new bus plumbing. |
| 5 | **SSE is coarse & drop-tolerant** — 3 event types, payloads best-effort | `internal/events/bus.go:15`, `internal/api/events.go:62`; §8 says SSE is a latency optimization, GET is source of truth on reconnect | Not a defect — a design property. `packages/core`'s SSE bus is built as a **coarse invalidator** (a `channel` event → invalidate channel queries → TanStack Query refetches), never a surgical patch stream. Reconnect re-reads GET. Documented so the FE never treats a missed event as data loss. |

**Net:** findings 1–4 are small, additive BE work; finding 5 is a documented design property the FE must respect. None require weakening a gate or a prime directive. `setup/status` completion (2 + 3) is actually closing a gap *against §13's own spec* ("runs all checks, returns structured results").

---

## 2. Phase 13.0 — Close the BE contract *(prerequisite PR, doc-first)*

**Goal:** every route the FE calls is present + typed in `api/openapi.yaml`; the wizard checklist and suggester progress are fully BE-backed.

Work items:
- **OpenAPI coverage** — register/emit the 6 routes (finding 1) into the exported spec. Verify `GET /v1/events` + `GET /v1/backup` are documented as non-generated (comment + docs), so their hand-written FE clients are a conscious choice, not an oversight.
- **`setup/status` completion** (findings 2, 3) — assemble all §13 checks with `ok`/`hint`/`docHref`, plus per-app webhook `lastReceived`. Keep each check's live-test cheap and independently failable (a down Seerr must not fail the media-server check).
- **Suggester progress events** (finding 4) — publish `searching/reasoning/scoring/done/failed` from `internal/suggest/worker.go`. Keep §8's contract: a dropped progress event is a latency bug; the terminal proposal state in the store is the truth.
- **Regenerate + verify** — `make openapi` → `make openapi-verify` green (spec drift = red). Extend `internal/integration` for the new `setup/status` shape and the progress-event sequence (assert the ordered phases arrive for a real grounded job against the testkit LLM double).
- **Docs** — reflect the completed `setup/status` shape and the progress-event contract in `docs/design.md` §13 / §8 *first* (doc-first), then implement.

**Gate (13.0):** `make check` + `make openapi-verify` green; every FE-called route in the exported spec; `setup/status` returns the full checklist incl. webhook timestamps; an integration test proves the ordered suggester progress phases. Recorded in PROGRESS.md with SHA + command.

---

## 3. Phase 13.1 — Workspace skeleton + token pipeline

Corresponds to the `frontend-design.md` §7 "Phase 1" deliverables, done now at the head of 13.

- **pnpm monorepo under `web/`:** `packages/tokens`, `packages/api`, `packages/core`, `apps/web`, and an `apps/mobile` placeholder (empty but reserved — the boundary is load-bearing from day one).
- **`packages/tokens`** (§2.5) — TS source of truth → generates `theme.css` (Tailwind v4 `@theme`), the **shared Tailwind preset** (web now, NativeWind later), and `tokens.json`. `make fe-tokens`; CI diff must be empty (committed-artifact discipline, like `openapi.yaml`).
- **Self-hosted Geist + Geist Mono** from the embed (offline rule + deterministic visual tests).
- **`packages/api`** — orval config generating types + TanStack Query hooks from `api/openapi.yaml`. Platform-agnostic (no DOM, no web-only imports) so RN can consume it verbatim.
- **`packages/core`** — the SSE `EventSource` hook + the **coarse invalidation bus** (map `title`/`channel`/`job` → query-key invalidations), zod schemas, domain hooks, formatters (mono ids, durations, EPG times). No platform-specific code.
- **`apps/web`** — Vite + React 18 + TanStack Router (file-based, typed) + Tailwind v4 + shadcn/ui (new-york). `AppShell` + routing skeleton + the SSE provider. **All JSON routes are orval-generated (1:1, design §12 "no hand-written glue")** — 13.0 brought `auth/*`, `setup/bootstrap`, `users/import`, `users/sync` into the exported spec. The **only** hand-written client surfaces are `/v1/events` (SSE stream) and `/v1/backup` (octet-stream download) — not JSON request/response operations, so they cannot be OpenAPI ops — living in `packages/core` (shared, so mobile reuses them). FE components + fixtures consume the generated DTOs **directly** (`ClipDTO`, `Proposal`, `ErrorModel`, `TitleDTOState`, …) — no hand-mirrored types. The proposal is a worked example: the BE stopped shipping it as `json.RawMessage` and types the field as `suggest.Proposal`, so orval generates the full `Proposal`/`ProposalItem`/`Scores`/`ChannelPolicy` schema. The two survivors are genuine FE view models with no API equivalent: the ⌘K `PaletteScope` (a superset of the API's `SearchScope`) and the derived `ChannelHealth` rollup (a page maps `channelDTOStatus` + signals into it).
- **Embed** the `apps/web` build into the Go binary (`embed.FS` at `/`) — single container, same-origin, no CORS.

**Gate (13.1):** `make fe` green (orval typegen + tsc + vitest); token artifacts committed + diff-empty; the shell boots, authenticates via `GET /v1/auth/me`, and subscribes to `/v1/events`.

---

## 4. Phase 13.2 — Design system + gallery + visual harness

- **Layer-2 components** (`frontend-design.md` §3 table) in `apps/web/src/components/loomarr/`: `AppShell`, `StateBadge`, `OnAirIndicator`, `ChannelCard`, `NowNextStrip`, `IntentInput`, `GenerationProgress`, `ProposalReview`, `PodTimeline`, `ClipCard`, `ChecklistItem`, `ApprovalQueueItem`, `SearchCommand`, `EmptyState`/`ErrorState`. Each: CVA variants, typed props from the orval client where applicable, a co-located `*.stories.tsx` enumerating all states, RFC 7807 rendered via the shared error patterns (never raw JSON).
- **Storybook 10** (`@storybook/react-vite`) — the component workshop *and* the gallery/contract. Co-located `*.stories.tsx` per component (CSF), every registered state. `make storybook` (dev workshop), `make storybook-build` (offline `storybook-static`). A story-coverage test enumerates the component barrel → **a component without a story = red build** (successor to "unregistered component = lint error"). Carries to the future mobile app via `@storybook/react-native` (§8).
- **Playwright visual suite** — `make fe-visual` / `make fe-visual-update` over `storybook-static`, `toHaveScreenshot()` `maxDiffPixelRatio: 0.001`, two viewports (1280×800, 390×844), in the **official Playwright Docker image**. Determinism kit: injected fixed clock, `document.fonts.ready`, forced `prefers-reduced-motion` + `animations:'disabled'`, shared `packages/fixtures` data (the same "test card" data the stories use). **Chromatic rejected** (hosted SaaS — offline/self-hosted rule).
- **a11y gate** — `@storybook/addon-a11y` (axe) in the workshop for authoring feedback; the CI gate runs `@axe-core/playwright` over every story in `storybook-static` (the same Playwright pass as visual — one browser layer). Zero serious/critical, WCAG AA. The token generator recomputes every fg/bg pairing at build time (contrast enforced twice).

**Baselines** are judged against **prototypes + the two reconciliation deltas** (`design/README.md`): badge text on tints uses the `-300` stops; `static-500` is disabled/decorative only.

**Gate (13.2):** story coverage = 100% of Layer-2 components; visual baselines committed at both viewports; axe clean (`addon-a11y` `test:'error'`); `fe-visual` green in the Docker image.

---

## 5. Phase 13.3 — Auth + onboarding wizard

- **Login** — local or imported-media-server credentials (§11); first-run routes into the wizard.
- **Operator wizard** (§13, §6), resume-safe from `GET /v1/setup/status`:
  1. **Bootstrap** — `POST /v1/setup/bootstrap` (owning admin; succeeds once).
  2. **Connection checklist** — the now-complete `setup/status`; each red check shows plain-language cause + exact fix + a deep link into the embedded docs. Per-dependency re-test via `POST /v1/setup/test`.
  3. **Connect Tunarr to the guide** — `POST /v1/setup/livetv-connect` (m3u tuner + XMLTV), idempotent, never silent.
  4. **Webhook handshake** — show URL + secret; **listen** via the new `setup/status` webhook `lastReceived` (per-app "listening… → green on receipt").
  5. **Wire Tunarr's media source** — `POST /v1/setup/tunarr-connect` (already built + live-proven) so channels get real programs, not dead-air.
  6. **Import media-server users** — `POST /v1/users/import` (optional; skippable solo).
  7. **Guided first channel** — template intent → generate → (admin self-approve) → the `ChannelCard` flips **ON AIR**.
- **`ChecklistItem`** states: pending · running · pass · fail(+hint+doc-link). No stack traces in the wizard, ever.

**Gate (13.3):** wizard e2e smoke vs mocked BE green; page-level snapshot per wizard step.

---

## 6. Phase 13.4 — Core product surfaces

Every screen maps to existing endpoints + the 3 SSE types (see §8 coverage map).

- **Channels + Channel detail** — `ChannelCard`, `OnAirIndicator`, `NowNextStrip`; "reconcile now" (`POST /v1/channels/{id}/reconcile`); "Managed by Loomarr" badge; drift flags. Live via `channel` SSE events.
- **Board / My proposals** — titles by provisioning state (`GET /v1/titles?state=`), retry/cancel, member journey framing (*pending → acquiring (3/7) → live on N*). Live via `title` SSE.
- **Suggestion workspace** — `IntentInput` (magenta focus, template chips) → `GenerationProgress` (real `searching/reasoning/scoring/done/failed` via `job` SSE) → `ProposalReview` (lineup + acquisitions + rationale + confidence + alternates; edit-via-search over `GET /v1/search`) → submit. **Approval queue** (admin) with `approve`/`deny`.
- **Filler library** — `ClipCard`, `PodTimeline`; `GET /v1/filler`, `sync`, `tag`, `PATCH /v1/filler/{id}`.
- **Users** (admin) — `GET /v1/users`, `PATCH /v1/users/{id}` (roles/disable), `POST /v1/users/sync`.
- **Settings** — the 5 groups (**Connections · Channels & Playback · AI · Users · Advanced**), per-key provenance chips ("set via environment" locks the field), secrets lifecycle (`POST /v1/settings/secrets/{name}/regenerate`), the AI/§8.1 model picker (`GET /v1/system/llm`, `select`, `test`, `pull` with `llm_pull` progress via `job` SSE), and the re-runnable connection checklist as the troubleshooting console.
- **Help** — embedded `docs/` markdown (`react-markdown` + `remark-gfm`), client-side search, offline.
- **⌘K `SearchCommand`** — over `GET /v1/search` scopes + channels + help.

Cross-cutting (`frontend-design.md` §6): every list has an `EmptyState` with exactly one next action; RFC 7807 `title` → sonner toast for mutations, inline RHF field errors for forms; skeletons not spinners; one polite `aria-live` announcer for SSE state changes; badges pair mono text with a sentence-case `aria-label`.

**Gate (13.4):** page snapshots for Channels / Suggest / Settings; e2e approve-flow smoke (member submit → admin approve → title enqueued); axe clean.

---

## 7. Phase 13.5 — Phase-13 gate

Aggregates `frontend-design.md` §7 + design §21 phase 13:
- gallery coverage = 100% of the registry; visual baselines committed at both viewports;
- axe clean; `fe-visual` green in the Playwright Docker image;
- e2e approve-flow smoke green; `make fe` green; `make openapi-verify` green (orval types current);
- PROGRESS.md phase-13 row: **done** with SHA + the exact commands.

Manual smoke half of the DoD (design §21) runs on the maintainer's real stack (wizard all-green → real intent → approved → a channel playing in Tunarr with ad pods) — best on the Linux GPU box (native transcode).

---

## 8. Page → endpoint → SSE coverage map (the "no gaps" proof)

| Screen | Reads | Writes | Live (SSE) |
| --- | --- | --- | --- |
| Login | `auth/me` | `auth/login`, `auth/logout` | — |
| Wizard | `setup/status` | `setup/bootstrap`, `setup/test`, `setup/livetv-connect`, `setup/tunarr-connect`, `users/import` | `job` (first-channel generate) |
| Channels + detail | `channels`, `channels/{id}` | `channels`, `channels/{id}/reconcile`, `channels/{id}` (DELETE) | `channel` |
| Board / My proposals | `titles`, `titles/{key}`, `suggestions` | `titles` (DELETE) | `title` |
| Suggest workspace | `suggestions`, `suggestions/{id}`, `search` | `suggestions`, `suggestions/{id}/approve`, `suggestions/{id}/deny` | `job` (progress) |
| Filler | `filler` | `filler/sync`, `filler/tag`, `filler/{id}` | `title`/`job` (sync) |
| Users | `users` | `users/{id}`, `users/import`, `users/sync` | — |
| Settings | `settings`, `system/llm`, `setup/status` | `settings`, `settings/secrets/{name}/regenerate`, `system/llm/select`, `system/llm/test`, `system/llm/pull` | `job` (`llm_pull`) |
| Help | embedded `docs/` | — | — |
| ⌘K | `search` | — | — |
| Backup (Settings/danger) | `backup` (download) | — | — |

All present after 13.0. `events` + `backup` are hand-written FE clients by design (SSE + file download).

---

## 9. Mobile-readiness (now) and the future Expo app (later)

**Now — the discipline that makes mobile a bolt-on (`frontend-design.md` §4.2):**
- Shared across platforms: `packages/tokens` (+ preset NativeWind consumes), `packages/api` (orval types + TanStack Query hooks — runs on RN), `packages/core` (zod, SSE handling, domain logic, formatters, shared data contracts, the hand-written auth/bootstrap client), `packages/fixtures` (deterministic "test card" story/test data), CVA variant *contracts*, Storybook story *contracts* (CSF states), the lucide icon vocabulary.
- Per-platform (never shared): component implementations (shadcn/Radix web ↔ React Native Reusables native), navigation (TanStack Router ↔ Expo Router), gestures/portals, styling where RN lacks the cascade, story *implementations* (`*.stories.tsx`), visual baselines.
- **Enforcement:** no DOM/web-only import may appear in `packages/{tokens,api,core,fixtures}`. A lint boundary (e.g. an import-restriction rule) fails CI if it does — this is the single rule that keeps "ready" honest.

**Later — the future Expo phase (own PR, needs a §14 update):**
- `apps/mobile`: Expo + NativeWind (shared preset) + React Native Reusables (rn-primitives), Expo Router, `PortalHost`, reusing `packages/{api,core,fixtures}`, with `@storybook/react-native` (on-device, via the `withStorybook` Expo wrapper) as its component workshop — same CSF, same fixtures.
- Scope: **read-and-approve** — Channels, Board, Approvals (matches `design/loomarr-prototype-mobile.dc.html`). Creation flows stay desktop-web.

**⚠ The one lurking surprise — mobile auth is a §11 + §14 conversation, not a UI task.** The web SPA is **same-origin, embedded, cookie-session, no CORS** (design §14). A native Expo app is a **different origin over the network** → it needs:
- **CORS** on the BE (today there is deliberately none), and
- **real per-user bearer-token auth** — the current model is `loomarr_session` HttpOnly cookies (same-origin only) plus a single break-glass `API_TOKEN`. A native app needs per-user tokens (issue/refresh/revoke), not the break-glass token.

This is the biggest gap between "mobile-ready" and "mobile-shipped," and it is intentionally **out of scope for phase 13**. Flag it when the Expo phase is scheduled; it updates §11 (auth) and §14 (stack/distribution: a store-shipped native app is a new surface beyond "embedded static assets").

---

## 10. Makefile / CI additions (join the CLAUDE.md command contract)

- `make fe-tokens` — regenerate token artifacts from `packages/tokens`; CI diff must be empty.
- `make storybook` / `make storybook-build` — Storybook dev workshop / offline `storybook-static` build.
- `make fe-visual` / `make fe-visual-update` — Playwright visual suite over `storybook-static`; sanctioned baseline-update path.
- `make fe` — orval typegen + Biome + tsc + vitest (jsdom units + Storybook browser tests) (already in the contract; wire it to the monorepo).
- `make e2e` — Playwright smoke vs mocked backend (approve-flow).
- CI mirrors: `make check` + `openapi-verify` + `test-pg` + `fe` + `fe-tokens` diff + `storybook-build` + `fe-visual` (Docker) + `e2e`.

---

## 11. Risks & open items

- **13.0 doc-first:** the `setup/status` shape and progress-event contract change §13/§8 — update the design doc in the same PR *before* implementing.
- **Progress events determinism (§8):** progress is latency-only; tests assert *ordering when delivered*, never that every event arrives (drop-tolerant bus). Don't let a visual/e2e test become flaky by depending on a mid-flight `reasoning` frame.
- **Prototype vs behavior:** where a `.dc.html` prototype shows illustrative strings/enums, the React build uses the real `openapi.yaml` values (design/README provenance rule). Baselines are prototypes **+ the two deltas**, not the prototypes verbatim.
- **Mobile boundary rot:** the shared-package import boundary must be lint-enforced from 13.1, or web-only code leaks in and "ready" quietly becomes "refactor."
- **Same seed everywhere:** visual determinism depends on `make seed` fixtures matching the backend testkit seed — keep them one source.
