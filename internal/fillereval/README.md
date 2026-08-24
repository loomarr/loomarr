# Filler admission certification

This package scores captured filler-admission decisions. It deliberately has no provider, network,
media-decoder, store, or application dependency: the same decision JSONL can be replayed after a
prompt, model, policy, or scorer change without spending money or changing production state.

The checked-in `corpus/seed-v1.json` is a schema and regression seed, **not a certification corpus**.
It contains synthetic evidence for failure classes such as conflicting years, brief end cards,
prompt injection, corrupt media, programme excerpts, and ambiguous compilation boundaries. A real
certification manifest references external, legally usable media by content hash and records source,
licence, similarity cluster, split, labels, evidence, and slices. Non-redistributable media never
enters git.

`make filler-eval-contract` verifies the scorer and seed. `make filler-eval-cert` scores a JSONL file
named by `LOOMARR_FILLER_EVAL_PREDICTIONS`; the remaining `LOOMARR_FILLER_EVAL_*` variables identify
the corpus and every versioned input. The scorer is fail-closed: fewer than 300 cases, missing or
duplicate predictions, cross-split similarity leakage, incomplete attribution, operational failure,
wrong role/taxonomy, weak confidence bounds, or a missed precision/coverage/review gate produces a
non-certifying report and nonzero exit.

Provider execution belongs to the later bounded bakeoff layer. That runner must require explicit
request, spend, and concurrency ceilings and write this package's prediction shape. The replay
command itself never contacts OpenRouter or starts local inference.
