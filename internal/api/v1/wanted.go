package v1

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jeffWelling/commentarr/internal/queue"
	"github.com/jeffWelling/commentarr/internal/search"
)

// WantedHandler exposes /api/v1/wanted endpoints.
type WantedHandler struct {
	q        *queue.Queue
	cand     *search.Repo
	r        *chi.Mux
}

// NewWantedHandler returns a WantedHandler.
func NewWantedHandler(q *queue.Queue, cand *search.Repo) *WantedHandler {
	h := &WantedHandler{q: q, cand: cand, r: chi.NewRouter()}
	h.r.Get("/", h.list)
	h.r.Post("/{id}/skip", h.skip)
	return h
}

func (h *WantedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.r.ServeHTTP(w, r) }

// list returns every wanted title with its candidates.
func (h *WantedHandler) list(w http.ResponseWriter, r *http.Request) {
	entries, err := h.q.ListByStatus(r.Context(), queue.StatusWanted)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	type entryView struct {
		TitleID    string               `json:"title_id"`
		Candidates []search.Candidate   `json:"candidates"`
		Misses     int                  `json:"search_misses"`
	}
	out := make([]entryView, 0, len(entries))
	for _, e := range entries {
		cands, _ := h.cand.ListCandidates(r.Context(), e.TitleID)
		out = append(out, entryView{TitleID: e.TitleID, Candidates: cands, Misses: e.SearchMisses})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"wanted": out})
}

func (h *WantedHandler) skip(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.q.MarkSkipped(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
