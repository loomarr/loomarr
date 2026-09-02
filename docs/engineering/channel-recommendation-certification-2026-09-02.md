# Channel recommendation certification — 2026-09-02

Issue #855's first frozen recommendation contract produced a valid no-ship decision. No stock
candidate certified, neither a shared planner model nor a distinct recommendation route is selected,
and the result does not justify Unsloth, LoRA/QLoRA, or Runpod work.

The contract is `channel-recommendation-v1`, fixture digest
`549966d2a7add2edc16033d13a76f5a893d0355fe53acc8b953887a7cb16da01`, prompt
`channel-concept-prompt-v1`, output schema `channel-concept-schema-v1`, and scorer
`channel-recommendation-scorer-v1`. All runs used the same eight certification families and the
pre-registered equal `mean_quality` selection metric with a `0.02` margin. Certification data remains
excluded from the training allowlist.

| Profile | Route | Cases | Hard failures | Tokens | Exact spend | Mean quality | Decision |
| --- | --- | ---: | ---: | ---: | ---: | ---: | --- |
| `qwen35-shared` | Ollama `qwen3.5:9b` · `6488c96fa5fa` | 4/8 | 1 | 1,941 | $0 | 0.786 | reject |
| `gemma4-recommendation` | Ollama `gemma4:12b` · `4eb23ef187e2` | 2/8 | 0 | 1,629 | $0 | 0.804 | reject |
| `gpt-oss20b-deepinfra-control` | OpenRouter · DeepInfra | 2/8 | 6 | 5,523 | $0.000565320 | 0.250 | reject |
| `claude-haiku45-bedrock-control` | OpenRouter · Amazon Bedrock | 0/8 | 8 | 3,098 | $0.008938000 | 0.000 | reject |

The four comparable scorecards account for 32 calls, 12,191 tokens, and exactly $0.009503320. The
separate GPT-4.1 Mini/OpenAI attempt was rejected before attributable inference because that pinned
endpoint did not satisfy the required zero-data-retention policy. Its ledger is incomplete, so its
zero observed calls/tokens/charge are not counted as a verified zero-cost claim.

The failures are not one repeatable model-quality gap. Qwen missed relevance, feasibility, policy,
schema, and abstention floors; Gemma mostly emitted safe abstentions and passed only two cases;
gpt-oss exhausted the output budget in reasoning on most cases; and the ZDR-capable Claude route
returned eight root-schema failures. Tuning the prompt or output contract against these heldout
results would contaminate the certification set.

The next safe step is a separate digest-pinned `development` corpus and content-safe protocol
diagnostic. It may inspect structural failure classes, never certification expectations, and may
change the prompt/schema only under a new version. A changed contract then needs a new untouched
certification holdout. Fine-tuning becomes eligible only if stock models still exhibit a stable,
pillar-specific quality gap after that protocol is sound.

Machine evidence is in
[`channel-recommendation-certification-2026-09-02.json`](evidence/channel-recommendation-certification-2026-09-02.json).
The source scorecards remain local content-safe run artifacts; their SHA-256 hashes are pinned in the
machine record.
