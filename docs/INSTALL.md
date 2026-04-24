# Installing Commentarr

Commentarr ships as a single Go binary with a React SPA embedded into it.
There's nothing else to run alongside it — the SQLite database lives on
disk, and connections to Plex / Jellyfin / Prowlarr / your download client
are configured at runtime through the UI or REST API.

## Requirements

- A filesystem or a media server (Plex, Jellyfin, Emby) containing the
  library you want to expand with commentaries.
- At least one Prowlarr instance reachable from the Commentarr pod/host.
- A supported download client: qBittorrent, Transmission, Deluge, or
  rTorrent (XMLRPC over HTTP).
- Optional: `ffmpeg` / `ffprobe` on PATH if you plan to run the
  classifier on audio directly (the default Docker image ships with both).

## Quickstart — Docker

```sh
mkdir -p ./data
docker run -d --name commentarr \
  -p 7878:7878 \
  -v "$PWD/data:/data" \
  -v /path/to/media:/media:ro \
  ghcr.io/jeffwelling/commentarr:latest \
  serve -addr :7878 -db /data/commentarr.db -migrations /migrations
```

The very first startup mints an API key and prints it to stderr:

```
=== first-run: API key minted ===
X-Api-Key: cm-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
(this is the only time the key is printed; save it now)
```

Save that value immediately — Commentarr only stores the hash, so there's
no way to recover it later.

Point your browser at `http://localhost:7878`, paste the API key into the
first-run prompt, then configure the admin account and the service
connections.

## Quickstart — Helm

```sh
helm upgrade --install commentarr ./deploy/helm/commentarr \
  --namespace commentarr --create-namespace \
  --set auth.adminPassword=$(openssl rand -base64 24) \
  --set mediaLibrary.type=pvc \
  --set mediaLibrary.existingClaim=my-media-pvc
```

See [`CONFIGURATION.md`](CONFIGURATION.md) for the full value reference.

> **Note:** The Helm chart only bootstraps the admin account and
> process-level settings. Service connections (Plex, Prowlarr, download
> client) are configured through the UI/API after install — that's by
> design: adding a Prowlarr doesn't require a pod restart.

## Bare-metal install

```sh
# Build
git clone https://github.com/jeffWelling/commentarr.git
cd commentarr/web && npm ci && npm run build && cd ..
cp -r web/dist cmd/commentarr/web-dist
go build -o commentarr ./cmd/commentarr

# Run
./commentarr serve \
  -addr :7878 \
  -db ./commentarr.db \
  -migrations ./migrations
```

If you prefer to run behind a reverse proxy, bind to `127.0.0.1:7878` and
front the process with caddy / nginx / traefik. Commentarr expects the
proxy to forward `X-Forwarded-For`; CIDR bypass (`-local-bypass-cidr`)
evaluates the direct socket peer, so only use it on trusted networks.

## Subcommands

Commentarr exposes a few one-shot subcommands useful for debugging and
cron-driven workflows. The long-running daemon is `serve`.

| Subcommand | Purpose |
|---|---|
| `scan`   | Walk a filesystem library and queue titles that lack commentary. |
| `search` | Run the Prowlarr search loop against titles that are due. |
| `import` | Run the post-download pipeline against a single file. |
| `serve`  | Start the HTTP + SSE daemon. |

Each subcommand's flags are listed by running it with `-h`.

## Upgrading

Each release is a drop-in binary swap. Migrations run on startup; the
SQLite database is append-only for schema purposes — downgrades are not
supported. Back the DB up before upgrading across major versions:

```sh
cp /data/commentarr.db /data/commentarr-$(date +%Y%m%d).db.bak
```

## Uninstalling

```sh
# Docker
docker rm -f commentarr

# Helm
helm uninstall commentarr -n commentarr

# bare metal
systemctl disable --now commentarr
rm /usr/local/bin/commentarr
```

Uninstall **does not** remove the data directory — if you want to start
fresh, delete `/data/commentarr.db*`.
