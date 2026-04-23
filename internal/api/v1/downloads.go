package v1

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// DownloadClientInfo describes one configured download client.
type DownloadClientInfo struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	BaseURL string `json:"base_url"`
	Enabled bool   `json:"enabled"`
}

// DownloadHandler exposes /api/v1/download-clients.
type DownloadHandler struct {
	list []DownloadClientInfo
	r    *chi.Mux
}

// NewDownloadHandler returns a DownloadHandler.
func NewDownloadHandler(list []DownloadClientInfo) *DownloadHandler {
	h := &DownloadHandler{list: list, r: chi.NewRouter()}
	h.r.Get("/", h.listAll)
	return h
}

func (h *DownloadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.r.ServeHTTP(w, r) }

func (h *DownloadHandler) listAll(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"clients": h.list})
}
