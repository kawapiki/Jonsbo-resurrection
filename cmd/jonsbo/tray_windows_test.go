//go:build windows

package main

import (
	"errors"
	"testing"
	"time"
)

func TestTrayStopCancelsPendingRetry(t *testing.T) {
	child := &trayChild{wanted: true, retryAt: time.Now().Add(-time.Second), exe: "missing.exe"}
	if err := child.stop(); err != nil {
		t.Fatal(err)
	}
	child.tick()
	if child.wanted || !child.retryAt.IsZero() || child.cmd != nil {
		t.Fatal("stop left retry active")
	}
}

func TestTrayRepeatedExitEventuallyRequiresManualStart(t *testing.T) {
	child := &trayChild{wanted: true}
	for i := 0; i < 3; i++ {
		child.finish(errors.New("failed"))
		if child.retryAt.IsZero() {
			t.Fatalf("no retry after failure %d", i)
		}
	}
	child.retryAt = time.Time{}
	child.finish(errors.New("failed"))
	if !child.retryAt.IsZero() {
		t.Fatal("retry limit exceeded")
	}
	if child.cmd != nil || child.done != nil {
		t.Fatal("exited child still appears running")
	}
}
