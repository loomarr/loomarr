# AI reconnect banner: live evidence for #1914

The setup was an isolated lane backend from this worktree (`make dev-be`, its own SQLite). It used the
demo library (`make demo-library`, with invented titles) and the Vite frontend (`make dev-fe`). The
filler jobs and `channel-maintenance` were paused first. The browser was Chromium, signed in as the
lane's dev admin. Each step was captured at 1440×900 and at 390×844.

Each run had the same shape:

1. With AI off (`features.suggestions: false`), open the manual builder (`/guide?manual=1`), pick a
   starter template for the name and description, and add three titles from "Build from your
   library".
2. Point the lane's LLM at the household AI server with `POST /v1/system/llm/select`
   (`provider: custom`, an OpenAI-compatible llama.cpp base URL, model `Qwen3.8-Flash`).
3. Wait for the banner, then click **Try AI suggestions**.

`features.suggestions` also needs `tmdb.api_key`. On a lane with no TMDB key the banner never shows
(the first attempt waited 90 s with seven `/v1/settings` reads, all `false`).

## Run 1 (placeholder TMDB key)

This run used a placeholder TMDB key, so only invented titles can appear. The model answered
(`suggest.chat` HTTP 200 with a tool call in 8.7 s), and then the job failed on grounding, because
the TMDB title search got a 401 from the placeholder key. Its screenshots were replaced by run 3.

## Run 2 (success path; screenshots kept local)

This run used a real TMDB key on the lane only. Its screenshots stay out of the repository, because
grounded AI titles are real media.

- Banner **8.4 s** after the select call.
- The revision ran for **about 32 s**, from the click to `awaiting_approval`.
- The review shows all **3 picks still selected**. The AI's **8 titles** sit unselected under
  "More suggestions · 8 options" as optional additions: the review's outlook request listed all 8 as
  `drop` and the 3 picks as `add`.
- **Defect found:** the review header showed the model's name. The typed "Cozy Mystery Nights" became
  "Rainy Village Mysteries". The server's grounding step always took the model's `channelName`.

Afterwards the TMDB key and every `llm.*` setting were cleared through the settings API. The lane was
stopped by PID and its SQLite database deleted.

## Run 3 (the name fix; committed screenshots)

The fix adds an optional fixed `channelName` to the Intent. The server keeps it over the model's
name, and "Try AI suggestions" sends the name typed in the builder. This run used a fresh lane from
the same worktree, set up as before (demo library, jobs paused, placeholder TMDB key). The browser
was Playwright's Chromium in the pinned container (`mcr.microsoft.com/playwright:v1.62.1-noble`,
host network). A starter template supplied the description, and the name typed over it was
**Lantern Lane Whodunits**. That name differs from every template's, so it can only have come from
typing.

| Step | Result |
| --- | --- |
| [01](ai-reconnect-2026-10-14/01-manual-picks-ai-off-desktop.png) · [390](ai-reconnect-2026-10-14/01-manual-picks-ai-off-390.png) | 3 picks, AI off, typed name in the field |
| [02](ai-reconnect-2026-10-14/02-banner-desktop.png) · [390](ai-reconnect-2026-10-14/02-banner-390.png) | Banner **13.7 s** after the select call |
| [03](ai-reconnect-2026-10-14/03-ai-running-desktop.png) · [390](ai-reconnect-2026-10-14/03-ai-running-390.png) | `POST /v1/discovery/manual-channel` 202, then `/revise` with the 3 picks in `currentLineup` and `"channelName":"Lantern Lane Whodunits"`; the header keeps the name while it runs |
| [04](ai-reconnect-2026-10-14/04-review-desktop.png) · [390](ai-reconnect-2026-10-14/04-review-390.png) | After 19.7 s, "We couldn't update the suggestions" (grounding: `discovery_budget_exhausted`); the header still reads **Lantern Lane Whodunits** and all 3 picks are still selected |

`GET /v1/proposal-jobs/{id}` afterwards showed the stored revision intent with
`channelName: "Lantern Lane Whodunits"`. The model was reached twice (`suggest.chat` 200, about
2.6 s per call). The successful re-grounding is covered by
`TestBuildProposalKeepsFixedChannelName`: a set name is kept over the model's, and with none set
the model's name is used. A live success run would need a real TMDB key, which means real titles,
so it was not repeated.

Afterwards `llm.provider`, `llm.url` and `llm.model` were cleared, and `llm.api_key` and
`tmdb.api_key` were deleted, all through the settings API. The lane's demo library, backend and
frontend were stopped by their process groups.
