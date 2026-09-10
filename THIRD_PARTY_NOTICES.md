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

The Apache-2.0 components (`prometheus/client_golang` and its transitive `prometheus/*`) carry a
§4(d) NOTICE requirement, satisfied here rather than deferred to the SBOM. The upstream `NOTICE`
reads:

> Prometheus instrumentation library for Go applications
> Copyright 2012-2015 The Prometheus Authors
>
> This product includes software developed at SoundCloud Ltd. (http://soundcloud.com/).

Upstream additionally attributes bundled components (`beorn7/perks`, Go support for Protocol
Buffers, and others; see the full [`NOTICE`](https://github.com/prometheus/client_golang/blob/main/NOTICE)).
The full Apache-2.0 license text also rides in the release SBOM.

## Frontend dependencies (bundled into the embedded SPA)

The web UI is built with Vite and **embedded into the Go binary** (`internal/web/embed.go`
`//go:embed all:dist`), so its RUNTIME dependencies ship inside the published image. Only the
production (non-`devDependencies`) tree is bundled; build/test tooling (Vite, Biome, Playwright,
Storybook, vitest) is not shipped and is out of scope here.

The bundled tree is entirely permissive and MIT-compatible. `pnpm licenses list --prod` over the
shipped closure resolves to **MIT (the large majority), plus ISC, Apache-2.0, and 0BSD** — no
copyleft (no GPL/LGPL/AGPL/MPL/EPL/CDDL) reaches the bundle. Notable direct runtime deps: `react`
/ `react-dom` (MIT), the TanStack set — `react-query` / `react-form` / `react-virtual` (MIT),
`@base-ui/react` and `@dnd-kit/*` (MIT), `react-markdown` / `remark-gfm` (MIT), `lucide-react`
(ISC), `hls.js` and `class-variance-authority` (Apache-2.0). The authoritative, versioned list is
[`web/pnpm-lock.yaml`](web/pnpm-lock.yaml); the full per-package inventory rides in the release SBOM.

## Rust dependencies (compiled into the image worker)

The image-processing worker (`internal/images/rustgen` → the `loomarr-image` crate) is compiled
during the image build and its binary ships in the image. Its dependency tree is **confined to an
explicit permissive allow-list, enforced on every build** by `cargo-deny` via [`deny.toml`](deny.toml)
and the `make rust-audit` gate: MIT, Apache-2.0 (incl. the LLVM exception), the BSD family, ISC,
Zlib, Unicode-3.0, Unlicense, CC0-1.0, NCSA, and 0BSD — a crate under any other license fails the
build rather than shipping. `cargo-deny` also denies unknown registries and git sources. The
authoritative, versioned list is [`Cargo.lock`](Cargo.lock); the full per-crate inventory rides in
the release SBOM.

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

The versioned [`redistribution-manifest-v1.json`](docs/engineering/evidence/redistribution-manifest-v1.json)
binds the pinned FFmpeg and yt-dlp asset identities to upstream release, source/build, and license
references. It is traceability evidence only: it does not retain corresponding source or provide
source-retention, distribution, or legal clearance.

The pinned FFmpeg and yt-dlp source materials are available without authentication or charge in
[Loomarr's August source release](https://github.com/loomarr/loomarr/releases/tag/third-party-sources-2026-08-31).
Download `source-materials.tar.gz` and `ffmpeg-original-download-cache.zip`, then verify them against
`SHA256SUMS`. `README.txt` explains the contents; `source-package-inventory.json` records all 490
source/license file hashes. The [distribution record](docs/engineering/evidence/source-distribution-2026-08-31.json)
binds public asset URLs, sizes and SHA256 digests to the accepted dependency pins. These directions
ship in the image at `/usr/share/doc/loomarr/THIRD_PARTY_NOTICES.md`. Application release notes link
the same source location alongside the image and bind the exact application commit/image separately.

| Binary | Upstream | License | Notes |
| --- | --- | --- | --- |
| `yt-dlp` standalone executable | https://github.com/yt-dlp/yt-dlp | **GPL-3.0-or-later** for the combined executable | The source project is primarily Unlicense, but upstream states that official PyInstaller executables bundle GPLv3+ dependencies and the combined work is GPLv3+. Loomarr ships `yt-dlp_linux` / `yt-dlp_linux_aarch64`, so the executable terms apply. See upstream's [license section](https://github.com/yt-dlp/yt-dlp/blob/master/README.md#license) and [third-party license inventory](https://github.com/yt-dlp/yt-dlp/blob/master/THIRD_PARTY_LICENSES.txt). |
| `ffmpeg` | https://github.com/BtbN/FFmpeg-Builds | **GPL-3.0-or-later** (the BtbN `-gpl-` build enables GPL and version 3) | Serves both yt-dlp stream merging (§10) and the playout encoder (§9.1). Source materials and build recipes are linked above. |
| `ffprobe` | https://github.com/BtbN/FFmpeg-Builds | **GPL-3.0-or-later** (same build) | Added with internal playout (§9.1). It shares ffmpeg's source materials and build recipes above. |
| `deno` | https://github.com/denoland/deno | MIT | JS runtime yt-dlp requires for YouTube extraction. |
| `whisper-cli`, `libwhisper`, `libggml` | https://github.com/ggml-org/whisper.cpp | MIT | Pinned `v1.9.1` binary and runtime-selected shared libraries used for compilation splitting and language identification. |
| `ggml-small.en.bin`, `ggml-tiny.bin` | https://huggingface.co/ggerganov/whisper.cpp | MIT | Revision- and SHA256-pinned Whisper model data; `small.en` transcribes and `tiny` identifies language. |
| `DejaVuSans.ttf` (Debian `fonts-dejavu-core`) | https://dejavu-fonts.github.io | Bitstream Vera Fonts (MIT-style) + public-domain DejaVu changes | The single font the "no signal"/dead-air playout card renders (§16); `debian:stable-slim` ships none, so `fonts-dejavu-core` is installed for it. Permissive and redistributable; the one condition is that the copyright + permission notice accompany the fonts — satisfied by the package's own `/usr/share/doc/fonts-dejavu-core/copyright` in the image. See [Font license](#dejavu-font-license-bitstream-vera) below. |

### DejaVu font license (Bitstream Vera)

The bundled `DejaVuSans.ttf` (Debian `fonts-dejavu-core`) is redistributable under the **Bitstream
Vera Fonts Copyright**, an MIT-style permissive license; the DejaVu project's own changes are placed
in the public domain. The image satisfies the license's notice condition by retaining the package's
`/usr/share/doc/fonts-dejavu-core/copyright`. The operative grant:

> Copyright (c) 2003 by Bitstream, Inc. All Rights Reserved. Bitstream Vera is a trademark of
> Bitstream, Inc.
>
> Permission is hereby granted, free of charge, to any person obtaining a copy of the fonts
> accompanying this license ("Fonts") and associated documentation files (the "Font Software"), to
> reproduce and distribute the Font Software, including without limitation the rights to use, copy,
> merge, publish, distribute, and/or sell copies of the Font Software, and to permit persons to whom
> the Font Software is furnished to do so, subject to the following conditions: [the copyright/
> trademark and this permission notice must accompany the Font Software; modified fonts must be
> renamed to exclude "Bitstream" and "Vera"]. The full text ships at the path above and at
> <https://dejavu-fonts.github.io/License.html>.

### GPL source distribution (ffmpeg)

The `ffmpeg` binary is a **GPL-3.0-or-later** BtbN build from the `n8.1` series. The
[`Dockerfile`](Dockerfile) pins it to a **retained monthly** archive — release
`autobuild-2026-08-31-13-27`, build `n8.1.2-50-g1a748fe2cd`, verified by per-architecture SHA256
(`FFMPEG_AMD64_SHA256` / `FFMPEG_ARM64_SHA256`) exactly as yt-dlp, deno, and whisper are — so the
bytes in the image are now identified by digest rather than by BtbN's mutable `latest` release.
The evidence manifest also binds BtbN's pruning policy at the release commit: ordinary daily
archives expire after the newest 14, while one archive per month is retained for 24 months.

The August build replaces July because its original upstream dependency download cache and build
logs were retained before expiry. The [retention record](docs/engineering/evidence/ffmpeg-august-source-retention.md)
binds those archives to the new pins and records the remaining distribution and validation work.

The source release retains the exact FFmpeg commit `1a748fe2cd43e3ead22fafb1b5b7d77f153898a8`,
BtbN build scripts at `8267213e26c1031621e6e1210fe3aa4867214f6a`, the original dependency source
cache, supplemental dependency sources and license texts. The unchanged `make test-ffmpeg` gate
passed on both Linux architectures at source-pin commit `c4c8b6a4`; final release-commit and image
acceptance remain required. The package does not assert bit-for-bit reproduction of upstream
outputs; the original rav1e lock predates an upstream build-helper update whose exact graph is
unproven. The source inventory includes optional/build/test dependencies without claiming every
listed component is linked into the binaries.

⚠ This section previously said the opposite — that redistributing the default image
"carries no such obligation — it contains no ffmpeg", which was true only while ffmpeg
shipped in a separate opt-in variant. Internal playout (§9.1) made ffmpeg load-bearing
for streaming, the variant collapsed into the one image, and this paragraph did not
follow. A stale licensing statement is worse than a stale feature description, which is
why it is corrected in place rather than quietly rewritten.

## Open redistribution review — beta blockers

This notice is an inventory, not release clearance. The following engineering evidence remains
required on the beta.5 release commit. Release engineering must bind the exact corresponding source
to the actual shipped artifacts and document source distribution mechanics for that image using the
public source record above:

- verify the final candidate uses the exact pinned FFmpeg, ffprobe and yt-dlp binaries tied to the
  published source materials, and record the unchanged full `make test-ffmpeg` gate on that commit;
- verify both release-platform images retain the referenced notices/license texts and bind their
  image digests and SBOM/provenance to the exact release source;
- ~~pin the runtime and build base images by digest~~ (done — all four `FROM` bases in
  [`Dockerfile`](Dockerfile) carry an immutable `@sha256:` alongside their tag) ~~and make the Debian
  package input reproducible~~ (done — the runtime stage repoints apt at a fixed
  `snapshot.debian.org` timestamp (`DEBIAN_SNAPSHOT`) matching the base image's own build snapshot, so
  every `apt-get install` resolves the same package versions on every build);
- ~~include the required DejaVu font license~~ (done — the Bitstream Vera grant is reproduced above,
  and the image retains the package's own `copyright`) ~~and complete the frontend/Rust transitive
  license-text inventory~~ (done — the Frontend and Rust dependency sections above state the shipped
  closures' licenses: the bundled JS tree is MIT/ISC/Apache-2.0/0BSD with no copyleft, and the Rust
  tree is confined to `deny.toml`'s permissive allow-list enforced by `make rust-audit`; the full
  per-package/per-crate texts ride in the release SBOM);
- ~~inspect and include any required Prometheus `NOTICE` material~~ (done — the upstream NOTICE is
  reproduced above); and
- verify the final application release notes and image notices point to the public source location
  above. Source distribution is published; the exact application release binding remains pending.

For beta.5, the maintainer removed the separate qualified legal/NOTICE reviewer sign-off requirement.
Release engineering owns the evidence above; an external reviewer or legal opinion is not a release
prerequisite. This policy change does not close missing source or notice evidence.

Until those items close, neither this file nor the BuildKit SBOM should be read as a claim that the
image is ready for redistribution.

## Reference specs (not redistributed as code)

- `api/vendor/tunarr-openapi.json` — a pinned copy of the [Tunarr](https://tunarr.com)
  OpenAPI spec, used to generate/validate the client. Tunarr is Zlib-licensed.
