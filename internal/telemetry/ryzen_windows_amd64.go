//go:build windows && amd64

package telemetry

import (
	_ "embed"
	"encoding/binary"
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

// Unmodified signed module from PawnIO.Modules0.2.11. See third_party/pawnio.
//
//go:embed resources/AMDFamily17.bin
var ryzenModule []byte

func cpuIdentity() (signature, vendorB, vendorD, vendorC uint32)

type RyzenTemperature struct {
	mu     sync.Mutex
	handle syscall.Handle
	pci    uintptr
	closed bool
}

var createPCIMutex = kernel.NewProc("CreateMutexExW")
var waitPCI = kernel.NewProc("WaitForSingleObject")
var releasePCI = kernel.NewProc("ReleaseMutex")

func NewRyzenTemperature() (*RyzenTemperature, error) {
	sig, b, d, c := cpuIdentity()
	family := (sig >> 8) & 15
	if family == 15 {
		family += (sig >> 20) & 255
	}
	model := ((sig >> 4) & 15) | ((sig >> 12) & 240)
	// Keep support explicit: Raphael and Granite Ridge. Older Ryzen parts have
	// additional model-specific Tctl offsets not handled by this collector.
	if b != 0x68747541 || d != 0x69746e65 || c != 0x444d4163 || !((family == 0x19 && model == 0x61) || (family == 0x1a && model == 0x44)) {
		return nil, fmt.Errorf("CPU temperature: unsupported CPU family %x model %x", family, model)
	}
	path, _ := syscall.UTF16PtrFromString(`\\?\GLOBALROOT\Device\PawnIO`)
	h, err := syscall.CreateFile(path, 3, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, fmt.Errorf("CPU temperature requires the signed PawnIO driver (and permission to open it): %w", err)
	}
	r := &RyzenTemperature{handle: h}
	ok := false
	defer func() {
		if !ok {
			r.Close()
		}
	}()
	var returned uint32
	if err := syscall.DeviceIoControl(h, 0xa1b22084, &ryzenModule[0], uint32(len(ryzenModule)), nil, 0, &returned, nil); err != nil {
		return nil, fmt.Errorf("load signed Ryzen sensor module: %w", err)
	}
	name, _ := syscall.UTF16PtrFromString(`Global\Access_PCI`)
	r.pci, _, err = createPCIMutex.Call(0, uintptr(unsafe.Pointer(name)), 0, 0x100001)
	if r.pci == 0 {
		return nil, fmt.Errorf("CPU sensor PCI mutex: %w", err)
	}
	ok = true
	return r, nil
}
func (r *RyzenTemperature) Sample() (float64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, fmt.Errorf("CPU sensor is closed")
	}
	// Win32 mutex ownership belongs to an OS thread, not a goroutine.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	status, _, err := waitPCI.Call(r.pci, 100)
	if status != 0 && status != 0x80 {
		return 0, fmt.Errorf("CPU sensor PCI mutex unavailable (0x%x): %v", status, err)
	}
	defer releasePCI.Call(r.pci)
	var input [40]byte
	copy(input[:32], "ioctl_read_smn")
	binary.LittleEndian.PutUint64(input[32:], 0x59800)
	var output [8]byte
	var returned uint32
	if err := syscall.DeviceIoControl(r.handle, 0xa1b22104, &input[0], 40, &output[0], 8, &returned, nil); err != nil {
		return 0, fmt.Errorf("read Ryzen temperature: %w", err)
	}
	if returned != 8 {
		return 0, fmt.Errorf("Ryzen sensor response length %d, want 8", returned)
	}
	return decodeRyzenTemperature(binary.LittleEndian.Uint32(output[:4]))
}
func (r *RyzenTemperature) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.closed = true
	if r.pci != 0 {
		syscall.CloseHandle(syscall.Handle(r.pci))
	}
	if r.handle != syscall.InvalidHandle {
		syscall.CloseHandle(r.handle)
	}
}
