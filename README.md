# Commentarr

*arr-ecosystem tool that finds and acquires commentary-bearing releases of
movies and TV episodes you already own.

Sibling projects:
- [commentary-classifier](https://github.com/jeffWelling/commentary-classifier) — shared classifier lib
- [commentary-detector](https://github.com/jeffWelling/commentary-detector) — interactive web tool

## Status

**v0.0 pre-release** — Plan 1 (foundation + discovery) complete. Subsequent
plans wire Prowlarr search, torrent-client handoff, safety rules, and the
web UI. Architecture lives at `~/claude/projects/commentarr/DESIGN.md`.

## What Plan 1 delivers

- Walk a filesystem-backed library, classify every video file's audio
  tracks, and persist a wanted queue of titles that lack commentary
- SQLite state store with migrations
- Adaptive scan-cadence math (library-size-aware)
- Prometheus classification metrics
- Container image via podman (`scripts/build-image.sh`)

## Build

```bash
go build ./cmd/commentarr
```

Requires a sibling checkout of [commentary-classifier](https://github.com/jeffWelling/commentary-classifier)
at `../commentary-classifier` (see the `replace` directive in `go.mod`).
The replace will go away once the classifier ships a tagged release.

## Scan a local library

```bash
./commentarr scan \
    -root /path/to/movies \
    -db commentarr.db \
    -migrations ./migrations
```

Walks the root, classifies each video file via ffmpeg + the classifier,
and records titles that lack commentary in a persistent wanted queue.
`-library` labels the scan for Prometheus metric partitioning (default
`local`).

## Container image

```bash
./scripts/build-image.sh
podman run --rm \
    -v /path/to/movies:/media:ro \
    -v "$PWD/data":/data \
    commentarr:plan1-dev \
    scan -root /media -db /data/commentarr.db
```

The helper script exists only while the classifier lives behind a local
`replace` directive; see D23 in the project's DECISIONS.md.

## Testing

```bash
go test ./... -race -count=1
```

Testing discipline matches Commentarr's NFR-4 (hard merge gate): unit +
integration tests written against requirements, not against the
implementation. No stubs, no decorative tests.

## License

GPL-3.0 — see [LICENSE](LICENSE).
