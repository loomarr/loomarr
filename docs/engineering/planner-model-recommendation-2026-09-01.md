# Planner-model recommendation

Date: 2026-09-01

## Decision

Do **not** ship or fine-tune a planner model from this experiment yet. No measured artifact cleared
the frozen `planner-certification-v3` contract, and the first failure cluster is repeated tool use
until the production Suggester's five-tool-call boundary is exhausted. Diagnose that shared
tool-result-to-final-answer transition before spending the Runpod reference budget or producing a
LoRA. Training around a prompt, template, or adapter defect would make the defect harder to see.

This is a channel-curation decision only. It does not reject model assistance for Loomarr's other
two pillars and does not transfer any certification evidence to them.

## Frozen decision contract

- Corpus: `planner-certification-v3`, 150 held-out Intents over 25 semantic families.
- Catalog fixture digest:
  `3c40ae9834cf5652388d09334721459296bab2424c939358d8fc10ae71823d89`.
- Base manifest digest:
  `70934f608f7e3a2ebc1e00e9c92b402b231805caccb5cb3c6d5dda5d6f2969da`.
- Scorer: `planner-scorer-v2`; scorecard schema 10.
- Quality margin: two percentage points; only fully certified candidates are eligible.
- Initial execution: one trial per artifact with an 18-minute suite context, explicit call/token/USD
  ceilings, and no judge. This is a bounded bake-off, not the required three-trial release
  certification.

## Measured evidence

| Lane | Hard passes | Weighted quality | p95 model latency | Residency | Spend | Decision |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| Qwen 3.5 9B, Ollama Q4_K_M | 44/150 | 0.312 | 24.69s | 5.24 GiB | $0 | Reject |
| Gemma 4 12B, Ollama Q4_K_M | 11/150 | 0.161 | 12.10s | 7.51 GiB | $0 | Reject |
| gpt-oss 20B, OpenRouter DeepInfra BF16 | 1/150 | 0.069 | 43.24s | unavailable | $0.03859952 | Reject |
| Qwen 3.5 9B, OpenRouter DeepInfra BF16 | 0/150 | 0.024 | 113.75s | unavailable | $0.03290770 | Reject |

The local Qwen run made 433 model calls and reported 1,290,275 tokens before the suite context
expired. Thirty-six cases exhausted the Suggester tool boundary; the timeout then made provider
usage unavailable and correctly latched the remaining 70 cases as budget failures. The hosted BF16
route made 105 calls and reported 320,531 tokens before the same bounded outcome: 18 generation
failures followed by 132 fail-closed accounting failures. Its exact upstream was DeepInfra with
fallbacks disabled, required parameter support, data collection denied, and zero-data-retention
routing requested.

Both Qwen lanes failed every aggregate floor and exceeded the p95 limit with five tool calls. The
hosted BF16 result is worse than the local quantized result, so additional numeric precision or an
NVIDIA host alone is not a supported remedy. The shared prompt/tool/adapter path is the first seam to
inspect.

Gemma was faster per measured local call than Qwen, but completed only 11 schema-valid cases and
scored about half as well overall while consuming 7.51 GiB. The gpt-oss structured-output control
produced one valid completion and also repeated tool calls to the ceiling. Neither changes the
diagnosis. Local Qwen is the best preliminary artifact, but its 0.312 score is not a near miss: it
remains ineligible and is not a shipping or training-base recommendation.

The reproducible compact record, including exact scorecard digests and failure counts, is
[the checked-in comparison evidence](evidence/planner-model-comparison-2026-09-01.json).

Exact local artifacts:

- Qwen: Ollama `qwen3.5:9b`, manifest
  `sha256:dec52a44569a2a25341c4e4d3fee25846eed4f6f0b936278e3a3c900bb99d37c`, Q4_K_M,
  Ollama 0.33.0.
- Gemma: Ollama `gemma4:12b`, manifest
  `sha256:1278394b693672ac2799eadc9a83fd98259a6a88a40acfb1dcaa6c6fc895a606`, Q4_K_M,
  Ollama 0.33.0.

Machine: Apple arm64 with 24 GiB unified memory. The planned 64 GiB Mac mini will provide much more
headroom, but this run shows that capacity is not the current blocker: both local artifacts fit
fully in unified memory without CPU spill. The larger Mac remains useful for local inference,
dataset preparation, MLX experiments, and release validation; it is not a substitute for the
publisher-weight NVIDIA reference lane.

## Three-pillar applicability

| Pillar | Model role | What this evidence permits |
| --- | --- | --- |
| Channel recommendation | Draft grounded channel concepts and Intents | Reuse the text/tool adapter as a starting point only. Build and pass #855's independent corpus before release. |
| Channel curation | Produce and refine grounded Proposals and ChannelPolicy | This is the measured pillar. No candidate is currently eligible. |
| Filler curation | Classify, tag, summarize, and rank bounded multimodal evidence | No transfer. Continue #549, #555, and #787; a separate multimodal model or adapter may win. |

Identity, grounding, approval, authorization, rights, admission, deletion, scheduling, and playback
remain deterministic Go authority regardless of model choice.

## Training framework

If a stock model later passes tool/structure behavior but retains a repeatable domain-policy or
proposal-ranking gap, use a pinned [Unsloth](https://github.com/unslothai/unsloth) LoRA/QLoRA recipe
as the first training implementation. Unsloth currently supports Gemma 4, Qwen 3.5, NVIDIA,
macOS/MLX, LoRA/QLoRA, and GGUF export. Amend
the design dependency and release contract before adding the offline recipe. Keep certification
Intents excluded from train/development data, record the base digest and adapter digest, and rerun
the untouched holdout after export.

Do not fine-tune merely because a stock artifact loses this bake-off. Tool-loop, chat-template,
provider-accounting, or runtime failures require adapter work first.

## Budget and infrastructure

The experiment has a $20 aggregate hard stop:

- OpenRouter transport allowance: $5. Observed scored spend is $0.07150722; two tool-call smokes add
  $0.00004760, for an exact experiment total of $0.07155482.
- Runpod A100 allowance: $10. OAuth is connected, the seven-day billing baseline is $0, and no Pod
  or volume exists. Live prices were $1.19/hour for community A100 PCIe 80 GB and $1.39/hour for
  secure PCIe or community SXM 80 GB.
- Operational reserve: $5.

Do not provision the A100 until the adapter smoke can consume one catalog result and emit one valid
final Proposal. Once that passes, use the Runpod reference lane to separate official weights from
the distribution quantization; stop and delete the Pod immediately after artifacts and digests are
copied.

## Next experiment

1. Complete #862: add a minimal diagnostic that captures the model's call immediately after a catalog tool result,
   including thinking-disabled behavior and the exact rendered chat template.
2. Correct the provider-neutral tool-result-to-final-answer contract without weakening grounding or
   raising the five-tool boundary.
3. Re-run one representative case from every semantic family against the same Gemma, Qwen, and
   structured-output control artifacts.
4. Run the complete one-trial corpus only for artifacts that clear the smoke set; then run three
   trials for release certification.
5. Provision the A100 reference lane only for the surviving official-weight candidate.
6. Authorize Unsloth LoRA/QLoRA only if the surviving stock model has a stable pillar-specific
   quality gap rather than an adapter/runtime failure.

Candidate and infrastructure sourcing is recorded in
[the stock-model bake-off research](research/planner-stock-model-bakeoff-2026-09-01.md).
