# Domain Docs

How the engineering skills should consume this repo's domain documentation when
exploring the codebase.

## ⚠ This repo already has a source of truth, and it is not `CONTEXT.md`

The skills' standard layout is a root `CONTEXT.md` plus `docs/adr/`. **Loomarr
deliberately does not use it.** CLAUDE.md names `docs/design.md` the single source of
truth and requires that code deviating from it update the doc *first*, in the same PR —
"never let the doc and the code disagree silently".

A `CONTEXT.md` scaffolded alongside that would be a second authority describing the
same domain, free to drift from the first. That is exactly the failure the prime
directive exists to prevent, so the domain model lives where it already lives.

**Do not create `CONTEXT.md`, `CONTEXT-MAP.md`, or `docs/adr/` in this repo.** If a
skill's instructions assume they exist, read the files below instead. If a skill wants
to *record* a decision, it belongs in `docs/design.md` or the relevant companion doc,
under the doc-first rule.

## Before exploring, read these

Read only what the task needs — the design doc is large, and CLAUDE.md's session ritual
is explicit that loading all of it wastes context.

- **`docs/design.md`** — authoritative on *behavior*: endpoints, flows, auth, phases,
  and the numbered sections (§) the whole codebase cites. Start from the phase →
  section map in CLAUDE.md rather than reading it end to end.
- **`PROGRESS.md`** — what is actually built, one row per phase, with the commit SHA and
  test command proving each gate. Also carries the deviations and findings; the "notes"
  column is where hard-won context is recorded.
- **`CLAUDE.md`** — the prime directives, the harness contract (`make` targets), the
  testing rules, and the do-nots. Read before proposing any process change.

Companion docs, each authoritative for its own domain:

- **`docs/programming-design.md`** — ChannelPolicy heuristics (scope, audience,
  separation, ordering, seasonality, the relaxation ladder).
- **`docs/config-design.md`** — the settings subsystem: typed registry,
  `env > database > default` resolution, hot-apply, secrets lifecycle.
- **`docs/frontend-design.md`** — tokens, palette, component library, visual testing.
- **`docs/integrations/media-server-livetv.md`** — Live TV wiring.

**Precedence:** `docs/design.md` wins on behavior; each companion wins on its own
domain. A companion that contradicts the design doc gets *corrected*, not followed.

## Use the design doc's vocabulary

When your output names a domain concept — an issue title, a refactor proposal, a
hypothesis, a test name — use the term as `docs/design.md` spells it, and cite the
section (`§11`, `§8.1`) the way the rest of the codebase does. The section numbers are
load-bearing: comments, commit messages, and `PROGRESS.md` rows all reference them.

If the concept you need is not in the design doc, that is a signal — either you are
inventing language the project does not use (reconsider), or there is a real gap, and
the gap gets closed doc-first before code.

## Flag design-doc conflicts

If your output contradicts `docs/design.md`, surface it explicitly rather than silently
overriding:

> _Contradicts §11 (the allowlist decides access) — but worth reopening because…_

Per CLAUDE.md, that is a design conversation with the maintainer, not something to work
around. The non-negotiables are the grounding rules (§8), the approval gate and
authorization model (§7, §11), and forward-only migrations (§16).
