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

Schema v3 distinguishes development seeds from certification manifests. A certification case also
locks its evidence packet and item metadata, records item-level rights adjudication and the bounded
source segment, and preserves two independent blind-review submission hashes. Matching submissions
become final directly; divergent submissions require a reasoned third-party adjudication. The report
records the exact manifest SHA-256 and scores only the explicitly selected development or holdout
split, so development examples cannot inflate certification.

`make filler-corpus-lock` combines a provenance-complete draft with two independently authored JSONL
review batches. Each line has `caseId`, `reviewerId`, `batchId`, `reviewedAt`, and `labels`; labels
contain disposition, reject class, content role, taxonomy, policy flags, slices, evidence, and the
answerable review question. A reviewer file must use one identity and one batch throughout. When the
two canonical label hashes differ, `LOOMARR_FILLER_CORPUS_ADJUDICATIONS` names a third JSONL file with
`caseId`, a distinct `adjudicatorId`, `adjudicatedAt`, `reason`, and final `labels`. The command writes
nothing until every draft case is covered and the complete certification manifest validates.

`make filler-corpus-archive` is the metadata-only acquisition preflight for Archive.org. It requires
an identified User-Agent, explicit snapshot time, request/item/per-item-byte/total-byte ceilings, and
a delay of at least 500 ms. It runs serially, caches the exact search and item responses, checks that
search and item licenses agree, excludes NC/ND licenses, records response hashes and retrieval times,
and selects a bounded video representation. Its output is only a candidate inventory: uploader
license metadata still needs independent item-level rights adjudication before it can enter a draft
manifest, and the command never downloads media or invokes a model.

`make filler-corpus-download` is the separately authorized media step. It accepts only `approved`
rights rows tied to the frozen metadata SHA-256, reviewer, review time, rationale, redistribution
decision, attribution, and restrictions; `held` rows remain out of the plan. Before the first request
it proves the approved count and predicted bytes fit explicit ceilings. Downloads remain serial and
identified, redirects stay on Archive.org, bodies cannot exceed their recorded size, Archive SHA-1/
MD5 values are checked when present, and the external ledger adds a locally computed SHA-256. A
failed or stale approval writes no ledger and cannot silently widen the selected corpus.

`make filler-eval-contract` verifies the scorer and seed. `make filler-eval-cert` scores a JSONL file
named by `LOOMARR_FILLER_EVAL_PREDICTIONS`; the remaining `LOOMARR_FILLER_EVAL_*` variables identify
the corpus, selected split, captured run time, every versioned input, and positive request, spend,
and concurrency ceilings. The scorer is fail-closed: fewer than 300 scored cases, missing or
duplicate predictions, cross-split similarity leakage, incomplete attribution, operational failure,
wrong role/taxonomy, exceeded run ceilings, weak confidence bounds, or a missed
precision/coverage/review gate produces a
non-certifying report and nonzero exit.

Predictions record the inference role/rung, requested and resolved route, derivative bounds, detailed
token categories, attempts, generation id, and the provider's exact charged decimal alongside an
integer nanodollar projection. Reports compare total cost, cost per correct automation, cost per
admit, and per-slice/per-rung cost; they never sum provider charges with binary floating point. The
scorer never reads its wall clock, and reports one-sided confidence bounds for every selective-risk,
coverage, review, and slice measure used in certification.

Provider execution belongs to the later bounded bakeoff layer. That runner must require explicit
request, spend, and concurrency ceilings and write this package's prediction shape. The replay
command itself never contacts OpenRouter or starts local inference.
