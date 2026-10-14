# August FFmpeg source retention

Tracking: [#661](https://github.com/loomarr/loomarr/issues/661). This is engineering traceability
for the August pin update. It does not establish final beta.5 artifact acceptance.

The July build's original dependency cache had expired. The replacement remains on BtbN's
retained monthly **n8.1** line, preserving the concat compatibility requirement in `Dockerfile`.
The exact binaries, FFmpeg source, build recipe, original dependency cache and build logs have
been retained in the maintainer's release-evidence archive. The source materials are now [publicly distributed](https://github.com/loomarr/loomarr/releases/tag/third-party-sources-2026-08-31);
[asset URLs and SHA256 digests](source-distribution-2026-08-31.json) bind the publication to the pins.

| Artifact | Immutable identity | SHA256 |
| --- | --- | --- |
| Linux amd64 binary archive | `autobuild-2026-08-31-13-27`, `n8.1.2-50-g1a748fe2cd` | `c733b4b2951e5957e15505f788b2c65a7a41b6da4b289e295852cc38079b4d2b` |
| Linux arm64 binary archive | Same release/build | `ae5da4f51b9052390f414005f8ab26c1eed1268f327cce7cb79aa076b29bd66e` |
| FFmpeg source archive | `1a748fe2cd43e3ead22fafb1b5b7d77f153898a8` | `519e42c103de98a34fc07c2a52c0d6d7b25031f2bd4e42d544bc409fe9fb6477` |
| BtbN build recipe archive | `8267213e26c1031621e6e1210fe3aa4867214f6a` | `08279484656a586c149e119a20d3853f2596ee08192d97595edd2a5ba3b4fd51` |
| Original dependency download cache ZIP | Run `33390969643`, artifact `9757562833`, 2,024,950,980 bytes | `c7c46c41c512872bf4ff3bd6e8165f14209c851149c9824d09241df18b8011cc` |

The [release manifest](redistribution-manifest-v1.json) binds the binary filenames, download URLs,
source commits and pruning policy to the active Dockerfile inputs. The retained policy keeps
14 daily builds and one build per month for 24 months; this tag is August's final build.

## n9.0 pin from the same release (#1549)

The Dockerfile now takes the release's n9.0 GPL build instead of its n8.1 one. The n8.1 ceiling
(the concat advance) went with the per-programme chain in #1542. The table above stays as the
record of the n8.1 archives and the published source.

| Artifact | Immutable identity | SHA256 |
| --- | --- | --- |
| Linux amd64 binary archive | `autobuild-2026-08-31-13-27`, `n9.0.1-11-ge47273f4d9` | `182c1b509720e939bb47bfb47dc29cc0c298640401128e3dce8627d10707eb5a` |
| Linux arm64 binary archive | Same release/build | `e2dd447c8a47849c5812d87e54a47b20ae0f3603d38989440f4a5fe1af8755b1` |
| FFmpeg source archive | `e47273f4d9227152dcbf543cebaf9e2430ddbcc4` | `6491dae95e3cf3cdbac02933b55860e782b0c4f0a6bd8f37cef30fded259283c` |
| BtbN build recipe archive | `8267213e26c1031621e6e1210fe3aa4867214f6a` (unchanged) | `08279484656a586c149e119a20d3853f2596ee08192d97595edd2a5ba3b4fd51` |
| Original dependency download cache ZIP | Run `33390969643`, artifact `9757562833` (unchanged) | `c7c46c41c512872bf4ff3bd6e8165f14209c851149c9824d09241df18b8011cc` |

Both binary digests were computed locally from the downloaded archives and match the release's
`checksums.sha256`. The BtbN build recipe and retention policy are the same
`8267213e26c1031621e6e1210fe3aa4867214f6a`. The source archives are GitHub's archives of those
commits, the same method as the n8.1 row: re-downloading the n8.1 archives on 2026-09-27
reproduced `519e42c1…` and `08279484…` byte for byte, and two downloads of the n9.0 archive agreed.
The source commit is on FFmpeg's `release/9.0` branch exactly 11 commits after `n9.0.1`, which is
the build ID `n9.0.1-11-ge47273f4d9`.

The source is [publicly distributed](https://github.com/loomarr/loomarr/releases/tag/third-party-sources-2026-09-27)
with the recipe and the dependency cache; [asset URLs and SHA256 digests](source-distribution-2026-09-27.json)
bind that publication to the pin. yt-dlp and the other runtime sources are unchanged and stay in the
August publication.

### The n9.0 build shares the n8.1 dependency cache

BtbN's job logs for run `33390969643` have expired (HTTP 410), so this rests on the run's job list,
the workflow at the recipe commit, and the retained cache:

1. **Same run.** The run (scheduled, `head_sha` `8267213e…`, published by its `Publish release`
   job) contains both `Build ffmpeg (linux64, gpl 8.1)` and `Build ffmpeg (linux64, gpl 9.0)`, and
   the matching `linuxarm64` jobs. All four ran on `ubuntu-latest`.
2. **One cache per run.** In `.github/workflows/build.yml` at `8267213e`, only the non-arm
   `Build base image` job runs `download.sh` and uploads the `download-cache` artifact. Every
   `Build target-variant image` job restores `download-cache-<key>` with `fail-on-cache-miss: true`.
   The key is `util/get_dl_cache_tag.sh`: the SHA256 of `download.sh hashonly`, computed with no
   target, variant or version addin (`vars.sh dl only`). Only the `-arm` runner suffix enters it,
   and none of these four jobs uses it. The `8.1` and `9.0` addins set only `GIT_BRANCH`.
3. **The retained cache is complete for the recipe.** Recomputing each download stage's hash from
   the recipe gives 113 stages, and all 113 have their archive in the retained `cache.tar.gz`.

Two recipe stages are gated on the FFmpeg version between 8.1 and 9.0. `50-shaderc` stops adding
`--enable-libshaderc` at 9.0. `50-libcurl` is enabled only above 8.1, so it is built into the n9.0
dependency image, but FFmpeg 9.0.1's `configure` has no libcurl option and the n9.0 binary contains
no libcurl code. Its source (`50-libcurl_045d8444…`) is in the cache anyway. No n9.0 dependency
comes from outside the cache. The n9.0 dependency-image digests are not recoverable from the expired
logs, and nothing here claims bit-for-bit reproducibility.

## Original dependency identities

The following commits were read from `.git/HEAD` inside the original upstream cache archives.
They are evidence for the August build, not retroactive evidence for July:

| Dependency | Original cached commit |
| --- | --- |
| OpenSSL | `aae016bfd52fcad2bc9657c2c782cfdf73b1ed5f` |
| mbedTLS | `ece41aa84d7879d7e55c59e955a5884b541f7f3b` |
| Vulkan-Headers | `f9973cd97e6f3584707e7ef1c425e336f1b92a5b` |
| rav1e | `564ae3b0007ae2b06893fd7166bf88c5a84c5b63` |

The dependency-image producer logs record the same image digests consumed by the final build:
`sha256:caef606c3c19b40021837a01c65367a07f34f6ab1193c1fe78facff13ff84dfc` (amd64) and
`sha256:ed88e7baf89c21bb0b94bcb498969aeb47aa95378c1b39089d606825012704ab` (arm64).
Cached build stages do not expose the complete resolved compiler graph. In particular, rav1e's
retained Cargo.lock predates the recipe's `cargo update cc`; the exact updated build-helper version
is unproven. This record does not claim bit-for-bit upstream build reproducibility.

## Source distribution and remaining release work

The source-only release contains the original FFmpeg download cache, exact FFmpeg and BtbN source,
yt-dlp source and retained runtime/dependency sources, Cargo crates and license texts. All 490
inventoried source/license files were hash-verified during assembly; GitHub's uploaded-asset hashes
and sizes match the local publication. Public README, checksums and inventory downloads were
verified without authentication. Supplemental optional/build/test sources do not assert linkage.

The unchanged full `make test-ffmpeg` passed for Linux amd64 and arm64 using source-pin commit
`c4c8b6a4` and the exact August archives. Since #661 (2026-09-28), the release-commit checks are
enforced on every release rather than recorded once: the release-candidate image builds run the
unchanged `make test-ffmpeg` against the `ffmpeg`/`ffprobe` copied out of each native amd64/arm64
image and compare the packaged notices with the tagged source, and publication refuses an index
without per-platform SBOM and provenance naming the tagged commit. The application release notes
bind the image to these public sources by hand (the v0.2.0-beta.8 candidates' notes link both).
The source-only release publishes no application binary and does not certify beta.5. No separate
external reviewer sign-off is required.
