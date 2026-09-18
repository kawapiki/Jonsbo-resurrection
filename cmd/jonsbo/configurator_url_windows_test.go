//go:build windows

package main

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfiguratorURLKeepsCredentialOutOfQuery(t *testing.T) {
	dir := t.TempDir()
	token := "configurator-test-token-01234567890123456789"
	if err := os.WriteFile(filepath.Join(dir, "api-token"), []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"listen":"127.0.0.1:8899"}`), 0600); err != nil {
		t.Fatal(err)
	}
	raw, err := configuratorURL(dir)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if u.Host != "127.0.0.1:8899" || u.RawQuery != "" || u.Fragment != "token="+token {
		t.Fatalf("unexpected connection URL shape host=%s", u.Host)
	}
	if strings.Contains(u.RequestURI(), token) {
		t.Fatal("credential sent as URL query")
	}
}
func TestConfiguratorURLRequiresExistingCredential(t *testing.T) {
	dir := t.TempDir()
	if _, err := configuratorURL(dir); err == nil {
		t.Fatal("missing token accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, "api-token")); !os.IsNotExist(err) {
		t.Fatal("opening editor created credential")
	}
}
