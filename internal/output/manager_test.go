package output

import (
	"context"
	"errors"
	"github.com/kawapiki/Jonsbo-resurrection/internal/device"
	"github.com/kawapiki/Jonsbo-resurrection/pkg/module"
	"image"
	"sync"
	"testing"
	"time"
)

type fakeSource struct{}

type closeErrorDriver struct{ fakeDriver }

func (*closeErrorDriver) Close() error { return errors.New("close failed") }
func TestOnceReturnsCleanupErrorWithoutBlocking(t *testing.T) {
	m, err := New(fakeSource{}, &closeErrorDriver{}, []device.Device{{Serial: "ABC", Kind: "fan"}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- m.Once(context.Background()) }()
	select {
	case err := <-done:
		if err == nil || err.Error() != "close failed" {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cleanup blocked on full result channel")
	}
}

func TestCyclopsGetsNativeViewAndFrameSize(t *testing.T) {
	driver := &fakeDriver{}
	m, err := New(fakeSource{}, driver, []device.Device{{Serial: "RJK-TEST", Kind: "cyclops"}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Displays()[0].Assignment; got.View != "cyclops" || got.Rotation != 0 {
		t.Fatal(got)
	}
	if err = m.Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	if driver.bounds != image.Rect(0, 0, 240, 240) {
		t.Fatal(driver.bounds)
	}
}

func (fakeSource) ValidateView(id, view string) error {
	if id != "hardware" || view == "bad" {
		return module.ErrNotFound
	}
	return nil
}
func (fakeSource) Frame(context.Context, string, string) (image.Image, error) {
	return image.NewRGBA(image.Rect(0, 0, 640, 180)), nil
}

type fakeDriver struct {
	mu        sync.Mutex
	calls     int
	failFirst bool
	bounds    image.Rectangle
}

func (f *fakeDriver) Send(_ context.Context, _ device.Device, im image.Image) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.bounds = im.Bounds()
	if f.failFirst && f.calls == 1 {
		return errors.New("unplugged")
	}
	return nil
}
func TestAssignmentValidationAndSafeStatus(t *testing.T) {
	m, err := New(fakeSource{}, &fakeDriver{}, []device.Device{{Serial: "ABC", Kind: "fan", Path: "private-device-path"}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Assign("ABC", module.Assignment{Module: "hardware", View: "bad", Rotation: 90}); !errors.Is(err, module.ErrNotFound) {
		t.Fatal(err)
	}
	if err = m.Assign("ABC", module.Assignment{Module: "hardware", View: "cpu", Rotation: 45}); !errors.Is(err, module.ErrInvalid) {
		t.Fatal(err)
	}
	if err = m.Assign("missing", module.Assignment{Module: "hardware", View: "cpu", Rotation: 90}); !errors.Is(err, module.ErrNotFound) {
		t.Fatal(err)
	}
	if m.Displays()[0].Assignment.Rotation != 90 {
		t.Fatal("fan correction lost")
	}
	if _, err := New(fakeSource{}, &fakeDriver{}, []device.Device{{Serial: "../bad", Kind: "fan"}}, Options{}); err == nil {
		t.Fatal("unsafe serial accepted")
	}
}
func TestWorkerRecoversAndRotates(t *testing.T) {
	d := &fakeDriver{failFirst: true}
	m, err := New(fakeSource{}, d, []device.Device{{Serial: "ABC", Kind: "fan"}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- m.Run(ctx, 10*time.Millisecond) }()
	until := time.Now().Add(2 * time.Second)
	for time.Now().Before(until) {
		if m.Displays()[0].Status == "live" {
			break
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.calls < 2 || d.bounds.Dx() != 180 || d.bounds.Dy() != 640 {
		t.Fatalf("no recovery/rotation: %+v", d)
	}
	if m.Displays()[0].LastSuccess.IsZero() {
		t.Fatal("success not recorded")
	}
}
