# Third-Party Notices

Loomarr itself is licensed under the MIT License (see [`LICENSE`](LICENSE)). It
bundles or depends on third-party software, listed here with its own license.
Nothing below changes Loomarr's own MIT terms; these are the terms of the
components Loomarr uses or ships alongside.

A complete, machine-readable dependency inventory (SBOM) is attached to each
release; this file is the human-readable summary of the notable pieces.

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

The Apache-2.0 components (`prometheus/client_golang` and its transitive
`prometheus/*`) carry a NOTICE requirement; their upstream NOTICE files are
reproduced in the release SBOM.

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

The image ships four external binaries: three for the in-app clip-ingest job (design
§10) and `ffmpeg`/`ffprobe` additionally for playout (§9.1). Loomarr invokes each as a
**separate process via `exec`** — it does not link against them. Under the GPL this is
*mere aggregation*: it does **not** make Loomarr a derivative work, and Loomarr's own
code remains MIT. However, because the image redistributes these binaries, the image as
a whole must honor each component's license, disclosed here.

| Binary | Upstream | License | Notes |
| --- | --- | --- | --- |
| `yt-dlp` | https://github.com/yt-dlp/yt-dlp | The Unlicense (public domain) | Self-contained `yt-dlp_linux` build (bundles its own Python via PyInstaller). |
| `ffmpeg` | https://github.com/BtbN/FFmpeg-Builds | **GPL-3.0** (the BtbN `-gpl-` build) | See the source-offer below. Serves BOTH yt-dlp stream merging (§10) and the playout encoder (§9.1). |
| `ffprobe` | https://github.com/BtbN/FFmpeg-Builds | **GPL-3.0** (same build) | Added with internal playout (§9.1) — Loomarr owns duration once it owns the encoder. Same source offer as `ffmpeg`. |
| `deno` | https://github.com/denoland/deno | MIT | JS runtime yt-dlp requires for YouTube extraction. |

### GPL source offer (ffmpeg)

The `ffmpeg` binary in the published `loomarr` image is a **GPL-3.0** build produced by
the [BtbN/FFmpeg-Builds](https://github.com/BtbN/FFmpeg-Builds) project from the
published [FFmpeg](https://ffmpeg.org/) source. In accordance with the GPL, the
corresponding source is available from FFmpeg (<https://ffmpeg.org/download.html>,
series `n8.1`) and the BtbN build scripts (<https://github.com/BtbN/FFmpeg-Builds>).
The exact version is pinned by `FFMPEG_TAG` in the [`Dockerfile`](Dockerfile) and is
the authority if this paragraph and that line ever disagree.

**If you redistribute the Loomarr image, you carry the GPL obligation** to make that
ffmpeg source available to your recipients. There is one image and it contains ffmpeg.

⚠ This section previously said the opposite — that redistributing the default image
"carries no such obligation — it contains no ffmpeg", which was true only while ffmpeg
shipped in a separate opt-in variant. Internal playout (§9.1) made ffmpeg load-bearing
for streaming, the variant collapsed into the one image, and this paragraph did not
follow. A stale licensing statement is worse than a stale feature description, which is
why it is corrected in place rather than quietly rewritten.

## Reference specs (not redistributed as code)

- `api/vendor/tunarr-openapi.json` — a pinned copy of the [Tunarr](https://tunarr.com)
  OpenAPI spec, used to generate/validate the client. Tunarr is Zlib-licensed.
