# Loomarr design prototypes — Phase-13 visual reference

These are the **authoritative visual reference** for the Phase-13 Web UI (CLAUDE.md
"Seed docs"; `docs/frontend-design.md` §7). They are Claude Design mockups in the
`.dc.html` format — a self-contained prototype runtime (`support.js`), NOT the shippable
frontend.

| File | Viewport | Screens |
| --- | --- | --- |
| `loomarr-prototype-desktop.dc.html` | 1280×800 | Login · First-run wizard (checklist / webhooks / Live TV / first channel) · Channels · Channel detail · Board · Suggestion workspace (centered + split hero) · Approval queue · Filler library · Users · Settings · Help · Member intro · User drawer · ⌘K command palette |
| `loomarr-prototype-mobile.dc.html` | 390×844 | Channels · Board · Approvals (the read-and-approve mobile-web surface) |
| `support.js` | — | The Claude Design runtime both prototypes load (generated; do not edit) |

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
