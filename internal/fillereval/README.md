# Filler admission certification

This package scores captured filler-admission decisions. It deliberately has no provider, network,
media-decoder, store, or application dependency: the same decision JSONL can be replayed after a
prompt, model, policy, or scorer change without spending money or changing production state.

The checked-in `corpus/seed-v1.json` is a schema and regression seed, **not a certification corpus**.
It contains synthetic evidence for failure classes such as conflicting years, brief end cards,
prompt injection, corrupt media, programme excerpts, and ambiguous compilation boundaries. A real
certification manifest references external, legally usable media by content hash and records source,
licence, similarity cluster, campaign, source family, split, labels, evidence, and slices. Non-redistributable media never
enters git.

Schema v5 distinguishes development seeds from certification manifests, preserves every inference
step in a multi-rung prediction, and carries campaign identity for diversity enforcement. A certification case also
locks its evidence packet and item metadata, records item-level rights adjudication and the bounded
source segment, and preserves two independent blind-review submission hashes. Matching submissions
become final directly; divergent submissions require a reasoned third-party adjudication. The report
records the exact manifest SHA-256 and scores only the explicitly selected development or holdout
split, so development examples cannot inflate certification.

Run `make filler-corpus-review` separately for each reviewer. It emits an independently shuffled,
reviewer-visible packet with random aliases and an owner-only map bound to the exact draft digest.
The packet excludes internal case IDs, split/cluster assignment, source filename, creator, campaign,
and labels. Keep each map from its reviewer. `make filler-corpus-lock` combines the draft, both maps,
and two independently authored JSONL review batches. Each line has `alias`, `reviewerId`, `batchId`,
`reviewedAt`, and `labels`; labels
contain disposition, reject class, content role, taxonomy, policy flags, slices, evidence, and the
answerable review question. A reviewer file must use one identity and one batch throughout. When the
two canonical label hashes differ, `LOOMARR_FILLER_CORPUS_ADJUDICATIONS` names a third JSONL file with
`caseId`, a distinct `adjudicatorId`, `adjudicatedAt`, `reason`, and final `labels`. The command writes
nothing until every draft case is covered and the complete certification manifest validates. The
draft must still be unlocked and contain no labels, reviews, or adjudication. Unknown and trailing
JSON fields fail rather than being retained as an implicit older format, and both blind submissions
must be complete even when a third reviewer chooses one side of a disagreement. There is no case-ID
review submission compatibility format because no completed certification artifact consumes one.

`make filler-corpus-archive` is the metadata-only acquisition preflight for Archive.org. It requires
an identified User-Agent, explicit snapshot time, request/item/per-item-byte/total-byte ceilings, and
a delay of at least 500 ms. It runs serially, caches the exact search and item responses, checks that
search and item licenses agree, excludes NC/ND licenses, records response hashes and retrieval times,
and selects a bounded video representation. Its output is only a candidate inventory: uploader
license metadata still needs independent item-level rights adjudication before it can enter a draft
manifest, and the command never downloads media or invokes a model.

LOC, NASA, CDC, and Commons adapters promote their bounded discovery lanes through the same strict
source-neutral inventory contract. `make filler-corpus-direct` adds the fixed 100-case modern
cohort without pretending a local folder grants rights: it requires 20 commercials, 20 promos, 25
bumpers, 25 station IDs, 5 trailers, and 5 PSAs; hashes 100 unique media payloads plus separate
rights and provenance evidence beneath one symlink-safe root; and rejects any quota, path, byte, or
wall-time violation. Public and direct inventories combine before one independent rights review.

`make filler-corpus-download` is the separately authorized media step. It accepts only `approved`
rights rows tied to the exact inventory and metadata SHA-256 values, reviewer, review time, rationale,
redistribution decision, attribution, and restrictions; `held` rows remain out of the plan. Before the first request
it proves the approved count and predicted bytes fit explicit ceilings. Downloads remain serial and identified,
redirects stay within each authority's frozen and built-in host policy, and bodies cannot exceed
their recorded size. Source checksums are checked when present, and the external ledger adds a
locally computed SHA-256. Already-local direct-cohort cases are not downloaded again. A
failed or stale approval writes no ledger and cannot silently widen the selected corpus.

`make filler-corpus-rights-review` converts a frozen mixed-authority inventory into a deterministic worksheet
bounded by explicit minimum and maximum item counts. It exposes the source assertions and selected
representation in immutable JSON plus a spreadsheet-safe CSV, but leaves every authority field
blank. Reviewers edit only `reviewer_id`, `reviewed_at`, `decision`, `basis`, `redistributable`,
`required_credit`, and `restrictions_json`. Local rows expose the exact media, rights-evidence, and
provenance-evidence paths and hashes. This is a review queue, not evidence that any row is legally reusable.

`make filler-corpus-rights-lock` validates the completed CSV against both the original byte-exact
inventory and the inert JSON worksheet. Every row must be present once, immutable source fields must
match, decisions must be complete and time-bound, and approved BY/BY-SA media must carry attribution.
Only a fully valid review is atomically converted to the JSONL consumed by the downloader.

`make filler-corpus-prepare` is the next mechanical boundary. It accepts only the complete approved
inventory plus an authored split/cluster/segment plan, re-hashes every source file, measures the
bounded segment, and stages the four frame and direct-video derivatives under aggregate resource
ceilings. It writes an unlabeled provenance-complete draft and the exact label-blind packet JSONL
consumed by the paid runner. The plan cannot contain truth, taxonomy, policy flags, evidence labels,
or a review answer; those exist only in the two independent review submissions.

`make filler-corpus-pilot-rights-review` prepares the distinct five-lane source-yield review packet
from the checked-in locked pilot. Its fifty rows bind every source assertion and representation to
the pilot digest while leaving all reviewer fields blank. `make filler-corpus-pilot-rights-lock`
requires one independently attested reviewer to complete every row, reports whether each lane has at
least five rights-approved and product-relevant candidates, and emits `downloadAuthority: false`.
This qualifies or rejects an adapter lane; it never authorizes acquisition.

`make filler-eval-contract` verifies the scorer and seed. `make filler-eval-cert` scores a JSONL file
named by `LOOMARR_FILLER_EVAL_PREDICTIONS`; the remaining `LOOMARR_FILLER_EVAL_*` variables identify
the corpus, selected split, captured run time, every versioned input, and positive request, spend,
and concurrency ceilings. The scorer is fail-closed: fewer than 1,126 independently clustered holdout cases, missing or
duplicate predictions, cross-split similarity leakage, incomplete attribution, operational failure,
wrong role/taxonomy, exceeded run ceilings, weak confidence bounds, or a missed
precision/coverage/review gate produces a
non-certifying report and nonzero exit.

Predictions record the inference role/rung, requested and resolved route, derivative bounds, detailed
token categories, attempts, generation id, and the provider's exact charged decimal alongside an
integer nanodollar projection. A failed call with missing or malformed settlement keeps those charge
fields missing and records its still-consumed reservation separately. Reports compare total cost, cost per correct automation, cost per
admit, and per-slice/per-rung cost; they never sum provider charges with binary floating point. The
scorer never reads its wall clock, and reports one-sided confidence bounds for every selective-risk,
coverage, review, and slice measure used in certification.

Provider execution belongs to `internal/fillerbakeoff`. That runner requires explicit request,
spend, and concurrency ceilings, accepts only locked certification manifests and label-blind
content-addressed packets, re-hashes external derivatives before spend, escalates through typed
text/frame/video/premium routes on named evaluator reasons, and writes this package's prediction shape. Multi-rung predictions retain
one immutable inference step per call so per-rung cost and attempts are not collapsed into the
terminal route. There is no scalar inference-ledger compatibility shape: schema v5 captures every
attempt in `steps`, while deterministic outcomes and pre-request holds have no step. An explicit
semantic abstention is a successful step with a bounded reason and no evidence; it is not rewritten
as provider failure and cannot be referenced as support. The replay command itself never contacts
OpenRouter or starts local inference.

`make filler-bakeoff-openrouter` is the paid/manual capture boundary. It consumes a locked manifest,
label-blind packet JSONL, external derivative root, and strict versioned JSON containing the run,
admission policy, and ordered routes. It also requires the immutable output of
`make filler-openrouter-snapshot`; both run snapshot identities must equal that artifact's SHA-256,
and every route must match a live ZDR endpoint recorded within the preceding 24 hours.
`OPENROUTER_API_KEY` is read only from the environment. The
adapter performs one request per reserved rung, pins the upstream provider with fallback disabled,
requires strict structured output and ZDR routing, records OpenRouter routing metadata and exact
`usage.cost`, and writes a private atomic prediction JSONL for separate `filler-eval-cert` replay.
