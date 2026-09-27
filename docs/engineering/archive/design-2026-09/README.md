# Archive — `design.md` slices (2026-09)

**Nothing in this directory is an instruction.** When `docs/design.md` was split into
[`docs/design/`](../../../design/README.md) (#779), passages that carry measured evidence or a
retired protocol worth re-reading were moved here verbatim. Everything else that left `design.md`
was either condensed into a subsystem doc or deleted; git history and the last full version keep it.

If a slice disagrees with `docs/design/`, the slice is wrong.

| Slice | Formerly | Why it is kept |
| --- | --- | --- |
| [`local-model-experiment.md`](local-model-experiment.md) | §8, specialized local model experiment and release contract | The experiment protocol; the models repository owns the live version and decision [0026](../../../design/decisions/0026-specialized-local-model.md) keeps the why |
| [`playout-2026-07-consequences.md`](playout-2026-07-consequences.md) | §9.1, consequences recorded honestly | What taking over playout cost, with the incidents behind each rule; the rules that still bind are in [`playout.md`](../../../design/playout.md#constraints-that-follow-from-owning-playout) |
