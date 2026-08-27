# Local open-weight filler bakeoff

Date: 2026-08-27

## Decision

Keep Gemma 4 26B A4B as the provisional local first-pass candidate. Its Q4_0 QAT build fit a 12 GB
RTX 3080 Ti with CPU offload and matched the hosted run's 11/11 transcript-plus-metadata role score.
It is usable for serial background ingestion, not interactive classification: the contended local
median was 29.1 seconds and the longest compilation case took 94.9 seconds.

Reject Qwen 3.8 27B Q4_K_M as a route on this hardware profile. The exact same 11-case role task
produced one valid, correct response in 295.6 seconds and ten five-minute timeouts. This is a local
execution and hardware-fit failure, not evidence that the model lacks the semantic capability: its
earlier hosted lane returned a correct role for every valid response, but also had one truncated JSON
failure. A larger accelerator or materially smaller quantization would be a different route and would
need its own measured identity and run.

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

The second comparator kept that protocol fixed:

- Model tag: `qwen3.8:27b-q4_K_M`
- Ollama registry digest: `25b843619e944cd0ae6069f94ff4e5e26a16e109ccbc0a66a0f05979ed70098e`
- Installed package: 17.45 GB GGUF, Q4_K_M; 27.3 billion parameters
- Server: the same Ollama 0.32.14 image in an isolated GPU-enabled container bound to
  `127.0.0.1:11436`
- Loaded allocation: 8.38 GB reported in VRAM and 5.19 GiB sampled container RAM; the host and
  evidence protocol otherwise matched the Gemma lane
- Request limit: five minutes; a timeout, empty response, truncation, thinking output, malformed
  JSON, or wrong role counts as an incorrect all-case outcome

The official model cards identify the checkpoints and licences; the official Ollama registries
record the available quantizations and sizes ([Google model card](https://huggingface.co/google/gemma-4-26B-A4B-it),
[Gemma tags](https://ollama.com/library/gemma4/tags),
[Qwen model card](https://huggingface.co/Qwen/Qwen3.8-27B),
[Qwen tags](https://ollama.com/library/qwen3.8/tags)). "Open-weight" remains the comparison term:
local executability and an open licence do not by themselves establish redistribution or appliance
acceptance for every model in the matrix.

## Results

| Metric | Gemma 4 26B A4B Q4_0 | Qwen 3.8 27B Q4_K_M |
| --- | ---: | ---: |
| Cases | 11 | 11 |
| Valid structured responses | 11/11 | 1/11 |
| Correct roles, all cases | 11/11 | 1/11 |
| Five-minute timeouts | 0/11 | 10/11 |
| Median all-case latency | 29.074 s | 300.027 s |
| Longest latency | 94.899 s | 300.043 s |
| Total matrix time | 384.125 s | 3,295.845 s |
| Reported prompt tokens | 4,755 | 351 on the completed response |
| Reported completion tokens | 1,404 | 117 on the completed response |
| Sampled output throughput | 3.13–12.20 tokens/s | 0.40 tokens/s on the completed response |
| Sampled peak total GPU memory | 10,852 MiB | 9,403 MiB |

The first cold probe intentionally used the model default and failed: it consumed the 300-token
ceiling in hidden reasoning and returned empty content. That is an operational failure, not a partial
success. Cold load took 57.8 seconds and the failed call took 110.0 seconds end to end. Setting
`think: false` made all 11 subsequent responses valid. The maintained adapter therefore requires
thinking off and rejects a thinking, empty, truncated, or malformed response.

The longest case was the deliberately invalid multi-clip compilation. Its 1,085-token prompt was
classified correctly, but it generated an unnecessarily verbose 264-token product list. The
certification extractor uses a tighter fact schema and 512-token hard ceiling; future balanced-corpus
runs must retain timeout and output-budget failures in the denominator.

Qwen's cold probe loaded in 34.0 seconds and completed a tiny seven-token structured response in about
69 seconds end to end. The one completed matrix response then generated 117 tokens at 0.40 tokens per
second. Ten later calls reached the five-minute request boundary without returning a usable response.
Their unreturned token counts are unknown and are deliberately not imputed into the token totals.
Although its sampled GPU total was lower than Gemma's, Qwen's dense checkpoint left substantial work
on the CPU; lower VRAM consumption therefore did not mean better appliance fit.

## What this changes

1. Gemma remains the provisional local survivor. The Qwen Q4 route is measured and rejected for this
   12 GB profile rather than left as an untested comparator.
2. Local routes use the same label-blind packet/evaluator/replay boundary as hosted routes. They do
   not receive gold labels or a friendlier scoring path.
3. The local adapter binds the configured tag to the installed registry digest before inference,
   accepts loopback HTTP only, disables thinking, permits one text route and one attempt, and records
   exact token and latency attribution.
4. The certification run stays at concurrency one. A later idle-host run may characterize the
   contention delta, but cannot replace this realistic shared-host evidence.
5. Promotion still requires the 300-case rights-approved development set, 1,126-case independent
   holdout, blind adjudication, and the predeclared issue #555 thresholds.
6. This local role comparison does not replace the planned hosted model and modality matrix. Audio,
   transcript, frames, and bounded direct-video cells still need the locked corpus, and direct video
   must prove incremental value on named temporal slices before it enters a cascade.

## Provisional local frame feasibility

Three smaller open-weight vision candidates received the identical ordered four-JPEG route through
the digest-pinned Ollama adapter. This was a transport and hardware feasibility probe on one Jell-O
commercial, not an accuracy score and not a substitute for the locked 300-case development replay.

| Candidate | Ollama registry digest | Identical route result | Wall latency |
| --- | --- | --- | ---: |
| Qwen3-VL 8B Instruct | `0533d74300e4f9bc367d675d4e64ffd073d50ff16a2b4096cc2e8a1cf8c96319` | valid structured response | 8.3 s |
| MiniCPM-V 4.5 8B | `0c40168f46d1cbf5cec399d8ced34b6d3347a79f69306866efa44203c08eeda3` | valid structured response | 10.7 s |
| Moondream 2 | `55fc3abd386771e5b5d1bbcc732f3c3f4df6e9f9f08f1131f9cc27ba2d1eec5b` | operational failure: 2,988 request tokens exceeded its hard 2,048-token context | — |

Qwen3-VL and MiniCPM therefore advance to the full local frame comparison. Moondream remains a
recorded operational failure; shrinking its route would make the comparison favorable rather than
identical. The current upstream cards identify all three as Apache-2.0
([Qwen3-VL](https://huggingface.co/Qwen/Qwen3-VL-8B-Instruct),
[MiniCPM-V](https://huggingface.co/openbmb/MiniCPM-V-4_5), and
[Moondream](https://huggingface.co/vikhyatk/moondream2)), but the exact local package/model provenance
still has to be locked with the prediction ledger before promotion.

This Ollama lane is ordered frames plus shared transcript, not direct video. A local direct-video cell
requires a runtime that actually accepts the bounded video derivative under the same packet, ceiling,
and accounting contract; it cannot be inferred from a model card or represented by denser frame
sampling.
