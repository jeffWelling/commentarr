package main

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// spaFS holds the built React SPA. The build artifact lives at
// web/dist; `npm run build` in the web/ directory produces it. If the
// directory is empty (developer hasn't run build), spaFS still embeds —
// it just serves no meaningful content.
//
//go:embed all:web-dist
var spaFS embed.FS

// spaFSSub returns the embedded SPA rooted at web-dist (so URL paths
// map 1:1 to asset files).
func spaFSSub() fs.FS {
	sub, err := fs.Sub(spaFS, "web-dist")
	if err != nil {
		return spaFS
	}
	return sub
}

// embeddedSPAHandler serves the embedded SPA for any path that doesn't
// match an /api or /healthz route. Unknown paths fall back to
// index.html so client-side routing works on hard refresh.
func embeddedSPAHandler() http.Handler {
	sub := spaFSSub()
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If the requested path exists in the embed, serve it as-is
		// (assets). Otherwise rewrite to index.html for SPA routing.
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(sub, p); err != nil {
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}
