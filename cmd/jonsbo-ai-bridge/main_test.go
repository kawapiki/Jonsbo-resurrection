package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func frame(s string) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, uint32(len(s)))
	return append(b, []byte(s)...)
}
func TestBridgeForwardsOnlyVisibleMetadata(t *testing.T) {
	var payload string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/ai/browser/events" || r.Header.Get("Authorization") != "Bearer scoped" {
			t.Error("wrong scope")
		}
		b, _ := io.ReadAll(r.Body)
		payload = string(b)
		w.WriteHeader(202)
	}))
	defer s.Close()
	raw := `{"url":"https://chatgpt.com/c/abc-123","provider":"openai","session_id":"browser:1:abc-123","activity":"running","title":"Visible title","account_key":"forged","tokens":{"total":999},"prompt":"secret"}`
	var output bytes.Buffer
	if err := serve(context.Background(), bytes.NewReader(frame(raw)), &output, Config{API: s.URL}, "scoped"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload, `"activity":"running"`) || strings.Contains(payload, "secret") || strings.Contains(payload, "forged") || strings.Contains(payload, "tokens") {
		t.Fatal(payload)
	}
	if output.Len() < 4 {
		t.Fatal("no native reply")
	}
}
func TestBridgeRejectsWrongHostAndOversize(t *testing.T) {
	for _, raw := range []string{`{"url":"https://evil.test/c/a","provider":"openai","session_id":"a"}`, `{"url":"https://chatgpt.com.evil.test/c/a","provider":"openai","session_id":"a"}`, `{"url":"https://claude.ai/chat/a","provider":"openai","session_id":"a"}`} {
		if _, err := project([]byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, maxMessage+1)
	if err := serve(context.Background(), bytes.NewReader(b), io.Discard, Config{}, ""); err == nil {
		t.Fatal("oversize accepted")
	}
	for _, url := range []string{"https://127.0.0.1:8787", "http://localhost:8787", "http://127.0.0.1:8787/evil", "http://127.0.0.1:8787/?x=1", "http://127.0.0.1"} {
		if validateAPI(url) == nil {
			t.Fatal(url)
		}
	}
}
