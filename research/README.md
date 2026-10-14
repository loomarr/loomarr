# Filler research tooling (archived)

This module holds the filler research and certification tooling that the server never links: the
`cmd/filler-*` tools, their private packages under `internal/` and `cmd/internal/`, and the
`fillervisualsafety` review tools. It moved out of the main module by maintainer decision D1
(#1560, 2026-09-28), so the product module, its CI and its dead-code count contain the product
plus deliberate tooling only.

**It is frozen, not maintained.** `go.mod` requires the main module at the commit just before the
move, not the working tree, so the tools keep compiling against the core they were written for
while that core moves on. Its evidence stays recorded in `docs/engineering/`.

- Run a tool from the repository root with its `make` target (`mk/eval.mk`), or
  `scripts/research-run.sh <tool> [args]`. Relative paths resolve from the repository root, as before.
- `make research-verify` compiles the module with every build tag. No CI job runs it.
- [`docs/filler-certification.md`](docs/filler-certification.md) is the operator guide to the
  corpus, review and certification workflow.
- Two tests read the main repository's pilot corpus by relative path
  (`../../../internal/fillercorpus/corpus/pilot/locked.json`), so they see the current copy of
  that file, not the pinned one.

To use current core code in a tool again, move that tool back into the main module in its own
change. Don't re-point the requirement at a newer core: nothing here is kept compatible with it.
