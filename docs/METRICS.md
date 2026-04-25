# Commentarr Metrics

Every Commentarr metric is exposed at `GET /metrics` in Prometheus
text format. The endpoint bypasses authentication so a Prometheus
scrape (or kube-prometheus ServiceMonitor) can pull metrics without
an API key.

Metric names start with `commentarr_`. Labels stay low-cardinality
intentionally — never `title_id`, never `release_title`, never URL
paths beyond the chi route pattern.

## Library + classifier

| Metric | Type | Labels | What it measures |
|---|---|---|---|
| `commentarr_classifications_total` | counter | `library`, `result` | Classifier invocations. `result` ∈ {has_commentary, no_commentary, error}. |
| `commentarr_classification_duration_seconds` | histogram | `library` | Per-file classifier wall time. Buckets cover 0.1s → 2min. |
| `commentarr_library_items_total` | gauge | `library`, `kind` | Titles known to Commentarr, by library and kind (movie / episode). |
| `commentarr_library_items_wanted_total` | gauge | `library`, `kind` | Titles currently in the wanted queue. |

## Indexer (Prowlarr) loop

| Metric | Type | Labels | What it measures |
|---|---|---|---|
| `commentarr_indexer_queries_total` | counter | `indexer`, `result` | Queries attempted. `result` ∈ {success, rate_limited, server_error, timeout, circuit_open, other}. |
| `commentarr_indexer_queries_rejected_by_server_total` | counter | `indexer`, `status_code` | 4xx/5xx responses. |
| `commentarr_indexer_query_duration_seconds` | histogram | `indexer` | Wall time per query. Buckets 0.1s → 1min. |
| `commentarr_indexer_circuit_state` | gauge | `indexer` | 0 = closed, 1 = open, 2 = half-open. |

## Picker (auto-pick) loop

| Metric | Type | Labels | What it measures |
|---|---|---|---|
| `commentarr_picker_decisions_total` | counter | `decision` | What the picker did per evaluated title. `decision` ∈ {queued, skipped_inflight, skipped_no_candidate, error}. |

Reading the four series together answers "is the picker doing nothing
because there's nothing to pick, or because everything is already in
flight?"

## Download client

| Metric | Type | Labels | What it measures |
|---|---|---|---|
| `commentarr_downloads_queued_total` | counter | `client` | `Add()` calls accepted by the client. |
| `commentarr_downloads_completed_total` | counter | `client`, `result` | Terminal states observed by the watcher. `result` ∈ {imported, failed, abandoned}. |
| `commentarr_download_duration_seconds` | histogram | `client` | Wall time from `Add()` to terminal state. Buckets 30s → 24h. |

## Watcher + auto-import routing

| Metric | Type | Labels | What it measures |
|---|---|---|---|
| `commentarr_watcher_events_total` | counter | `client`, `kind` | Events drained by the importer consumer. `kind` ∈ {completed, error}. |
| `commentarr_auto_import_routing_errors_total` | counter | `reason` | Watcher events that failed to route. `reason` ∈ {job_not_found, title_not_found, no_main_video, import_error}. These never reach `commentarr_imports_total`. |

## Importer + post-download pipeline

| Metric | Type | Labels | What it measures |
|---|---|---|---|
| `commentarr_imports_total` | counter | `library`, `mode`, `result` | Import attempts. `mode` ∈ {sidecar, replace, separate-library}. `result` ∈ {success, safety_violation, non_compliant, error}. |
| `commentarr_import_duration_seconds` | histogram | `library`, `mode` | Post-download pipeline wall time. |
| `commentarr_replaces_total` | counter | `library`, `result` | Replace attempts specifically. |
| `commentarr_safety_violations_total` | counter | `rule_name` | Violations by rule. Includes built-in rule names + user CEL rule names. |
| `commentarr_non_compliant_files_total` | counter | `reason`, `extension` | Files rejected during validation. |

## Trash

| Metric | Type | Labels | What it measures |
|---|---|---|---|
| `commentarr_trash_items` | gauge | `library` | Current count in trash. |
| `commentarr_trash_size_bytes` | gauge | `library` | Total trash bytes. |
| `commentarr_trash_items_purged_total` | counter | `library` | Purges (auto + manual). |
| `commentarr_trash_items_restored_total` | counter | `library` | Restores. |

## Webhooks

| Metric | Type | Labels | What it measures |
|---|---|---|---|
| `commentarr_webhook_deliveries_total` | counter | `event`, `result` | Deliveries attempted. `result` ∈ {success, retried, failed}. |
| `commentarr_webhook_delivery_duration_seconds` | histogram | `event` | Per-delivery HTTP wall time. |
| `commentarr_webhook_queue_depth` | gauge | — | Pending deliveries. |

## HTTP server (the API itself)

| Metric | Type | Labels | What it measures |
|---|---|---|---|
| `commentarr_http_requests_total` | counter | `method`, `route`, `status` | Every request received. `route` is the chi route pattern (low cardinality), not the raw URL. |
| `commentarr_http_request_duration_seconds` | histogram | `method`, `route` | Server-handler wall time. |
| `commentarr_http_requests_in_flight` | gauge | — | Current concurrency. |

## Recipes

**Queue depth over time:**

```promql
commentarr_library_items_wanted_total
```

**Picker effectiveness — what fraction of evaluations result in a
queued download?**

```promql
sum(rate(commentarr_picker_decisions_total{decision="queued"}[15m]))
  /
sum(rate(commentarr_picker_decisions_total[15m]))
```

**End-to-end success rate:**

```promql
sum(rate(commentarr_imports_total{result="success"}[1h]))
  /
sum(rate(commentarr_imports_total[1h]))
```

**Where are titles getting stuck?**

```promql
sum by (decision) (rate(commentarr_picker_decisions_total[15m]))
```

```promql
sum by (result) (rate(commentarr_imports_total[1h]))
```

```promql
sum by (reason) (rate(commentarr_auto_import_routing_errors_total[1h]))
```

**Indexer health:**

```promql
commentarr_indexer_circuit_state
```

```promql
sum by (result) (rate(commentarr_indexer_queries_total[15m]))
```

## kube-prometheus integration

The Helm chart ships an optional `ServiceMonitor`. Enable it with
`metrics.serviceMonitor.enabled=true` and (optionally)
`metrics.serviceMonitor.namespace` to point at a non-default
namespace.

```yaml
metrics:
  serviceMonitor:
    enabled: true
    interval: 30s
```

## Cardinality notes

- All current metric labels are bounded sets ({success, error, …},
  client name from a small list, library name configured at startup).
  No `title_id`, no `release_title`, no full URL paths.
- The `route` label on HTTP metrics uses chi's route patterns
  (`/api/v1/wanted/{title_id}` not `/api/v1/wanted/tt-12345`), so
  cardinality is bounded by the API surface, not by traffic.
