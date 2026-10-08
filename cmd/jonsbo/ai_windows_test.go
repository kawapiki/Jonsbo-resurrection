//go:build windows

package main

import (
	"encoding/json"
	"github.com/kawapiki/Jonsbo-resurrection/internal/config"
	"path/filepath"
	"testing"
)

func TestAISubscriptionStorageAndRegistration(t *testing.T) {
	c := config.Default()
	c.TokenFile = filepath.Join(t.TempDir(), "api-token")
	c.Modules = map[string]config.Module{"ai-subscriptions": {Enabled: true}}
	dir, err := aiDirectory(c, c.Modules["ai-subscriptions"].Options)
	if err != nil || dir != filepath.Join(filepath.Dir(c.TokenFile), "ai-subscriptions") {
		t.Fatalf("directory %s: %v", dir, err)
	}
	r, err := newRuntime(c)
	if err != nil {
		t.Fatal(err)
	}
	defer closeRuntime(r)
	s := r.Snapshot()
	if len(s) != 1 || s[0].Module.ID != "ai-subscriptions" {
		t.Fatalf("module missing: %+v", s)
	}
	if _, err := aiDirectory(c, json.RawMessage(`{"token":"secret"}`)); err == nil {
		t.Fatal("unknown options accepted")
	}
}
