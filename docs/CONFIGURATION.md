# Configuring Commentarr

Commentarr is configured in two layers:

1. **Startup flags** on the `commentarr serve` command (listen address,
   database path, CIDR bypass). These never change at runtime.
2. **Runtime settings** stored in the database and edited through the UI
   or the REST API (libraries, indexers, download clients, safety rules,
   webhooks). These take effect immediately.

This split is deliberate: you should be able to edit a safety rule
without a pod restart, but the DB DSN shouldn't change out from under
you.

## Startup flags

```
commentarr serve
  -addr              HTTP listen address            default ":7878"
  -db                SQLite DSN                     default "commentarr.db"
  -migrations        migrations directory           default "./migrations"
  -local-bypass-cidr CIDR that bypasses auth        default ""  (disabled)
  -initial-key-label label for the first API key    default "default"
  -prowlarr-url      Prowlarr base URL              default ""  (no card shown)
  -prowlarr-api-key  Prowlarr API key               default ""  (search loop disabled without it)
  -prowlarr-name     Prowlarr instance label        default "prowlarr"
  -prowlarr-rpm      Prowlarr requests-per-minute   default 6
  -prowlarr-burst    Prowlarr token-bucket burst    default 3
  -search-interval   in-process search-loop period  default 15m  (0 disables)
  -score-threshold   likely-commentary score gate   default 8
  -qbit-url          qBittorrent base URL           default ""  (no card shown)
  -qbit-username     qBittorrent Web UI username    default ""  (watcher disabled without it)
  -qbit-password     qBittorrent Web UI password    default ""
  -qbit-name         qBittorrent instance label     default "qbittorrent"
  -watch-interval    in-process watcher poll period default 30s  (0 disables)
  -watch-category    category/label to watch        default "commentarr"
  -picker-interval   auto-pick loop period          default 5m   (0 disables)
  -placement-mode    sidecar | replace | separate-library  default "sidecar"
  -placement-template auto-import filename template default "{title} ({year}) - {edition}.{ext}"
  -placement-separate-root  alt library root        default ""   (required for separate-library)
  -placement-trash-dir      trash directory         default ""   (required for replace)
  -confidence-min    auto-import classifier gate    default 0.85
```

The in-process pipeline runs end-to-end when **all three** config
groups are set:

1. **Search** — `-prowlarr-url` + `-prowlarr-api-key`. Every
   `-search-interval` the daemon walks the wanted queue for titles
   whose `next_search_at` has elapsed, queries Prowlarr, and persists
   scored candidates.
2. **Pick** — `-qbit-url` + `-qbit-username` + `-qbit-password` (the
   download client is what makes the picker useful). Every
   `-picker-interval` the picker walks every wanted title, finds the
   top likely-commentary candidate above `-score-threshold`, and
   submits it to the download client. A title with an existing
   in-flight download job is skipped — only failed jobs allow retry.
3. **Watch + import** — same qBit credentials. Every
   `-watch-interval` the watcher polls qBit for completions tagged
   with `-watch-category`. Each completion is matched back to its
   `download_jobs` row, the largest video file under the SavePath is
   located, and the importer runs the full pipeline (validate →
   classify → safety → place → trash → webhook). The job row is then
   marked `imported` or `error`.

Disable any individual stage by leaving its credentials empty or
setting its interval to 0. Disabled stages can be replaced with the
matching CLI subcommands (`commentarr search`, `commentarr import`)
running from cron.

Notes:

- `-db` accepts any modernc.org/sqlite DSN. `:memory:` is for tests only
  — Commentarr pins `MaxOpenConns=1` but the WAL journal won't persist.
- `-local-bypass-cidr` is evaluated against the direct socket peer.
  Use it when you're running behind a reverse proxy that lives on
  loopback, or when exposing the daemon only on a home LAN.
- `-migrations` must be reachable from the running process. In the
  Docker image this is `/migrations`; in a Helm install it's baked into
  the container at the same path.

## Runtime configuration

### Libraries

Commentarr supports four library backends:

| Kind | Source | Auth |
|---|---|---|
| `filesystem` | walks a root directory | N/A |
| `plex`       | `/library/sections` XML | `X-Plex-Token` |
| `jellyfin`   | `/Items` JSON          | `X-Emby-Token` (API key) |
| `emby`       | same as Jellyfin       | `X-Emby-Token` (API key) |

Each library has a **mode**: `scan-only` (discover titles but do nothing
else), or `full-pipeline` (scan → search → import). Modes can be
overridden per-library.

### Indexers

One or more Prowlarr instances can be registered. Commentarr talks to
each through `/api/v1/search`, rate-limited by a per-instance token
bucket (default 6 requests/minute, burst 3). A circuit breaker trips
after 5 consecutive failures and stays open for an hour.

### Download clients

Register one of qBittorrent, Transmission, Deluge, or rTorrent. Each
client has a category/label that Commentarr uses to track jobs it
enqueued — by default this is `commentarr`.

| Client | Transport | Notes |
|---|---|---|
| qBittorrent  | Web API (`/api/v2`) + cookie session | set Web UI password |
| Transmission | JSON-RPC + `X-Transmission-Session-Id` handshake | no auth required if on trusted LAN |
| Deluge       | JSON-RPC via Web UI (`/json`)       | requires Web UI password + `label` plugin for categories |
| rTorrent     | XMLRPC over HTTP                    | typically via ruTorrent's `/RPC2` endpoint |

### Placement

Three modes decide what happens once a download finishes:

- `replace` — new file takes the place of the original, original goes
  to trash with a 28-day TTL.
- `sidecar` — new file lives alongside the original, same directory,
  renamed via the configured template.
- `separate-library` — new file goes under a different root entirely,
  preserving the original layout. Useful when the original library is
  mounted read-only.

The default filename template is:

```
{title} ({year}) - {edition}.{ext}
```

Editable tokens: `{title}`, `{year}`, `{edition}`, `{ext}`, `{resolution}`.

### Safety rules

See [`SAFETY_RULES_REFERENCE.md`](SAFETY_RULES_REFERENCE.md).

### Webhooks

Register URLs to receive events. Dispatcher retries failed deliveries
with exponential backoff. Naming follows the *arr baseline (`OnX`):

| Event | Fires when |
|---|---|
| `OnSearch`          | A search pass runs against an indexer. |
| `OnGrab`            | The picker hands a release to the download client. |
| `OnDownload`        | A download reaches a terminal state. |
| `OnImport`          | The importer places a file. |
| `OnReplace`         | A replace-mode import swaps an original out. |
| `OnTrash`           | A file is moved to trash. |
| `OnTrashExpire`     | The auto-purge ticker removes an aged trash item. |
| `OnRestore`         | A trash item is restored to the library. |
| `OnVerifyFail`      | Title-regex verification rejects a release. |
| `OnSafetyViolation` | A safety rule fails (built-in or CEL). |
| `OnHealthIssue`     | Future: degraded subsystem (indexer circuit open, etc). |
| `OnTest`            | Manually fired from the UI to verify a webhook. |

Every payload uses the same envelope:

```json
{
  "event_type": "OnImport",
  "timestamp": "2026-04-25T16:30:00Z",
  "version": "1",
  "payload": { ... }
}
```

The `payload` shape varies per event. `version` is `"1"` today;
breaking changes bump it.

## Helm values (summary)

The Helm chart covers process-level concerns only: admin bootstrap,
listen address, CIDR bypass, persistence, and deployment plumbing.
Service connections (Plex / Jellyfin / Prowlarr / download clients)
are deliberately **not** in values.yaml — they're configured at runtime
through the API. See
[`deploy/helm/commentarr/values.yaml`](../deploy/helm/commentarr/values.yaml)
for the full set; key groups:

- `auth.*` — admin username/password (or `existingSecret`). Provisions
  the admin row on first startup via `COMMENTARR_ADMIN_USERNAME` +
  `COMMENTARR_ADMIN_PASSWORD`.
- `localBypassCIDR` — a single CIDR that bypasses API-key auth.
- `connections.prowlarr.*` / `connections.qbittorrent.*` — optional
  baseURL + label for the read-only connection cards in the UI.
- `persistence.*` — data PVC size and storage class.
- `mediaLibrary.*` — how the media mount is sourced (emptyDir / PVC / hostPath).
- `ingress.*` — optional Ingress with annotation pluggability.
- `metrics.serviceMonitor.*` — optional kube-prometheus-stack integration.

## Metrics

Commentarr exposes Prometheus metrics at `/metrics`. The full catalogue
with promql recipes lives in [`docs/METRICS.md`](METRICS.md). Notable
counters:

| Metric | Labels | Meaning |
|---|---|---|
| `commentarr_titles_scanned_total`       | `library` | Titles seen during a scan. |
| `commentarr_search_runs_total`          | `outcome` | Search attempts. |
| `commentarr_downloads_queued_total`     | `client`  | Add calls issued to a download client. |
| `commentarr_imports_total`              | `outcome` | Import pipeline results. |
| `commentarr_safety_rule_evaluations_total` | `rule`, `result` | CEL rule decisions. |
| `commentarr_webhook_deliveries_total`   | `event`, `outcome` | Webhook dispatch. |
