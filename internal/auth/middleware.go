package auth

import (
	"net"
	"net/http"
	"strings"
)

// MiddlewareConfig configures auth behaviour.
type MiddlewareConfig struct {
	// LocalBypassCIDRs is a list of CIDRs whose traffic is accepted
	// without a key. Empty = no bypass.
	LocalBypassCIDRs []string
}

// publicPaths bypass auth entirely (health + metrics).
var publicPaths = map[string]bool{
	"/healthz": true,
	"/readyz":  true,
	"/metrics": true,
}

// NewMiddleware returns a chi-compatible middleware enforcing API-key
// auth (via X-Api-Key header or ?apikey= query) with optional local-
// network bypass. Paths in publicPaths skip auth.
func NewMiddleware(repo *Repo, cfg MiddlewareConfig) func(http.Handler) http.Handler {
	nets := parseCIDRs(cfg.LocalBypassCIDRs)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if publicPaths[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}
			if ipBypass(r, nets) {
				next.ServeHTTP(w, r)
				return
			}
			key := extractKey(r)
			if key != "" && repo.ValidateAPIKey(r.Context(), key) {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("WWW-Authenticate", `X-Api-Key realm="commentarr"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		})
	}
}

func extractKey(r *http.Request) string {
	if v := r.Header.Get("X-Api-Key"); v != "" {
		return v
	}
	return r.URL.Query().Get("apikey")
}

func parseCIDRs(raw []string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(raw))
	for _, s := range raw {
		_, n, err := net.ParseCIDR(strings.TrimSpace(s))
		if err != nil {
			continue
		}
		out = append(out, n)
	}
	return out
}

func ipBypass(r *http.Request, nets []*net.IPNet) bool {
	if len(nets) == 0 {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
