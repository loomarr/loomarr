# yt-dlp search contract — V33 capture

**Captured:** 2026-07-31, against live YouTube via `yt-dlp 2026.07.04` (the Homebrew build; the
image pins its own — see the version warning below).

    yt-dlp "ytsearch5:1980s cereal commercial" --dump-json --flat-playlist --no-warnings

Exit 0, five JSON-lines rows. Trimmed to the fields a listing renders (`ytsearch.jsonl`);
the raw rows carry 44 keys each, mostly format ladders and thumbnail arrays.

## ⚠ `license` is ALWAYS null — measured, not assumed

All five search rows return `license: null`. That is not a flat-playlist artefact: a FULL
extraction of one of them (`yt-dlp <url> --dump-json --skip-download`, no `--flat-playlist`)
also returns `license: null`.

This is the concrete asymmetry with Archive.org, and it decides what the UI can honestly show:

| | archive.org | YouTube |
| --- | --- | --- |
| Licence available | ~8% of items declare `licenseurl` | **never, via yt-dlp** |
| What absence means | unknown — the item may still be reusable | unknown, for every single result |
| Badge can say | "CC BY-NC-SA 4.0" or "unknown" | only "unknown" |

So a YouTube result **cannot** carry a licence badge that ever says anything. Rendering an
always-"unknown" chip on every row is worse than rendering none: it implies the field was
checked per item and found empty, when it was never available at all. The surface must say this
once, about the source, rather than once per row.

⚠ Note this does NOT mean YouTube clips are unlicensed or unusable — it means yt-dlp cannot tell
us, and Loomarr must not imply it knows. The operator is the one deciding what they may reuse.

## Cost per search

One `yt-dlp` subprocess per query, ~2-5s (network-bound). Contrast Archive's single HTTP GET,
which is why the two paths are not interchangeable even though both "search".

## ⚠ The shape is yt-dlp's, and it moves

Archive's JSON is a public API contract; yt-dlp's `--dump-json` output is a tool's stdout, and a
version bump can change field names or nesting. This fixture pins what 2026.07.04 emitted, which
is what the parser is written against — but a stale fixture here fails differently from a stale
Archive one: Archive would keep working and the fixture would drift; yt-dlp can change under a
working fixture. The image pins its own yt-dlp version (Dockerfile), so the pair to keep in sync
is *that* pin and this capture.

## Not in the core gate

Parsing is unit-tested against this fixture with NO live network (AGENTS.md rule). The search
above is a manual capture, not a committed test.
