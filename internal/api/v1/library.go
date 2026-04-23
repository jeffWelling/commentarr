package v1

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jeffWelling/commentarr/internal/title"
)

// LibraryHandler exposes /api/v1/library endpoints.
type LibraryHandler struct {
	repo title.Repo
	r    *chi.Mux
}

// NewLibraryHandler returns a LibraryHandler.
func NewLibraryHandler(repo title.Repo) *LibraryHandler {
	h := &LibraryHandler{repo: repo, r: chi.NewRouter()}
	h.r.Get("/titles", h.listTitles)
	return h
}

// ServeHTTP routes sub-paths.
func (h *LibraryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.r.ServeHTTP(w, r) }

func (h *LibraryHandler) listTitles(w http.ResponseWriter, r *http.Request) {
	titles, err := h.repo.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"titles": titles})
}
