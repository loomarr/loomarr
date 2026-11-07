# The demo site's image: a Loomarr image plus the demo-library tool, built from the same source.
# It leaves the release Dockerfile alone, so a release never carries demo code. Build it from the
# commit LOOMARR_IMAGE came from: the seeder migrates the store, and a newer seeder would leave a
# database the older server refuses to open.
ARG LOOMARR_IMAGE

FROM --platform=$BUILDPLATFORM golang:1.27.3-bookworm@sha256:972b4110ba0f47539a975c31e52c64765e1ff19f2d74469ae4a595c6676e80a5 AS build
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o /out/demo-library ./cmd/demo-library

FROM ${LOOMARR_IMAGE}
COPY --from=build /out/demo-library /usr/local/bin/demo-library
# The stand-in reports absolute paths and Loomarr reads those files directly, so both containers
# mount the assets at this one path. A fresh named volume copies this directory's owner.
USER root
RUN mkdir -p /demo && chown 65532:65532 /demo
USER 65532:65532
VOLUME /demo
