package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestObserverForwardsMetadataOnlyAndOutputsSilentHook(t *testing.T) {
	var body []byte
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("a", 64) {
			t.Error("missing scoped credential")
		}
		w.WriteHeader(202)
	}))
	defer s.Close()
	token := filepath.Join(t.TempDir(), "observer.token")
	os.WriteFile(token, []byte(strings.Repeat("a", 64)), 0600)
	var out bytes.Buffer
	err := run([]string{"hook", "--provider", "openai", "--api", s.URL, "--token-file", token}, strings.NewReader(`{"session_id":"s","hook_event_name":"Stop","prompt":"private","transcript_path":"secret"}`), &out)
	if err != nil || out.String() != "{}\n" || strings.Contains(string(body), "private") || strings.Contains(string(body), "secret") {
		t.Fatalf("%s %s %v", out.String(), body, err)
	}
}
func TestObserverRejectsExternalDestinations(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"hook", "--provider", "claude", "--api", "https://evil.example", "--token-file", "unused"}, strings.NewReader(`{}`), &out); err == nil {
		t.Fatal("external destination accepted")
	}
}
func TestStatuslineOutputSurvivesControllerUnavailable(t *testing.T) {
	var out bytes.Buffer
	err := run([]string{"statusline", "--provider", "claude", "--api", "http://127.0.0.1:1", "--token-file", filepath.Join(t.TempDir(), "missing")}, strings.NewReader(`{"session_id":"s","model":{"id":"opus"},"context_window":{"used_percentage":42}}`), &out)
	if err != nil || !strings.Contains(out.String(), "42%") {
		t.Fatalf("%s %v", out.String(), err)
	}
}
