package output

import (
	"context"
	"github.com/kawapiki/Jonsbo-resurrection/internal/device"
	"image"
	"sync"
	"testing"
	"time"
)

type farewellSource struct {
	fakeSource
	elapsed []time.Duration
}

func (s *farewellSource) FarewellFrame(ctx context.Context, _, _ string, elapsed time.Duration) (image.Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.elapsed = append(s.elapsed, elapsed)
	return image.NewRGBA(image.Rect(0, 0, 240, 240)), nil
}

type farewellDriver struct {
	mu              sync.Mutex
	ready           chan struct{}
	once            sync.Once
	calls, closedAt int
	closed          bool
}

func (d *farewellDriver) Send(ctx context.Context, _ device.Device, _ image.Image) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		panic("frame after close")
	}
	d.calls++
	d.once.Do(func() { close(d.ready) })
	return nil
}
func (d *farewellDriver) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	d.closedAt = d.calls
	return nil
}

func TestFarewellFinishesBeforeDriverClose(t *testing.T) {
	s := &farewellSource{}
	d := &farewellDriver{ready: make(chan struct{})}
	m, err := New(s, d, []device.Device{{Serial: "RJK-TEST", Kind: "cyclops"}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- m.Run(ctx, time.Second) }()
	<-d.ready
	// Wait until the first successful send is recorded before cancellation.
	for m.Displays()[0].LastSuccess.IsZero() {
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("farewell did not finish within shutdown budget")
	}
	if len(s.elapsed) != 13 || s.elapsed[0] != 0 || s.elapsed[12] != 1500*time.Millisecond {
		t.Fatal("farewell incomplete", s.elapsed)
	}
	if !d.closed || d.closedAt != 14 {
		t.Fatal("incorrect driver lifetime", d.closedAt)
	}
	if m.Displays()[0].Status != "stopped" {
		t.Fatal("farewell did not report completion")
	}
}
