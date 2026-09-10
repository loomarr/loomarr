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
`c4c8b6a4` and the exact August archives. Final release-commit, image notice and SBOM checks remain,
as does binding the application image to these public sources in the application release notes.
The source-only release publishes no application binary and does not certify beta.5. No separate
external reviewer sign-off is required.
