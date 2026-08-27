# Authentic filler temporal and model bakeoff

Date: 2026-08-26

## Decision

Use a transcript plus source metadata as the provisional first-pass evidence lane for filler-role
classification. Carry Google Gemma 4 26B forward as the inexpensive open-weight candidate and Gemini
3.7 Flash as the hosted fallback. Do not add a direct-video rung: it cost more, failed more often, and
was less accurate than transcript-based evidence in this sample.

This is an 11-clip development experiment, not certification. The archive titles are unusually
descriptive, the gold labels have not received the independent two-pass adjudication required by the
certification plan, and a perfect score here is not evidence of production accuracy. Issue #555 and its
locked schema-v5 holdout remain the release boundary.

## Question and method

The earlier frame bakeoff established that sparse images were insufficient. This run tested the
missing questions directly: whether temporal evidence helps, whether less expensive models match the
frontier models, and whether an open-weight model is credible enough to operate ourselves later.

The same 11 authentic Internet Archive clips covered commercials, promos, a bumper, a PSA, a station
ID, a trailer/TV spot, and an intentionally invalid mixed compilation. Every classifier used the exact
role enum:

`commercial`, `promo`, `bumper`, `psa`, `station_id`, `trailer`, `interstitial`,
`programme_excerpt`, `compilation`, or `unknown`.

The 308-cell matrix compared six models across metadata, four sampled frames, a neutral transcript,
frames plus transcript, and—where supported—bounded audio or video. Routes were pinned with provider
fallback disabled. Failed or malformed structured responses count as incorrect. The two open-weight
models were Gemma 4 26B and Qwen 3.8 27B; this run used hosted inference and did not benchmark local
hardware, quantization, throughput, or memory use.

The shared transcript was produced once from each clip's audio with `openai/gpt-audio-mini`. One silent
six-second station ID produced no transcript, which is retained as an honest negative rather than
filled from metadata. OpenAI's current documentation confirms that
[GPT-4.1 mini](https://developers.openai.com/api/docs/models/gpt-4.1-mini) and
[GPT-5 mini](https://developers.openai.com/api/docs/models/gpt-5-mini) accept images but not audio or
video, so their temporal lanes necessarily consumed the neutral transcript.

## Role results

Accuracy below uses all 11 cases as the denominator, including request and structured-output failures.

| Model | Metadata | Frames + metadata | Transcript + metadata | Frames + transcript + metadata | Direct media + metadata |
| --- | ---: | ---: | ---: | ---: | ---: |
| Claude Sonnet 4.6 | 10/11 | 10/11 | 10/11 | 10/11 | — |
| Gemini 3.7 Flash | 9/11 | 10/11 | **11/11** | **11/11** | audio **11/11**; video 9/11 |
| Gemma 4 26B, open weight | 10/11 | 10/11 | **11/11** | **11/11** | video 6/11, including 2 failures |
| GPT-4.1 mini | 10/11 | 9/11 | 10/11 | 10/11 | — |
| GPT-5 mini | 10/11 | 10/11 | **11/11** | 10/11 | — |
| Qwen 3.8 27B, open weight | 10/11 | 10/11 | 10/11, including 1 failure | 10/11, including 1 failure | video 9/11 |

Transcript evidence produced the only repeatable improvement: Gemini rose from 9/11 with metadata and
10/11 with frames to 11/11; Gemma rose from 10/11 to 11/11; and GPT-5 mini rose from 10/11 to 11/11.
Adding frames to an existing transcript did not improve any model's role score. Direct video did not
match the transcript lane for Gemini, Gemma, or Qwen and introduced provider-specific failures.

The transcript lanes also recovered every eligible known date for Gemini, Gemma, GPT-4.1 mini,
GPT-5 mini, and Qwen. Gemma transcript plus metadata recovered 8/9 known brands; adding frames recovered
9/9. The role decision should therefore use transcript plus metadata, with frames added only when
brand or other visual facts justify their extra cost.

## Open-weight result

Gemma is the candidate to advance. Its transcript-plus-metadata lane returned 11/11 valid role labels,
10/10 eligible dates, and 8/9 eligible brands for $0.00150765 across all 11 classifications. Adding
frames retained 11/11 roles, reached 9/9 brands, and cost $0.00338385. Median hosted latency across its
successful lanes was 4.2 seconds.

Qwen's successful transcript calls classified every returned case correctly, but two temporal calls
returned truncated JSON. Counting those operational failures yields 10/11 for both transcript lanes.
Transcript plus metadata cost $0.02423000 and median hosted latency across successful lanes was 19.6
seconds, with a 50.6-second p95. It remains useful as a secondary open-weight comparison, not the
default candidate.

"Open weight" is deliberate: this experiment establishes model quality through hosted endpoints. It
does not yet establish that either model's licence, local serving stack, quantization, or appliance
resource envelope meets Loomarr's distribution and operational requirements.

## Cost and operations

The retained 308 predictions contained 304 successful structured responses and four failures. Their
recorded classification cost was $0.64158377. Shared transcription cost $0.00973740. Total account
usage attributable to this expanded stage was $0.93163754, including discarded provider diagnostics
and the clean provider-pinned rerun; the gap is the cost of learning which advertised routes actually
worked.

Provider-pinned median latency was 2.4 seconds for GPT-4.1 mini, 4.2 for Gemma, 4.8 for Claude, 4.9 for
Gemini, 6.7 for GPT-5 mini, and 19.6 for Qwen. Gemma image/text calls ran on Google Vertex; its separate
video route used Darkbloom and was materially unstable. Qwen ran on CoreWeave after another route
repeatedly exhausted its output budget. Capability metadata alone was not a sufficient preflight.

## What this changes

1. The certification evidence ladder should be metadata, then one shared transcript, then optional
   frames for visual fields. Direct video is removed unless a later balanced corpus disproves this run.
2. Gemma transcript plus metadata becomes the provisional economical first pass. Gemini transcript or
   direct audio is the hosted fallback; GPT-5 mini is a useful text-only ceiling comparison.
3. Transcription should be a shared evidence artifact with provenance, not repeated inside each model
   request. A silent or refused transcript is a valid missing signal.
4. The runner must pin provider and model, prove exact input/output support with one cheap request, cap
   output, retain failures, and record resolved route, latency, tokens, and cost.
5. The next run needs a larger balanced development set with less title leakage, independently
   adjudicated labels, per-role precision and recall, abstention scoring, and compilation rejection.
   Only then should Loomarr benchmark Gemma locally and lock the schema-v5 certification matrix.
