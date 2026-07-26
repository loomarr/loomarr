---
description: Verify a phase against its gate using a reviewer that never saw the implementation reasoning
---

# Gate review

Check whether the current work satisfies its phase gate — using a reviewer with **fresh
context**, because that isolation is the entire mechanism. A reviewer that saw the
reasoning which produced the code will ratify that reasoning. One that sees only the
criteria and the artifact will not.

## Procedure

**1. Assemble the brief. Do not analyse it yourself.**

- `git diff main...HEAD` (or the working diff if not on a branch)
- The phase's gate text, verbatim, from `docs/engineering/v2-build-plan.md`
- The doc sections the gate references — the actual text, not your summary
- The list of files touched

**2. Launch a subagent with ONLY that brief.**

This matters more than anything else here. The subagent must receive the artifact and the
criteria and nothing else. Do **not** include: your reasoning, the conversation that
produced the code, your assessment of whether it works, or any framing like "this should
satisfy the gate." Every one of those turns an independent check into an echo.

Brief the subagent as:

> You are reviewing a change against a written acceptance gate. You did not write this
> code and have no stake in it passing. For each clause of the gate, decide PASS, FAIL, or
> UNVERIFIABLE-FROM-DIFF, and cite the specific file:line that supports your verdict. Do
> not infer intent from names — a function called `preservesPolicy` proves nothing about
> whether policy is preserved. If a clause requires running something you cannot run, say
> UNVERIFIABLE and name the command that would settle it.

**3. Report the subagent's findings unedited**, then add your own response separately.
Do not merge the two. If you disagree with a FAIL, say so as your own view, below the
review, rather than softening the finding.

## What the review must check beyond the gate text

These are the failure modes this repo has actually produced. Check them every time, even
when the gate is silent:

- **Doc-first.** Did the losing document get corrected in this PR? `design.md` wins on
  policy, companions win on their domain. A code change that contradicts a doc without
  fixing the doc is an incomplete phase, whatever the gate says.
- **A capability with no door.** Does this add an API field, setting, or endpoint with no
  UI location and no explicit "API-only, documented" mark? `autoCurate`, `strategy`,
  `group` and editable season windows all shipped this way.
- **A doc promise with no code.** Does a doc in this diff describe something that does not
  exist? `RestartRequired` was documented as "a flag for honesty" and never built.
- **PROGRESS.md.** Did a phase ship without a row? Five did (V7/V7b/V7c/V19/V24), and a
  later session spent four investigations rediscovering finished work.
- **Retired names.** Run `make retired-verify`.
- **Backward compatibility, both layers.** V25 broke it twice: a value request body made
  huma require one (400s on empty), and the pointer fix did not cover orval, which still
  emitted `data` as required. Runtime compatibility and generated-client compatibility are
  different things — check both.
- **Test weakening.** Was any assertion loosened, skipped, or deleted to get green? This is
  a hard stop regardless of everything else.

## Output

```
GATE: <phase id> — <one line>

<clause>          PASS | FAIL | UNVERIFIABLE     <file:line or the command needed>
...

CROSS-CUTTING
doc-first         PASS | FAIL   <which doc, which section>
capability door   PASS | FAIL | N/A
doc promise       PASS | FAIL | N/A
PROGRESS row      PASS | FAIL
retired names     PASS | FAIL
backward compat   PASS | FAIL | N/A
test weakening    PASS | FAIL

VERDICT: ship | fix first
```

Be willing to fail it. A gate review that always passes is theatre.
