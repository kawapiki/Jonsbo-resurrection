package winusb

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

var (
	hidDLL           = syscall.NewLazyDLL("hid.dll")
	hidPreparsed     = hidDLL.NewProc("HidD_GetPreparsedData")
	hidFreePreparsed = hidDLL.NewProc("HidD_FreePreparsedData")
	hidCaps          = hidDLL.NewProc("HidP_GetCaps")
	hidAttributes    = hidDLL.NewProc("HidD_GetAttributes")
	hidKernel        = syscall.NewLazyDLL("kernel32.dll")
	hidCreateEvent   = hidKernel.NewProc("CreateEventW")
	hidGetOverlapped = hidKernel.NewProc("GetOverlappedResult")
)

// Inspect before writing: only the Cyclops VID/PID and unnumbered 1024-byte
// output / 8-byte feature reports are supported. Windows includes report ID 0.
func (c *Connection) inspectHID() error {
	attrs := struct {
		Size                     uint32
		Vendor, Product, Version uint16
	}{Size: 12}
	ok, _, err := hidAttributes.Call(uintptr(c.file), uintptr(unsafe.Pointer(&attrs)))
	if ok == 0 {
		return fmt.Errorf("Cyclops HID identity: %w", err)
	}
	if attrs.Vendor != 0x1f3a || attrs.Product != 0xb128 {
		return fmt.Errorf("unexpected HID identity")
	}
	var data uintptr
	ok, _, err = hidPreparsed.Call(uintptr(c.file), uintptr(unsafe.Pointer(&data)))
	if ok == 0 {
		return fmt.Errorf("Cyclops HID descriptor: %w", err)
	}
	defer hidFreePreparsed.Call(data)
	// HIDP_CAPS consists of 32 USHORT fields (64 bytes).
	var caps [32]uint16
	status, _, _ := hidCaps.Call(data, uintptr(unsafe.Pointer(&caps[0])))
	if uint32(status) != 0x00110000 {
		return fmt.Errorf("HID capabilities status 0x%x", status)
	}
	if caps[3] != 1025 || caps[4] != 9 {
		return fmt.Errorf("unsupported Cyclops HID report sizes: output=%d feature=%d (expected 1025/9)", caps[3], caps[4])
	}
	c.Pipes = []Pipe{{Type: 3, ID: 1, MaxPacketSize: 1024}}
	c.Out = 1
	return nil
}

// Same wire semantics as libusb's Windows HID backend, without shipping its DLL:
// SET_REPORT (feature, ID 0) -> IOCTL_HID_SET_FEATURE; endpoint OUT -> WriteFile.
// Both use overlapped I/O with a bounded wait and cancellation/drain on timeout.
func (c *Connection) hidTransfer(payload []byte, feature bool) (int, error) {
	return c.hidTransferContext(context.Background(), payload, feature)
}

func (c *Connection) hidTransferContext(ctx context.Context, payload []byte, feature bool) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	expected := 1024
	if feature {
		expected = 8
	}
	if c.file == 0 || c.file == syscall.InvalidHandle || len(payload) != expected {
		return 0, fmt.Errorf("invalid Cyclops HID transfer")
	}
	report := make([]byte, len(payload)+1)
	copy(report[1:], payload)
	event, _, err := hidCreateEvent.Call(0, 1, 0, 0)
	if event == 0 {
		return 0, fmt.Errorf("HID event: %w", err)
	}
	defer syscall.CloseHandle(syscall.Handle(event))
	ov := syscall.Overlapped{HEvent: syscall.Handle(event)}
	var n uint32
	if feature {
		// CTL_CODE(FILE_DEVICE_KEYBOARD, 100, METHOD_IN_DIRECT, FILE_ANY_ACCESS).
		err = syscall.DeviceIoControl(c.file, 0x000b0191, &report[0], uint32(len(report)), &report[0], uint32(len(report)), &n, &ov)
	} else {
		err = syscall.WriteFile(c.file, report, &n, &ov)
	}
	if err != nil && err != syscall.ERROR_IO_PENDING {
		return 0, fmt.Errorf("Cyclops HID send: %w", err)
	}
	if e := waitHIDTransfer(ctx, ov.HEvent); e != nil {
		syscall.CancelIoEx(c.file, &ov)
		// Drain before releasing the report, OVERLAPPED, or event: cancellation
		// requests completion but does not make kernel-owned memory safe to free.
		hidGetOverlapped.Call(uintptr(c.file), uintptr(unsafe.Pointer(&ov)), uintptr(unsafe.Pointer(&n)), 1)
		runtime.KeepAlive(report)
		return 0, e
	}
	ok, _, e := hidGetOverlapped.Call(uintptr(c.file), uintptr(unsafe.Pointer(&ov)), uintptr(unsafe.Pointer(&n)), 0)
	runtime.KeepAlive(report)
	if ok == 0 {
		return 0, fmt.Errorf("Cyclops HID completion: %w", e)
	}
	// Feature IOCTL byte counts exclude report ID 0; WriteFile includes it.
	expectedCount := len(report)
	if feature {
		expectedCount = len(payload)
	}
	if int(n) != expectedCount {
		return 0, fmt.Errorf("Cyclops HID wrote %d of %d bytes: %w", n, expectedCount, io.ErrShortWrite)
	}
	return len(payload), nil
}

func waitHIDTransfer(ctx context.Context, event syscall.Handle) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		// Completed transfers return immediately. Short waits let cancellation
		// interrupt a pending native transfer without a goroutine owning ov.
		wait, err := syscall.WaitForSingleObject(event, 10)
		if err != nil {
			return err
		}
		switch wait {
		case syscall.WAIT_OBJECT_0:
			return nil
		case uint32(syscall.WAIT_TIMEOUT):
			continue
		default:
			return fmt.Errorf("unexpected HID wait result: %d", wait)
		}
	}
}

// WriteContext preserves the existing WinUSB path; HID waits also observe the
// frame's cancellation and deadline, including the farewell shutdown budget.
func (c *Connection) WriteContext(ctx context.Context, b []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if c.hid {
		return c.hidTransferContext(ctx, b, false)
	}
	return c.Write(b)
}

func (c *Connection) ControlContext(ctx context.Context, b []byte) (int, error) {
	if !c.hid {
		return 0, fmt.Errorf("Cyclops control transfers require HID")
	}
	return c.hidTransferContext(ctx, b, true)
}

// Control sends the exact eight-byte SET_REPORT through the factory HID driver.
func (c *Connection) Control(b []byte) (int, error) {
	if !c.hid {
		return 0, fmt.Errorf("Cyclops control transfers require HID")
	}
	return c.hidTransfer(b, true)
}
