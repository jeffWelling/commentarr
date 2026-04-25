package v1

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// IndexerInfo is the snapshot returned by GET /indexers. Richer status
// (rate-limit tokens, circuit state) is available via Prometheus —
// see commentarr_indexer_circuit_state and the indexer_queries_total
// series in docs/METRICS.md.
type IndexerInfo struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	BaseURL string `json:"base_url"`
	Enabled bool   `json:"enabled"`
}

// IndexerHandler exposes /api/v1/indexers as a read-only list of the
// statically-configured indexers serve was launched with. CRUD lives
// behind a future DB-backed registry.
type IndexerHandler struct {
	list []IndexerInfo
	r    *chi.Mux
}

// NewIndexerHandler returns an IndexerHandler.
func NewIndexerHandler(list []IndexerInfo) *IndexerHandler {
	h := &IndexerHandler{list: list, r: chi.NewRouter()}
	h.r.Get("/", h.listAll)
	return h
}

func (h *IndexerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.r.ServeHTTP(w, r) }

func (h *IndexerHandler) listAll(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"indexers": h.list})
}
