package aiadapters

import (
	"context"
	data "github.com/kawapiki/Jonsbo-resurrection/pkg/aisubscriptions"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScopedIngressNeverAuthorizesController(t *testing.T) {
	m, err := New(t.TempDir(), func(context.Context, data.Observation) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path, token, method string
		want                bool
	}{
		{"/v1/ai/browser/events", m.browserToken, "POST", true},
		{"/v1/ai/providers/openai/connect", m.browserToken, "POST", false},
		{"/v1/ai/browser/events", m.observerToken, "POST", false},
		{"/v1/ai/browser/events", m.browserToken, "GET", false},
		{"/v1/ai/observer/claude/hook", m.observerToken, "POST", true},
		{"/v1/ai/telemetry/openai/v1/logs", m.observerToken, "POST", true},
	} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		r.Header.Set("Authorization", "Bearer "+tc.token)
		if got := m.AuthenticateIngress(r); got != tc.want {
			t.Fatalf("%s %s: %v", tc.method, tc.path, got)
		}
	}
}

func TestNullConnectionFileLeavesWritableProviderMap(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "connections.json"), []byte("null"), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, func(context.Context, data.Observation) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if m.enabled == nil {
		t.Fatal("null config removed provider state")
	}
}

func TestBrowserRejectsOriginAndDropsConsumptionAndIdentity(t *testing.T) {
	var got []data.Observation
	m, _ := New(t.TempDir(), func(_ context.Context, o data.Observation) error { got = append(got, o); return nil })
	raw := `{"provider":"openai","session_id":"s","activity":"running","account_key":"forged","tokens":{"total":999},"source":"forged"}`
	r := httptest.NewRequest("POST", "/v1/ai/browser/events", strings.NewReader(raw))
	r.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("bad origin: %d", w.Code)
	}
	r = httptest.NewRequest("POST", "/v1/ai/browser/events", strings.NewReader(raw))
	r.Header.Set("Authorization", "Bearer "+m.browserToken)
	w = httptest.NewRecorder()
	m.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusAccepted || len(got) != 1 || got[0].AccountKey != "" || got[0].Tokens != nil || got[0].Source != "browser" {
		t.Fatalf("%d %+v", w.Code, got)
	}
}
