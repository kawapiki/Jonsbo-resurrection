package winusb

import (
	"context"
	"errors"
	"syscall"
	"testing"
	"time"
)

// Real Windows events exercise the native wait without opening a USB device.
func TestHIDWaitObservesFrameDeadlineAndCancellation(t *testing.T) {
	for _, cancelEarly := range []bool{false, true} {
		name := "deadline"
		if cancelEarly {
			name = "cancellation"
		}
		t.Run(name, func(t *testing.T) {
			event, _, err := hidCreateEvent.Call(0, 1, 0, 0)
			if event == 0 {
				t.Fatal(err)
			}
			defer syscall.CloseHandle(syscall.Handle(event))
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			want := context.DeadlineExceeded
			if cancelEarly {
				want = context.Canceled
				timer := time.AfterFunc(25*time.Millisecond, cancel)
				defer timer.Stop()
			}
			start := time.Now()
			err = waitHIDTransfer(ctx, syscall.Handle(event))
			if !errors.Is(err, want) {
				t.Fatalf("wait error: %v; want %v", err, want)
			}
			if elapsed := time.Since(start); elapsed > time.Second {
				t.Fatalf("frame cancellation ignored: %v", elapsed)
			}
		})
	}
}

func TestHIDWaitAcceptsCompletedTransfer(t *testing.T) {
	event, _, err := hidCreateEvent.Call(0, 1, 1, 0)
	if event == 0 {
		t.Fatal(err)
	}
	defer syscall.CloseHandle(syscall.Handle(event))
	if err := waitHIDTransfer(context.Background(), syscall.Handle(event)); err != nil {
		t.Fatal(err)
	}
}
