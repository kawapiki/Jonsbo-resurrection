package tray

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestLaunchUsesOptionalConfigWithoutOverridingModules(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "config.json")
	if err := os.WriteFile(config, []byte(`{"modules":{"example":{"enabled":false}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	args := ServerArgs(dir, "Local\\JonsboResurrectionChild-123")
	found := false
	for i, arg := range args {
		if arg == "--example" {
			t.Fatal("custom config must control example module")
		}
		if arg == "--config" {
			if i+1 >= len(args) || args[i+1] != config {
				t.Fatalf("config path: %#v", args)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("missing optional config: %#v", args)
	}
}

func TestLaunchDoesNotTreatConfigDirectoryAsFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "config.json"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, arg := range ServerArgs(dir, "Local\\JonsboResurrectionChild-123") {
		if arg == "--config" {
			t.Fatal("directory must not be passed as configuration file")
		}
	}
}

func TestLaunchUsesPrivateAbsoluteDataPaths(t *testing.T) {
	dir := t.TempDir()
	args := ServerArgs(dir, "Local\\JonsboResurrectionChild-123")
	want := []string{"serve", "--all", "--allow-no-displays", "--example", "--token-file", filepath.Join(dir, "api-token"), "--log-file", filepath.Join(dir, "server.log"), "--control-name", "Local\\JonsboResurrectionChild-123"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %#v", args)
	}
}

func TestRetriesAreBoundedAndSlowEnoughForLogin(t *testing.T) {
	for attempt := 0; attempt < 3; attempt++ {
		if d := RetryDelay(attempt); d < 5*time.Second || d > time.Minute {
			t.Fatalf("retry %d: %v", attempt, d)
		}
	}
	if RetryDelay(3) != 0 {
		t.Fatal("must stop retrying after three retries")
	}
}
