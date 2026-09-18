// Package configuratorweb serves the self-contained display editor. Authentication
// and loopback request checks are provided by the API router that mounts Files.
package configuratorweb

import (
	"embed"
	"net/http"
)

//go:embed index.html app.js style.css signature.png
var files embed.FS

// Files serves only the editor's explicit public entry points, never directories.
func Files() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name, mime := "", ""
		switch r.URL.Path {
		case "/":
			name, mime = "index.html", "text/html; charset=utf-8"
		case "/app.js":
			name, mime = "app.js", "text/javascript; charset=utf-8"
		case "/style.css":
			name, mime = "style.css", "text/css; charset=utf-8"
		case "/signature.png":
			name, mime = "signature.png", "image/png"
		default:
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		data, err := files.ReadFile(name)
		if err != nil {
			http.Error(w, "Editor unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", mime)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method == http.MethodGet {
			_, _ = w.Write(data)
		}
	})
}
