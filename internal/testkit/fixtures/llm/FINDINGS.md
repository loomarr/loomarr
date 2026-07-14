# LLM (Ollama) tool-use contract spike — Phase 11 findings

**Captured:** 2026-07-13, against the local Ollama (homelab default provider, §8).
**Model:** `llama3.1:8b` (tool-capable). **Endpoint:** `POST http://localhost:11434/api/chat`.

## The grounding round-trip (verified end-to-end)

The two-turn tool-use loop that enforces §8 grounding works against a real model:

1. **Turn 1 — model requests a tool call.** Given a `tools` array (one `catalog_search`
   function) and a system prompt forbidding invented titles, the model returns
   `message.tool_calls[]` with `content:""`. See `ollama_toolcall_{request,response}.json`.
2. **Turn 2 — feed the tool result back, get grounded structured output.** Appending a
   `{"role":"tool","content":"<json>"}` message with real catalog rows and `"format":"json"`,
   the model returns `message.content` = the final JSON, selecting **only** the ids from the tool
   result (100/101, "Speed"/"The Rock"). See `ollama_final_response.json`.

## Contract facts (must inform the LLM adapter)

1. **`tool_calls[].function.arguments` is a PARSED JSON OBJECT**, not a string. This differs from
   OpenAI/Anthropic (which send a JSON *string*). The Ollama adapter unmarshals it directly; the
   Anthropic adapter will need to parse the string. Provider-neutral code must normalize this.
2. **`tool_calls[].id` may be present** (`call_xxxx`) but Ollama does not require echoing it back on
   the tool-result message (the result above omitted the id and still worked). The Anthropic adapter
   *does* require `tool_use_id` correlation — another normalization point.
3. **`"format":"json"`** on the request forces the final message to be valid JSON — used for the
   structured proposal output. Without it the model may wrap JSON in prose.
4. **`"stream":false`** returns one complete response object (we don't stream the LLM; SSE progress
   is our own per-job bus, §8, not token streaming).
5. **`done_reason:"stop"`** on both turns; `message.tool_calls` is absent (None) on the final turn.

## Why this matters for the gate

The grounding gate (§19) uses a **mock** LLM, but this capture proves the *shape* the mock must
emulate: a first response carrying `tool_calls`, a second carrying grounded `content`. The mock's
"fabricated titles" case returns a `content` with ids that are NOT in any tool result — and the
suggester's validation pass must drop them (zero unresolvable reach a proposal). Modeling the mock
on this real shape is what makes the gate meaningful rather than testing a strawman.

## Fixtures (this dir)

- `ollama_toolcall_request.json` — the request with a `tools` array (system prompt forbids invention).
- `ollama_toolcall_response.json` — turn-1 response: `message.tool_calls[]`, empty content.
- `ollama_final_response.json` — turn-2 response after the tool result: grounded JSON `content`.

Durations/timestamps stripped (non-deterministic). No secrets — local model, synthetic prompts.

## Deferred (maintainer, non-blocking)

- **TMDB** `/search/multi` + validate-exists live capture: no `TMDB_API_KEY` in `.phase0.env` this
  session. Adapter built against TMDB's documented v3 shape; pin real fixtures when a key is supplied
  (tracked like the Sonarr import fixture). Grounding gate is green on mocks regardless.
- **Anthropic** tool-use shape: opt-in provider; captured when wired against a real key.
