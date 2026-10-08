package configuratorweb

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExplicitFiles(t *testing.T) {
	for path, mime := range map[string]string{"/": "text/html", "/app.js": "text/javascript", "/style.css": "text/css", "/signature.png": "image/png", "/ai-assets/openai.png": "image/png", "/ai-assets/claude.png": "image/png", "/ai-assets/claude-crab.svg": "image/svg+xml"} {
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

func TestAIEditorControlsAreAccessibleAndEmbedded(t *testing.T) {
	html, err := files.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{`id="ai-setup"`, `id="ai-connection-status" role="status"`, `id="prop-provider"`, `id="prop-detail"`, `id="prop-animate" type="checkbox"`, `data-ai-widget="openai"`, `data-ai-widget="claude"`, `aria-label="Native client setup proposal"`} {
		if !strings.Contains(string(html), required) {
			t.Fatalf("missing control: %s", required)
		}
	}
	widgetSettings := strings.SplitN(string(html), `id="ai-fields"`, 2)
	if len(widgetSettings) != 2 {
		t.Fatal("missing AI widget settings")
	}
	widgetSettings = strings.SplitN(widgetSettings[1], `</fieldset>`, 2)
	for _, required := range []string{`id="ai-connection-status"`, `id="ai-sign-in"`, `aria-controls="ai-sign-in-help"`, `id="ai-monitor"`, `id="ai-setup"`, `data-ai-action="check"`, `codex login`, `claude auth login --claudeai`} {
		if !strings.Contains(widgetSettings[0], required) {
			t.Fatalf("missing inline connection control: %s", required)
		}
	}
	for _, removed := range []string{`id="ai-usage"`, `id="nav-ai"`, `id="ai-manage"`, `data-library="ai"`, `data-ai-action="connect"`} {
		if strings.Contains(string(html), removed) {
			t.Fatalf("separate AI page or browser login control remains: %s", removed)
		}
	}
	script, err := files.ReadFile("app.js")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(script), "window.open") || strings.Contains(string(script), "aiAuthURL") {
		t.Fatal("OAuth browser launch remains in editor")
	}
}
