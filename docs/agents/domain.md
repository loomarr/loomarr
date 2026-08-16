# Domain Docs

How the engineering skills should consume this repo's domain documentation when
exploring the codebase.

## Two files, two questions — `CONTEXT.md` and `docs/design.md`

**`CONTEXT.md` (repo root) is the glossary: what a word MEANS.** It is a glossary and
nothing else — deliberately devoid of behavior, endpoints, and decisions. Read it first
so your output uses the project's own vocabulary.

**`docs/design.md` is the source of truth: what the system DOES.** AGENTS.md's prime
directive stands unchanged — code deviating from it updates the doc *first*, in the same
PR, and its numbered sections (`§7`, `§11`, …) are cited from ~2,600 places in the code.

These are not competing authorities, which is the thing worth being precise about: a
glossary and a behavior spec answer different questions. **Where they appear to overlap,
`docs/design.md` wins** — every `CONTEXT.md` entry carries the § reference that governs
its behavior, so the glossary points at the authority rather than restating it.

⚠ **Do not let `CONTEXT.md` grow into a spec.** The moment it starts describing flows,
endpoints, or decisions, it becomes the second authority that AGENTS.md's doc-first rule
exists to prevent. Add a term; put the behavior behind its §.

⚠ **`docs/adr/` does not exist here, and creating one is a decision, not a default.**
Architectural decisions are recorded in `docs/design.md` and the companion docs, in prose,
with the reasoning inline. A skill that wants to record an ADR should say so and let the
maintainer choose.

## Before exploring, read these

Read only what the task needs — the design doc is large, and AGENTS.md's session ritual
is explicit that loading all of it wastes context.

- **`docs/design.md`** — authoritative on *behavior*: endpoints, flows, auth, phases,
  and the numbered sections (§) the whole codebase cites. Follow the active plan's section links, or
  search its table of contents, rather than reading it end to end.
- **`PROGRESS.md`** — what is actually built, one row per phase, with the commit SHA and
  test command proving each gate. Also carries the deviations and findings; the "notes"
  column is where hard-won context is recorded.
- **`AGENTS.md`** — the prime directives, the harness contract (`make` targets), the
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

Per AGENTS.md, that is a design conversation with the maintainer, not something to work
around. The non-negotiables are the grounding rules (§8), the approval gate and
authorization model (§7, §11), and forward-only migrations (§16).
