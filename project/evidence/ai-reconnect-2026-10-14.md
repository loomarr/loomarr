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

## Run 1 (committed screenshots)

This run used a placeholder TMDB key, so only invented titles can appear.

| Step | Result |
| --- | --- |
| [01](ai-reconnect-2026-10-14/01-manual-picks-ai-off-desktop.png) · [390](ai-reconnect-2026-10-14/01-manual-picks-ai-off-390.png) | 3 picks, AI off |
| [02](ai-reconnect-2026-10-14/02-banner-desktop.png) · [390](ai-reconnect-2026-10-14/02-banner-390.png) | Banner **8.8 s** after the select call |
| [03](ai-reconnect-2026-10-14/03-ai-running-desktop.png) · [390](ai-reconnect-2026-10-14/03-ai-running-390.png) | `POST /v1/discovery/manual-channel` 202, then `/revise` with all 3 picks in `currentLineup` |
| [04](ai-reconnect-2026-10-14/04-review-desktop.png) · [390](ai-reconnect-2026-10-14/04-review-390.png) | "We couldn't update the suggestions"; all 3 picks still selected |

The model answered: `suggest.chat` returned HTTP 200 with a tool call in 8.7 s. The job then failed
on grounding, because the TMDB title search got a 401 from the placeholder key.

## Run 2 (success path; screenshots kept local)

This run used a real TMDB key on the lane only. Its screenshots stay out of the repository, because
grounded AI titles are real media.

- Banner **8.4 s** after the select call.
- The revision ran for **about 32 s**, from the click to `awaiting_approval`.
- The review shows all **3 picks still selected**. The AI's **8 titles** sit unselected under
  "More suggestions · 8 options" as optional additions: the review's outlook request listed all 8 as
  `drop` and the 3 picks as `add`.

Afterwards the TMDB key and every `llm.*` setting were cleared through the settings API. The lane was
stopped by PID and its SQLite database deleted.
