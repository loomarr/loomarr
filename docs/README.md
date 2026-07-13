# Loomarr docs

Two kinds of documentation live here, kept deliberately separate:

## `design.md` — the single source of truth

The full design doc (§1–§21). **Doc-first rule:** if code must deviate, update `design.md` in
the same PR *before* implementing (CLAUDE.md prime directive). The phase build plan is §21.

## `engineering/` — process & findings (for the build team)

Notes that support the phased build but aren't shipped to end users.

- [`engineering/phase-0-findings.md`](engineering/phase-0-findings.md) — index of the Phase-0
  contract-spike evidence. The detailed per-service findings live **next to their fixtures** in
  `internal/testkit/fixtures/*/FINDINGS.md` (kept there on purpose — findings explain the pinned
  fixtures they sit beside; see CLAUDE.md "Fixtures are pinned truth").

## `product/` — the §13 documentation set (embedded in the binary → in-app Help)

Built out in Phase 13/14. These render offline via `react-markdown` at `/docs` and in the app's
Help view (§7.1, §13). Planned set (§13): Quickstart, Integrations, Concepts, Member guide,
Filler guide, Troubleshooting (keyed to onboarding-checklist items). Empty until Phase 13.
