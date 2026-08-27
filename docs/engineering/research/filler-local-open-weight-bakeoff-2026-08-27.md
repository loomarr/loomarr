# Local open-weight filler bakeoff

Date: 2026-08-27

## Decision

Keep Gemma 4 26B A4B as the provisional local first-pass candidate. Its Q4_0 QAT build fit a 12 GB
RTX 3080 Ti with CPU offload and matched the hosted run's 11/11 transcript-plus-metadata role score.
It is usable for serial background ingestion, not interactive classification: the contended local
median was 29.1 seconds and the longest compilation case took 94.9 seconds.

This is the same small, title-rich development sample used by the earlier temporal experiment. It is
not independently adjudicated and does not satisfy issue #555. The result establishes that local
execution is real and viable on representative enthusiast hardware; it does not establish
certification accuracy, an appliance minimum, or a safe concurrency above one.

## Locked environment

- Model tag: `gemma4:26b-a4b-it-qat`
- Ollama registry digest: `2dd70431afed94dd3688d790443768c1487ed086b57147ff083851116ae4c4e4`
- Model file: 13.43 GiB GGUF, Q4_0 QAT; 25.2 billion total and about 4 billion active parameters
- Server: Ollama 0.32.14 in an isolated GPU-enabled container bound to `127.0.0.1:11435`
- Hardware: NVIDIA RTX 3080 Ti, 12,288 MiB; Intel i9-12900K; 32 GiB system RAM; no swap
- Input: one shared transcript plus Archive title, creator, date, and collections
- Output: one closed role enum plus bounded brand, programme/product, date, and evidence fields
- Execution: concurrency one, temperature zero, 4,096-token context, 400-token output ceiling,
  thinking disabled, strict JSON Schema

The official model card identifies the checkpoint and licence; the official Ollama registry records
the available quantizations and sizes ([Google model card](https://huggingface.co/google/gemma-4-26B-A4B-it),
[Ollama tags](https://ollama.com/library/gemma4/tags)). "Open-weight" remains the comparison term:
local executability and an open licence do not by themselves establish redistribution or appliance
acceptance for every model in the matrix.

## Results

| Metric | Result |
| --- | ---: |
| Cases | 11 |
| Valid structured responses | 11/11 |
| Correct roles, all cases | 11/11 |
| Median warm latency | 29.074 s |
| Longest warm latency | 94.899 s |
| Total warm matrix time | 384.125 s |
| Prompt tokens | 4,755 |
| Completion tokens | 1,404 |
| Sampled output throughput | 3.13–12.20 tokens/s |
| Sampled peak total GPU memory | 10,852 MiB |

The first cold probe intentionally used the model default and failed: it consumed the 300-token
ceiling in hidden reasoning and returned empty content. That is an operational failure, not a partial
success. Cold load took 57.8 seconds and the failed call took 110.0 seconds end to end. Setting
`think: false` made all 11 subsequent responses valid. The maintained adapter therefore requires
thinking off and rejects a thinking, empty, truncated, or malformed response.

The longest case was the deliberately invalid multi-clip compilation. Its 1,085-token prompt was
classified correctly, but it generated an unnecessarily verbose 264-token product list. The
certification extractor uses a tighter fact schema and 512-token hard ceiling; future balanced-corpus
runs must retain timeout and output-budget failures in the denominator.

## What this changes

1. Gemma remains the local survivor; Qwen remains the second open-weight comparator rather than a
   production default.
2. Local routes use the same label-blind packet/evaluator/replay boundary as hosted routes. They do
   not receive gold labels or a friendlier scoring path.
3. The local adapter binds the configured tag to the installed registry digest before inference,
   accepts loopback HTTP only, disables thinking, permits one text route and one attempt, and records
   exact token and latency attribution.
4. The certification run stays at concurrency one. A later idle-host run may characterize the
   contention delta, but cannot replace this realistic shared-host evidence.
5. Promotion still requires the 300-case rights-approved development set, 1,126-case independent
   holdout, blind adjudication, and the predeclared issue #555 thresholds.
