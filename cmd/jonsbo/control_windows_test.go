package main

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestMonitorControlExclusiveAndGracefulStop(t *testing.T) {
	name := fmt.Sprintf("Local\\JonsboGoTest-%d", os.Getpid())
	ctx, close, err := monitorControl(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	if _, release, err := monitorControl(context.Background(), name); err == nil {
		release()
		t.Fatal("duplicate monitor allowed")
	}
	if err := signalMonitorStop(name); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("stop did not cancel monitor")
	}
	close()
	_, release, err := monitorControl(context.Background(), name)
	if err != nil {
		t.Fatal("handles leaked:", err)
	}
	release()
}
