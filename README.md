# Commentarr

> ⚠️ **Work in progress — not ready for end users.** Commentarr is at
> roughly the "it ran end-to-end once on the author's homelab" stage.
> The code, schema, flags, container image, and chart values can all
> change between commits without notice. There's no upgrade path
> guarantee, no battle-testing in any second deployment, and several
> known gaps (see `docs/OPEN_QUESTIONS.md`-equivalent items in the
> issue tracker / project notes). If you're tempted to point this at a
> real library you care about, **don't** — or run with `-dry-run` and
> `-placement-mode=sidecar` so nothing destructive happens. Bug
> reports + design feedback welcome; install instructions are aimed at
> people who want to read the code, not at people who want a working
> tool today.

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

**Pre-alpha.** v0.1.0 was tagged so the container image has a name,
not because the project is ready to use. What exists today:

- Pipeline runs end-to-end against real services (homelab Prowlarr +
  qBittorrent + SMB-mounted media library). Classifier benchmark on
  the author's library: precision 0.98, recall 1.00, F1 0.99 across
  139 titles. **One** end-to-end real-world download has been
  performed (Brazil 1985 Criterion).
- 22 Go packages, ~250 unit + integration tests (race-clean), React
  SPA, Helm chart, multi-arch Dockerfile, GitHub Actions CI +
  release-on-tag workflow.

What this means in practice:

- **It hasn't been deployed by anyone but the author.** No second
  homelab has tried it. The Helm chart works against my K8s cluster
  and helm-lint passes; that's the entire deployment validation
  beyond `docker run`.
- **The data model isn't stable.** The DB migrations are versioned,
  but I'll happily reorder/replace them if it makes the schema
  cleaner. Don't expect to upgrade across releases without wiping
  state.
- **Flags + chart values are still moving.** A flag I shipped in
  v0.1.0 may be renamed in v0.1.1.
- **Several real-world gaps known** — slow classifier on
  remote-mounted storage (33 min for 9.2GB over SMB-over-VPN; ~Nx
  faster on local disk), modern blockbusters often have no
  commentary-tagged release in any indexer (Deadpool 2016: 3 of 177
  candidates had "Commentary" in the title), no integration with
  Radarr/Sonarr's first-time grab decision (Q8 in
  `~/claude/projects/commentarr/OPEN_QUESTIONS.md`).

When the daemon is configured (Prowlarr URL+key, qBit URL+creds), it
runs the full loop: search → pick top candidate → download → watch
for completion → validate → classify → safety → place → trash. Each
stage is independently togglable via flags or chart values; disabled
stages can run as `commentarr search` / `commentarr import` from
cron instead. **Always start with `-dry-run`** if you do try it
against real services — the dry-run mode logs what *would* be queued
without actually submitting torrents or moving files.

## Build

```bash
go build ./cmd/commentarr
```

Pulls [commentary-classifier](https://github.com/jeffWelling/commentary-classifier)
as a normal Go module (pinned in `go.mod`) — no sibling checkout
required.

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
| `commentarr version`| Print version + commit + Go version. |

Each exposes its flags via `-h`. Full reference in [docs/CONFIGURATION.md](docs/CONFIGURATION.md).

Two flags worth knowing about for first-deploy testing:

- **`-dry-run`** turns the daemon into a read-mostly observer:
  picker logs what it *would* queue, watcher polls without routing
  events to the importer. Smoke-test against real services before
  letting the real auto-pipeline take over.
- **`-path-translate-from` + `-path-translate-to`** rewrite qBit's
  save-path prefix so a daemon on one host can find files written
  by qBit on another (e.g., daemon on Mac with SMB mount of qBit's
  `/downloads` at `/Volumes/downloads`).

## Quickstart — Docker (try-it-out, not production)

```bash
docker run -d --name commentarr \
  -p 7878:7878 \
  -v "$PWD/data:/data" \
  -v /path/to/media:/media:ro \
  ghcr.io/jeffwelling/commentarr:latest \
  serve -dry-run \
    -prowlarr-url <…> -prowlarr-api-key <…> \
    -qbit-url <…> -qbit-username <…> -qbit-password <…>
```

`-dry-run` is the right mode for first contact: the daemon will
search, pick, and *log* what it would queue, without actually
submitting torrents or touching files. Drop the flag once you've
satisfied yourself it's picking sensible releases. **Don't drop the
flag against a library you care about** until you've also reviewed
the placement-mode + safety-rule defaults in
`docs/SAFETY_RULES_REFERENCE.md`.

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

For Radarr/Sonarr setups, you can prefer commentary-tagged releases
at *grab* time (instead of waiting for Commentarr to retrofit) — see
[docs/RADARR_FIRST_TIME_RIGHT.md](docs/RADARR_FIRST_TIME_RIGHT.md).

## Testing

```bash
go test ./... -race -count=1
```

Testing discipline matches Commentarr's NFR-4 (hard merge gate): unit
and integration tests are written against requirements, not against
the implementation. No stubs, no decorative tests.

## License

GPL-3.0 — see [LICENSE](LICENSE).
