# Planner tool-finalization diagnosis — 2026-09-02

Issue [#862](https://github.com/loomarr/loomarr/issues/862) is resolved at the adapter seam, but no
stock model is certified and model training remains gated.

## Root cause and correction

The production Suggester kept `catalog_search` available after a useful non-empty result. Tool-biased
models therefore repeated a successful search until the hard boundary instead of producing a final
Proposal. The provider-neutral contract now has two explicit phases:

1. Retrieval exposes the single sequential catalog tool. Empty/error results keep it available for
   one of the alternate search modes.
2. The first non-empty grounded result starts finalization. Tools are removed, JSON mode is enabled,
   and that state persists through bounded schema repairs.

Ollama additionally keeps thinking disabled during JSON-only finalization. OpenAI-compatible models
can still infer a prior tool from conversation history and emit it despite receiving no tool schema;
Loomarr now treats that as malformed final output and never executes it. Grounding, surfaced-ID
validation, and the production call/tool ceilings are unchanged.

Hermetic race-enabled regressions cover successful finalization, persistent tool removal during
repair, empty-result recovery, and an unsolicited finalization tool call. The explicit
`make planner-tool-diagnostic` command runs the frozen post-result turn and emits only versioned
hashes, roles, option flags, provider attribution, JSON validity, and repeated-tool status.

## Family smoke outcome

Each local artifact ran one held-out base Intent from all 25 planner families. Qwen completed 24/25
case-level checks; Gemma completed 21/25. Neither met the aggregate release floors. Qwen failed
genre discovery by making five empty keyword searches; Gemma had more empty selections, exhausted
ambiguous/empty paths, and failed adversarial tool cases. Qwen remains the leading local development
artifact, not a shipping recommendation.

The pinned OpenRouter/DeepInfra gpt-oss control recovered from a transient upstream 429 and proved
the stricter edge: after one legitimate grounded lookup it emitted unsolicited tool calls during
finalization. Loomarr executed no additional catalog calls. The bounded repair sequence then ended
in a provider failure; fail-closed usage accounting stopped the remaining families after four model
calls, 7,703 reported tokens, and $0.00028862. This is sufficient to reject that control for the
current planner contract, not to claim a 25-case quality score.

Exact metrics and hashes are in
[`planner-tool-finalization-smoke-2026-09-02.json`](evidence/planner-tool-finalization-smoke-2026-09-02.json).

## Training and infrastructure decision

Do not provision Runpod and do not begin an Unsloth recipe yet. The prerequisite was that at least
one stock artifact clear every family-smoke case; none did. A LoRA/QLoRA experiment becomes justified
only after the remaining errors are shown to be stable planner-skill gaps against a green adapter,
with train/development examples kept disjoint from the frozen certification holdout.

Scorecard-accounted OpenRouter spend is $0.07184344 of the $20 authorization, plus one successful
single-turn diagnostic whose earlier command version did not emit its charge. Runpod spend remains
$0. The missing diagnostic charge is explicitly not guessed.
