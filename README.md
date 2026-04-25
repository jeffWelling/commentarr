# Commentarr

*arr-ecosystem tool that finds and acquires commentary-bearing releases
of movies and TV episodes you already own. Think Radarr, but it hunts
for director's commentaries, Criterion editions, and special-edition
cuts instead of whatever copy of the movie.

- Integrates with Plex, Jellyfin, Emby, or a plain filesystem as a
  library source.
- Classifies every audio track via
  [commentary-classifier](https://github.com/jeffWelling/commentary-classifier)
  (ffprobe metadata + loudness heuristics) to decide whether a title
  lacks commentary.
- Searches Prowlarr for likely replacements, scores candidates with a
  title-regex rubric, and hands the winner to your download client
  (qBittorrent, Transmission, Deluge, or rTorrent).
- Validates the downloaded file, re-classifies the new audio tracks,
  evaluates configurable CEL safety rules, and places the result into
  the library (replace, sidecar, or separate-library mode).
- Trashes originals with a 28-day restore window and webhook
  notifications.

Sibling projects:
- [commentary-classifier](https://github.com/jeffWelling/commentary-classifier) — shared classifier lib
- [commentary-detector](https://github.com/jeffWelling/commentary-detector) — interactive single-title web tool

## Status

**v1.0** — full pipeline end-to-end. 22 Go packages, React 19 + Vite
SPA embedded via `go:embed`, Helm chart, multi-arch Dockerfile.
Classifier benchmark on 139 titles: precision 0.98, recall 1.00,
F1 0.99.

The in-process daemon runs the trash-purge tick and the search loop
(when `-prowlarr-url` + `-prowlarr-api-key` are set). The download
watcher and auto-pick→auto-download chain ship next; until then,
`commentarr import` runs the post-download pipeline from cron.

## Build

```bash
go build ./cmd/commentarr
```

Requires a sibling checkout of [commentary-classifier](https://github.com/jeffWelling/commentary-classifier)
at `../commentary-classifier` (see the `replace` directive in `go.mod`).
The replace goes away once the classifier ships a tagged release.

For the web SPA:

```bash
cd web && npm ci && npm run build
cp -r dist ../cmd/commentarr/web-dist
go build ./cmd/commentarr
```

## Subcommands

| Subcommand | Purpose |
|---|---|
| `commentarr scan`   | Walk a filesystem library and queue titles lacking commentary. |
| `commentarr search` | Run the Prowlarr search loop against titles due for a re-check. |
| `commentarr import` | Run the post-download pipeline against a single file. |
| `commentarr serve`  | Start the HTTP + SSE daemon (UI, REST API, Prometheus `/metrics`). |

Each exposes its flags via `-h`. Full reference in [docs/CONFIGURATION.md](docs/CONFIGURATION.md).

## Quickstart — Docker

```bash
docker run -d --name commentarr \
  -p 7878:7878 \
  -v "$PWD/data:/data" \
  -v /path/to/media:/media:ro \
  ghcr.io/jeffwelling/commentarr:latest
```

First startup mints an API key and prints it to stderr once — save it.
Open http://localhost:7878 and paste the key at the first-run prompt.

See [docs/INSTALL.md](docs/INSTALL.md) for Docker, Helm, and bare-metal
details.

## Configuration

Startup flags cover listen address, database path, CIDR bypass, and
the connection cards shown in the UI. Libraries, indexers, download
clients, safety rules, and webhooks are edited at runtime through the
UI or REST API — adding an indexer doesn't require a pod restart.

See [docs/CONFIGURATION.md](docs/CONFIGURATION.md) and
[docs/SAFETY_RULES_REFERENCE.md](docs/SAFETY_RULES_REFERENCE.md).

## Testing

```bash
go test ./... -race -count=1
```

Testing discipline matches Commentarr's NFR-4 (hard merge gate): unit
and integration tests are written against requirements, not against
the implementation. No stubs, no decorative tests.

## License

GPL-3.0 — see [LICENSE](LICENSE).
