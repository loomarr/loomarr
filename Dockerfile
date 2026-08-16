# Loomarr's single release image (design §16): a cgo-free Go server, the mandatory
# release-matched Rust image worker, and the media tooling required by playout/ingest.
#
# Stages: fe (SPA) → build (Go) + image-worker (Rust) → runtime (one Debian image).

# ---- frontend ----
# Build the Vite/React SPA so the Go stage can embed it (internal/web/embed.go's
# `//go:embed all:dist`). WITHOUT this stage the image embeds only the .gitkeep
# placeholder and serves a "not built" notice — the UI would be missing. Runs on
# the BUILD platform (native), never emulated: the output is portable static assets.
# codegen reads the committed api/openapi.yaml (orval) — no running server needed.
# Node 22.22.2 satisfies the repository's >=22.5 <23 contract and includes the
# built-in `node:sqlite` pnpm 11.13 uses for its store index. Keep the image on the
# same major CI and contributors certify; a release build is not the place to trial
# the next Node line. Corepack is separately pinned because it is no longer bundled
# in newer official Node images.
FROM --platform=$BUILDPLATFORM node:22.22.2-bookworm-slim AS fe
RUN npm install -g corepack@0.35.0 && corepack enable
WORKDIR /src
COPY web ./web
COPY api/openapi.yaml ./api/openapi.yaml
COPY scripts/check-fe-bundle.mjs ./scripts/check-fe-bundle.mjs
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    cd web && pnpm install --frozen-lockfile \
    && pnpm codegen \
    && pnpm --filter @loomarr/web build
# vite outDir is ../../../internal/web/dist ⇒ /src/internal/web/dist

# ---- build ----
# Cross-compile the cgo-free binary on the BUILD platform for the TARGET arch —
# far faster than compiling under QEMU emulation, and correct because the static
# pure-Go build has no arch-specific C toolchain to satisfy.
FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS build
ARG TARGETARCH
WORKDIR /src
# Cache modules first.
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Overlay the built SPA over the committed .gitkeep placeholder so `go build`'s
# embed bakes the real UI into the binary.
COPY --from=fe /src/internal/web/dist ./internal/web/dist
# Stamped so the running instance can say what it is (§13 Help/About, §16 upgrades).
# Unset ARGs are fine: buildinfo falls back to Go's embedded VCS stamps, then to "dev".
ARG VERSION=""
ARG COMMIT=""
ARG BUILT_AT=""
# Static, stripped, reproducible-ish.
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build \
    -ldflags="-s -w \
      -X github.com/mantonx/loomarr/internal/buildinfo.version=${VERSION} \
      -X github.com/mantonx/loomarr/internal/buildinfo.commit=${COMMIT} \
      -X github.com/mantonx/loomarr/internal/buildinfo.builtAt=${BUILT_AT}" \
    -o /out/loomarr ./cmd/loomarr

# Required image renderer (§14, §22). Build natively for each Buildx target so the bundled
# libwebp and Rust standard library always match the runtime architecture.
FROM rust:1.93-bookworm AS image-worker
WORKDIR /src
COPY Cargo.toml Cargo.lock rust-toolchain.toml ./
COPY rust ./rust
ARG VERSION=""
RUN LOOMARR_RELEASE="${VERSION:-dev}" cargo build --release --locked -p loomarr-image

# ---- runtime ----------------------------------------------------------------
# THE image. One tag, one release unit, all required binaries and tooling (§16 — revised).
#
# Loomarr previously published two tags: a 31MB distroless `loomarr:latest` with no
# media tooling, and a 549MB `loomarr:filler` that added it for the §10 ingest job.
# That split existed to keep media tooling out of the default image — the same goal
# that had earlier motivated a separate ingest sidecar, itself already reversed in
# favour of the opt-in tag.
#
# §9.1 makes `ffmpeg` load-bearing for PLAYOUT, not just ingest. A tag without an
# encoder can no longer serve a channel, so the "slim" variant would not be a smaller
# Loomarr — it would be a Loomarr that cannot do the main thing. Two tags where one is
# functionally incomplete is a support burden, not a choice, so the split collapses.
#
# The cost, stated plainly: the default download grows ~18x (31MB → 549MB) and every
# install carries an encoder whether or not it uses internal playout. This is the THIRD
# time this packaging question has been decided (sidecar → opt-in tag → single image);
# each reversal followed a change in what the tooling was FOR. If a future change makes
# the encoder optional again, revisit it with that history in view.
#
# Ported from the removed Dockerfile.ingest: the sidecar is gone, but its hard-won
# base-image findings are not.
#
# debian:stable-slim (glibc), NOT Alpine: the upstream yt-dlp + ffmpeg binaries are
# built against glibc and fail on musl with "exec: no such file or directory"
# (missing loader). This is what costs us distroless — the static base cannot run
# them, and §9.1 means we can no longer do without them.
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
FROM debian:stable-slim AS runtime
ARG TARGETARCH
ARG YTDLP_VERSION=2026.07.04
ARG DENO_VERSION=v2.9.2
# ⚠⚠ DO NOT BUMP THIS WITHOUT RUNNING `make test-ffmpeg` AGAINST THE NEW BUILD.
#
# **ffmpeg n9 BREAKS INTERNAL PLAYOUT ENTIRELY** (§9.1), so this pin has a CEILING, not just a
# floor. n9's HTTP protocol treats a response with no Content-Length as having length UINT64_MAX,
# so when the body ends it reports
#
#   [http] Stream ends prematurely at 85916, should be 18446744073709551615
#   [in#0/concat] Error during demuxing: Input/output error
#
# and the concat demuxer STOPS instead of opening the next playlist entry.
# `/v1/playout/program/{id}` streams a live encode and therefore can NEVER send a Content-Length,
# so on n9 every internal-playout channel plays one programme and then repeats it forever.
#
# Measured 2026-08-09, identical harness, chunked entry:
#
#   n7.1.5 → 5 entry fetches, advances ✓
#   n8.1.2 → 5 entry fetches, advances ✓   ← this pin
#   n9.0   → 1 fetch, 3× EIO            ✗
#
# Mitigations that do NOT work on n9 (all tested — do not spend time re-trying): dropping the
# -reconnect flags, -reconnect_streamed, -seekable 0/1, connection-close (HTTP/1.0) framing,
# -ignore_io_errors (HLS-only, not a concat option), -multiple_requests (hangs). n9's concat
# demuxer exposes no error-tolerance option at all. Getting onto n9 needs an ARCHITECTURE change
# — the parent must stop consuming a chunked HTTP stream — not a flag.
#
# `TestLive_ConcatAdvancesPastAChunkedHTTPEntry` is the guard and runs in under a second. Nothing
# else catches this: `-stream_loop -1` masks it by replaying the buffered programme, so the parent
# still emits continuous output and exits 0, and the failure presents as "the channel repeats one
# show" rather than as an error.
#
# n8.1 is the newest series BtbN publishes in the `latest` release (it ships 7.1 and 8.1 only), and
# the full `make test-ffmpeg` playout suite is green against it.
ARG FFMPEG_TAG=n8.1-latest
ARG WHISPER_VERSION=v1.9.1
# The model is pinned by REVISION + SHA256, not by a floating branch name: a model file
# that silently changes content changes transcription, and transcription decides where
# clips get cut. Same reasoning as the version pins above, higher stakes.
ARG WHISPER_MODEL_REV=5359861c739e955e79d9a303bcbc70fb988958b1
ARG WHISPER_MODEL_SHA256=c6138d6d58ecc8322097e0f987c32f1be8bb0a18532a3f88f734d1bbf9c41e5d
# ⚠ A SECOND model, for language identification only (§10 V40) — and the `.en` suffix is
# exactly why it is needed. `ggml-small.en.bin` above is an ENGLISH-ONLY build: it does not
# perform language identification at all, it assumes English and transcribes accordingly.
# Asked "what language is this?" about a Spanish advert it answers `en`, so the language
# gate would silently never reject anything — a feature that looks shipped and does nothing.
#
# `tiny` (multilingual, ~74MB) is adequate here in a way it was NOT for splitting: language
# ID is CLASSIFICATION over the first seconds, not transcription. The gate that ruled out
# `tiny.en` for splitting was "does it drop audible speech", which this task never asks.
# Measured 2026-08-03: 77,691,713 bytes, sha256 be07e048… (verified by download, not
# copied from a listing).
ARG WHISPER_LANG_MODEL_SHA256=be07e048e1e599ad46341c8d2a135645097a538221678b7acdd1b1919c6e1b21
RUN set -eux; \
    case "$TARGETARCH" in \
      amd64) YTDLP_ASSET=yt-dlp_linux;         DENO_ARCH=x86_64;  FFMPEG_ARCH=linux64;    WHISPER_ARCH=x64 ;; \
      arm64) YTDLP_ASSET=yt-dlp_linux_aarch64; DENO_ARCH=aarch64; FFMPEG_ARCH=linuxarm64; WHISPER_ARCH=arm64 ;; \
      *) echo "unsupported TARGETARCH: $TARGETARCH" >&2; exit 1 ;; \
    esac; \
    # BtbN's asset name repeats the release series in the SUFFIX
    # (ffmpeg-n8.1-latest-linux64-gpl-8.1.tar.xz), so it is DERIVED from FFMPEG_TAG rather than
    # written out a second time. It used to be a literal `gpl-7.1`, which meant bumping the pin in
    # one place produced a 404 at build time instead of a new ffmpeg — the same hand-maintained
    # coupling this repo has been bitten by elsewhere.
    FFMPEG_SERIES="${FFMPEG_TAG%-latest}"; FFMPEG_SERIES="${FFMPEG_SERIES#n}"; \
    FFMPEG_BUILD="ffmpeg-${FFMPEG_TAG}-${FFMPEG_ARCH}-gpl-${FFMPEG_SERIES}"; \
    apt-get update; \
    apt-get install -y --no-install-recommends ca-certificates curl xz-utils unzip; \
    # HARDWARE-ENCODE DRIVER LIBRARIES (§9.1). ffmpeg dlopen()s these at runtime, so
    # without them EVERY hardware family fails the capability probe on EVERY host —
    # measured, not assumed: the probe reported "Unable to open the libvulkan library",
    # "libva-drm.so.2 ... No such file", "libX11.so.6 ... No such file", "DLL
    # libamfrt64.so.1 failed to open". The ffmpeg build supported all of them; the image
    # simply could not load any.
    #
    # Deliberately VENDOR-NEUTRAL — one package set, every GPU:
    #   libva2 / libva-drm2   VAAPI  → Intel AND AMD on Linux
    #   mesa-va-drivers       the open VAAPI drivers (AMD radeonsi, Intel i965/iHD era)
    #   intel-media-va-driver Intel iHD — QSV and modern Intel VAAPI (amd64 only; see below)
    #   libvulkan1 + mesa-vulkan-drivers  Vulkan → cross-vendor
    #   libx11-6 / libxext6   VAAPI's X11 display backend (h264_vaapi dlopens it even
    #                         headless; its absence is what broke vaapi above)
    #   libdrm2               the common KMS/DRM layer under all of them
    #
    # ⚠ NVENC needs NOTHING here: libcuda.so.1 is INJECTED by the nvidia-container-toolkit
    # at run time (`--gpus all` / the `nvidia` runtime), never installed into the image.
    # Baking a CUDA runtime in would bloat the image by ~1GB and still be wrong, because
    # the injected driver must match the host kernel module.
    #
    # Cost is ~120MB. Accepted: §9.1 makes playout a core capability, and an image that
    # can only ever encode in software is not a smaller Loomarr but a slower one.
    #
    # ⚠ intel-media-va-driver is amd64-ONLY and must stay out of the arm64 install list.
    # It is the Intel iHD/QSV driver, so there is nothing for it to drive on ARM, and
    # Debian ships no arm64 candidate: including it unconditionally made `apt-get`
    # exit 100 with "Package 'intel-media-va-driver' has no installation candidate",
    # which broke the arm64 half of the release build (release.yml builds
    # linux/amd64,linux/arm64) AND every local build on an Apple-Silicon Mac. Everything
    # else in this list is arch-neutral and stays shared — the VAAPI/Vulkan stack is
    # what an ARM SBC with a Mali/V3D GPU actually uses.
    # A FONT IS A FUNCTIONAL DEPENDENCY OF PLAYOUT, not a nicety (§16, §9.1).
    #
    # The offline/test card labels itself with ffmpeg's `drawtext`, which fails at filter
    # INIT on a missing fontfile — so playout.FindFont stats real paths and, finding none,
    # emits no `-vf` at all rather than killing the encode. That fail-safe is correct, but
    # in an image with zero fonts the degradation is TOTAL: the card's source is
    # `color=c=black` plus `anullsrc`, so it renders as an unlabelled black frame with
    # silent audio — indistinguishable from the dead channel the card exists to replace.
    #
    # This was live: the base is debian:stable-slim, which ships no /usr/share/fonts at
    # all, and nothing above pulls a font in transitively. `font.go` asserted the opposite
    # ("The image installs fonts-dejavu-core (§16)") and §16 had never said so — a comment
    # claiming a dependency exists is not the dependency existing. §16 now records it.
    #
    # `fonts-dejavu-core` (not `fonts-dejavu`) is the ~1.5MB subset carrying DejaVuSans.ttf
    # — the first entry in fontCandidates — and it is arch-neutral, so unlike
    # intel-media-va-driver below it stays in the shared list.
    apt-get install -y --no-install-recommends \
      libva2 libva-drm2 libvulkan1 libdrm2 libx11-6 libxext6 \
      mesa-va-drivers mesa-vulkan-drivers \
      fonts-dejavu-core \
      libgomp1; \
    if [ "$TARGETARCH" = "amd64" ]; then \
      apt-get install -y --no-install-recommends intel-media-va-driver; \
    fi; \
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
    # ffmpeg AND ffprobe (revised, §9.1). ffprobe was excluded to save ~99MB on the
    # grounds that "Loomarr never probes media — Tunarr assigns duration during its
    # `local`-source scan". Internal playout owns the encoder, so it owns duration and
    # cut points too, and that premise is gone. ffmpeg now serves two callers: yt-dlp's
    # stream merging (§10) and the playout encoder (§9.1).
    cp "/tmp/${FFMPEG_BUILD}/bin/ffmpeg" "/tmp/${FFMPEG_BUILD}/bin/ffprobe" /usr/local/bin/; \
    chmod +x /usr/local/bin/ffmpeg /usr/local/bin/ffprobe; \
    rm -rf /tmp/ffmpeg.tar.xz "/tmp/${FFMPEG_BUILD}"; \
    # whisper-cli (§14, V34) — the fifth vendored binary, for compilation splitting.
    #
    # ⚠ UNLIKE yt-dlp, THIS ONE IS NOT SELF-CONTAINED. It links libwhisper + libggml,
    # so the shared objects ship beside it in /usr/local/lib/whisper. Copying only the
    # executable produces the failure this file already warns about in another form: a
    # build that succeeds and a binary that dies at run time with "error while loading
    # shared libraries". There are TWO distinct lookups here and they resolve
    # differently — the linked libs via ldconfig, the compute BACKEND via a directory
    # scan (see the layout warning further down); satisfying only the first is what
    # makes `--help` work and transcription abort.
    #
    # ⚠ COPY THE WHOLE libggml SET, never a chosen subset. On amd64 upstream ships 15
    # libggml-cpu-*.so microarchitecture variants (alderlake, zen4, sse42, …) and picks
    # one AT RUN TIME from the host CPU; arm64 ships a single libggml-cpu.so. Pruning to
    # "the one that works here" is the same class of bug as the hardcoded x86_64 URLs —
    # it builds fine and then fails only on the hosts you did not test.
    #
    # libgomp1 is an apt dependency, NOT bundled: whisper-cli is OpenMP-threaded and
    # debian:stable-slim does not ship libgomp. Measured — without it the binary aborts
    # at load with "libgomp.so.1: cannot open shared object file".
    curl -fsSL -o /tmp/whisper.tar.gz \
      "https://github.com/ggml-org/whisper.cpp/releases/download/${WHISPER_VERSION}/whisper-bin-ubuntu-${WHISPER_ARCH}.tar.gz"; \
    tar -xzf /tmp/whisper.tar.gz -C /tmp; \
    install -d /usr/local/lib/whisper; \
    cp "/tmp/whisper-bin-ubuntu-${WHISPER_ARCH}"/libwhisper.so* \
       "/tmp/whisper-bin-ubuntu-${WHISPER_ARCH}"/libggml*.so* /usr/local/lib/whisper/; \
    # ⚠ whisper-cli LIVES BESIDE ITS LIBRARIES, and /usr/local/bin gets a symlink.
    # ggml's compute backends are dlopen()ed at run time, and it searches the
    # EXECUTABLE'S OWN DIRECTORY — not ld.so.conf, and not a directory in an env var.
    # So the obvious layout (binary in /usr/local/bin, libs in /usr/local/lib/whisper +
    # ldconfig) links fine and then aborts on first real use with
    # "GGML_ASSERT(device) failed" — no backend found. Measured on arm64: `--help`
    # SUCCEEDS in that layout because it never initialises a backend, which is exactly
    # why the build-time proof below transcribes real audio instead.
    # (GGML_BACKEND_PATH is not an escape hatch — it wants a FILE, not a directory.)
    cp "/tmp/whisper-bin-ubuntu-${WHISPER_ARCH}/whisper-cli" /usr/local/lib/whisper/whisper-cli; \
    chmod +x /usr/local/lib/whisper/whisper-cli; \
    ln -s /usr/local/lib/whisper/whisper-cli /usr/local/bin/whisper-cli; \
    echo /usr/local/lib/whisper > /etc/ld.so.conf.d/whisper.conf; \
    ldconfig; \
    rm -rf /tmp/whisper.tar.gz "/tmp/whisper-bin-ubuntu-${WHISPER_ARCH}"; \
    # The model. ⚠ `small.en` is a CORRECTNESS floor, not a quality preference, and at
    # 466MB it is the single largest thing in this image (rootfs ~821MB → ~1.3GB, of
    # which the binary + libs are only ~20MB). Measured against THIS binary on a real 244s
    # 1990 commercial break (archive.org witi-6-commercial-breaks-10-24-1990): `tiny.en`
    # silently dropped a whole 20s advert whose audio sits at the file's average
    # loudness, and `base.en` dropped 7s of equally audible speech; `small.en` was the
    # smallest with NO gap over audible content (its one 5s gap is true near-silence at
    # -54dB, i.e. correctly transcribing nothing). §6.4's finding, reproduced against the
    # vendored binary rather than the Python package — which is what the gate demanded,
    # and it also rules out `base.en`, a size the plan never measured.
    install -d /usr/local/share/whisper; \
    curl -fsSL -o /usr/local/share/whisper/ggml-small.en.bin \
      "https://huggingface.co/ggerganov/whisper.cpp/resolve/${WHISPER_MODEL_REV}/ggml-small.en.bin"; \
    echo "${WHISPER_MODEL_SHA256}  /usr/local/share/whisper/ggml-small.en.bin" | sha256sum -c -; \
    # The language-ID model (§10 V40) — see the ARG comment for why `small.en` cannot do this job.
    curl -fsSL -o /usr/local/share/whisper/ggml-tiny.bin \
      "https://huggingface.co/ggerganov/whisper.cpp/resolve/${WHISPER_MODEL_REV}/ggml-tiny.bin"; \
    echo "${WHISPER_LANG_MODEL_SHA256}  /usr/local/share/whisper/ggml-tiny.bin" | sha256sum -c -; \
    apt-get purge -y curl xz-utils unzip; \
    apt-get autoremove -y; \
    rm -rf /var/lib/apt/lists/*; \
    # Prove the tooling actually runs on THIS arch, at build time. Without this the
    # x86_64-on-arm64 mistake ships silently.
    /usr/local/bin/yt-dlp --version; \
    /usr/local/bin/ffmpeg -version | head -1; \
    /usr/local/bin/ffprobe -version | head -1; \
    /usr/local/bin/deno --version | head -1; \
    # ⚠ whisper is NOT proved by `--help`: it returns 0 without ever initialising a compute
    # backend, and did so on arm64 in a layout where the first real transcription aborted
    # with "GGML_ASSERT(device) failed". What actually has to be proved is that ggml can
    # LOAD ITS BACKEND through the /usr/local/bin symlink — the same path the app execs —
    # which is what catches a wrong lib layout, a pruned libggml set, or an arch mismatch.
    #
    # This probe does exactly that and nothing more. It is invoked with no usable model or
    # input on purpose: the backend line is printed during init, BEFORE any inference, so
    # the check is instant and the nonzero exit that follows is expected (hence `|| true`,
    # and the grep is the real assertion).
    #
    # ⚠ IT DELIBERATELY DOES NOT TRANSCRIBE. Running the model here cost ~3s natively and
    # ~341s under QEMU on CI's arm64 leg — whisper is dense matrix math, the worst case for
    # instruction-level emulation, and it made the image job by far the slowest thing in
    # CI. The full transcription proof still runs, on the NATIVE amd64 leg below, where it
    # is cheap. Do not "restore" the transcription to this shared step.
    { whisper-cli -m /dev/null -f /dev/null 2>&1 || true; } | grep -q 'load_backend: loaded'; \
    # The FULL proof — real audio through the real model — but only where it is native.
    # This is what a bad model download or a broken model file fails on.
    if [ "$TARGETARCH" = "amd64" ]; then \
      ffmpeg -v error -f lavfi -i anullsrc=r=16000:cl=mono -t 1 -y /tmp/probe.wav; \
      whisper-cli -m /usr/local/share/whisper/ggml-small.en.bin -f /tmp/probe.wav -np >/dev/null; \
      rm -f /tmp/probe.wav; \
    fi
ARG VERSION=""
COPY --from=build /out/loomarr /loomarr
COPY --from=image-worker /src/target/release/loomarr-image /usr/local/bin/loomarr-image
RUN contract="$(/usr/local/bin/loomarr-image capabilities --protocol 1 --self-test)"; \
    echo "$contract" | grep -q '"selfTest":true'; \
    echo "$contract" | grep -q "\"release\":\"${VERSION:-dev}\""
# Pre-create /data owned by nonroot. Docker seeds a fresh NAMED volume from the image's
# directory at that path — including its ownership — so this is what makes the documented
# zero-env `docker run -v loomarr-data:/data loomarr` actually work. Without it the volume
# arrives root-owned, the app cannot create loomarr.db, and boot dies with
# "unable to open database file (14)".
#
# Previously masked: DATABASE_URL had no default (S1), so the app never tried to open a
# file and failed later and differently. Fixing the default made the real problem the
# first thing that happens. compose works around it with a one-shot chown init container;
# that stays for BIND mounts (host-owned paths the image cannot pre-seed), but a named
# volume no longer needs it.
# /data/filler alongside it: FILLER_DIR defaults there (§15), and seeding it here means a
# fresh named volume arrives with the drop-folder already present and nonroot-owned. The app
# also MkdirAll's it at boot — belt and braces, because a BIND mount ignores what the image
# seeded and only the runtime create covers that case.
# /data/images for the same reason (§22, V52): `images.dir` defaults there. ⚠ This became
# load-bearing with phase 3b rather than merely tidy — `images-fetch` runs EVERY MINUTE, so on a
# zero-env first run the very first thing to touch this path is a background job. Without the
# pre-create it writes into a root-owned volume, fails, and reports the failure on the Tasks page
# once a minute forever, with nothing connecting it to a directory nobody created.
# /data/prepared is the persistent playout publication root (§9.1 V56). Unlike the live HLS
# scratch directory it survives viewers and restarts, so a fresh named volume must make it writable
# before the readiness job runs for the first time.
RUN install -d -o 65532 -g 65532 /data /data/filler /data/images /data/prepared
VOLUME /data
# These paths are what the `ingest` feature gate probes for. Set here rather than
# discovered, so the gate is a config question with an operator override (§10).
ENV INGEST_YTDLP_PATH=/usr/local/bin/yt-dlp \
    INGEST_FFMPEG_PATH=/usr/local/bin/ffmpeg \
    INGEST_WHISPER_PATH=/usr/local/bin/whisper-cli \
    INGEST_WHISPER_MODEL=/usr/local/share/whisper/ggml-small.en.bin \
    FILLER_LANGUAGE_MODEL=/usr/local/share/whisper/ggml-tiny.bin
ARG COMMIT=""
# licenses is the SPDX expression for the AGGREGATE image: Loomarr (MIT) plus the
# bundled GPL ffmpeg. See THIRD_PARTY_NOTICES.md for the source offer.
LABEL org.opencontainers.image.title="loomarr" \
      org.opencontainers.image.description="Turn a natural-language channel intent into a live, self-maintaining Tunarr channel." \
      org.opencontainers.image.source="https://github.com/mantonx/loomarr" \
      org.opencontainers.image.licenses="MIT AND GPL-3.0-only" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}"
EXPOSE 8080
# The base now HAS a shell, so a HEALTHCHECK could shell out — but the orchestrator's
# HTTP check (compose) still owns readiness, and keeping the image free of a
# wget/curl dependency is worth more than an in-image probe. /v1/healthz is the contract
# (the bare /healthz alias answers identically, for checks configured outside this repo).
USER nonroot:nonroot
ENTRYPOINT ["/loomarr"]
