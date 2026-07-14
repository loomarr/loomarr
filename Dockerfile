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
