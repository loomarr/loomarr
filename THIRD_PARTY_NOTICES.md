# Third-Party Notices

Loomarr itself is licensed under the MIT License (see [`LICENSE`](LICENSE)). It
bundles or depends on third-party software, listed here with its own license.
Nothing below changes Loomarr's own MIT terms; these are the terms of the
components Loomarr uses or ships alongside.

The release image carries a BuildKit-generated SBOM and provenance as OCI attestations. An SBOM
inventories what tooling can identify; it is not a substitute for the human-readable inventory of
downloaded binaries, shared libraries, and model data below.

## Go dependencies (compiled into the binary)

All direct dependencies are under permissive licenses (MIT / BSD / Apache-2.0 /
ISC), compatible with MIT redistribution. The authoritative, versioned list is
[`go.mod`](go.mod); the notable direct ones:

| Module | License |
| --- | --- |
| `github.com/danielgtaylor/huma/v2` | MIT |
| `github.com/jackc/pgx/v5` | MIT |
| `github.com/pressly/goose/v3` | MIT |
| `github.com/caarlos0/env/v11` | MIT |
| `github.com/prometheus/client_golang` | Apache-2.0 |
| `github.com/testcontainers/testcontainers-go` (test only) | MIT |
| `golang.org/x/crypto`, `golang.org/x/time` | BSD-3-Clause |
| `modernc.org/sqlite` | BSD-3-Clause |

The Apache-2.0 components (`prometheus/client_golang` and its transitive `prometheus/*`) are
represented in the dependency inventory. A release review must inspect their upstream NOTICE
requirements separately; an SBOM does not reproduce NOTICE text.

## Compose deployment companion (not in the Loomarr image)

The supported Docker Compose topology starts the official `traefik:v3.7.1` image as its HTTP edge,
pinned by multi-architecture manifest digest in `docker/compose.yaml`. Traefik is MIT-licensed. It
is pulled separately and is not part of Loomarr's OCI image or Loomarr's BuildKit SBOM.

## Rust image worker (compiled into the required worker binary)

The exact resolved inventory is [`Cargo.lock`](Cargo.lock) and the release SBOM. Direct crates are
permissively licensed: `serde`, `serde_json`, `sha2`, `base64`, `image`, `fast_image_resize`,
`webp`, and `webp-animation` are MIT and/or Apache-2.0; `thumbhash` is MIT. The AVIF stack
(`ravif`, `rav1e`, `avif-serialize`) is BSD-2-Clause/BSD-3-Clause. The two WebP wrappers compile
upstream libwebp into the worker; those bindings and libwebp are MIT/BSD-3-Clause. These terms are
compatible with Loomarr's MIT redistribution. Resolved package metadata is represented in the
machine-readable SBOM; license texts remain an explicit release-review concern.

## Vendored binaries — the published image

**Revised (design §9.1/§16): there is now ONE image.** Loomarr previously published a
distroless `loomarr:latest` bundling none of the following, plus an opt-in
`loomarr:filler` variant that added them for clip ingest (retired-ok — named to record
what was retired). Internal playout made `ffmpeg` load-bearing for streaming, not just
ingest, so the variant collapsed into the single published `loomarr:latest`, which is no
longer distroless: it is `debian:stable-slim`, because the vendored binaries below are
glibc builds.

**This changes the scope of what follows.** These binaries used to ship only if an
operator opted into a variant; they now ship in **everything we publish**, so the
aggregate licensing below applies to the default image rather than an opt-in one.

The image ships five external executables, the Whisper shared-library set, and two model files.
The ingest job uses all of them; `ffmpeg`/`ffprobe` additionally serve playout (§9.1). Loomarr invokes
each executable as a separate process via `exec`. Loomarr's source remains MIT-licensed, while the
published image must also satisfy every redistributed component's terms. This inventory does not
make a legal conclusion about aggregation or derivative-work status; that conclusion belongs to the
final redistribution review recorded below.

| Binary | Upstream | License | Notes |
| --- | --- | --- | --- |
| `yt-dlp` standalone executable | https://github.com/yt-dlp/yt-dlp | **GPL-3.0-or-later** for the combined executable | The source project is primarily Unlicense, but upstream states that official PyInstaller executables bundle GPLv3+ dependencies and the combined work is GPLv3+. Loomarr ships `yt-dlp_linux` / `yt-dlp_linux_aarch64`, so the executable terms apply. See upstream's [license section](https://github.com/yt-dlp/yt-dlp/blob/master/README.md#license) and [third-party license inventory](https://github.com/yt-dlp/yt-dlp/blob/master/THIRD_PARTY_LICENSES.txt). |
| `ffmpeg` | https://github.com/BtbN/FFmpeg-Builds | **GPL-3.0-or-later** (the BtbN `-gpl-` build enables GPL and version 3) | Serves both yt-dlp stream merging (§10) and the playout encoder (§9.1). Exact corresponding source remains open below. |
| `ffprobe` | https://github.com/BtbN/FFmpeg-Builds | **GPL-3.0-or-later** (same build) | Added with internal playout (§9.1). It shares ffmpeg's open corresponding-source blocker. |
| `deno` | https://github.com/denoland/deno | MIT | JS runtime yt-dlp requires for YouTube extraction. |
| `whisper-cli`, `libwhisper`, `libggml` | https://github.com/ggml-org/whisper.cpp | MIT | Pinned `v1.9.1` binary and runtime-selected shared libraries used for compilation splitting and language identification. |
| `ggml-small.en.bin`, `ggml-tiny.bin` | https://huggingface.co/ggerganov/whisper.cpp | MIT | Revision- and SHA256-pinned Whisper model data; `small.en` transcribes and `tiny` identifies language. |

### GPL source status (ffmpeg; release blocker)

The `ffmpeg` binary is a **GPL-3.0-or-later** BtbN build from the `n8.1` series. The
[`Dockerfile`](Dockerfile) now pins it to an **immutable** archive — release
`autobuild-2026-08-16-13-00`, build `n8.1.2-44-g7c533d0f86`, verified by per-architecture SHA256
(`FFMPEG_AMD64_SHA256` / `FFMPEG_ARM64_SHA256`) exactly as yt-dlp, deno, and whisper are — so the
bytes in the image are now identified by digest rather than by BtbN's mutable `latest` release.

**Still open (release-artifact action, not a Dockerfile change):** record and provide the exact
corresponding source for that pinned build — the FFmpeg commit `n8.1.2-44-g7c533d0f86`, BtbN's
build scripts at that release, and the bundled GPL dependency sources — and confirm `make test-ffmpeg`
is green against the pinned build on the release commit. The immutable digest is what makes such a
corresponding-source offer *possible*; retaining the bundle itself is the remaining step.

⚠ This section previously said the opposite — that redistributing the default image
"carries no such obligation — it contains no ffmpeg", which was true only while ffmpeg
shipped in a separate opt-in variant. Internal playout (§9.1) made ffmpeg load-bearing
for streaming, the variant collapsed into the one image, and this paragraph did not
follow. A stale licensing statement is worse than a stale feature description, which is
why it is corrected in place rather than quietly rewritten.

## Open redistribution review — beta blockers

This notice is an inventory, not release clearance. The first beta remains blocked until all of the
following have evidence on the release commit:

- ~~pin the BtbN ffmpeg archive immutably~~ (done — `FFMPEG_RELEASE`/`FFMPEG_BUILD_ID` +
  per-arch SHA256 in [`Dockerfile`](Dockerfile)) and retain the exact corresponding source for FFmpeg,
  build scripts, and bundled GPL dependencies at that pinned build; rerun the unchanged full
  `make test-ffmpeg` gate against it (still open — corresponding-source retention is a release-artifact
  step, and the gate result must be recorded on the release commit);
- retain the exact corresponding source and license texts for the GPLv3+ dependencies bundled in
  both official yt-dlp standalone executables;
- ~~pin the runtime and build base images by digest~~ (done — all four `FROM` bases in
  [`Dockerfile`](Dockerfile) carry an immutable `@sha256:` alongside their tag) and make the Debian
  package input reproducible (still open — `apt-get install` in the runtime stage is unpinned);
- include the required DejaVu font license and complete the frontend/Rust transitive license-text
  inventory;
- inspect and include any required Prometheus `NOTICE` material; and
- complete a final qualified legal/NOTICE review of the assembled image and its downloadable source
  materials.

Until those items close, neither this file nor the BuildKit SBOM should be read as a claim that the
image is ready for redistribution.

## Reference specs (not redistributed as code)

- `api/vendor/tunarr-openapi.json` — a pinned copy of the [Tunarr](https://tunarr.com)
  OpenAPI spec, used to generate/validate the client. Tunarr is Zlib-licensed.
