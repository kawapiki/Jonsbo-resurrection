package configuratorweb

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExplicitFiles(t *testing.T) {
	for path, mime := range map[string]string{"/": "text/html", "/app.js": "text/javascript", "/style.css": "text/css", "/signature.png": "image/png"} {
		t.Run(path, func(t *testing.T) {
			w := httptest.NewRecorder()
			Files().ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
			if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Type"), mime) || w.Body.Len() == 0 {
				t.Fatalf("file response: %d %s (%d bytes)", w.Code, w.Header().Get("Content-Type"), w.Body.Len())
			}
			if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatal("missing response security headers")
			}
		})
	}
}

func TestFilesRejectBrowseAndMutations(t *testing.T) {
	for _, path := range []string{"/index.html", "/../web.go", "/internal/configuratorweb/", "/app.test.mjs", "/signature.png/", "/unknown"} {
		w := httptest.NewRecorder()
		Files().ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusNotFound {
			t.Fatalf("%s returned %d, want 404", path, w.Code)
		}
	}
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		w := httptest.NewRecorder()
		Files().ServeHTTP(w, httptest.NewRequest(method, "/", nil))
		if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != "GET, HEAD" {
			t.Fatalf("%s returned %d", method, w.Code)
		}
	}
	w := httptest.NewRecorder()
	Files().ServeHTTP(w, httptest.NewRequest(http.MethodHead, "/", nil))
	if w.Code != http.StatusOK || w.Body.Len() != 0 {
		t.Fatal("HEAD must return headers without content")
	}
}
