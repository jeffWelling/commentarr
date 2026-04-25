package v1

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jeffWelling/commentarr/internal/download"
)

// JobView is the JSON shape served from /api/v1/jobs. Field names use
// snake_case to match the rest of the API; ImportedAt is a pointer so
// it serializes as null when the job hasn't reached a terminal state.
type JobView struct {
	ID           int64      `json:"id"`
	ClientName   string     `json:"client_name"`
	ClientJobID  string     `json:"client_job_id"`
	TitleID      string     `json:"title_id"`
	ReleaseTitle string     `json:"release_title"`
	Edition      string     `json:"edition"`
	Status       string     `json:"status"`
	Outcome      string     `json:"outcome"`
	AddedAt      time.Time  `json:"added_at"`
	ImportedAt   *time.Time `json:"imported_at,omitempty"`
}

// JobsHandler exposes /api/v1/jobs.
type JobsHandler struct {
	repo *download.JobRepo
	r    *chi.Mux
}

// NewJobsHandler returns a JobsHandler.
func NewJobsHandler(repo *download.JobRepo) *JobsHandler {
	h := &JobsHandler{repo: repo, r: chi.NewRouter()}
	h.r.Get("/", h.list)
	return h
}

func (h *JobsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.r.ServeHTTP(w, r) }

func (h *JobsHandler) list(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"jobs": []JobView{}})
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	jobs, err := h.repo.ListRecent(r.Context(), limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := make([]JobView, 0, len(jobs))
	for _, j := range jobs {
		v := JobView{
			ID: j.ID, ClientName: j.ClientName, ClientJobID: j.ClientJobID,
			TitleID: j.TitleID, ReleaseTitle: j.ReleaseTitle, Edition: j.Edition,
			Status: j.Status, Outcome: j.Outcome, AddedAt: j.AddedAt,
		}
		if !j.ImportedAt.IsZero() {
			t := j.ImportedAt
			v.ImportedAt = &t
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"jobs": out})
}
