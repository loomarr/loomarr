# Channel recommendation certification v2 — 2026-09-02

No candidate certified on the untouched `channel-recommendation-v2` contract. The result remains
no-ship: no shared planner model or distinct recommendation route is selected, and no Unsloth,
LoRA/QLoRA, Runpod, deployment, or production authority is justified.

The frozen contract used fixture
`2caf971fd7ad14cc9c673c6c4bf92d481305086e9506d0fd67eb8f63cff1e17c`,
`channel-concept-prompt-v1`, `channel-concept-schema-v1`,
`channel-recommendation-scorer-v1`, JSON mode, and exactly 1,024 maximum output tokens per call.
The retained v1 no-ship holdout and the development corpus were not inspected or changed.

| Profile | Result | Hard failures | Mean quality | Calls | Tokens | Exact spend |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| `gemma4-recommendation-v2` | 4/8 cases; reject | 0 | 0.893 | 8 | 1,862 | $0 |
| `qwen35-shared-v2` | 1/8 cases; reject | 4 invalid-schema | 0.393 | 8 | 1,821 | $0 |
| `gpt-oss20b-deepinfra-v2` | operational failure; reject | 0 observed | 0.000 | 0 attributable | 0 attributable | incomplete |

Gemma was structurally sound and cleared novelty, diversity, catalog feasibility, policy safety, and
schema validity. It missed relevance and abstention at 0.625 against 0.800 floors. Its eight-case p95
model latency was 3.495 seconds. This is one small held-out trial: it identifies a candidate residual
quality gap but does not prove the repeatable trainable gap required to authorize fine-tuning.

Qwen missed every quality floor, emitted four invalid-schema hard failures, and had 4.085-second p95
model latency. Raising the output ceiling fixed the earlier hosted reasoning truncation diagnosis; it
did not make this local Qwen artifact suitable for recommendation.

The OpenRouter key preflight returned HTTP 200, but the pinned `openai/gpt-oss-20b` / DeepInfra route
returned an upstream 429 before attributable inference. The runner stopped on the first case. Per the
predeclared contract it was not retried and no alternate upstream was used. Its scorecard has zero
observed calls, tokens, and charge but incomplete accounting, so those zeros are not a verified
zero-cost inference claim.

The two completed local lanes account exactly for 16 calls, 3,683 tokens, and $0. The program's exact
scorecard-accounted OpenRouter spend therefore remains $0.082016440, while the new upstream failure
raises the explicitly unledgered event count from two to three. The matrix's $0.005 hosted ceiling was
not raised or reallocated.

Do not tune against this holdout or repeat it to search for a favorable draw. The next training
decision needs disjoint development evidence showing that Gemma's relevance/abstention gap repeats
across a materially larger set and is not better owned by deterministic ranking or abstention policy.
Any later release claim needs another untouched certification contract. Until then, retain stock
Gemma only as a development candidate and keep Unsloth and Runpod closed.

The content-safe machine rollup is
[`channel-recommendation-certification-v2-2026-09-02.json`](evidence/channel-recommendation-certification-v2-2026-09-02.json).
Raw scorecards remain gitignored; their SHA-256 identities are pinned in that rollup.
