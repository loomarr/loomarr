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

## `help/` — the §13 documentation set (embedded in the binary → in-app Help)

Rendered offline via `react-markdown` at `/v1/docs` and in the app's Help view (§7.2, §13).
Written lean for the household admin: Quickstart, Integrations, Concepts, Member guide, Filler
guide, and Troubleshooting (keyed to the onboarding-checklist items — every red check deep-links
into it). Embedded by `docs/embed.go` (`//go:embed help/*.md`).

## Companion design docs — authoritative for their own domains

- [`programming-design.md`](programming-design.md) — the ChannelPolicy heuristics (§8/§9).
- [`config-design.md`](config-design.md) — the settings subsystem (§13/§15).
- [`frontend-design.md`](frontend-design.md) — the "Test Card" design system, tokens, visual
  testing (§12/§14). *Incorporated in Phase 14.*
- [`integrations/media-server-livetv.md`](integrations/media-server-livetv.md) — the Emby/Jellyfin
  Live TV wiring (§6). *Incorporated in Phase 14.*

Precedence: `design.md` wins on behavior; each companion wins on its own domain (see CLAUDE.md).
