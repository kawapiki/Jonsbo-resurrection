//go:build windows

package telemetry

import (
	"os"
	"syscall"
	"unsafe"
)

var getLogicalDrives = kernel.NewProc("GetLogicalDrives")
var getDriveType = kernel.NewProc("GetDriveTypeW")
var getDiskFreeSpace = kernel.NewProc("GetDiskFreeSpaceExW")

func newStorageSampler() *storageSampler {
	return &storageSampler{
		systemDrive: os.Getenv("SystemDrive"),
		logicalDrives: func() (uint32, error) {
			mask, _, err := getLogicalDrives.Call()
			if mask == 0 {
				return 0, err
			}
			return uint32(mask), nil
		},
		driveType: func(path string) uint32 {
			root, err := syscall.UTF16PtrFromString(path)
			if err != nil {
				return 0
			}
			kind, _, _ := getDriveType.Call(uintptr(unsafe.Pointer(root)))
			return uint32(kind)
		},
		diskSpace: func(path string) (uint64, uint64, error) {
			root, err := syscall.UTF16PtrFromString(path)
			if err != nil {
				return 0, 0, err
			}
			var available, total uint64
			// Both outputs are quota-aware for the calling user. The fourth output is
			// volume-wide free space and deliberately omitted to keep ratios consistent.
			ok, _, err := getDiskFreeSpace.Call(uintptr(unsafe.Pointer(root)), uintptr(unsafe.Pointer(&available)), uintptr(unsafe.Pointer(&total)), 0)
			if ok == 0 {
				return 0, 0, err
			}
			return total, available, nil
		},
	}
}
