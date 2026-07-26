---
description: Audit design docs against the code they describe, one claim at a time
---

# Doc drift audit

Find places where a document asserts something the code no longer does. This is the
judgment half of drift — `make retired-verify` already catches dead identifiers
mechanically, and this exists for the claims a grep cannot evaluate: *"§5's IA table
describes six settings pages"* when the app has seven.

**Argument:** `$ARGUMENTS` — a doc path, or a section (`design.md §12`). Audit only that.
The corpus is far too large to sweep in one pass, and a shallow sweep is worse than a deep
slice because it produces confident silence about things it never checked.

## Procedure

**Extract claims first, verify second.** Read the target and list every falsifiable
assertion about the system as a flat inventory before checking any of them. Claims are
things like: this endpoint exists, this field is editable, this default is X, this
component renders Y, these are the N pages. Skip rationale, philosophy, and intent — a
doc explaining *why* a decision was made cannot drift; a doc claiming *what the code does*
can.

**Then verify each claim independently.** These are genuinely independent, so errors do
not compound — check each against the code without letting the previous verdict colour the
next. For each: find the code, cite `file:line`, decide.

- **HOLDS** — code matches
- **DRIFTED** — code contradicts the claim
- **UNBUILT** — the doc describes something that does not exist yet, and does not say so
- **UNVERIFIABLE** — cannot determine from the source; say what would settle it

## Rules

- **Cite or drop it.** Every DRIFTED and UNBUILT finding carries `file:line` for both the
  claim and the code. A finding you cannot cite is a guess, and a guess in this report is
  worse than a miss because it will be trusted.
- **Read the code, not the identifier.** A field named `autoCurate` proves the field
  exists, not that it works or that anything calls it.
- **A comment is a claim too.** `declared.go:10` asserts `WEBHOOK_SECRET` lives in
  `secrets.go`; `secrets.go` declares only `session_secret` and `playout_token`. Code
  comments drift exactly like prose and are read as authoritative more often.
- **Aspirational prose is not drift.** "Track T will own airtimes" is a plan. "Loomarr
  computes airtimes" when Tunarr computes them is drift. When a doc marks something as
  future work, that is correct documentation.
- **Check the reverse direction too.** Code that contradicts *no* doc is often a capability
  nobody documented — worth reporting as a gap.
- **Do not fix anything.** This command reports. Fixes are their own PR with their own
  gate, and an auditor that edits loses the ability to be trusted about what it found.

## Output

```
AUDIT: <target>
claims: <n>   holds: <n>   drifted: <n>   unbuilt: <n>   unverifiable: <n>

DRIFTED
  <doc:line>  "<the claim, quoted>"
      actual: <what the code does>  <file:line>

UNBUILT
  <doc:line>  "<the claim>"     no implementation found

UNDOCUMENTED
  <file:line>  <capability with no doc coverage>
```

Order findings by blast radius: anything shipped to users (`docs/help/` is embedded in the
binary), error copy, onboarding first; internal design docs second.
