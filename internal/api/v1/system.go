package v1

import (
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5"
)

// SystemInfo is the JSON shape served from /api/v1/system. Stable
// surface — anything the UI displays in its status chip lives here.
type SystemInfo struct {
	Version    string `json:"version"`
	Commit     string `json:"commit,omitempty"`
	BuiltAt    string `json:"built_at,omitempty"`
	GoVersion  string `json:"go_version,omitempty"`
	GOOS       string `json:"goos,omitempty"`
	GOARCH     string `json:"goarch,omitempty"`
	StartedAt  time.Time `json:"started_at"`
	UptimeSecs float64   `json:"uptime_secs"`
}

// SystemHandler exposes /api/v1/system. The version + start time are
// captured at construction (i.e., daemon startup) — not at request
// time — so the chip in the UI shows uptime since pod start, not
// since the last request.
type SystemHandler struct {
	version   string
	startedAt time.Time
	r         *chi.Mux
}

// NewSystemHandler returns a SystemHandler. version is the
// human-readable build version (e.g., "v0.2.0" or "dev"); pass
// time.Now() for startedAt at daemon startup.
func NewSystemHandler(version string, startedAt time.Time) *SystemHandler {
	h := &SystemHandler{version: version, startedAt: startedAt, r: chi.NewRouter()}
	h.r.Get("/", h.show)
	return h
}

func (h *SystemHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.r.ServeHTTP(w, r) }

func (h *SystemHandler) show(w http.ResponseWriter, r *http.Request) {
	info := SystemInfo{
		Version:    h.version,
		StartedAt:  h.startedAt,
		UptimeSecs: time.Since(h.startedAt).Seconds(),
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		info.GoVersion = bi.GoVersion
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				info.Commit = s.Value
			case "vcs.time":
				info.BuiltAt = s.Value
			case "GOOS":
				info.GOOS = s.Value
			case "GOARCH":
				info.GOARCH = s.Value
			}
		}
	}
	writeJSON(w, http.StatusOK, info)
}
