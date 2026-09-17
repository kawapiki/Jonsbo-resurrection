package tray

import (
	"os"
	"path/filepath"
	"time"
)

func ServerArgs(dir, control string) []string {
	args := []string{"serve", "--all", "--allow-no-displays"}
	config := filepath.Join(dir, "config.json")
	if info, err := os.Stat(config); err == nil && info.Mode().IsRegular() {
		args = append(args, "--config", config)
	} else {
		args = append(args, "--example")
	}
	return append(args, "--token-file", filepath.Join(dir, "api-token"), "--log-file", filepath.Join(dir, "server.log"), "--control-name", control)
}

func RetryDelay(attempt int) time.Duration {
	if attempt < 0 || attempt >= 3 {
		return 0
	}
	return time.Duration(5*(1<<attempt)) * time.Second
}
