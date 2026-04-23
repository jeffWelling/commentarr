# Commentarr container image.
#
# The build context is expected to contain both `commentarr/` and
# `commentary-classifier/` at its root — the scripts/build-image.sh helper
# assembles this for local development. Once the classifier module is
# published + tagged on GitHub we drop the replace directive, delete the
# script, and this Dockerfile can be built against the commentarr repo
# alone.

# Build stage
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY commentary-classifier /src/commentary-classifier
COPY commentarr /src/commentarr
WORKDIR /src/commentarr
RUN go mod download
RUN CGO_ENABLED=0 go build -o /out/commentarr ./cmd/commentarr

# Runtime stage
FROM alpine:3.20 AS runtime
RUN apk add --no-cache ffmpeg
COPY --from=build /out/commentarr /usr/local/bin/commentarr
COPY --from=build /src/commentarr/migrations /app/migrations
WORKDIR /app
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/commentarr"]
CMD ["scan", "--help"]
