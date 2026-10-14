# Agent skills

Loomarr keeps a small, curated skill set under `.agents/skills/`. A skill earns its place when it
adds non-obvious project-relevant judgment or a repeatable workflow. Generic productivity tools,
personal integrations, one-time setup skills, and wrappers that merely repeat `AGENTS.md` do not
belong in the repository.

Automatic means the harness may select the skill when its description clearly matches a request.
Explicit means a maintainer chooses it by name; those workflows are powerful but too costly or
specialized to load implicitly.

## Core engineering skills

| Skill | Invocation | Use it for |
| --- | --- | --- |
| `diagnosing-bugs` | Automatic | A hard bug or regression that needs a red feedback loop before a fix |
| `tdd` | Automatic | Behavior built test-first through a public seam |
| `codebase-design` | Automatic | Deep-module interfaces, seams, locality, and testability |
| `domain-modeling` | Automatic | Sharpening Loomarr vocabulary or recording a durable domain decision |
| `resolving-merge-conflicts` | Automatic | An in-progress merge or rebase conflict |

## Independent thinking and validation

| Skill | Invocation | Use it for |
| --- | --- | --- |
| `code-review` | Automatic | Fresh-context standards and spec review of a fixed diff |
| `design-an-interface` | Automatic | Several genuinely different module interfaces before implementation |
| `research` | Automatic | Primary-source research captured as a cited repo note |
| `prototype` | Automatic | Throwaway code that answers one UI or state-model question |
| `grilling` | Automatic | Stress-testing a plan or decision with the maintainer |

These are the strongest reasons to use subagents: the work is independent, parallelizable, or
benefits from fresh context. The owning agent still integrates the result and owns delivery.

## Product and issue workflows

| Skill | Invocation | Use it for |
| --- | --- | --- |
| `qa` | Automatic | Conversational QA that produces exact, user-focused GitHub issues |
| `request-refactor-plan` | Automatic | A detailed, incremental refactor issue built with the maintainer |
| `triage` | Explicit | Moving incoming issues through Loomarr's triage state machine |
| `to-spec` | Explicit | Turning settled context into one buildable GitHub issue |
| `to-tickets` | Explicit | Splitting a plan into tracer-bullet issues with blocking edges |
| `wayfinder` | Explicit | Mapping a large, foggy initiative as decision tickets |
| `improve-codebase-architecture` | Explicit | Surveying deepening opportunities before choosing a refactor |

The issue-oriented skills read `docs/agents/issue-tracker.md`, `docs/agents/triage-labels.md`, and
`CONTEXT.md`. They do not replace `PROGRESS.md` for active work or phase evidence.

## What was removed

The original import contained 41 skills. Twenty-four were removed because they were personal or
course-specific, duplicated another skill, performed one-time setup that is already complete, or
conflicted with Loomarr's delivery and dependency rules. Notable examples included a hard-coded
Obsidian vault, exercise scaffolding, generic Husky setup, Shoehorn migration, article-writing
flows, handoff wrappers tied to one harness, and an implementation wrapper that stopped at a local
commit instead of the required pull request.

The retained 17 skills stay pinned in `skills-lock.json`. `make agent-assets-verify` checks that the
lock, skill directories, Claude adapters, durable workflows, and this catalog agree.
