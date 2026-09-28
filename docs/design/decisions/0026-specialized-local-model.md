# 0026. A specialized local model is an optional Suggester optimisation

- **Date:** 2026-09-02
- **Status:** accepted

## Context

Small open-weight models are unreliable at choosing the right Catalog operation, honouring Intent
qualifiers and producing the Proposal schema. Fine-tuning one for Loomarr could help, but it could
also become a second authority or a hard dependency of local AI.

## Decision

A Loomarr-tuned model is an optional optimisation of the existing Suggester, never a new authority
and never a prerequisite. It learns tool choice, qualifiers, schema and abstention, not title
identity, ratings, quotas or authorization: every pick still passes the grounding chokepoint
([0004](0004-grounding.md)). Certification comes first and may end the project; hard gates are
pass/fail, not terms in a weighted score.

Training and conversion live in the separate `loomarr-models` repository. Loomarr consumes a
certified model only through its HTTP provider boundary; no trainer, converter or weights enter
this repository or its image.

## Consequences

- The ordinary AI profile stays model-less; hosted and custom providers stay first-class.
- A new model version is pulled explicitly, verified by digest, probed and canaried before
  activation, and the previous one stays available for rollback.
- The experiment protocol is archived in
  [`local-model-experiment.md`](../../../project/design-archive-2026-09/local-model-experiment.md).
