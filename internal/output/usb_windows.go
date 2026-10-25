//go:build windows

package output

import (
	"context"
	"fmt"
	"github.com/kawapiki/Jonsbo-resurrection/internal/device"
	"github.com/kawapiki/Jonsbo-resurrection/internal/display"
	"github.com/kawapiki/Jonsbo-resurrection/internal/winusb"
	"image"
	"sync"
)

// USBDriver keeps handle ownership and transport recovery outside modules.
type USBDriver struct {
	mu       sync.Mutex
	sessions map[string]*usbSession
	// Native resource boundaries are injectable for lifecycle tests.
	enumerate func() ([]device.Device, error)
	open      func(device.Device) (usbConnection, error)
}
type usbConnection interface {
	display.Transport
	Close() error
}
type usbSession struct {
	mu         sync.Mutex
	connection usbConnection
}

func (s *usbSession) close() {
	if s.connection != nil {
		s.connection.Close()
		s.connection = nil
	}
}

// A failed frame is never retried in place. Reopen on the next refresh.
func (s *usbSession) send(ctx context.Context, kind string, im image.Image) error {
	_, err := display.Send(ctx, s.connection, kind, im)
	if err != nil {
		s.close()
	}
	return err
}

func (driver *USBDriver) Send(ctx context.Context, selected device.Device, im image.Image) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var persistent *usbSession
	if selected.Kind == "cyclops" {
		driver.mu.Lock()
		if driver.sessions == nil {
			driver.sessions = map[string]*usbSession{}
		}
		persistent = driver.sessions[selected.Serial]
		if persistent == nil {
			persistent = &usbSession{}
			driver.sessions[selected.Serial] = persistent
		}
		driver.mu.Unlock()
		persistent.mu.Lock()
		defer persistent.mu.Unlock()
		if persistent.connection != nil {
			return persistent.send(ctx, selected.Kind, im)
		}
	}
	enumerate := driver.enumerate
	if enumerate == nil {
		enumerate = winusb.Enumerate
	}
	open := driver.open
	if open == nil {
		open = func(d device.Device) (usbConnection, error) { return winusb.Open(d) }
	}
	devices, err := enumerate()
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
	if persistent != nil {
		persistent.connection, err = open(d)
		if err != nil {
			return err
		}
		return persistent.send(ctx, d.Kind, im)
	}
	c, err := open(d)
	if err != nil {
		return err
	}
	defer c.Close()
	_, err = display.Send(ctx, c, d.Kind, im)
	return err
}

// Close is called after the manager's workers finish, including cancellation.
func (driver *USBDriver) Close() error {
	driver.mu.Lock()
	defer driver.mu.Unlock()
	for _, s := range driver.sessions {
		s.mu.Lock()
		s.close()
		s.mu.Unlock()
	}
	driver.sessions = nil
	return nil
}
