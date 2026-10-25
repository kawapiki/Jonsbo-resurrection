package output

import (
	"context"
	"errors"
	"github.com/kawapiki/Jonsbo-resurrection/internal/device"
	"image"
	"testing"
	"time"
)

type testUSBConnection struct {
	closes int
	fail   bool
}

func (c *testUSBConnection) Write(b []byte) (int, error) {
	if c.fail {
		return 0, errors.New("unplugged")
	}
	return len(b), nil
}
func (c *testUSBConnection) Read([]byte) (int, error)           { return 0, errors.New("unexpected read") }
func (c *testUSBConnection) SetReadTimeout(time.Duration) error { return nil }
func (c *testUSBConnection) Control(b []byte) (int, error)      { return len(b), nil }
func (c *testUSBConnection) Close() error                       { c.closes++; return nil }

func TestCyclopsConnectionLivesAcrossFramesAndRecovers(t *testing.T) {
	d := device.Device{Serial: "RJK-TEST", Kind: "cyclops", Path: "hid-path"}
	connections := []*testUSBConnection{}
	enumerations := 0
	driver := &USBDriver{enumerate: func() ([]device.Device, error) { enumerations++; return []device.Device{d}, nil }, open: func(device.Device) (usbConnection, error) {
		c := &testUSBConnection{}
		connections = append(connections, c)
		return c, nil
	}}
	frame := image.NewRGBA(image.Rect(0, 0, 240, 240))
	for i := 0; i < 2; i++ {
		if err := driver.Send(context.Background(), d, frame); err != nil {
			t.Fatal(err)
		}
	}
	if enumerations != 1 {
		t.Fatalf("rediscovered healthy connection: %d enumerations", enumerations)
	}
	if len(connections) != 1 || connections[0].closes != 0 {
		t.Fatal("connection closed between frames")
	}
	connections[0].fail = true
	if err := driver.Send(context.Background(), d, frame); err == nil {
		t.Fatal("disconnect ignored")
	}
	if enumerations != 1 {
		t.Fatal("failed frame should not retry discovery")
	}
	if connections[0].closes != 1 {
		t.Fatal("broken connection not closed")
	}
	if err := driver.Send(context.Background(), d, frame); err != nil {
		t.Fatal(err)
	}
	if enumerations != 2 {
		t.Fatal("recovery must rediscover once")
	}
	if len(connections) != 2 {
		t.Fatal("connection not reopened")
	}
	if err := driver.Close(); err != nil {
		t.Fatal(err)
	}
	if connections[1].closes != 1 {
		t.Fatal("connection leaked at shutdown")
	}
	driver.Close()
	if connections[1].closes != 1 {
		t.Fatal("connection closed twice")
	}
}

func TestCyclopsAcquisitionFailureAndCancellation(t *testing.T) {
	d := device.Device{Serial: "RJK-TEST", Kind: "cyclops", Path: "hid-path"}
	discoveryErr := errors.New("discovery unavailable")
	calls, opens := 0, 0
	driver := &USBDriver{enumerate: func() ([]device.Device, error) { calls++; return []device.Device{d}, discoveryErr }, open: func(device.Device) (usbConnection, error) { opens++; return &testUSBConnection{}, nil }}
	defer driver.Close()
	frame := image.NewRGBA(image.Rect(0, 0, 240, 240))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := driver.Send(ctx, d, frame); !errors.Is(err, context.Canceled) || calls != 0 || opens != 0 {
		t.Fatal("cancelled frame acquired USB resources", err, calls, opens)
	}
	if err := driver.Send(context.Background(), d, frame); !errors.Is(err, discoveryErr) || opens != 0 {
		t.Fatal("failed discovery opened connection", err, opens)
	}
	discoveryErr = nil
	if err := driver.Send(context.Background(), d, frame); err != nil || calls != 2 || opens != 1 {
		t.Fatal("discovery did not recover", err, calls, opens)
	}
	// Healthy transfers no longer depend on repeated discovery succeeding.
	discoveryErr = errors.New("unrelated enumeration failure")
	if err := driver.Send(context.Background(), d, frame); err != nil || calls != 2 || opens != 1 {
		t.Fatal("healthy connection rediscovered", err, calls, opens)
	}
}
