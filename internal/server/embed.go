package server

import (
	"embed"
	"io/fs"
	"net/http"
)

// webDist holds the built React SPA static files.
// The go:embed directive is commented out until the web UI is built.
// To enable: run `cd web && npm install && npm run build` first.
//
//go:embed all:web_dist
var webDistFS embed.FS

// spaHandler serves static files from the embedded FS, falling back to
// index.html for paths that do not match a real file (SPA client-side routing).
func spaHandler(fsys http.FileSystem) http.Handler {
	fileServer := http.FileServer(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		// Try to open the file
		f, err := fsys.Open(path)
		if err != nil {
			// File doesn't exist, serve index.html for SPA routing
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}
		f.Close()
		fileServer.ServeHTTP(w, r)
	})
}

// RegisterSPA adds the SPA file server to the router.
// Falls back to index.html for client-side routing.
func (s *Server) RegisterSPA() error {
	dist, err := fs.Sub(webDistFS, "web_dist")
	if err != nil {
		return err
	}

	s.router.Handle("GET /", spaHandler(http.FS(dist)))

	return nil
}
