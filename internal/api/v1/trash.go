package v1

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jeffWelling/commentarr/internal/trash"
)

// TrashHandler exposes /api/v1/trash.
type TrashHandler struct {
	repo *trash.Repo
	r    *chi.Mux
}

// NewTrashHandler returns a TrashHandler.
func NewTrashHandler(repo *trash.Repo) *TrashHandler {
	h := &TrashHandler{repo: repo, r: chi.NewRouter()}
	h.r.Get("/", h.list)
	h.r.Delete("/{id}", h.delete)
	return h
}

func (h *TrashHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.r.ServeHTTP(w, r) }

func (h *TrashHandler) list(w http.ResponseWriter, r *http.Request) {
	library := r.URL.Query().Get("library")
	if library == "" {
		http.Error(w, "library query param required", http.StatusBadRequest)
		return
	}
	items, err := h.repo.ListByLibrary(r.Context(), library)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": items})
}

func (h *TrashHandler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := h.repo.MarkPurged(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
