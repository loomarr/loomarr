# v0.2.0-beta.8 release evidence (#661)

Tracking: [#661](https://github.com/loomarr/loomarr/issues/661). Reference release:
[`v0.2.0-beta.8`](https://github.com/loomarr/loomarr/releases/tag/v0.2.0-beta.8), image
`ghcr.io/loomarr/loomarr:0.2.0-beta.8@sha256:59cf8d7f90a34441d697c201545308a93af10d3e4a66cc00b22a0e9ebe26e42e`,
source commit `b9667d60cb19fdb8dbf9ecd8c8c3065cb7545929`.

Maintainer decision, 2026-09-10 UTC (recorded on the issue): a separate qualified legal/NOTICE
reviewer sign-off is not required. Release engineering owns this evidence instead of an external
review.

## 1. Source identity verification (read-only)

- **FFmpeg.** The Dockerfile pins `FFMPEG_AMD64_SHA256=182c1b509720e939bb47bfb47dc29cc0c298640401128e3dce8627d10707eb5a`
  and `FFMPEG_ARM64_SHA256=e2dd447c8a47849c5812d87e54a47b20ae0f3603d38989440f4a5fe1af8755b1` at the
  `v0.2.0-beta.8` commit. The [third-party-sources-2026-09-27](https://github.com/loomarr/loomarr/releases/tag/third-party-sources-2026-09-27)
  release's `source-package-inventory.json` records the identical pair of SHA-256 values for both
  architectures, the FFmpeg source commit `e47273f4d9227152dcbf543cebaf9e2430ddbcc4`, and the BtbN
  build-recipe commit `8267213e26c1031621e6e1210fe3aa4867214f6a` — an exact match between the shipped
  binary identifiers and the published source's recorded identifiers.
- **yt-dlp.** The Dockerfile pins `YTDLP_VERSION=2026.08.19` with
  `YTDLP_AMD64_SHA256=58162f9bfdc27458ea47bfcb311cf47028f17d8154a8bf7d689861d46399230a` and
  `YTDLP_ARM64_SHA256=b16e4dab368a816cd05d477d698a605a6ae87ccee1c8ffd38fa21d7254141fcc`. The
  [third-party-sources-2026-08-31](https://github.com/loomarr/loomarr/releases/tag/third-party-sources-2026-08-31)
  release's inventory records the exact matching upstream tag (`ytDlpRelease: "2026.08.19"`), and
  [`redistribution-manifest-v1.json`](redistribution-manifest-v1.json) records the identical
  per-architecture asset SHA-256 values bound to that same tag and to upstream commit
  `3a08beaf031ab68f966401ead017ac81fe8486cf`.
- **Release-asset integrity.** Both GitHub Releases' `SHA256SUMS`, `README.txt`, and
  `source-package-inventory.json` files were downloaded directly from
  `github.com/loomarr/loomarr/releases` and their digests compared against
  [`source-distribution-2026-09-27.json`](source-distribution-2026-09-27.json) and
  [`source-distribution-2026-08-31.json`](source-distribution-2026-08-31.json). All six recorded
  assets' names, byte sizes, and SHA-256 digests match exactly; no large binary archive
  (`ffmpeg-original-download-cache.zip`, `source-materials.tar.gz`) was re-downloaded, per the "one
  heavy thing at a time" constraint — their digests were cross-checked between the two releases'
  own `SHA256SUMS` instead, where `ffmpeg-original-download-cache.zip` is byte-identical in both.

No discrepancy was found between the Dockerfile pins, the two public source releases, and this
repository's evidence JSON files for the `v0.2.0-beta.8` reference release.

## 2. `make test-ffmpeg` and the notice/SBOM gate, both architectures

- Release-candidate CI run [`37128632082`](https://github.com/loomarr/loomarr/actions/runs/37128632082)
  ran at head commit `b9667d60cb19fdb8dbf9ecd8c8c3065cb7545929` — the tagged `v0.2.0-beta.8` commit —
  and completed with an overall `success` conclusion.
- Both `Image — release build (linux/amd64)` and `Image — release build (linux/arm64)` jobs in that
  run executed `make test-ffmpeg` against the `ffmpeg`/`ffprobe` copied out of the just-built
  release-candidate image, and both steps completed successfully.
- The same two jobs enforce the packaged-notice and SBOM/provenance checks already described as
  closed in `THIRD_PARTY_NOTICES.md`'s "Open redistribution review" section (its first bullet); this
  run is the concrete, dated instance of that enforcement for the `v0.2.0-beta.8` commit on both
  published architectures.

## 3. Source distribution mechanics and public location

- Two public GitHub Releases carry the retained corresponding source, downloadable without
  authentication or charge:
  - [`third-party-sources-2026-09-27`](https://github.com/loomarr/loomarr/releases/tag/third-party-sources-2026-09-27) —
    FFmpeg `n9.0.1-11-ge47273f4d9` source, the BtbN build recipe, and the shared original dependency
    download cache.
  - [`third-party-sources-2026-08-31`](https://github.com/loomarr/loomarr/releases/tag/third-party-sources-2026-08-31) —
    yt-dlp `2026.08.19` and the other retained runtime/build dependency sources, with a 490-entry
    source/license file inventory.
- Each release publishes its own `README.txt` (verified above), a `SHA256SUMS` file covering every
  other asset in that release, and a machine-readable `source-package-inventory.json`. Anyone can
  fetch the assets anonymously over HTTPS and verify them against `SHA256SUMS` without contacting
  Loomarr's maintainer — this is the offer/source-link mechanism that satisfies GPLv3's
  corresponding-source requirement for these two components.
- The `v0.2.0-beta.8` release notes ([`project/release/v0.2.0-beta.8.md`](../release/v0.2.0-beta.8.md))
  link both source releases directly beneath the image description, binding the exact application
  release to its corresponding-source location. `THIRD_PARTY_NOTICES.md` links the same two releases
  and their distribution-record JSON files from the published image's own notices file.
- [`redistribution-manifest-v1.json`](redistribution-manifest-v1.json) binds the pinned asset
  identities (upstream release/commit/build-script revisions, license URLs) to the Dockerfile ARGs.
  [`source-distribution-2026-09-27.json`](source-distribution-2026-09-27.json) and
  [`source-distribution-2026-08-31.json`](source-distribution-2026-08-31.json) bind the published
  GitHub Release assets (names, byte sizes, SHA-256) to those same pins. Together with section 1
  above, they close the "immutable identifiers matching the shipped bytes" acceptance item.

## 4. What remains open

Nothing in #661's acceptance criteria is open for the `v0.2.0-beta.8` reference release. Two related
items are intentionally outside this record's scope:

- A qualified legal/NOTICE reviewer sign-off — explicitly waived by the maintainer's 2026-09-10
  decision; not reintroduced here.
- Future FFmpeg or yt-dlp pin bumps must repeat sections 1–3 above for the new pins before
  `THIRD_PARTY_NOTICES.md` may again describe the sourced/verified state as current. The fail-closed
  `make test-ffmpeg` plus notice/SBOM gates in `internal/releaseverify` continue to enforce shipped-
  binary identity on every release-candidate run regardless of this record, and are unchanged by it.

## Evidence trail

- Issue: <https://github.com/loomarr/loomarr/issues/661>
- Maintainer decision: 2026-09-10 UTC, recorded on the issue thread.
- Release: <https://github.com/loomarr/loomarr/releases/tag/v0.2.0-beta.8> (commit
  `b9667d60cb19fdb8dbf9ecd8c8c3065cb7545929`, image
  `ghcr.io/loomarr/loomarr:0.2.0-beta.8@sha256:59cf8d7f90a34441d697c201545308a93af10d3e4a66cc00b22a0e9ebe26e42e`).
- Release-candidate CI run: <https://github.com/loomarr/loomarr/actions/runs/37128632082>.
- Source releases: `third-party-sources-2026-09-27`, `third-party-sources-2026-08-31`.
- [`redistribution-manifest-v1.json`](redistribution-manifest-v1.json),
  [`source-distribution-2026-09-27.json`](source-distribution-2026-09-27.json),
  [`source-distribution-2026-08-31.json`](source-distribution-2026-08-31.json).
