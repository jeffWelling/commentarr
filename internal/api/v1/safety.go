package v1

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jeffWelling/commentarr/internal/safety"
)

// SafetyHandler exposes /api/v1/safety.
type SafetyHandler struct {
	repo *safety.ProfileRepo
	r    *chi.Mux
}

// NewSafetyHandler returns a SafetyHandler.
func NewSafetyHandler(repo *safety.ProfileRepo) *SafetyHandler {
	h := &SafetyHandler{repo: repo, r: chi.NewRouter()}
	h.r.Get("/rules", h.listRules)
	h.r.Post("/rules/validate", h.validateRule)
	h.r.Post("/rules", h.saveRule)
	h.r.Delete("/rules/{id}", h.deleteRule)
	return h
}

// listRules returns every stored rule.
func (h *SafetyHandler) listRules(w http.ResponseWriter, r *http.Request) {
	rules, err := h.repo.ListRules(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if rules == nil {
		rules = []safety.StoredRule{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"rules": rules})
}

// deleteRule removes a stored rule.
func (h *SafetyHandler) deleteRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.repo.DeleteRule(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *SafetyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.r.ServeHTTP(w, r) }

// validateRule compiles a CEL expression without persisting it. On
// success returns 204; on failure, the compile error as text/plain.
func (h *SafetyHandler) validateRule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Expression string `json:"expression"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if _, err := safety.CompileRule(req.Expression); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// saveRule upserts a stored rule.
func (h *SafetyHandler) saveRule(w http.ResponseWriter, r *http.Request) {
	var req safety.StoredRule
	if !readJSON(w, r, &req) {
		return
	}
	if _, err := safety.CompileRule(req.Expression); err != nil {
		http.Error(w, "invalid expression: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}
	if err := h.repo.SaveRule(r.Context(), req); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, req)
}
