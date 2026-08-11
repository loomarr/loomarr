# Decision record — Channels section refinement (2026-07-24)

Status: **accepted** · Scope: the channel model and its surfaces (design.md §7/§9/§12, programming-design.md §2/§5/§8, the Phase-13 FE). This record is the "short ADR" for the multi-phase refinement; design.md §12 + programming-design.md §2.1 are the authoritative behavior/heuristics.

## Context

A channel accumulated eight independently-designed features (manual lineup, AI refine, wall-clock rules, rotation, re-curation, per-channel filler, season windows, icons) with no unifying model. The confusion was structural and appeared at every layer: three artifacts described three different channel-detail pages, "what plays" had 3–5 flat-sibling mechanisms, a live status-vocabulary bug, finished capabilities with no UI, the schedule missing from the viewer surface, and a `policy_json` blob whose ownership was enforced by defensive capture/restore — which caused **invisible data loss** (an operator's era/ceiling/ordering/separation edit silently reverted on the next refine, with no trace in the review).

## Decisions

1. **A channel detail page is four intent-grouped surfaces, two audiences.**
   - **Overview** (viewer, read-only): status, now/next, an Upcoming guide strip; admin-only diagnostics disclosure (relaxations, drift, Tunarr link).
   - **Programming** (admin): one surface — *what plays* (lineup + scope incl. audience ceiling) → *how it's ordered* → *when it changes* (rules) — with the cycle preview docked. Refine-with-AI is a header action here, not a tab.
   - **Filler** (admin): the per-channel selection + live sandbox.
   - **Settings** (admin): identity + lifecycle (auto-curate opt-in, pause/resume, danger zone). Identity/lifecycle only.
   - The audience ceiling lives in Programming → What plays (a content filter), never Settings. The hand-made "New channel" door is a channels-*list* action (origination), not per-channel Settings.

2. **Origination vs evolution.** Creation is `describe → review → approve` (the everyday door), with hand-made single-series / empty seeds as express paths into the same object. Evolution (manual edit / refine / re-curate) shapes a live channel and never re-originates it. Resolves the "no create-a-channel screen" vs three create modes contradiction.

3. **Policy ownership → operator-dirty tracking** (ratified over "ratify today's boundary + show the delta"). A proposal-owned field (scope/audience/ordering/separation/seasonal) is refreshed by a later refine/re-curate **only until the operator explicitly sets it**; then it is marked operator-set (`operatorSet` field-paths, riding the flat `policy_json`, no migration) and cannot be overwritten. The audience ceiling is additionally never *relaxed* (safety asymmetry outranks stickiness). The refine review must render policy deltas (incl. "kept your setting").

4. **Curation rules → provenance-aware merge** (ratified over "operator-locked after first seed"). Each `SchedulingRule` carries `source` (`llm`|`operator`) + stable id; a refine replaces only the `llm` rules and preserves operator rules, so refine-as-a-rule works. The WHEN/WHAT/HOW vocabulary + lowering becomes BE-authoritative (`GET /v1/programming/vocabulary`), killing the FE/BE lowering-table drift.

5. **Ordering has one operator knob.** Precedence: per-rule `How.Ordering` > `policy.ordering` > `Channel.Strategy` (stored default, not separately editable).

6. **Artifact precedence.** The `design/` prototypes are authoritative for palette/typography/idiom, **not** structure. Where a prototype's structure disagrees with design.md §12 (the prototypes depict a rejected operator console with a "Reconcile now" button), §12 wins. `archive/frontend-build-plan.md` corrected to drop the "reconcile now" button.

## Consequences

- A **channel surface map** (design.md §12) is the standing guardrail: every capability has one home + audience, and a PR adding a channel capability updates the map (new CLAUDE.md phase-13 gate line, P9).
- The BE ownership refactor (P3) is a **behavior change** (operator-dirty + provenance), not merely a neutral refactor — it fixes the live data-loss bug. Sequenced after the status-truth fix (P2) and doc alignment (P1).
- Full phased plan: P1 docs (this) → P2 status truth → P3 ownership → P4 lineup primitive → P5 Programming surface → P6 preview+vocabulary → P7 Overview guide → P8 capability closure → P9 guardrail.
