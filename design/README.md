# Loomarr design prototypes — Phase-13 visual reference

These are the **authoritative reference for palette, typography, and idiom** — the
"transmitted test card" look (CLAUDE.md "Seed docs"; `docs/frontend-design.md` §7). They
are Claude Design mockups in the `.dc.html` format — a self-contained prototype runtime
(`support.js`), NOT the shippable frontend.

> **They are NOT authoritative for structure / information architecture.** The prototypes
> depict the channel detail as an *operator console* (a "Reconcile now" button, a scheduler
> DRIFT banner, a desired-slot table) — a model `docs/design.md` §9/§12 deliberately
> **rejected** in favor of a self-maintaining appliance with no rebuild button. Where a
> prototype's *structure* disagrees with `design.md` §12, **§12 wins** (the channel detail is
> the four-surface IA: Overview · Programming · Filler · Settings). The prototypes remain the
> baseline for *look*; `design.md` §12 is the baseline for *what the page is and contains*.
> (This supersedes the prior "authoritative visual reference" framing, which read as
> authoritative for structure too — the state that let the console mock and the shipped app
> silently disagree.)

| File | Viewport | Screens |
| --- | --- | --- |
| `loomarr-prototype-desktop.dc.html` | 1280×800 | Login · First-run wizard (checklist / webhooks / Live TV / first channel) · Channels · Channel detail · Board · Suggestion workspace (centered + split hero) · Approval queue · Filler library · Users · Settings · Help · Member intro · User drawer · ⌘K command palette |
| `loomarr-prototype-mobile.dc.html` | 390×844 | Channels · Board · Approvals (the read-and-approve mobile-web surface) |
| `support.js` | — | The Claude Design runtime both prototypes load (generated; do not edit) |
| `image-slot.js` | — | Claude Design starter scaffold the **v2** desktop prototype loads (generated; do not edit) |

## v2 prototypes (imported 2026-07-24 — research inputs, not yet ratified)

| File | Viewport | Screens |
| --- | --- | --- |
| `loomarr-prototype-desktop-v2.dc.html` | 1280×800 | Login · Wizard (6 steps, **Database first**) · **Dashboard** · **Guide** (time-grid) · Channel detail · My requests · Suggest · Approvals · Filler (+ Coverage / Catalog / Discover / Sources) · **People** · Settings (Connections · AI · Defaults · **System** · **Security** · **All settings**) · **Account** · Help · Member intro · User drawer · Toast |
| `loomarr-prototype-mobile-v2.dc.html` | 375×812 | Dashboard · Guide · Queue · People · Settings · Help · Account |

> ⚠ **`loomarr-prototype-desktop-v2.dc.html` is STALE for the Filler screen (2026-08-01).** The
> design project's copy was updated; the Filler screen was rewritten (4 tabs → 3, a new Incoming
> tab, an approval-gated "pull") and Approvals gained a `FILLER PULL` card. **The new markup could
> not be merged in** — the same 256 KiB cap truncates the fetch before the file's ~192 KB of JS, and
> the committed JS defines `covBars`/`registry`/`dscFilters` while the new markup binds
> `poolStats`/`asks`/`reels`/`services`, so a partial splice renders nothing. The delta is recorded
> verbatim in **`FILLER-DELTA-2026-08-01.md`**; the file itself needs another maintainer export.
> `support.js` and `image-slot.js` were re-fetched in the same pass and are **byte-identical**, so
> the export will render as-is.

Both v2 files are **complete** — the desktop is 502,509 bytes, obtained by maintainer export after
the `DesignSync.get_file` 256 KiB cap silently truncated 48% of it. `Loomarr Prototype v2.dc.html`
and `… v2 copy.dc.html` are byte-identical; the "copy" is a duplicate.

> The structure-vs-look precedence note above applies to the v2 prototypes **with more force, not
> less**: they propose a top-level IA change (Dashboard landing, Channels→Guide, Users→People) and
> pre-answer several maintainer decisions. See `docs/engineering/v2-mock-delta-2026-07-24.md` for the
> verified delta and the ratified decisions — and `SYNC-LOG-2026-07-24.md`, which shows the mocks
> were built *from* this repo, so much of the apparent delta is the mock reflecting shipped code.

## How Phase 13 uses these

**Recreate the visual output pixel-perfectly** in the decided stack (Vite + React 18 +
Tailwind v4 + shadcn/ui — design doc §14, `docs/frontend-design.md` §3–4). Match the
*rendered picture*, not the prototype's internal structure (`.dc.html` templating is a
mock tool; production is React components + the token layer). The Playwright visual suite
(`make fe-visual`, `docs/frontend-design.md` §5) judges the React build against these
prototypes as the baseline — the "transmitted test card".

## Two reconciliation deltas (apply ON TOP of the prototypes)

The prototypes predate the palette's contrast calibration. Per `docs/frontend-design.md`
§2.1 and §7, the React recreation must diverge from the prototypes in exactly two ways, and
the gallery baselines are judged against **prototypes + these deltas**:

1. **Badge/pill text on accent tints uses the `-300` stops**, not the base accent — 11px
   badge text is small text under WCAG AA, and the base stops fail on the composited 15%
   tints (`onair` 4.02:1, `suggest` 3.86:1). Use `onair-300` `#E85A5F` and `suggest-300`
   `#DC5BAC` (both pass with margin). The token generator recomputes these ratios in CI.
2. **`static-500` `#5A6170` is demoted to disabled-state and decorative-glyph use only**
   (2.94:1 on cards — fails for informational text). Any text carrying information uses
   `static-400` muted or better.

## Provenance

Imported 2026-07-13 from the maintainer's Claude Design project ("Shared file archive",
`dc543738-…`). The design-doc precedence rule applies: `docs/design.md` wins on *behavior*
(endpoints, flows, auth); `docs/frontend-design.md` + these prototypes win on *look*.
Where a prototype shows behavior that contradicts the design doc, the design doc wins and
the recreation follows it (e.g. real state/enum values from `api/openapi.yaml`, not the
prototype's illustrative strings).
