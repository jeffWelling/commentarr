package v1

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jeffWelling/commentarr/internal/download"
	"github.com/jeffWelling/commentarr/internal/queue"
	"github.com/jeffWelling/commentarr/internal/search"
	"github.com/jeffWelling/commentarr/internal/upgrade"
)

// UpgradesHandler exposes /api/v1/upgrades. Computes the upgrade list
// on demand by walking every resolved title and calling
// upgrade.Find — no separate cache or "upgrades" table. The cost is
// O(resolved-titles) DB queries per request, which is fine until
// resolved-set sizes get into the tens of thousands.
type UpgradesHandler struct {
	queue     *queue.Queue
	cands     *search.Repo
	jobs      *download.JobRepo
	threshold int
	r         *chi.Mux
}

// NewUpgradesHandler returns an UpgradesHandler. threshold mirrors
// the picker's score-threshold so the API uses the same "qualifies"
// rule the daemon does.
func NewUpgradesHandler(q *queue.Queue, cands *search.Repo, jobs *download.JobRepo, threshold int) *UpgradesHandler {
	h := &UpgradesHandler{
		queue: q, cands: cands, jobs: jobs, threshold: threshold,
		r: chi.NewRouter(),
	}
	h.r.Get("/", h.list)
	return h
}

func (h *UpgradesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.r.ServeHTTP(w, r) }

func (h *UpgradesHandler) list(w http.ResponseWriter, r *http.Request) {
	resolved, err := h.queue.ListByStatus(r.Context(), queue.StatusResolved)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	ids := make([]string, 0, len(resolved))
	for _, e := range resolved {
		ids = append(ids, e.TitleID)
	}
	upgrades, err := upgrade.Find(r.Context(), h.cands, h.jobs, ids, h.threshold)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if upgrades == nil {
		upgrades = []upgrade.Info{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"upgrades": upgrades})
}
