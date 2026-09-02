# Channel recommendation protocol diagnosis — 2026-09-02

Issue #875 isolated the recommendation transport failure without inspecting or changing the frozen
`channel-recommendation-v1` certification holdout. The development fixture is
`channel-recommendation-development-v1`, digest
`24837f912b13477d7ac6d83b02c7c459e194d178d86dc7ef23f9e6bb25c67df4`. Its case ids and normalized
snapshot-content digests are mechanically disjoint from certification.

## Decision

Keep JSON mode and `channel-concept-prompt-v1`. Do not add a full structured-output transport or
change the output schema on this evidence. The repeatable hosted failure was output-budget handling:
GPT-OSS spent much of a 512-token completion allowance on reasoning and failed to reach root JSON on
two of five development cases. The same pinned model and provider completed all five structural
contracts with a 1,024-token allowance. Both local models completed all ten JSON-mode trials at the
512-token allowance; prompt-only transport completed root JSON on only one of ten.

This is a protocol diagnosis, not a model certification. It does not revise the no-ship decision in
the channel-recommendation certification report. A reasoning-model route would need a newly frozen,
untouched certification holdout and a full quality run at the 1,024-token candidate allowance before
any ship claim. The local models' certification gap remains model behavior—primarily abstention and
quality—not a demonstrated JSON transport defect.

## Content-free results

| Profile | Protocol / allowance | Root JSON | Required fields | Ceiling hits | Calls | Tokens | Exact cost |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Qwen 3.5 9B `6488c96fa5fa` | JSON mode / 512 | 5/5 | 5/5 | 0 | 5 | 1,094 | $0 |
| Qwen 3.5 9B `6488c96fa5fa` | prompt only / 512 | 1/5 | 1/5 | 4 | 5 | 3,490 | $0 |
| Gemma 4 12B `4eb23ef187e2` | JSON mode / 512 | 5/5 | 5/5 | 0 | 5 | 1,043 | $0 |
| Gemma 4 12B `4eb23ef187e2` | prompt only / 512 | 0/5 | 0/5 | 3 | 5 | 3,060 | $0 |
| GPT-OSS 20B · DeepInfra | JSON mode / 512 | 3/5 | 3/5 | 2 | 5 | 2,975 | $0.000287030 |
| GPT-OSS 20B · DeepInfra | JSON mode / 1,024 | 5/5 | 5/5 | 0 | 5 | 3,658 | $0.000382650 |

Every completed JSON object had zero unknown and zero effectful fields. The six runs totaled 30 calls,
15,320 tokens, and $0.000669680 in exact hosted charges. Total scorecard-accounted OpenRouter spend
across the planner, recommendation certification, and this diagnosis is now $0.082016440. Two earlier
events remain explicitly unledgered: one successful planner diagnostic and one ZDR-rejected request;
neither is converted into an inferred charge.

The machine-readable, content-safe rollup is
[`channel-recommendation-protocol-diagnosis-2026-09-02.json`](evidence/channel-recommendation-protocol-diagnosis-2026-09-02.json).
The source diagnostic artifacts remain gitignored under `.artifacts`; their hashes bind this report
without retaining prompts, raw responses, reasoning, generation ids, credentials, or provider payloads.

## Training and deployment consequence

Unsloth, LoRA/QLoRA, and Runpod remain closed. Adequate output allowance repaired the hosted structural
failure, while neither local model showed a development JSON-schema defect. Training cannot repair a
transport ceiling, and the existing certification evidence has not yet demonstrated a stable,
pillar-specific capability gap that warrants an adapter. No deployment route changes in this milestone.
