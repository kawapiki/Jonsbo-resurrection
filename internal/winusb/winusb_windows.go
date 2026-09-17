// Package winusb uses the existing Windows driver without replacing or installing drivers.
package winusb

import (
	"encoding/binary"
	"fmt"
	"os"
	"runtime"
	"sort"
	"syscall"
	"time"
	"unsafe"

	"jonsbo-display/internal/device"
)

var (
	setup          = syscall.NewLazyDLL("setupapi.dll")
	getClassDevs   = setup.NewProc("SetupDiGetClassDevsW")
	enumInterfaces = setup.NewProc("SetupDiEnumDeviceInterfaces")
	getDetail      = setup.NewProc("SetupDiGetDeviceInterfaceDetailW")
	destroyList    = setup.NewProc("SetupDiDestroyDeviceInfoList")
	usb            = syscall.NewLazyDLL("winusb.dll")
	initialize     = usb.NewProc("WinUsb_Initialize")
	free           = usb.NewProc("WinUsb_Free")
	queryInterface = usb.NewProc("WinUsb_QueryInterfaceSettings")
	queryPipe      = usb.NewProc("WinUsb_QueryPipe")
	setPolicy      = usb.NewProc("WinUsb_SetPipePolicy")
	writePipe      = usb.NewProc("WinUsb_WritePipe")
	readPipe       = usb.NewProc("WinUsb_ReadPipe")
)

type guid struct {
	A    uint32
	B, C uint16
	D    [8]byte
}
type interfaceData struct {
	Size     uint32
	GUID     guid
	Flags    uint32
	Reserved uintptr
}

// Registry DeviceInterfaceGUID values read from these exact installed controllers.
var interfaceGUIDs = []guid{
	{0x88bae032, 0x5a81, 0x49f0, [8]byte{0xbc, 0x3d, 0xa4, 0xff, 0x13, 0x82, 0x16, 0xd6}},
	{0x12345678, 0x1234, 0x1344, [8]byte{0x12, 0x34, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc}},
}

func Enumerate() ([]device.Device, error) {
	devices := []device.Device{}
	seen := map[string]bool{}
	for _, g := range interfaceGUIDs {
		paths, err := enumerateGUID(g)
		if err != nil {
			return nil, err
		}
		for _, path := range paths {
			if d, ok := device.ParseInterface(path); ok && !seen[path] {
				devices = append(devices, d)
				seen[path] = true
			}
		}
	}
	sort.Slice(devices, func(i, j int) bool { return devices[i].Serial < devices[j].Serial })
	return devices, nil
}

func enumerateGUID(g guid) ([]string, error) {
	h, _, e := getClassDevs.Call(uintptr(unsafe.Pointer(&g)), 0, 0, 0x12) // PRESENT | DEVICEINTERFACE
	if h == ^uintptr(0) {
		return nil, fmt.Errorf("SetupDiGetClassDevs: %w", e)
	}
	defer destroyList.Call(h)
	var paths []string
	for i := uint32(0); ; i++ {
		d := interfaceData{}
		d.Size = uint32(unsafe.Sizeof(d))
		ok, _, err := enumInterfaces.Call(h, 0, uintptr(unsafe.Pointer(&g)), uintptr(i), uintptr(unsafe.Pointer(&d)))
		if ok == 0 {
			if err == syscall.Errno(259) {
				break
			}
			return nil, fmt.Errorf("enumerate interface: %w", err)
		}
		var needed uint32
		getDetail.Call(h, uintptr(unsafe.Pointer(&d)), 0, 0, uintptr(unsafe.Pointer(&needed)), 0)
		if needed < 6 || needed > 65536 {
			return nil, fmt.Errorf("invalid interface detail size %d", needed)
		}
		buf := make([]byte, needed)
		cbSize := uint32(8)
		if unsafe.Sizeof(uintptr(0)) == 4 {
			cbSize = 6
		}
		binary.LittleEndian.PutUint32(buf, cbSize)
		ok, _, err = getDetail.Call(h, uintptr(unsafe.Pointer(&d)), uintptr(unsafe.Pointer(&buf[0])), uintptr(needed), uintptr(unsafe.Pointer(&needed)), 0)
		if ok == 0 {
			return nil, fmt.Errorf("interface detail: %w", err)
		}
		chars := make([]uint16, (len(buf)-4)/2)
		for j := range chars {
			chars[j] = binary.LittleEndian.Uint16(buf[4+2*j:])
		}
		paths = append(paths, syscall.UTF16ToString(chars))
	}
	return paths, nil
}

type Pipe struct {
	Type          uint32 `json:"type"`
	ID            byte   `json:"id"`
	MaxPacketSize uint16 `json:"max_packet_size"`
	Interval      byte   `json:"interval"`
}

type Connection struct {
	file    syscall.Handle
	handle  uintptr
	Pipes   []Pipe
	In, Out byte
}

func Open(d device.Device) (*Connection, error) {
	if _, ok := device.ParseInterface(d.Path); !ok {
		return nil, fmt.Errorf("unsupported device interface")
	}
	path, err := syscall.UTF16PtrFromString(d.Path)
	if err != nil {
		return nil, err
	}
	h, err := syscall.CreateFile(path, syscall.GENERIC_READ|syscall.GENERIC_WRITE, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", d.Serial, err)
	}
	c := &Connection{file: h}
	ok, _, e := initialize.Call(uintptr(h), uintptr(unsafe.Pointer(&c.handle)))
	if ok == 0 {
		syscall.CloseHandle(h)
		return nil, fmt.Errorf("WinUSB initialize %s (close the vendor app first): %w", d.Serial, e)
	}
	var desc [9]byte
	ok, _, e = queryInterface.Call(c.handle, 0, uintptr(unsafe.Pointer(&desc[0])))
	if ok == 0 {
		c.Close()
		return nil, fmt.Errorf("query interface: %w", e)
	}
	for i := byte(0); i < desc[4]; i++ {
		var p Pipe
		ok, _, e = queryPipe.Call(c.handle, 0, uintptr(i), uintptr(unsafe.Pointer(&p)))
		if ok == 0 {
			c.Close()
			return nil, fmt.Errorf("query pipe: %w", e)
		}
		c.Pipes = append(c.Pipes, p)
		if p.Type == 2 || p.Type == 3 {
			if p.ID&0x80 != 0 {
				c.In = p.ID
			} else if p.Type == 2 {
				c.Out = p.ID
			}
			ms := uint32(2000)
			ok, _, e = setPolicy.Call(c.handle, uintptr(p.ID), 3, 4, uintptr(unsafe.Pointer(&ms)))
			if ok == 0 {
				c.Close()
				return nil, fmt.Errorf("set pipe timeout: %w", e)
			}
		}
	}
	if c.In == 0 || c.Out == 0 {
		c.Close()
		return nil, fmt.Errorf("device lacks bulk OUT and bulk/interrupt IN endpoints")
	}
	return c, nil
}

// Close must be called after transfers finish. A connection has one owner and is not concurrent.
func (c *Connection) Close() error {
	if c.handle != 0 {
		free.Call(c.handle)
		c.handle = 0
	}
	if c.file != 0 && c.file != syscall.InvalidHandle {
		err := syscall.CloseHandle(c.file)
		c.file = 0
		return err
	}
	return nil
}

func (c *Connection) Write(b []byte) (int, error) { return c.transfer(b, false) }
func (c *Connection) Read(b []byte) (int, error)  { return c.transfer(b, true) }

func (c *Connection) SetReadTimeout(d time.Duration) error {
	if c.handle == 0 {
		return fmt.Errorf("USB connection is closed")
	}
	ms := d.Milliseconds()
	if ms < 1 || ms > 0xffffffff {
		return fmt.Errorf("invalid USB timeout")
	}
	value := uint32(ms)
	ok, _, err := setPolicy.Call(c.handle, uintptr(c.In), 3, 4, uintptr(unsafe.Pointer(&value)))
	if ok == 0 {
		return fmt.Errorf("set read timeout: %w", err)
	}
	return nil
}
func (c *Connection) transfer(b []byte, read bool) (int, error) {
	if c.handle == 0 {
		return 0, fmt.Errorf("USB connection is closed")
	}
	if len(b) == 0 {
		return 0, nil
	}
	if uint64(len(b)) > 0xffffffff {
		return 0, fmt.Errorf("transfer too large")
	}
	fn, endpoint := writePipe, c.Out
	if read {
		fn, endpoint = readPipe, c.In
	}
	var n uint32
	ok, _, e := fn.Call(c.handle, uintptr(endpoint), uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), uintptr(unsafe.Pointer(&n)), 0)
	runtime.KeepAlive(b)
	if ok == 0 {
		if e == syscall.Errno(121) {
			return int(n), fmt.Errorf("USB endpoint 0x%02x: %w", endpoint, os.ErrDeadlineExceeded)
		}
		return int(n), fmt.Errorf("USB endpoint 0x%02x: %w", endpoint, e)
	}
	return int(n), nil
}
