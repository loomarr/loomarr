# Loomarr core image (design §16): multi-stage → distroless static, non-root.
# Pure-Go SQLite driver ⇒ CGO_ENABLED=0 ⇒ a fully static binary with no glibc,
# no Python/ffprobe in the core (the filler design §10 depends on this).
# The Node FE build stage is added in Phase 13 (web/ embedded at /).

# ---- build ----
# Debian-based build stage (consistent with the sidecar image); CGO_ENABLED=0
# still yields a fully static binary, so the distroless runtime below is unaffected.
FROM golang:1.26-bookworm AS build
WORKDIR /src
# Cache modules first.
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Static, stripped, reproducible-ish. Build tag placeholder for the embedded FE
# (no-op until Phase 13).
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /out/loomarr ./cmd/loomarr

# ---- runtime variant: loomarr:filler ----------------------------------------
# The SAME binary as above, in a base that also carries the §10 ingest tooling.
# Build with `--target filler`; everything else — config, endpoints, migrations —
# is identical, so moving between tags is a restart, not a topology change (§16).
#
# Ported from the removed Dockerfile.ingest: the sidecar is gone (ingest runs in
# the core now), but its hard-won base-image findings are not.
#
# debian:stable-slim (glibc), NOT Alpine: the upstream yt-dlp + ffmpeg binaries are
# built against glibc and fail on musl with "exec: no such file or directory"
# (missing loader). Only THIS variant carries the tooling; loomarr:latest stays
# static/distroless, so a glibc base here costs nothing there.
#
# §14 tooling policy — bundle the LATEST upstream binaries, not distro packages:
# distros ship yt-dlp months behind, and a stale yt-dlp is a real failure mode
# (YouTube changes player/signature extraction constantly; old versions get
# throttled or cannot extract at all).
#   - yt-dlp: official self-contained `yt-dlp_linux` (bundles its own Python via
#     PyInstaller — no system Python, so §14's all-Go rule holds).
#   - ffmpeg: BtbN static GPL build. Needed so yt-dlp can merge separate
#     video/audio streams; without it high-res sources fail or silently downgrade.
#   - deno: a JS runtime yt-dlp REQUIRES for YouTube — modern yt-dlp deprecated
#     YouTube extraction without one ("No supported JavaScript runtime"), so the
#     YouTube leg fails without it. Auto-detected on PATH. (Found the hard way by
#     a live smoke: the YouTube fetch errored until deno was present.)
# All three are version-pinned so a build cannot silently pull a changed binary.
#
# ARCHITECTURE: all three are fetched per TARGETARCH. The removed Dockerfile.ingest
# hardcoded x86_64 URLs, so on an arm64 host it BUILT FINE and then failed at run
# time with "rosetta error: failed to open elf at /lib64/ld-linux-x86-64.so.2" —
# a broken image that looks healthy until someone actually ingests. Keep these
# arch-aware, and never assume the build host is amd64.
FROM debian:stable-slim AS filler
ARG TARGETARCH
ARG YTDLP_VERSION=2026.07.04
ARG DENO_VERSION=v2.9.2
ARG FFMPEG_TAG=n7.1-latest
RUN set -eux; \
    case "$TARGETARCH" in \
      amd64) YTDLP_ASSET=yt-dlp_linux;         DENO_ARCH=x86_64;  FFMPEG_ARCH=linux64 ;; \
      arm64) YTDLP_ASSET=yt-dlp_linux_aarch64; DENO_ARCH=aarch64; FFMPEG_ARCH=linuxarm64 ;; \
      *) echo "unsupported TARGETARCH: $TARGETARCH" >&2; exit 1 ;; \
    esac; \
    FFMPEG_BUILD="ffmpeg-${FFMPEG_TAG}-${FFMPEG_ARCH}-gpl-7.1"; \
    apt-get update; \
    apt-get install -y --no-install-recommends ca-certificates curl xz-utils unzip; \
    useradd -u 65532 -m -s /usr/sbin/nologin nonroot; \
    curl -fsSL -o /usr/local/bin/yt-dlp \
      "https://github.com/yt-dlp/yt-dlp/releases/download/${YTDLP_VERSION}/${YTDLP_ASSET}"; \
    chmod +x /usr/local/bin/yt-dlp; \
    curl -fsSL -o /tmp/deno.zip \
      "https://github.com/denoland/deno/releases/download/${DENO_VERSION}/deno-${DENO_ARCH}-unknown-linux-gnu.zip"; \
    unzip -q /tmp/deno.zip -d /usr/local/bin; \
    chmod +x /usr/local/bin/deno; \
    rm -f /tmp/deno.zip; \
    curl -fsSL -o /tmp/ffmpeg.tar.xz \
      "https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/${FFMPEG_BUILD}.tar.xz"; \
    tar -xJf /tmp/ffmpeg.tar.xz -C /tmp; \
    # ffmpeg ONLY, no ffprobe: it is ~99MB and Loomarr never probes media — Tunarr
    # assigns duration during its `local`-source scan (§10). ffmpeg is here solely
    # so yt-dlp can merge separate video/audio streams.
    cp "/tmp/${FFMPEG_BUILD}/bin/ffmpeg" /usr/local/bin/; \
    chmod +x /usr/local/bin/ffmpeg; \
    rm -rf /tmp/ffmpeg.tar.xz "/tmp/${FFMPEG_BUILD}"; \
    apt-get purge -y curl xz-utils unzip; \
    apt-get autoremove -y; \
    rm -rf /var/lib/apt/lists/*; \
    # Prove the tooling actually runs on THIS arch, at build time. Without this the
    # x86_64-on-arm64 mistake ships silently.
    /usr/local/bin/yt-dlp --version; \
    /usr/local/bin/ffmpeg -version | head -1; \
    /usr/local/bin/deno --version | head -1
COPY --from=build /out/loomarr /loomarr
# These paths are what the `ingest` feature gate probes for. Set here rather than
# discovered, so the gate is a config question with an operator override (§10).
ENV INGEST_YTDLP_PATH=/usr/local/bin/yt-dlp \
    INGEST_FFMPEG_PATH=/usr/local/bin/ffmpeg
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/loomarr"]

# ---- runtime ----
# distroless static: no shell, no package manager, non-root by default.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/loomarr /loomarr
EXPOSE 8080
# distroless has no shell, so HEALTHCHECK uses the binary's own probe. Phase 1
# ships /healthz; a `loomarr healthcheck` subcommand can replace this later.
# For now rely on the orchestrator's HTTP check (compose below); keep the image
# free of a wget/curl dependency.
USER nonroot:nonroot
ENTRYPOINT ["/loomarr"]
