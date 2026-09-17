//go:build windows

package output

import (
	"context"
	"fmt"
	"image"
	"github.com/kawapiki/Jonsbo-resurrection/internal/device"
	"github.com/kawapiki/Jonsbo-resurrection/internal/display"
	"github.com/kawapiki/Jonsbo-resurrection/internal/winusb"
)

// USBDriver keeps handle ownership and transport recovery outside modules.
type USBDriver struct{}

func (USBDriver) Send(ctx context.Context, selected device.Device, im image.Image) error {
	devices, err := winusb.Enumerate()
	if err != nil {
		return err
	}
	d, err := device.Select(devices, selected.Serial)
	if err != nil {
		return err
	}
	if d.Kind != selected.Kind {
		return fmt.Errorf("display kind changed")
	}
	c, err := winusb.Open(d)
	if err != nil {
		return err
	}
	defer c.Close()
	_, err = display.Send(ctx, c, d.Kind, im)
	return err
}
