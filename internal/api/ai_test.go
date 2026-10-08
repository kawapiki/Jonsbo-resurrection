package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeAI struct{ called int }

func (a *fakeAI) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { a.called++; w.WriteHeader(202) })
}
func (*fakeAI) AuthenticateIngress(r *http.Request) bool {
	return r.Method == "POST" && r.URL.Path == "/v1/ai/browser/events" && r.Header.Get("Authorization") == "Bearer scoped"
}
func TestAIAuthScopeAndOrigin(t *testing.T) {
	a := new(fakeAI)
	h, err := NewHandlerWithAI(&fakeBackend{}, nil, testToken, nil, nil, a)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path, host, origin, auth, method string
		want                             int
	}{
		{"/v1/ai/browser/events", "127.0.0.1:8787", "", "scoped", "POST", 202},
		{"/v1/ai/setup", "127.0.0.1:8787", "", "scoped", "GET", 401},
		{"/v1/modules", "127.0.0.1:8787", "", "scoped", "GET", 401},
		{"/v1/ai/browser/events", "attacker.invalid", "", "scoped", "POST", 403},
		{"/v1/ai/browser/events", "127.0.0.1:8787", "https://chatgpt.com", "scoped", "POST", 403},
		{"/v1/ai/setup", "127.0.0.1:8787", "http://127.0.0.1:8787", testToken, "GET", 202},
		{"/v1/ai/setup", "127.0.0.1:8787", "", "", "GET", 401},
	} {
		r := httptest.NewRequest(tc.method, "http://127.0.0.1:8787"+tc.path, strings.NewReader(`{}`))
		r.Host = tc.host
		if tc.auth != "" {
			r.Header.Set("Authorization", "Bearer "+tc.auth)
		}
		if tc.origin != "" {
			r.Header.Set("Origin", tc.origin)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("%s %s %s: got %d want %d", tc.method, tc.path, tc.origin, w.Code, tc.want)
		}
	}
	if a.called != 2 {
		t.Fatalf("AI controller called %d times", a.called)
	}
}
