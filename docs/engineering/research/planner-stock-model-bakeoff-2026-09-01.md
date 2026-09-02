# Planner stock-model bakeoff

Date: 2026-09-01

## Recommendation

Run a four-model certification, but make the actual selection contest **Gemma 4 12B versus Qwen
3.5 9B**. Use `openai/gpt-oss-20b` and Mistral Small 3.2 24B as diagnostic comparators: they reveal
whether native structured output or a larger dense model materially improves Loomarr's grounded
planner, without turning the first experiment into an open-ended model survey.

Do not choose Gemma—or train any model—before this run. Gemma 4 is now a much stronger candidate
than Gemma 3: it has native function-calling control tokens, a system role, a 256K context at 12B,
official QAT builds, and Apache-2.0 weights. Qwen 3.5 9B is the provisional challenger-to-beat because
it is smaller, explicitly agent-trained, has a native 262,144-token context, and has documented
tool-call parsers in both vLLM and SGLang. The held-out Loomarr score, not general benchmarks, should
decide between them.

Use two lanes:

1. **Distribution-realistic lane:** the exact Ollama package that a Loomarr user would pull, with
   thinking disabled where the production adapter does so. Record the immutable Ollama digest and
   installed quantization before the run.
2. **Reference lane:** official publisher weights behind one OpenAI-compatible vLLM endpoint on an
   NVIDIA A100 80 GB. This distinguishes a weak model from a weak local quantization/runtime.

The Mac mini can run every distribution-realistic package below in 64 GB unified memory, but that is
a deployment and latency test, not a numerically identical substitute for the NVIDIA reference lane.

## Candidate matrix

| Candidate | Frozen identifiers | Context and tool/structure support | Licence and deployment constraints | Why it is in the bakeoff |
| --- | --- | --- | --- | --- |
| **Gemma 4 12B Instruct** | Publisher: `google/gemma-4-12B-it`; Ollama: `gemma4:12b` | Google documents 256K context for the 12B class, native system prompts, and native function calling. The tool protocol has dedicated declaration, call, and response tokens. The official 12B memory estimates are 26.7 GB BF16, 13.4 GB SFP8, and 6.7 GB Q4_0; Ollama's current package is 7.6 GB and advertises `tools`. General JSON-Schema-constrained final output is not separately promised by the publisher, so Loomarr must retain its own parsing and validation. ([overview](https://ai.google.dev/gemma/docs/core), [function calling](https://ai.google.dev/gemma/docs/capabilities/text/function-calling-gemma4), [Ollama package](https://ollama.com/library/gemma4)) | The exact repository declares Apache 2.0. This is different from older Gemma releases governed by the Gemma-specific terms; Google's terms page explicitly sends Gemma 4 users to the Gemma 4 licence. Transformers function-calling examples require `transformers>=5.10.1`, so older serving images are not interchangeable. ([model repository](https://huggingface.co/google/gemma-4-12B-it), [older-version terms boundary](https://ai.google.dev/gemma/terms)) | Primary product candidate: current Gemma generation, native tools, compact official QAT, and a first-party Ollama package suitable for a 64 GB Mac. |
| **Qwen 3.5 9B** | Publisher: `Qwen/Qwen3.5-9B`; Ollama: `qwen3.5:9b`; OpenRouter: `qwen/qwen3.5-9b` | Native context is 262,144 tokens. The publisher calls out agentic training and tool calling; the documented vLLM launch requires `--enable-auto-tool-choice --tool-call-parser qwen3_coder`, and SGLang likewise needs `--tool-call-parser qwen3_coder`. OpenRouter currently exposes `tools`, `tool_choice`, and `response_format`. ([model card](https://huggingface.co/Qwen/Qwen3.5-9B), [OpenRouter route](https://openrouter.ai/qwen/qwen3.5-9b/pricing), [Ollama tags](https://ollama.com/library/qwen3.5/tags)) | Apache 2.0. At the snapshot date the publisher requires main/nightly vLLM or SGLang rather than an ordinary stable serving image; freezing the serving commit is therefore part of the experiment identity. The current Ollama package is 6.6 GB and advertises vision, tools, and thinking. | Provisional challenger-to-beat: smallest serious candidate, strong explicit agent/tool training, and the lowest local footprint in the main contest. |
| **OpenAI gpt-oss 20B** | Publisher: `openai/gpt-oss-20b`; Ollama: `gpt-oss:20b`; OpenRouter: `openai/gpt-oss-20b` | 21B total/3.6B active parameters, 128K context, native function calling and Structured Outputs. The released MXFP4 weights fit in about 16 GB; Ollama's current artifact is 14 GB and advertises tools and thinking. ([OpenAI release](https://openai.com/index/introducing-gpt-oss/), [model repository](https://huggingface.co/openai/gpt-oss-20b), [Ollama tags](https://ollama.com/library/gpt-oss/tags)) | Apache 2.0, subject also to OpenAI's gpt-oss usage policy. The Harmony response format is mandatory; a runtime or template that silently treats it as ordinary chat is not a valid comparison. It is text-only and its full chain of thought must not be shown to users. | Structured-output control: if it wins schema validity but not grounded proposal quality, that identifies an adapter/format problem rather than a reason to adopt it. |
| **Mistral Small 3.2 24B Instruct** | Publisher: `mistralai/Mistral-Small-3.2-24B-Instruct-2506`; Ollama: `mistral-small3.2:24b-instruct-2506-q8_0`; OpenRouter: `mistralai/mistral-small-3.2-24b-instruct` | 128K/131,072 context, a robust native function-calling template, and JSON output. The publisher's vLLM command enables `--tool-call-parser mistral` and `--enable-auto-tool-choice`; OpenRouter exposes tools and JSON-schema `response_format`. ([model card and vLLM recipe](https://huggingface.co/mistralai/Mistral-Small-3.2-24B-Instruct-2506), [OpenRouter route](https://openrouter.ai/mistralai/mistral-small-3.2-24b-instruct), [Ollama tags](https://ollama.com/library/mistral-small3.2/tags)) | Apache 2.0. Publisher BF16/FP16 inference needs about 55 GB GPU RAM and its example uses tensor parallelism; use the A100 80 GB reference host or the explicit 26 GB Ollama Q8 package, not an unlabeled conversion. The publisher recommends temperature 0.15, which must be recorded if it differs from the frozen certification protocol. | Larger dense control: tests whether the compact models are losing grounded planning quality because of capacity rather than prompt/tool formatting. |

The Ollama tag is not the identity by itself: tags can move. Each scorecard must add the registry
digest, byte size, quantization, Ollama version, runtime parameters, context allocation, and whether
thinking was enabled. The behavioral capability probe remains authoritative even when the registry
advertises `tools`.

## Hosted NVIDIA and managed-provider options

### Controlled reference host

**Runpod Pod, A100 PCIe 80 GB** is the simplest first choice. Runpod's live catalog returned
$1.19/hour for community cloud and $1.39/hour for secure cloud during setup; Pods are billed per
second. A single 80 GB card has room for the publisher-reported
55 GB Mistral checkpoint and the other three candidates, avoiding a hardware change inside the
reference lane. The same page lists L40S 48 GB at $0.99/hour, but it does not fit Mistral's reported
BF16 requirement and would force a quantized or split comparison. Runpod network storage is
$0.07/GB/month under 1 TB. ([Runpod pricing](https://www.runpod.io/pricing))

**Modal** is the best scripted/serverless alternative when automatic teardown matters more than an
interactive VM. Its current rates are $0.000694/second ($2.4984/hour) for A100 80 GB and
$0.000542/second ($1.9512/hour) for L40S, plus separately metered CPU and memory. Modal explicitly
recommends L40S for inference and permits multiple GPUs, but its container lifecycle and extra
resource meters make cost attribution less direct for this first bakeoff. ([GPU selection](https://modal.com/docs/guide/gpu),
[pricing](https://modal.com/pricing))

**Lambda Cloud** is a conventional-VM fallback if Runpod capacity is unavailable. Its on-demand list
currently shows A6000 48 GB at $1.09/GPU-hour, A100 40 GB at $1.99, and H100 80 GB at $4.29. The
A6000 is suitable for the three smaller/reference-quantized candidates, but the full Mistral BF16
lane needs an 80 GB option or model parallelism. ([Lambda instances](https://lambda.ai/instances))

**NVIDIA Build/NIM** is useful only as a free smoke-test endpoint when the exact checkpoint is in its
catalog. NVIDIA grants Developer Program members hosted prototype endpoints and research/development
NIM use on up to 16 GPUs, but production NVIDIA AI Enterprise starts at $4,500/GPU/year. It is not a
cost-appropriate production dependency for this experiment, and a different checkpoint in the NIM
catalog must not stand in for one of the four frozen candidates. ([NIM access and pricing](https://docs.api.nvidia.com/nim/re/docs/run-anywhere),
[model catalog](https://build.nvidia.com/models))

### Managed inference for transport checks

OpenRouter is already Loomarr's blessed hosted path and is useful for checking the production
OpenAI-compatible transport independently of self-hosted vLLM. At this snapshot it lists:

| OpenRouter slug | Input / output per 1M tokens | Notes |
| --- | ---: | --- |
| `qwen/qwen3.5-9b` | $0.10 / $0.15 | Five upstream providers; tools and response format exposed. ([route](https://openrouter.ai/qwen/qwen3.5-9b/providers)) |
| `openai/gpt-oss-20b` | from $0.029 / $0.14 on the pricing view | Multiple quantizations/providers; pin the provider and quantization or treat this only as a transport check. ([route](https://openrouter.ai/openai/gpt-oss-20b/pricing)) |
| `mistralai/mistral-small-3.2-24b-instruct` | $0.075 / $0.20 | Tools and JSON-schema response format exposed. ([route](https://openrouter.ai/mistralai/mistral-small-3.2-24b-instruct)) |

OpenRouter did not expose the exact Gemma 4 12B route in the checked catalog. SiliconFlow advertises
`google/gemma-4-12B-it` at $0.10/$0.30 per million tokens with tools and JSON mode, but its same page
contains contradictory parameter/architecture metadata and says Structured Outputs are unsupported;
use it only through Loomarr's Custom endpoint after the behavioral probe, not as the canonical
reference. ([SiliconFlow model page](https://www.siliconflow.com/models/gemma-4-12b-it))

Google's Gemini API is also not an exact substitute: it currently hosts only
`gemma-4-31b-it` and `gemma-4-26b-a4b-it`, not 12B. Gemma 4 access there is free-tier only, with no
paid token tier, and free-tier content may be used to improve Google's products. That is useful for
manual exploration of the larger family members, not for the frozen 12B decision. ([supported
models](https://ai.google.dev/gemma/docs/core/gemma_on_gemini_api), [pricing and data treatment](https://ai.google.dev/gemini-api/docs/pricing))

## Budget and execution envelope

The maintainer authorized a **$20 absolute stop** for the initial experiment. Do not provision or
call beyond this aggregate total without a new explicit authorization:

| Envelope | Cap | Purpose |
| --- | ---: | --- |
| OpenRouter transport runs | $5 | Enough for the three low-cost listed routes with per-run token and spend limits; provider/quantization must be pinned. |
| Runpod A100 80 GB | $10 | At most about 7.2–8.4 GPU-hours at the live $1.19–$1.39/hour Pod rates, including model-load and failed-run time; use ephemeral storage. |
| Operational retry reserve | $5 | Held back for one clean rerun after a transport or capacity failure. |
| **Absolute maximum** | **$20** | Stop before provisioning or calling beyond this total. |

Run one tool-capability smoke case before the full corpus, then one complete trial per model before
starting repetitions. An operationally invalid model should fail visibly rather than consume the
remaining repetition budget. Keep the same frozen catalog, prompt/tool contract, token ceilings,
sequential tool loop, and scorer for every model. Separate results by lane; never pool Ollama and
vLLM trials into one quality rate.

## Selection rule

Promote a stock model only if it clears every pre-registered planner-certification threshold and its
lower-confidence quality margin over the runner-up is large enough to survive ordinary trial
variance. Among models inside that quality equivalence band, choose in this order:

1. valid grounded tool operation and abstention behavior;
2. distribution-realistic p95 latency and peak resident memory;
3. simplest faithful Ollama package and permissive redistribution terms;
4. hosted cost only as a final tiebreak.

If no stock model clears the thresholds, inspect failure clusters before authorizing LoRA. Repeated
domain-policy or proposal-ranking errors may justify tuning; malformed tool calls, Harmony/template
mistakes, or runtime-specific failures justify adapter work instead. A fine-tune is not the next step
merely because Gemma loses the bakeoff.

## Measured outcome

The bounded first run is complete. No lane was eligible: local Qwen scored 0.312, local Gemma
0.161, hosted gpt-oss 0.069, and hosted Qwen 0.024. All four hit the five-tool-call p95 ceiling and
failed the frozen aggregate thresholds. The result changes the immediate priority from model
selection to the shared tool-result-to-final-answer adapter seam. See the
[dated recommendation](../planner-model-recommendation-2026-09-01.md) and its checked-in evidence
for exact metrics, costs, and immutable scorecard digests.
