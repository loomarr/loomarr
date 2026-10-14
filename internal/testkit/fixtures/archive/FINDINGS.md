# Archive.org ingest contract — Phase 12 follow-up (deferred walk, now done)

**Captured:** 2026-07-13, against the live Archive.org public JSON APIs (no key — that's why the
design chose plain net/http here, §10). Verified end-to-end with a real download.

## The walk (implemented in internal/clipfetch/archive.go)

1. **Resolve** an `archive.org/details/<id>` (or `/metadata/<id>`, or bare id) URL → id.
2. **`GET /metadata/<id>`** → `{server, dir, metadata{mediatype,title,description}, files[]}`.
3. **Collection vs item:** `metadata.mediatype == "collection"` → list members via
   `GET /advancedsearch.php?q=collection:<id>&fl[]=identifier&output=json`, then walk each item
   (capped at maxPer=50/pass). Otherwise it's an item → download it.
4. **Pick a VIDEO file** from `files[]` (by `format`: MPEG4/h.264/etc.). Real items carry BOTH a
   huge original (e.g. 246MB `.mp4`) and a small Archive derivative (e.g. 8.9MB `.ia.mp4` — **27x
   smaller**). **Default = smallest (the derivative)**: filler is short broadcast-era SD content that
   Tunarr re-encodes at playout anyway, so the original stores grain, not detail, and a library of
   thousands of clips at hundreds of MB each is absurd. `INGEST_PREFER_ORIGINAL=true` flips to the
   full-quality master for anyone who wants it. (Program content wants quality — but that's the *arr
   pipeline, not this sidecar.) Non-video files (thumbnails, torrents, metadata xml) are skipped.
5. **Download** from `https://<server><dir>/<url-encoded file.name>` (the server host + dir come from
   the metadata response, NOT a fixed host — Archive load-balances). Atomic `.part`→rename so the
   media server never scans a half-file.
6. **Write an info-JSON sidecar** (`<base>.info.json`) preserving `title`/`description` — the text
   signals the core's AI tagging reads (§10), matching the shape yt-dlp's `--write-info-json` writes.
7. **Idempotent:** skip if the target media file already exists in the drop-folder.

## Live verification (2026-07-13)

Ran the real walk against `warning-cic-logo-paramount-logo-nickelodeon-logo`:
- Correctly picked the **8.9MB `.ia.mp4` derivative** over the 246MB `.mp4` original.
- Wrote the media + a 516-byte `.info.json` sidecar (title + description) into the drop-folder.
- `fetched=1 skipped=0`; a second run would skip (idempotent).

## Fixtures (this dir)

- `metadata_item.json` — a real `/metadata/<id>` response trimmed to the fields the walk reads (incl.
  BOTH the derivative and the original, to exercise smallest-derivative selection). Public metadata,
  no secrets.

### V33 capture (2026-07-31) — discovery + licences

Captured live against the same public JSON APIs, no auth, read-only. **V33 was blocked on this**:
the 2026-07-13 capture above was taken for the DOWNLOAD walk and pins only `mediatype`/`title`/
`description`, so it could satisfy neither the "license badges render" gate nor a discovery listing.

- `metadata_item_licensed.json` — `GET /metadata/cm-1993-4`, HTTP 200. Trimmed the same way as
  `metadata_item.json` (3 of 28 files kept). Two things it carries that the older fixture does not:
  - **`metadata.licenseurl`** = `https://creativecommons.org/licenses/by-nc-sa/4.0/`. ⚠ The field is
    `licenseurl`, one word — NOT `license` and NOT `rights`; both were checked and are absent.
  - a **non-ASCII title** (Japanese) and a 75 MB original, which the earlier fixture's ASCII title
    and small files never exercised.
- `collection_search.json` — `GET /advancedsearch.php?q=collection:classic_tv_commercials&…&output=json`,
  HTTP 200, 5 of 8362 docs. The shape discovery lists from.

  ⚠ **Re-captured with more fields (same day) once discovery was being built.** The first
  version requested `fl[]=identifier` only — enough for the download walk, which fetches each
  item's metadata anyway, and useless for a listing that has to show titles and licences without
  downloading anything. Asking for `title`/`licenseurl`/`year` is one request either way.

  ⚠ **The docs are DELIBERATELY MIXED, because the live API is.** Solr omits an absent field
  entirely rather than sending an empty value, so the five docs cover all four real shapes:
  licence+year, licence only, year only, and neither. Two of the five carry a licence — the
  other three do not, which is the 92%-absent rate showing up in miniature. A fixture where
  every doc looked the same would let a parser that assumes `title`, `year` or `licenseurl` is
  always present pass here and fail on any real collection.

⚠ **`licenseurl` is OFTEN ABSENT, and that is the normal case rather than an edge one.** The item
the older fixture uses (`warning-cic-logo-…`) has no licence at all — re-checked live during this
capture. In `classic_tv_commercials`, **667 of 8362 items declare one**, so roughly 92% do not. A UI
that renders a licence badge unconditionally would show an empty chip on most clips; a missing
licence means *unknown*, never *public domain*.

## Not in the core gate

The walk's HTTP + filesystem are injected, so it's unit-tested against a mock Archive server with NO
live network (`archive_test.go`, AGENTS.md rule). The live run above is a manual smoke, not committed
as a test. The bundled yt-dlp path (YouTube sources) is likewise manual-smoke only.
