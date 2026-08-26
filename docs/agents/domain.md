# Domain documentation for skills

Engineering skills use three authorities that answer different questions:

| File | Question | Reading rule |
| --- | --- | --- |
| `CONTEXT.md` | What does this word mean? | Read the relevant glossary entries before naming domain concepts |
| `docs/design.md` | What does Loomarr do? | Read only the sections cited by the task or plan |
| `PROGRESS.md` | What is active or proven shipped? | Read **Active work** first; load historical rows only when needed |

`AGENTS.md` owns the session, safety, gate, and delivery contract. Read it before a skill changes
files or external state.

## Companion authority

The main design delegates detail to three companion documents:

- `docs/programming-design.md` — Channel Policy and scheduling heuristics;
- `docs/config-design.md` — settings information architecture and persistence;
- `docs/frontend-design.md` — shared client architecture and visual language.

The main design wins when behavior overlaps. A companion wins inside its named domain when it adds
detail without contradiction.

## Vocabulary discipline

Use the terms in `CONTEXT.md` in issue titles, test names, plans, and reviews. Cite the governing
design section when a statement depends on behavior. Do not grow the glossary into a second spec:
add the term to `CONTEXT.md` and put its behavior in the appropriate design section.

If a needed term or behavior is absent, report the gap. Do not silently invent a synonym or treat a
dated engineering plan as current product authority.

## Decisions

Loomarr records architectural decisions inline in `docs/design.md` and its companions, next to the
behavior and rationale they govern. There is no `docs/adr/` directory. Creating a separate ADR
system is a maintainer decision, not a skill default.

Grounding, admin approval, authorization, and forward-only migrations are hard constraints. A skill
that proposes changing one must stop at the design decision rather than implement around it.
