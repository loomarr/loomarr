# Engineering plans and evidence

This directory holds current plans, dated findings, and research that supports design decisions.
It does not override `docs/design.md`, `CONTEXT.md`, or `PROGRESS.md`.

| Kind | Location | Use |
| --- | --- | --- |
| Current delivery plans | [`plans/README.md`](plans/README.md) | Scope, sequencing, and acceptance criteria for active initiatives |
| Research | [`research/README.md`](research/README.md) | Primary-source or measured evidence used by a plan or design decision |
| Dated findings | This directory | Stable observations from spikes, audits, or device work |
| Superseded records | [`archive/`](archive/README.md) | Historical context that is no longer an instruction |

A plan must name its outcome, owner issue or initiative, current dependencies, acceptance evidence,
and the authoritative design sections it affects. When the plan ships or is replaced, move it to
`archive/` or rewrite it as a dated finding. Do not leave completed work phrased as the next action.

The active-work table in [`PROGRESS.md`](../../PROGRESS.md) is the index of what is happening now.
This directory supplies the detail, not a second status register.
