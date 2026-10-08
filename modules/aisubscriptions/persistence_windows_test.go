//go:build windows

package aisubscriptions

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// Windows readers that omit FILE_SHARE_DELETE temporarily prevent replacement,
// as filesystem indexers/sync clients can do even after our own file is closed.
func holdReplacement(t *testing.T, dir string) syscall.Handle {
	t.Helper()
	name, err := syscall.UTF16PtrFromString(filepath.Join(dir, "state-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	h, err := syscall.CreateFile(name, syscall.GENERIC_READ, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	return h
}
func TestTransientWindowsReaderDoesNotLoseDurableUsage(t *testing.T) {
	m, dir, now := fixture(t)
	apply(t, m, account(now, "a"))
	h := holdReplacement(t, dir)
	released := make(chan struct{})
	time.AfterFunc(100*time.Millisecond, func() { syscall.CloseHandle(h); close(released) })
	err := m.Observe(context.Background(), usage(now, "a", "s", "r", 50))
	<-released
	if err != nil {
		var native syscall.Errno
		errors.As(err, &native)
		t.Fatalf("replacement failed while temporary reader held old file (Windows errno=%d): %v", native, err)
	}
	reopened, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	reopened.now = m.now
	apply(t, reopened, usage(now, "a", "s", "r", 50))
	if n := reopened.Snapshot().Providers[0].WeeklyTokens; n == nil || *n != 50 {
		t.Fatal("replacement/replay lost exact durable count")
	}
}
func TestCancellationDuringWindowsReplacementPreservesPreviousFile(t *testing.T) {
	m, dir, now := fixture(t)
	apply(t, m, account(now, "a"))
	before, err := os.ReadFile(filepath.Join(dir, "state-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	h := holdReplacement(t, dir)
	defer syscall.CloseHandle(h)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := m.Observe(ctx, usage(now, "a", "s", "r", 50)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("replacement did not honor cancellation: %v", err)
	}
	after, err := os.ReadFile(filepath.Join(dir, "state-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || m.Snapshot().Providers[0].WeeklyTokens != nil {
		t.Fatal("cancelled replacement published or changed previous state")
	}
	leftovers, err := filepath.Glob(filepath.Join(dir, ".state-*.tmp"))
	if err != nil || len(leftovers) != 0 {
		t.Fatalf("cancelled write leaked staging files: %v %v", leftovers, err)
	}
}
func TestPersistentWindowsReaderReturnsBoundedFailure(t *testing.T) {
	m, dir, now := fixture(t)
	apply(t, m, account(now, "a"))
	before, err := os.ReadFile(filepath.Join(dir, "state-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	h := holdReplacement(t, dir)
	defer syscall.CloseHandle(h)
	done := make(chan error, 1)
	go func() { done <- m.Observe(context.Background(), usage(now, "a", "s", "r", 50)) }()
	select {
	case err := <-done:
		if !errors.Is(err, syscall.ERROR_ACCESS_DENIED) {
			t.Fatalf("permanent lock lost native failure: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("replacement retry was unbounded")
	}
	after, err := os.ReadFile(filepath.Join(dir, "state-v1.json"))
	if err != nil || string(before) != string(after) || m.Snapshot().Providers[0].WeeklyTokens != nil {
		t.Fatal("permanent lock changed durable/public state", err)
	}
}
