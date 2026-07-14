# TMDB contract spike — Phase 11 follow-up (deferred capture, now done)

**Captured:** 2026-07-13, against the live TMDB v3 API with the maintainer's read-only key
(`api_read` scope). Key stored in git-ignored `.phase0.env`; never committed.

## Result: the adapter was ALREADY correct

The Phase-11 TMDB adapter (`internal/tmdb`) was built against TMDB's documented v3 shape while the
key was unavailable. The live capture **confirms every assumption** — no adapter change needed. This
is the sanctioned live-verification of a previously-deferred contract.

## Pinned contract

| Op | Request | Result |
| --- | --- | --- |
| Auth | `GET /authentication` w/ `Authorization: Bearer <key>` | `{"success":true}` — bearer auth works (key stays out of the URL, §6 anti-leak) |
| Search | `GET /search/multi?query=matrix&include_adult=false` | `{page,results[],total_pages,total_results}`; each result has `id`, `media_type` (`movie`/`tv`/`person`), movies carry `title`+`release_date`, tv carry `name`+`first_air_date` |
| Exists (movie) | `GET /movie/603` | **200** — The Matrix (real id) |
| Exists (fabricated) | `GET /movie/88888888` | **404** — the grounding drop path |
| Exists (tv) | `GET /tv/1396` | **200** — Breaking Bad |

Key facts the adapter depends on, all confirmed:
1. **`media_type` distinguishes movie/tv/person** in `/search/multi`; the adapter keeps movie+tv and
   **drops person** (verified: `?query=keanu` returns `person` results).
2. **Movie title field is `title`; tv is `name`.** Dates are `release_date` / `first_air_date`.
   Year parsed from the first 4 chars.
3. **Exists-check: 200 = real, 404 = fabricated.** This is the belt-and-suspenders re-validation
   that drops any id the LLM proposed but that doesn't exist (§8 grounding).
4. **The Matrix is `id=603`** — matches the testkit TMDB mock AND the Emby search fixture
   (`emby/search_matrix.json`), so the whole grounding chain (Emby search → TMDB dedup by id →
   exists-validation) is consistent with reality.

## Fixtures (this dir)

- `search_multi_matrix.json` — a representative `/search/multi` slice: a movie (603), two tv results,
  and a **person** result (to prove the adapter filters it out). Trimmed to the fields the adapter
  reads; no secrets (public metadata).
- `movie_603.json` — a minimal exists-check 200 body.

## Still deferred (non-blocking)

- **Anthropic** LLM provider tool-use shape (opt-in; needs an Anthropic key). Ollama remains the
  captured/default provider.
