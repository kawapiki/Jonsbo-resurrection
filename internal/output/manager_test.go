package output

import (
	"context"
	"errors"
	"image"
	"github.com/kawapiki/Jonsbo-resurrection/internal/device"
	"github.com/kawapiki/Jonsbo-resurrection/pkg/module"
	"sync"
	"testing"
	"time"
)

type fakeSource struct{}

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
