package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultsAndStrictConfig(t *testing.T) {
	c, err := Load("")
	if err != nil || !c.Modules["hardware"].Enabled || c.Modules["example"].Enabled {
		t.Fatalf("bad defaults: %+v %v", c, err)
	}
	for _, data := range []string{`{"typo":true}`, `{"listen":"0.0.0.0:8787"}`, `{"frame_interval":"0s"}`, `{} {}`} {
		p := filepath.Join(t.TempDir(), "config.json")
		os.WriteFile(p, []byte(data), 0600)
		if _, err := Load(p); err == nil {
			t.Fatalf("accepted %s", data)
		}
	}
}
func TestOversizedConfigRejected(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, []byte(`{}`+strings.Repeat(" ", 1<<20)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("accepted oversized whitespace-padded config")
	}
}
func TestTokenStableAndInvalidRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "token")
	a, err := Token(path)
	if err != nil || len(a) < 32 {
		t.Fatal(err)
	}
	b, err := Token(path)
	if err != nil || a != b {
		t.Fatal("token changed", err)
	}
	if err := os.WriteFile(path, []byte("weak"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Token(path); err == nil {
		t.Fatal("weak token accepted")
	}
}
