package server

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// webDist holds the built React SPA static files.
// The go:embed directive is commented out until the web UI is built.
// To enable: run `cd web && npm install && npm run build` first.
//
//go:embed all:web_dist
var webDistFS embed.FS

// RegisterSPA adds the SPA file server to the router.
// Falls back to index.html for client-side routing.
func (s *Server) RegisterSPA() error {
	dist, err := fs.Sub(webDistFS, "web_dist")
	if err != nil {
		return err
	}

	fileServer := http.FileServer(http.FS(dist))

	s.router.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		// Serve static files; fall back to index.html for SPA routes
		path := r.URL.Path
		if path == "/" || strings.HasPrefix(path, "/api") {
			if strings.HasPrefix(path, "/api") {
				http.NotFound(w, r)
				return
			}
			r.URL.Path = "/index.html"
		}
		fileServer.ServeHTTP(w, r)
	})

	return nil
}
