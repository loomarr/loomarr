# 0047. Loomarr identifies media without Sonarr or Radarr, and keeps rich facts in queryable tables

- **Date:** 2026-11-08
- **Status:** accepted
- **Supersedes:** two points of [0046](0046-loomarr-scans-its-own-media-files.md): "a filename plus a
  TMDB search produces only a candidate", and "episodes bridge between providers … never by season
  and episode number" (now: never by number where the show has more than one ordering)

## Context

0046 made Sonarr and Radarr the main identity source and sent every other file to Unmatched.
Households may never run the arrs (#2151), and the parity run on a real library showed what that
costs: without Radarr and without NFOs, nearly every movie waits for a person, and an episode with
neither Sonarr nor an NFO gets no description at all. TMDB, the one source every install can reach,
was not asked unless a key was configured.

The same run showed the stored facts are rich in shape but thin in practice and hard to query: cast
and community score were empty everywhere, most movies carried a name only, and every rich fact sat
in per-source JSON that no feature could filter on.

## Decision

- **A file with no arr record and no id is matched when the evidence agrees, never guessed.**
  - A movie: a TMDB search by the cleaned name, matched only when exactly one result agrees on title
    (or original or alternative title), year within one, and runtime within about 10% of the file's
    measured duration.
  - A show folder: one TMDB search, matched only when exactly one show agrees on name, year when the
    folder has one, and the seasons and episode numbers on disk.
  - Anything short of that waits in Needs you with the candidates.
- **NFOs are read in full.** An id in an NFO, including the older `<imdbid>`, `<tmdbid>` and `<id>`
  tags, decides identity with no search. NFO details fill what the services leave empty and never
  override them. A deliberate override is made in Loomarr, not in the file.
- **Only movies and episodes become titles.** Extras (trailers, samples, featurettes, behind the scenes,
  deleted scenes, interviews, shorts) and theme media are not imported: no row, no header read.
- **Episode details from TMDB:** by the episode's own TVDB id when known; by season and number only
  for a show with a single ordering on TMDB; otherwise name and number only, in Needs you.
- **Loomarr ships a TMDB key.** A household's own key overrides it; TMDB is credited on the About
  screen as its terms require.
- **Rich facts are resolved into `library_*` tables**, rewritten in the same transaction as each
  inventory write: one answer per title with the provider that supplied it, one row per file with
  the copy that plays marked, one row per track (in-file and external subtitles and audio), people
  and credits keyed by TMDB person id, chapters, `.edl` markers and trickplay references. Each
  source's own observation stays as inventory JSON, the evidence behind every provenance note.

The full list (decisions 39–61, editions, filmographies, labels, language) is in #2151.

## Consequences

- A library with no arrs and no NFOs is mostly identified on its first scan. The cost is one TMDB
  search per movie and one per show folder.
- An automatic match can still be wrong where a person would have caught it. The evidence bar (one
  result, three agreeing facts for a movie) keeps that rare, and Fix match undoes it.
- The extra-item code path is removed. Existing extra rows go when the write version rewrites their
  folders.
- Every feature that filters on resolution, language, rating, people or editions reads columns,
  not JSON. The scheduler moves onto them at cutover.
- The bundled TMDB key is public in the repository. TMDB rate-limits by address, so households do
  not share a quota.
