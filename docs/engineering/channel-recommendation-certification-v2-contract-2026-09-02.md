# Channel recommendation certification v2 contract — 2026-09-02

The active `channel-recommendation-v2` contract is frozen before live inference. It preserves the
v1 no-ship evidence, reuses the sound JSON-mode prompt/schema/scorer contract, and changes only the
untouched certification evidence plus the development-proven 1,024-token output ceiling.

- Corpus: `channel-recommendation-v2`
- Fixture SHA-256: `2caf971fd7ad14cc9c673c6c4bf92d481305086e9506d0fd67eb8f63cff1e17c`
- Prompt: `channel-concept-prompt-v1`
- Output schema: `channel-concept-schema-v1`
- Scorer: `channel-recommendation-scorer-v1`
- Cases: eight, one for each pre-registered sparse, broad, repetitive, family, seasonal, era-heavy,
  conflicting, and adversarial family
- Per-case output ceiling: exactly 1,024 tokens

The loader proves that v2 case ids and normalized snapshot contents are disjoint from both the
immutable v1 holdout and `channel-recommendation-development-v1`. Certification remains excluded
from training data. A different output ceiling is rejected before provider construction rather than
recorded as comparable evidence.

## Predeclared matrix

| Profile | Provider route | Calls | Suite tokens | Suite spend ceiling | Inference authorized by dry run |
| --- | --- | ---: | ---: | ---: | --- |
| `qwen35-shared-v2` | Ollama `qwen3.5:9b` · `6488c96fa5fa` | 8 | 100,000 | $0.000000001 | no |
| `gemma4-recommendation-v2` | Ollama `gemma4:12b` · `4eb23ef187e2` | 8 | 100,000 | $0.000000001 | no |
| `gpt-oss20b-deepinfra-v2` | OpenRouter `openai/gpt-oss-20b` · DeepInfra, fallback disabled | 8 | 100,000 | $0.005000000 | no |

Each profile has a two-minute per-case wall-clock ceiling and serial execution. Local nanodollar
ceilings satisfy the positive fail-closed budget contract but Ollama is explicitly unbilled. The
hosted ceiling is a hard maximum, not an expected charge. Any accounting uncertainty stops the run.

The three provider-free dry runs constructed no provider, made zero model calls, used zero tokens,
and spent $0. They only proved the exact candidate and resource envelopes. The machine artifact is
[`channel-recommendation-certification-v2-contract-2026-09-02.json`](evidence/channel-recommendation-certification-v2-contract-2026-09-02.json).

Structural success is not certification. A ship decision still requires complete scorecards for the
predeclared matrix, all hard gates and quality floors, and the existing two-point selection margin.
No Unsloth, LoRA/QLoRA, Runpod, deployment, or production authority follows from this contract.
