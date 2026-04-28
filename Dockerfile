# Commentarr container image.
#
# Multi-stage, multi-arch (linux/amd64, linux/arm64). The web SPA is
# built in a Node stage, copied into the Go source tree as web-dist,
# and then compiled into the binary via go:embed. Runtime stage is
# alpine + ffmpeg (provides ffprobe) + the binary only. Build from
# the repo root: `docker build .`

# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.25
ARG NODE_VERSION=20
ARG ALPINE_VERSION=3.20

# --- SPA build stage ---------------------------------------------------
FROM node:${NODE_VERSION}-alpine AS web-build
WORKDIR /web
COPY web/package.json web/package-lock.json* ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# --- Go build stage ----------------------------------------------------
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY . .
# Overlay the freshly-built SPA into the Go embed directory so go:embed
# picks it up regardless of whatever was committed.
COPY --from=web-build /web/dist /src/cmd/commentarr/web-dist
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags='-s -w' -o /out/commentarr ./cmd/commentarr

# --- Runtime stage -----------------------------------------------------
FROM alpine:${ALPINE_VERSION} AS runtime

# ffprobe ships with the ffmpeg apk package; tini reaps zombie children
# from the http/download-client clients; ca-certificates for TLS to
# indexers and download clients.
RUN apk add --no-cache ffmpeg ca-certificates tini \
 && addgroup -S -g 65532 commentarr \
 && adduser  -S -u 65532 -G commentarr commentarr

COPY --from=build /out/commentarr /usr/local/bin/commentarr
COPY --from=build /src/migrations /migrations

# Data dir is the sole writable mount Commentarr needs.
RUN mkdir -p /data && chown -R 65532:65532 /data
VOLUME ["/data"]

USER 65532:65532
WORKDIR /data
EXPOSE 7878

HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
  CMD wget -qO- http://127.0.0.1:7878/healthz >/dev/null || exit 1

ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/commentarr"]
CMD ["serve", "-addr", ":7878", "-db", "/data/commentarr.db", "-migrations", "/migrations"]

# OCI image annotations.
LABEL org.opencontainers.image.title="Commentarr" \
      org.opencontainers.image.description="Automated finder for commentary-bearing releases of movies and TV you already own." \
      org.opencontainers.image.source="https://github.com/jeffWelling/commentarr" \
      org.opencontainers.image.licenses="GPL-3.0-or-later" \
      org.opencontainers.image.vendor="Jeff Welling"
