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
)
