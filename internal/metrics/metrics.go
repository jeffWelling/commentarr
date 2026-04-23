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
)
