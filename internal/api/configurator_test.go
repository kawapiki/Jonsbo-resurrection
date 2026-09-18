package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConfiguratorBoundary(t *testing.T) {
	web := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("editor shell")) })
	extension := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("private config")) })
	h, err := NewHandlerWithUI(&fakeBackend{}, nil, testToken, extension, web)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path, host, origin, token string
		want                      int
	}{
		{"/", "127.0.0.1:8787", "", "", 200},
		{"/app.js", "evil.test", "", "", 403},
		{"/", "127.0.0.1:8787", "http://evil.test", "", 403},
		{"/v1/configurator", "127.0.0.1:8787", "", "", 401},
		{"/v1/configurator", "127.0.0.1:8787", "", testToken, 200},
		{"/v1/configurator/assets", "127.0.0.1:8787", "http://evil.test", testToken, 403},
	} {
		r := httptest.NewRequest("GET", "http://127.0.0.1:8787"+tc.path, nil)
		r.Host = tc.host
		if tc.origin != "" {
			r.Header.Set("Origin", tc.origin)
		}
		if tc.token != "" {
			r.Header.Set("Authorization", "Bearer "+tc.token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("%s %s got %d want %d", tc.path, tc.host, w.Code, tc.want)
		}
		if strings.Contains(w.Body.String(), testToken) {
			t.Fatal("token leaked in body")
		}
		if w.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Fatal("missing referrer policy")
		}
	}
}
