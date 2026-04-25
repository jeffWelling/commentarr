package v1

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jeffWelling/commentarr/internal/webhook"
)

// WebhooksHandler exposes /api/v1/webhooks.
type WebhooksHandler struct {
	repo       *webhook.Repo
	dispatcher *webhook.Dispatcher
	r          *chi.Mux
}

// NewWebhooksHandler returns a WebhooksHandler.
func NewWebhooksHandler(repo *webhook.Repo, dispatcher *webhook.Dispatcher) *WebhooksHandler {
	h := &WebhooksHandler{repo: repo, dispatcher: dispatcher, r: chi.NewRouter()}
	h.r.Get("/", h.list)
	h.r.Post("/", h.save)
	h.r.Post("/test", h.test)
	h.r.Delete("/{id}", h.delete)
	return h
}

func (h *WebhooksHandler) list(w http.ResponseWriter, r *http.Request) {
	subs, err := h.repo.ListAll(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if subs == nil {
		subs = []webhook.Subscriber{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"webhooks": subs})
}

func (h *WebhooksHandler) delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.repo.Delete(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *WebhooksHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.r.ServeHTTP(w, r) }

func (h *WebhooksHandler) save(w http.ResponseWriter, r *http.Request) {
	var s webhook.Subscriber
	if !readJSON(w, r, &s) {
		return
	}
	if err := h.repo.SaveSubscriber(r.Context(), s); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, s)
}

func (h *WebhooksHandler) test(w http.ResponseWriter, r *http.Request) {
	if err := h.dispatcher.Dispatch(r.Context(), webhook.EventTest, map[string]interface{}{
		"hello": "world",
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
