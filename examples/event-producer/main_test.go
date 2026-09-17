package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProducerSendsTypedAuthenticatedMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/modules/example/events" || r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Errorf("wrong request route/auth")
		}
		var event struct {
			Type    string `json:"type"`
			Payload struct {
				Text string `json:"text"`
			} `json:"payload"`
		}
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			t.Error(err)
		}
		if event.Type != "message" || event.Payload.Text != "Build complete" {
			t.Errorf("wrong event %+v", event)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("test-secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run(context.Background(), []string{"-url", server.URL, "-token-file", path, "-message", "Build complete"}, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "test-secret") {
		t.Fatal("printed token")
	}
}

func TestProducerRejectsRemoteAndDoesNotLeakErrors(t *testing.T) {
	t.Setenv("JONSBO_API_TOKEN", "private-token")
	for _, url := range []string{"http://example.com", "http://127.0.0.1.evil.test", "http://user:pass@127.0.0.1", "https://127.0.0.1"} {
		if err := send(context.Background(), url, "private-token", "hello"); err == nil {
			t.Fatalf("accepted unsafe URL %s", url)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte("private-token private payload"))
	}))
	defer server.Close()
	err := run(context.Background(), []string{"-url", server.URL, "-message", "hello"}, new(bytes.Buffer))
	if err == nil || strings.Contains(err.Error(), "private-token") || strings.Contains(err.Error(), "private payload") {
		t.Fatalf("unsafe error: %v", err)
	}
}

func TestProducerDoesNotFollowRedirect(t *testing.T) {
	contacted := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { contacted = true }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer source.Close()
	if err := send(context.Background(), source.URL, "secret", "hello"); err == nil || contacted {
		t.Fatal("followed redirect or accepted redirect as success")
	}
}
