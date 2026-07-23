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

## Hugging Face GGUF discovery (§8.1 "compatible to download" source)

**Captured:** 2026-07-23, anonymous (no auth). Two endpoints:
- List: `GET https://huggingface.co/api/models?filter=gguf&sort=downloads&direction=-1&limit=N`
- Files: `GET https://huggingface.co/api/models/<repo>?blobs=true` → `siblings[]` with each file's
  `rfilename` + real `size` (bytes).

Why HF: there is **no** first-party Ollama API for "what can I download" — `/api/search` is
unshipped (404), ollama.com serves HTML only. HF's model API is a live, anonymous, popularity-ranked
source of GGUF repos, AND — the key finding — the files endpoint exposes **each quant file's real
size before any download**. So Loomarr can size a repo against detected VRAM up front and show only
compatible models, ranked best-first — no search box. `ollama pull hf.co/<repo>:<quant>` pulls the
specific chosen quant (verified). Tool-capability is confirmed AFTER pull via the Ollama probe
(`/api/show capabilities`) — HF has no reliable tool-calling tag.

Contract facts (inform `internal/llm/huggingface.go`):

1. List response is a **JSON array**; fields bound: `id`, `downloads`, `tags[]`, `pipeline_tag`,
   `private`. Files response: `siblings[].{rfilename,size}` (LFS files carry the real size).
2. A repo holds **many quants** (BF16 ~8GB down to Q3 ~2GB for a 4B). `bestFittingQuant` picks the
   largest quant whose size ≤ VRAM (bigger quant = better quality); repos where nothing fits are
   dropped. Quant is parsed from the filename (`…-Q4_K_M.gguf` → `Q4_K_M`); `-of-` shards are skipped.
3. `filter=gguf` is broad and some non-chat repos carry no telling tag/pipeline (a live check found an
   `embeddinggemma` repo slipping through) — so `isChatGGUF` also rejects on obvious id substrings
   (embedding/reranker/tts/whisper/bge) as a backstop. Err toward inclusion otherwise.
4. Ranking: comfortable-fits before tight, then by **popularity** (downloads) — chosen-quant size is
   a poor cross-model capability proxy (a 4B BF16 is larger on disk than a 9B Q8, not better).
5. The pull bridge is `pullRef = "hf.co/" + id` (BARE — Ollama's implicit `:latest`). A
   synthesized `:quant` tag is NOT reliable: Ollama resolves `hf.co/repo:TAG` against the
   repo's own manifest, and many repos expose only `latest`, so `hf.co/repo:Q8_0` returns
   `400 "tag not available"` (found live pulling deepseek-v4). `latest` always resolves and
   is the Q4_K_M-class build we size against, so what we show == what pulls. `quant` is kept
   as an informational label only (the file we sized), never appended to the pull ref.

## Fixtures (this dir)

- `huggingface_model_files.json` — a pinned `?blobs=true` capture (one repo's quant files + sizes)
  the size parser + `bestFittingQuant` run against.
