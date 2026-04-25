// Package metrics registers Commentarr's Prometheus metrics. Naming +
// cardinality rules per METRICS.md. Only the subset needed by Plan 1 is
// registered here; later plans add their metrics to this package.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// ClassificationsTotal counts classifier calls by library + result.
	ClassificationsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "commentarr_classifications_total",
		Help: "Total classifier invocations, partitioned by library and outcome.",
	}, []string{"library", "result"})

	// ClassificationDurationSeconds measures per-file classification time.
	ClassificationDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "commentarr_classification_duration_seconds",
		Help:    "Per-file classification duration.",
		Buckets: []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120},
	}, []string{"library"})

	// LibraryItemsTotal is the current count of titles per library+kind.
	LibraryItemsTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "commentarr_library_items_total",
		Help: "Titles known to Commentarr, by library and kind.",
	}, []string{"library", "kind"})

	// LibraryItemsWantedTotal is the wanted-queue depth per library+kind.
	LibraryItemsWantedTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "commentarr_library_items_wanted_total",
		Help: "Titles currently in the wanted queue, by library and kind.",
	}, []string{"library", "kind"})

	// IndexerQueriesTotal counts every indexer query by result.
	// result ∈ {success, rate_limited, server_error, timeout, circuit_open, other}
	IndexerQueriesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "commentarr_indexer_queries_total",
		Help: "Indexer queries attempted, partitioned by indexer and result.",
	}, []string{"indexer", "result"})

	// IndexerQueriesRejectedByServerTotal counts 4xx/5xx from indexers.
	IndexerQueriesRejectedByServerTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "commentarr_indexer_queries_rejected_by_server_total",
		Help: "Indexer queries rejected with an HTTP error, partitioned by indexer and status code.",
	}, []string{"indexer", "status_code"})

	// IndexerQueryDurationSeconds measures wall-clock time per query.
	IndexerQueryDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "commentarr_indexer_query_duration_seconds",
		Help:    "Indexer query wall time, in seconds.",
		Buckets: []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60},
	}, []string{"indexer"})

	// IndexerCircuitState exposes the current circuit state per indexer.
	// 0=closed, 1=open, 2=half-open.
	IndexerCircuitState = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "commentarr_indexer_circuit_state",
		Help: "Current circuit-breaker state: 0=closed, 1=open, 2=half-open.",
	}, []string{"indexer"})

	// DownloadsQueuedTotal counts Add() calls.
	DownloadsQueuedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "commentarr_downloads_queued_total",
		Help: "Downloads enqueued with the client.",
	}, []string{"client"})

	// DownloadsCompletedTotal counts observed terminal states.
	// result ∈ {imported, failed, abandoned}
	DownloadsCompletedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "commentarr_downloads_completed_total",
		Help: "Downloads that reached a terminal state, by client and result.",
	}, []string{"client", "result"})

	// DownloadDurationSeconds is the grab→terminal duration per client.
	DownloadDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "commentarr_download_duration_seconds",
		Help:    "Wall time from Add() to terminal status, per client.",
		Buckets: []float64{30, 120, 600, 1800, 3600, 14400, 86400},
	}, []string{"client"})

	// TrashItems is the current count of items in trash per library.
	TrashItems = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "commentarr_trash_items",
		Help: "Current count of items in trash, partitioned by library.",
	}, []string{"library"})

	// TrashSizeBytes is the total size of items in trash per library.
	TrashSizeBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "commentarr_trash_size_bytes",
		Help: "Total size of items in trash, bytes.",
	}, []string{"library"})

	// TrashItemsPurgedTotal counts purges (age-out, manual delete).
	TrashItemsPurgedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "commentarr_trash_items_purged_total",
		Help: "Total trash items purged, partitioned by library.",
	}, []string{"library"})

	// TrashItemsRestoredTotal counts restores.
	TrashItemsRestoredTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "commentarr_trash_items_restored_total",
		Help: "Total trash items restored, partitioned by library.",
	}, []string{"library"})

	// WebhookDeliveriesTotal counts deliveries by event and result.
	WebhookDeliveriesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "commentarr_webhook_deliveries_total",
		Help: "Total webhook deliveries attempted.",
	}, []string{"event", "result"})

	// WebhookDeliveryDurationSeconds is per-delivery HTTP time.
	WebhookDeliveryDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "commentarr_webhook_delivery_duration_seconds",
		Help:    "Webhook HTTP POST wall time, seconds.",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	}, []string{"event"})

	// WebhookQueueDepth is the pending delivery count.
	WebhookQueueDepth = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "commentarr_webhook_queue_depth",
		Help: "Pending webhook deliveries waiting to fire.",
	})

	// ImportsTotal counts completed imports by library/mode/result.
	ImportsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "commentarr_imports_total",
		Help: "Imports attempted, partitioned by library, mode, and result.",
	}, []string{"library", "mode", "result"})

	// ImportDurationSeconds measures post-download import wall time.
	ImportDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "commentarr_import_duration_seconds",
		Help:    "Import pipeline wall time, seconds.",
		Buckets: []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60},
	}, []string{"library", "mode"})

	// ReplacesTotal counts replace attempts by outcome.
	ReplacesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "commentarr_replaces_total",
		Help: "Replace attempts, partitioned by library and result.",
	}, []string{"library", "result"})

	// SafetyViolationsTotal counts violations by rule.
	SafetyViolationsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "commentarr_safety_violations_total",
		Help: "Safety rule violations, partitioned by rule name.",
	}, []string{"rule_name"})

	// SafetyRuleEvaluationsTotal counts CEL rule evaluations.
	// result ∈ {pass, fail}
	SafetyRuleEvaluationsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "commentarr_safety_rule_evaluations_total",
		Help: "CEL safety rule evaluations.",
	}, []string{"rule", "result"})

	// SafetyCompileErrorsTotal counts CEL rules that fail to compile
	// (typically at startup when ProfileRepo loads them).
	SafetyCompileErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "commentarr_safety_compile_errors_total",
		Help: "CEL safety rules that failed to compile.",
	}, []string{"rule"})

	// NonCompliantFilesTotal counts files that tripped validation.
	NonCompliantFilesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "commentarr_non_compliant_files_total",
		Help: "Files rejected during validation, partitioned by reason and extension.",
	}, []string{"reason", "extension"})

	// HTTPRequestsTotal counts every request received.
	HTTPRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "commentarr_http_requests_total",
		Help: "HTTP requests received.",
	}, []string{"method", "route", "status"})

	// HTTPRequestDurationSeconds is the server-handler wall time.
	HTTPRequestDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "commentarr_http_request_duration_seconds",
		Help:    "HTTP request-handling wall time, seconds.",
		Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
	}, []string{"method", "route"})

	// HTTPRequestsInFlight is the current request concurrency.
	HTTPRequestsInFlight = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "commentarr_http_requests_in_flight",
		Help: "HTTP requests currently being served.",
	})

	// PickerDecisionsTotal counts what the picker did per evaluated
	// title, by decision.
	// decision ∈ {queued, skipped_inflight, skipped_no_candidate, error}
	PickerDecisionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "commentarr_picker_decisions_total",
		Help: "Picker outcomes per evaluated title.",
	}, []string{"decision"})

	// WatcherEventsTotal counts events emitted by the in-process
	// watcher and routed by the importer consumer.
	// kind ∈ {completed, error}
	WatcherEventsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "commentarr_watcher_events_total",
		Help: "Watcher events processed by the importer consumer.",
	}, []string{"client", "kind"})

	// AutoImportRoutingErrorsTotal counts cases where the consumer
	// couldn't even attempt an import (no matching job row, no main
	// video file under SavePath, etc) — these never reach ImportsTotal.
	// reason ∈ {job_not_found, title_not_found, no_main_video, import_error}
	AutoImportRoutingErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "commentarr_auto_import_routing_errors_total",
		Help: "Watcher events that failed to route into the importer.",
	}, []string{"reason"})
)
