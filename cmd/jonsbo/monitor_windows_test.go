package main

import "testing"

func TestMonitorRejectsInvalidOptionsBeforeHardwareAccess(t *testing.T) {
	for _, args := range [][]string{nil, {"--all", "--serial", "x"}, {"--all", "--role", "gpu"}, {"--serial", "x", "--role", "typo"}, {"--all", "--interval", "0s"}, {"--all", "--duration", "-1s"}, {"--all", "stray"}} {
		if err := monitor(args); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
func TestStatsRejectsInvalidOptions(t *testing.T) {
	for _, args := range [][]string{{"--count", "-1"}, {"--interval", "0s"}, {"stray"}} {
		if err := stats(args); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
