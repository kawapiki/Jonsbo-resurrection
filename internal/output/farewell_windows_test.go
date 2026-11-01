package output

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/kawapiki/Jonsbo-resurrection/internal/device"
)

// Stall the final frame at either native transfer boundary. The legacy methods
// model HID's per-transfer timeout; the context methods model cancellable I/O.
type stalledFarewellConnection struct {
	testUSBConnection
	controls     int
	stallControl bool
}

func (c *stalledFarewellConnection) transfer(ctx context.Context) (int, error) {
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-timer.C:
		return 0, os.ErrDeadlineExceeded
	}
}
func (c *stalledFarewellConnection) Control(b []byte) (int, error) {
	return c.ControlContext(context.Background(), b)
}
func (c *stalledFarewellConnection) ControlContext(ctx context.Context, b []byte) (int, error) {
	c.controls++
	if c.stallControl && c.controls == 25 {
		return c.transfer(ctx)
	}
	return len(b), nil
}
func (c *stalledFarewellConnection) Write(b []byte) (int, error) {
	return c.WriteContext(context.Background(), b)
}
func (c *stalledFarewellConnection) WriteContext(ctx context.Context, b []byte) (int, error) {
	if !c.stallControl && c.controls == 26 {
		return c.transfer(ctx)
	}
	return len(b), nil
}

func TestFarewellCancelsStalledFinalTransfer(t *testing.T) {
	for _, control := range []bool{true, false} {
		name := "write"
		if control {
			name = "control"
		}
		t.Run(name, func(t *testing.T) {
			d := device.Device{Serial: "FAKE-FAREWELL", Kind: "cyclops"}
			connection := &stalledFarewellConnection{stallControl: control}
			driver := &USBDriver{
				enumerate: func() ([]device.Device, error) { return []device.Device{d}, nil },
				open:      func(device.Device) (usbConnection, error) { return connection, nil },
			}
			defer driver.Close()
			m, err := New(&farewellSource{}, driver, []device.Device{d}, Options{})
			if err != nil {
				t.Fatal(err)
			}
			s := m.screens[d.Serial]
			s.state.LastSuccess = time.Now()
			started := time.Now()
			m.farewell(s)
			if elapsed := time.Since(started); elapsed > 2500*time.Millisecond {
				t.Fatalf("farewell exceeded its two-second budget: %v", elapsed)
			}
			if connection.closes != 1 {
				t.Fatal("failed transfer was not closed after cancellation")
			}
		})
	}
}
