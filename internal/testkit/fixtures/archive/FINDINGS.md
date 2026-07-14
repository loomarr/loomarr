# Archive.org ingest contract — Phase 12 follow-up (deferred walk, now done)

**Captured:** 2026-07-13, against the live Archive.org public JSON APIs (no key — that's why the
design chose plain net/http here, §10). Verified end-to-end with a real download.

## The walk (implemented in internal/ingestkit/archive.go)

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

## Not in the core gate

The walk's HTTP + filesystem are injected, so it's unit-tested against a mock Archive server with NO
live network (`archive_test.go`, CLAUDE.md rule). The live run above is a manual smoke, not committed
as a test. The bundled yt-dlp path (YouTube sources) is likewise manual-smoke only.
