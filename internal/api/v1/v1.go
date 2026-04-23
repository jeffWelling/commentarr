// Package v1 holds the REST handlers for /api/v1/*. Each resource
// exposes a Handler that satisfies http.Handler; cmd/commentarr/serve
// wires them together under the auth middleware.
package v1

import (
	"encoding/json"
	"net/http"
)

// writeJSON is shared by every handler.
func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// readJSON decodes req.Body into dst, returning a bad-request error
// string on failure (already written to w).
func readJSON(w http.ResponseWriter, r *http.Request, dst interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}
