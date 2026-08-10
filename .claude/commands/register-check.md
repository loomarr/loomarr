---
description: Re-verify the build plan's open phases and known-gaps against current code
---

# Register check

Re-check what the plan claims is outstanding against the code as it stands. Run this
**before starting a wave** — the plan decays, and acting on a stale one wastes sessions.

⚠ **Adapted for this repo.** The upstream version of this command targets
`loomarr-program-plan.md` §3, a defect register with `S`/`H`/`D`/`C` severity rows. That
file does not exist here — the v2 build plan references defect ids like `S2`, `S3`, `H1`
without ever defining them, and `v2-build-plan.md` §8 records the companion plans as
missing. So this checks the two registers this repo actually has:

- **`docs/engineering/archive/v2-build-plan.md` §4** — the phase table. Every row not marked done
  in `PROGRESS.md`.
- **`docs/engineering/archive/v2-build-plan.md` §8** — "Known gaps in this plan".

This is not hypothetical. `V7`, `V7b`, `V7c`, `V19` and `V24` were all complete in code and
absent from `PROGRESS.md`, so the plan's "next up" pointed at finished work and a session
spent four investigations discovering it.

## Procedure

High-volume and mechanical — this is the cheap-triage half of the workflow, so work
through it steadily rather than deeply. Each row is independent.

For each phase the plan lists as outstanding, use its **gate text as the starting point**,
not its description. The description says what it wants; the gate says what would prove it.
Then:

- **OPEN** — genuinely not built. Say what is missing.
- **DONE-UNRECORDED** — the code satisfies the gate but `PROGRESS.md` has no row. This is
  the failure mode this repo actually produces; propose the row.
- **PARTIAL** — some clauses satisfied, others not. Name which. V7 is the worked example:
  the endpoint shipped and the screen did not, and the PR claimed to close the defect.
- **CHANGED** — the phase still makes sense but its gate no longer describes what should be
  built. The easiest verdict to miss: a phase that got overtaken reads as OPEN at a glance.

## Rules

- **Verify, do not trust.** The plan was accurate when written and is a hypothesis now.
- **DONE needs positive evidence.** "I could not find it missing" is not done. Cite the
  file that satisfies each gate clause.
- **A capability with no door is PARTIAL, not DONE.** The gate asked whether the code
  works; this asks whether anyone can reach it. See `/surface-audit`.
- **Do not fix anything.** Report only.

## Output

```
REGISTER CHECK — <date> @ <commit>
open: <n>   done-unrecorded: <n>   partial: <n>   changed: <n>

DONE BUT UNRECORDED
  <id>  <one line>              evidence: <file:line>
        proposed PROGRESS row:  <the row>

PARTIAL
  <id>  satisfied: <clauses>
        missing:   <clauses>    <file:line>

STILL OPEN
  <id>  <one line>              blocked by: <deps or "nothing">
```

Then propose the diff to `PROGRESS.md` — and to §4 of the build plan if a phase CHANGED —
so the record is corrected in the same session it was checked. A register that is checked
but not updated will be stale again next time, and the check will have bought nothing.
